package keeper

import (
	"bytes"
	"context"

	"yamale/blockchain/x/paymsg/types"

	errorsmod "cosmossdk.io/errors"
)

// ApproveParticipant is executed exclusively by the governance module
// account once a proposal to approve or reject a pending participant
// application has passed. Approval assigns the address its ISPB-equivalent
// participant code and permits it to appear as an InstructingParticipant or
// InstructedParticipant on payment instructions.
func (k msgServer) ApproveParticipant(ctx context.Context, msg *types.MsgApproveParticipant) (*types.MsgApproveParticipantResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authority) {
		expectedAuthorityStr, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expectedAuthorityStr, msg.Authority)
	}

	application, err := k.ParticipantApplication.Get(ctx, msg.Participant)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotFound, "no application found for %s", msg.Participant)
	}
	if application.Status != types.StatusPending {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotPending, "application for %s has status %s", msg.Participant, application.Status)
	}

	if msg.Approve {
		application.Status = types.StatusApproved
		if err := k.ApprovedParticipant.Set(ctx, msg.Participant, types.ApprovedParticipant{
			Participant: msg.Participant,
			Code:        application.Code,
			Name:        application.Name,
		}); err != nil {
			return nil, err
		}
	} else {
		application.Status = types.StatusRejected
	}

	if err := k.ParticipantApplication.Set(ctx, msg.Participant, application); err != nil {
		return nil, err
	}

	return &types.MsgApproveParticipantResponse{}, nil
}
