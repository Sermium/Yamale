package keeper

import (
	"bytes"
	"context"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/selfcustody/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns the write side of the door out.
func NewMsgServerImpl(keeper Keeper) types.MsgServer { return &msgServer{Keeper: keeper} }

var _ types.MsgServer = msgServer{}

/* ========================================================================== */
/*  Governance                                                                */
/* ========================================================================== */

// UpdateParams sets the waiting period and publishes the warning text.
//
// Governance and not the operator, which is the point of putting it here at
// all: an operator able to set its own waiting period per customer has not
// given anybody a right, it has given itself a discretion.
func (m msgServer) UpdateParams(
	ctx context.Context, msg *types.MsgUpdateParams,
) (*types.MsgUpdateParamsResponse, error) {
	if err := m.assertGovernance(msg.Authority); err != nil {
		return nil, err
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, m.Params.Set(ctx, msg.Params)
}

/* ========================================================================== */
/*  Ask, wait, ask again                                                      */
/* ========================================================================== */

// RequestDisclosure starts the waiting period.
func (m msgServer) RequestDisclosure(
	ctx context.Context, msg *types.MsgRequestDisclosure,
) (*types.MsgRequestDisclosureResponse, error) {
	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := m.addressCodec.StringToBytes(msg.Account); err != nil {
		return nil, types.ErrInvalidAccount.Wrapf("%q", msg.Account)
	}

	// Already gone. Refused rather than recorded a second time: a key that is
	// out cannot be taken out again, and letting the message succeed would
	// produce a second, later date on which the platform's obligations
	// supposedly ended — when in fact they ended on the first one.
	if disclosed, err := m.IsDisclosed(ctx, msg.Account); err != nil {
		return nil, err
	} else if disclosed {
		return nil, types.ErrAlreadyDisclosed.Wrapf("account %s", msg.Account)
	}

	if err := params.MatchesPublished(msg.AcknowledgementVersion, msg.AcknowledgementHash); err != nil {
		return nil, err
	}

	now := blockTime(ctx)

	// A second request while one is live is refused, but an EXPIRED one is
	// swept aside rather than blocking. Otherwise a request the holder forgot
	// about a year ago would permanently prevent them from making a new one,
	// and the only way out would be to ask the operator — which is the one
	// thing this pathway exists to avoid needing.
	if existing, err := m.Request.Get(ctx, msg.Account); err == nil {
		if now < existing.RequestedAt+params.RequestExpiry {
			return nil, types.ErrRequestPending.Wrapf(
				"account %s asked at %d and may confirm from %d",
				msg.Account, existing.RequestedAt, existing.RequestedAt+params.DisclosureDelay)
		}
	} else if !isNotFound(err) {
		return nil, err
	}

	if err := m.Request.Set(ctx, msg.Account, types.Request{
		Account:                msg.Account,
		RequestedAt:            now,
		AcknowledgementVersion: msg.AcknowledgementVersion,
		AcknowledgementHash:    msg.AcknowledgementHash,
	}); err != nil {
		return nil, err
	}

	// Emitted so that every device enrolled on this account can be told,
	// without any of them having to poll. The notice is the other half of the
	// waiting period: time nobody is told about protects nobody.
	if err := sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventDisclosureRequested{
		Account:     msg.Account,
		RequestedAt: now,
		EligibleAt:  now + params.DisclosureDelay,
		ExpiresAt:   now + params.RequestExpiry,
	}); err != nil {
		return nil, err
	}

	return &types.MsgRequestDisclosureResponse{
		EligibleAt: now + params.DisclosureDelay,
	}, nil
}

// CancelDisclosure stops a pending request.
//
// This is the message a person whose phone was stolen reaches for. It is
// cheap, it is available at any moment before confirmation, and it leaves
// nothing behind — deliberately, because somebody who changed their mind
// should not carry a permanent mark for having thought about it.
func (m msgServer) CancelDisclosure(
	ctx context.Context, msg *types.MsgCancelDisclosure,
) (*types.MsgCancelDisclosureResponse, error) {
	if _, err := m.Request.Get(ctx, msg.Account); err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoRequest.Wrapf("account %s", msg.Account)
		}
		return nil, err
	}
	if err := m.Request.Remove(ctx, msg.Account); err != nil {
		return nil, err
	}

	if err := sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventDisclosureCancelled{
		Account:     msg.Account,
		CancelledAt: blockTime(ctx),
		Reason:      strings.TrimSpace(msg.Reason),
	}); err != nil {
		return nil, err
	}
	return &types.MsgCancelDisclosureResponse{}, nil
}

