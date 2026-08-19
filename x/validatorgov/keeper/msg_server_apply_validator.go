package keeper

import (
	"context"

	"yamale/blockchain/x/validatorgov/types"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ApplyValidator lets a prospective validator self-nominate. It records a
// pending application; the candidate is only allowed to submit
// MsgCreateValidator once a governance proposal approves them via
// MsgApproveValidator (enforced by the validatorgov ante decorator).
//
// The declaration is required and validated here, at the only point where it
// can be. A concentration ceiling is computed over declared entities, owners
// and jurisdictions, so an application that carried none would create a
// validator belonging to no group and therefore bounded by no ceiling — and by
// the time the epoch check saw it, it would already be state. It is also what
// the admission vote is meant to be judging: a set asked to approve a candidate
// whose owner and jurisdiction it cannot see is being asked to approve an
// address.
func (k msgServer) ApplyValidator(ctx context.Context, msg *types.MsgApplyValidator) (*types.MsgApplyValidatorResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	declaration := types.NormaliseDeclaration(msg.LegalEntityId, msg.BeneficialOwnerId, msg.Jurisdiction)
	if err := declaration.Validate(); err != nil {
		return nil, errorsmod.Wrap(types.ErrInvalidDeclaration, err.Error())
	}
	declaration.AttestedAtHeight = sdk.UnwrapSDKContext(ctx).BlockHeight()

	if err := k.ValidatorApplication.Set(ctx, msg.Creator, types.ValidatorApplication{
		Candidate:   msg.Creator,
		Status:      types.StatusPending,
		Declaration: declaration,
	}); err != nil {
		return nil, err
	}

	return &types.MsgApplyValidatorResponse{}, nil
}
