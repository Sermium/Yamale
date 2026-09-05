package keeper

import (
	"context"

	"yamale/blockchain/x/builderfee/types"

	errorsmod "cosmossdk.io/errors"
)

// RegisterBuilder lets a dApp developer propose themselves as the fee-share
// recipient for a given Msg type URL (e.g. "/blockchain.amm.v1.MsgSwap").
// It records a pending application; the payout only activates once a
// governance proposal approves it via MsgApproveBuilder.
func (k msgServer) RegisterBuilder(ctx context.Context, msg *types.MsgRegisterBuilder) (*types.MsgRegisterBuilderResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}
	if _, err := k.addressCodec.StringToBytes(msg.PayoutAddress); err != nil {
		return nil, errorsmod.Wrap(err, "invalid payout address")
	}

	// Checked before anything is read or written. This field is the store key
	// for two collections and the message is permissionless, so without a bound
	// it is an arbitrary-length attacker-chosen key.
	if err := types.ValidateMsgTypeURL(msg.MsgTypeUrl); err != nil {
		return nil, err
	}
	// And it must name a message this chain actually has. A builder fee against
	// a type nothing can send is a reservation, not an application — and
	// reservations are exactly what the squat below was built out of.
	if _, err := k.cdc.InterfaceRegistry().Resolve(msg.MsgTypeUrl); err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidMsgTypeURL,
			"%s is not a message type registered on this chain", msg.MsgTypeUrl)
	}

	if has, err := k.BuilderApplication.Has(ctx, msg.MsgTypeUrl); err != nil {
		return nil, err
	} else if has {
		return nil, errorsmod.Wrapf(types.ErrBuilderExists, "an application for %s already exists", msg.MsgTypeUrl)
	}
	if has, err := k.ApprovedBuilder.Has(ctx, msg.MsgTypeUrl); err != nil {
		return nil, err
	} else if has {
		return nil, errorsmod.Wrapf(types.ErrBuilderExists, "%s already has an approved builder", msg.MsgTypeUrl)
	}

	if err := k.BuilderApplication.Set(ctx, msg.MsgTypeUrl, types.BuilderApplication{
		MsgTypeUrl:    msg.MsgTypeUrl,
		Status:        types.StatusPending,
		PayoutAddress: msg.PayoutAddress,
		Creator:       msg.Creator,
	}); err != nil {
		return nil, err
	}

	return &types.MsgRegisterBuilderResponse{}, nil
}
