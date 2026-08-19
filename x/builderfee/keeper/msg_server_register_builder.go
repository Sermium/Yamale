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
