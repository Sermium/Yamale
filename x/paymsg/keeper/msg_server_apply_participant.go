package keeper

import (
	"context"

	"yamale/blockchain/x/paymsg/types"

	errorsmod "cosmossdk.io/errors"
)

// ApplyParticipant lets a prospective payment service provider self-nominate
// with a proposed participant code (an ISPB/BIC-equivalent identifier). It
// records a pending application; the applicant only becomes an approved
// participant once a governance proposal approves it via
// MsgApproveParticipant.
func (k msgServer) ApplyParticipant(ctx context.Context, msg *types.MsgApplyParticipant) (*types.MsgApplyParticipantResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	if has, err := k.ParticipantApplication.Has(ctx, msg.Creator); err != nil {
		return nil, err
	} else if has {
		return nil, errorsmod.Wrapf(types.ErrApplicationExists, "an application for %s already exists", msg.Creator)
	}
	if has, err := k.ApprovedParticipant.Has(ctx, msg.Creator); err != nil {
		return nil, err
	} else if has {
		return nil, errorsmod.Wrapf(types.ErrApplicationExists, "%s is already an approved participant", msg.Creator)
	}

	if err := k.ParticipantApplication.Set(ctx, msg.Creator, types.ParticipantApplication{
		Creator: msg.Creator,
		Status:  types.StatusPending,
		Code:    msg.Code,
		Name:    msg.Name,
	}); err != nil {
		return nil, err
	}

	return &types.MsgApplyParticipantResponse{}, nil
}
