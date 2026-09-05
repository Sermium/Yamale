package keeper

import (
	"bytes"
	"context"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/stablecoin/types"

	errorsmod "cosmossdk.io/errors"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// ApproveIssuer admits the sole address permitted to mint and burn a denom, and
// publishes its bank denom metadata.
//
// Two signers are accepted, and the second is the whole of the jurisdictional
// change here:
//
//   - governance, as before. It is the body that grants the roles below, so
//     requiring it to hold one of them would be circular.
//   - the monetary authority of the country the applicant is recorded in. That
//     is the office that actually licenses a currency issuer in the deployments
//     this chain is built for, and routing every admission through a chain-wide
//     governance vote instead was the thing the perimeter design set out to fix:
//     it made every national decision everybody's business.
//
// The perimeter is what keeps the second from being a widening. A monetary
// authority granted NG can admit an issuer recorded in NG and cannot touch one
// recorded in SN, and an applicant the chain cannot place is refused to both of
// them.
func (k msgServer) ApproveIssuer(ctx context.Context, msg *types.MsgApproveIssuer) (*types.MsgApproveIssuerResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	governance := bytes.Equal(k.GetAuthority(), authority)

	application, err := k.IssuerApplication.Get(ctx, msg.Denom)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotFound, "no application found for denom %s", msg.Denom)
	}

	// The applicant is the target, not the denom: a denom has no country, and
	// the account asking to issue it does. Read from the application rather than
	// from the message, so the signer cannot name whose perimeter they are acting
	// inside.
	if !governance {
		if err := k.assertScope(ctx, msg.Authority, application.Creator); err != nil {
			return nil, err
		}
	}
	if application.Status != types.StatusPending {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotPending, "application for %s has status %s", msg.Denom, application.Status)
	}

	if msg.Approve {
		application.Status = types.StatusApproved
		if err := k.ApprovedIssuer.Set(ctx, msg.Denom, types.ApprovedIssuer{
			Denom:  msg.Denom,
			Issuer: application.Creator,
		}); err != nil {
			return nil, err
		}

		k.bankKeeper.SetDenomMetaData(ctx, banktypes.Metadata{
			Description: application.Description,
			Base:        application.Denom,
			Display:     application.DisplayDenom,
			Name:        application.Name,
			Symbol:      application.Symbol,
			DenomUnits: []*banktypes.DenomUnit{
				{Denom: application.Denom, Exponent: 0},
				{Denom: application.DisplayDenom, Exponent: uint32(application.Exponent)},
			},
		})
	} else {
		// The record goes, rather than staying behind marked Rejected.
		//
		// RegisterCurrency is permissionless and keyed by denom, and it refuses
		// a second application for a denom that already has one. Keeping a
		// rejected record therefore killed the denomination permanently: one
		// transaction fee and uusd, ueur, ugbp, uchf or ujpy could never be
		// registered by anybody, with no withdrawal, expiry or clearing path.
		// Every currency in the oracle's accepted set was open to it.
		//
		// A rejection is a decision about an application. It is not a decision
		// that the denomination may never exist.
		if err := k.IssuerApplication.Remove(ctx, msg.Denom); err != nil {
			return nil, err
		}
		return &types.MsgApproveIssuerResponse{}, nil
	}

	if err := k.IssuerApplication.Set(ctx, msg.Denom, application); err != nil {
		return nil, err
	}

	return &types.MsgApproveIssuerResponse{}, nil
}

// assertScope refuses a signer admitting an issuer outside its perimeter.
//
// Fails closed when no registry is wired in: without it there is no way to know
// whose perimeter the applicant is in, and "cannot tell" must never resolve to
// "go ahead". Governance is unaffected, so a chain assembled without the
// registry can still admit issuers by vote — it just cannot delegate that to a
// national authority, which is the honest consequence of not having the
// perimeter.
func (k Keeper) assertScope(ctx context.Context, actor, target string) error {
	if k.scopeKeeper == nil {
		return aliastypes.ErrNoScopeKeeper
	}
	// "Not an authority at all" before "not this perimeter", so that a random
	// account sending this message is told it may not send it rather than told
	// something about the applicant. This branch permits nothing — the assertion
	// below is the gate, and it runs whatever this returns.
	holds, err := k.scopeKeeper.HoldsRole(ctx, actor, aliastypes.ROLE_MONETARY_AUTHORITY)
	if err != nil {
		return err
	}
	if !holds {
		expected, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return errorsmod.Wrapf(types.ErrInvalidSigner,
			"invalid authority; expected %s or a monetary authority, got %s", expected, actor)
	}
	return k.scopeKeeper.AssertScope(ctx, actor, aliastypes.ROLE_MONETARY_AUTHORITY, target)
}