// ConfirmDisclosure ends the pathway and discharges the platform's custody of
// this account's key.
//
// What this message does NOT do is release anything. No key material passes
// through the chain and none could: the shares are held off-chain, and the
// assembly happens on the holder's own device. This is the authorisation and
// the receipt, and the custodian releases its share against it.
//
// Note what continues. Per the operator's decision, the platform keeps its
// share and keeps co-signing after a disclosure — the account does not stop
// working and the holder does not lose access to the platform. What ends is the
// GUARANTEE: with the key in the holder's hands, two-of-three is no longer a
// property anybody can rely on, because one of the three is now sufficient on
// its own for whoever holds the exported key. Keeping the share is a
// convenience; the acknowledgement is explicit that it is no longer a
// protection.
func (m msgServer) ConfirmDisclosure(
	ctx context.Context, msg *types.MsgConfirmDisclosure,
) (*types.MsgConfirmDisclosureResponse, error) {
	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	if disclosed, err := m.IsDisclosed(ctx, msg.Account); err != nil {
		return nil, err
	} else if disclosed {
		return nil, types.ErrAlreadyDisclosed.Wrapf("account %s", msg.Account)
	}

	request, err := m.Request.Get(ctx, msg.Account)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoRequest.Wrapf(
				"account %s must ask first and then wait %d seconds",
				msg.Account, params.DisclosureDelay)
		}
		return nil, err
	}

	now := blockTime(ctx)
	eligibleAt := request.RequestedAt + params.DisclosureDelay
	if now < eligibleAt {
		return nil, types.ErrTooSoon.Wrapf(
			"account %s may confirm from %d, and it is %d (%d seconds to go)",
			msg.Account, eligibleAt, now, eligibleAt-now)
	}
	if now >= request.RequestedAt+params.RequestExpiry {
		return nil, types.ErrRequestExpired.Wrapf(
			"account %s asked at %d and the request lapsed at %d",
			msg.Account, request.RequestedAt, request.RequestedAt+params.RequestExpiry)
	}

	// Checked against the parameters NOW, not against what the holder
	// acknowledged when they asked. If governance published a new warning
	// during the waiting period then the terms have changed, this message
	// fails, and the holder is sent back to read the new text. That is the
	// correct outcome and it is the reason the acknowledgement is repeated on
	// this message at all.
	if err := params.MatchesPublished(msg.AcknowledgementVersion, msg.AcknowledgementHash); err != nil {
		return nil, err
	}

	if err := m.Disclosure.Set(ctx, msg.Account, types.Disclosure{
		Account:                msg.Account,
		RequestedAt:            request.RequestedAt,
		DisclosedAt:            now,
		DisclosedAtHeight:      sdk.UnwrapSDKContext(ctx).BlockHeight(),
		AcknowledgementVersion: msg.AcknowledgementVersion,
		AcknowledgementHash:    msg.AcknowledgementHash,
	}); err != nil {
		return nil, err
	}
	if err := m.Request.Remove(ctx, msg.Account); err != nil {
		return nil, err
	}

	if err := sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventDisclosed{
		Account:                msg.Account,
		RequestedAt:            request.RequestedAt,
		DisclosedAt:            now,
		DisclosedAtHeight:      sdk.UnwrapSDKContext(ctx).BlockHeight(),
		AcknowledgementVersion: msg.AcknowledgementVersion,
		AcknowledgementHash:    msg.AcknowledgementHash,
	}); err != nil {
		return nil, err
	}

	return &types.MsgConfirmDisclosureResponse{}, nil
}

/* ========================================================================== */
/*  Helpers                                                                   */
/* ========================================================================== */

func (m msgServer) assertGovernance(signer string) error {
	want, err := m.addressCodec.BytesToString(m.GetAuthority())
	if err != nil {
		return err
	}
	got, err := m.addressCodec.StringToBytes(signer)
	if err != nil {
		return types.ErrNotAuthority.Wrapf("%q is not an address", signer)
	}
	if !bytes.Equal(got, m.GetAuthority()) {
		return types.ErrNotAuthority.Wrapf(
			"this message is governance-only: expected %s, got %s", want, signer)
	}
	return nil
}

// blockTime rather than a height, because the waiting period is a period a
// frightened person lives through, and they experience hours.
func blockTime(ctx context.Context) int64 {
	return sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
}
