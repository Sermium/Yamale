package keeper

import (
	"bytes"
	"context"
	"strconv"

	"yamale/blockchain/x/validatorgov/types"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ApproveValidator is executed exclusively by the governance module account
// (see the "authority" signer on MsgApproveValidator) once a proposal to
// approve or reject a pending validator application has passed. Approval
// adds the candidate to the ApprovedValidator allowlist, which the
// validatorgov ante decorator checks before permitting MsgCreateValidator.
func (k msgServer) ApproveValidator(ctx context.Context, msg *types.MsgApproveValidator) (*types.MsgApproveValidatorResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authority) {
		expectedAuthorityStr, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expectedAuthorityStr, msg.Authority)
	}

	if _, err := k.addressCodec.StringToBytes(msg.Candidate); err != nil {
		return nil, errorsmod.Wrap(err, "invalid candidate address")
	}

	application, err := k.ValidatorApplication.Get(ctx, msg.Candidate)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotFound, "no application found for candidate %s", msg.Candidate)
	}
	if application.Status != types.StatusPending {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotPending, "application for %s has status %s", msg.Candidate, application.Status)
	}

	if msg.Approve {
		application.Status = types.StatusApproved
		// The declaration is copied from the application at approval, and from
		// here on the approval's copy is the one the ceilings read. The two
		// then diverge as the operator re-attests, which is deliberate: the
		// application is what the set voted for and the approval is what is
		// claimed now, and a register with only one of those cannot show that
		// anything changed.
		declaration := application.Declaration
		declaration.AttestedAtHeight = sdk.UnwrapSDKContext(ctx).BlockHeight()
		if err := k.ApprovedValidator.Set(ctx, msg.Candidate, types.ApprovedValidator{
			Candidate:   msg.Candidate,
			Approved:    strconv.FormatBool(true),
			Declaration: declaration,
		}); err != nil {
			return nil, err
		}
	} else {
		application.Status = types.StatusRejected
	}

	if err := k.ValidatorApplication.Set(ctx, msg.Candidate, application); err != nil {
		return nil, err
	}

	return &types.MsgApproveValidatorResponse{}, nil
}
