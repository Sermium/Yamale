package keeper

import (
	"context"

	"yamale/blockchain/x/validatorgov/types"

	errorsmod "cosmossdk.io/errors"
)

// ApplyValidator lets a prospective validator self-nominate. It records a
// pending application; the candidate is only allowed to submit
// MsgCreateValidator once a governance proposal approves them via
// MsgApproveValidator (enforced by the validatorgov ante decorator).
func (k msgServer) ApplyValidator(ctx context.Context, msg *types.MsgApplyValidator) (*types.MsgApplyValidatorResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	if err := k.ValidatorApplication.Set(ctx, msg.Creator, types.ValidatorApplication{
		Candidate: msg.Creator,
		Status:    types.StatusPending,
	}); err != nil {
		return nil, err
	}

	return &types.MsgApplyValidatorResponse{}, nil
}
