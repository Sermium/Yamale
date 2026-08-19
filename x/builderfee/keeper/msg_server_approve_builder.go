package keeper

import (
	"bytes"
	"context"

	"yamale/blockchain/x/builderfee/types"

	errorsmod "cosmossdk.io/errors"
)

// ApproveBuilder is executed exclusively by the governance module account
// once a proposal to approve or reject a pending builder registration has
// passed. Approval activates the fee split for that message type.
func (k msgServer) ApproveBuilder(ctx context.Context, msg *types.MsgApproveBuilder) (*types.MsgApproveBuilderResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authority) {
		expectedAuthorityStr, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expectedAuthorityStr, msg.Authority)
	}

	application, err := k.BuilderApplication.Get(ctx, msg.MsgTypeUrl)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotFound, "no application found for %s", msg.MsgTypeUrl)
	}
	if application.Status != types.StatusPending {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotPending, "application for %s has status %s", msg.MsgTypeUrl, application.Status)
	}

	if msg.Approve {
		application.Status = types.StatusApproved
		if err := k.ApprovedBuilder.Set(ctx, msg.MsgTypeUrl, types.ApprovedBuilder{
			MsgTypeUrl:    msg.MsgTypeUrl,
			PayoutAddress: application.PayoutAddress,
		}); err != nil {
			return nil, err
		}
	} else {
		application.Status = types.StatusRejected
	}

	if err := k.BuilderApplication.Set(ctx, msg.MsgTypeUrl, application); err != nil {
		return nil, err
	}

	return &types.MsgApproveBuilderResponse{}, nil
}
