package keeper

import (
	"bytes"
	"context"

	"yamale/blockchain/x/paymsg/types"

	errorsmod "cosmossdk.io/errors"
)

// ApproveParticipant admits an institution to the rail: it assigns the address
// its ISPB-equivalent participant code and permits it to appear as an
// InstructingParticipant or InstructedParticipant on payment instructions.
//
// Two signers are accepted:
//
//   - governance, as before. It is the body that grants the roles, so requiring
//     it to hold one would be circular.
//   - the payments authority of the country the applicant is recorded in. This is
//     the office that licenses payment service providers in the deployments this
//     chain is built for, and making every such licence a chain-wide governance
//     vote made every national decision everybody's business.
//
// The perimeter is what stops that being a widening: an authority granted GH can
// admit an applicant recorded in GH and nothing else, and an applicant the chain
// cannot place is refused to it. Note that this makes recording the applicant's
// jurisdiction a precondition of the delegated path — which is the intended
// order of events, since an institution nobody has placed is an institution no
// authority is accountable for.
func (k msgServer) ApproveParticipant(ctx context.Context, msg *types.MsgApproveParticipant) (*types.MsgApproveParticipantResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	governance := bytes.Equal(k.GetAuthority(), authority)

	application, err := k.ParticipantApplication.Get(ctx, msg.Participant)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotFound, "no application found for %s", msg.Participant)
	}

	// The applicant is the target. Taken from the message's participant field,
	// which is also the application's key, so there is nothing for the signer to
	// name that would move the act into a perimeter they hold.
	if !governance {
		if err := k.assertScope(ctx, msg.Authority, msg.Participant); err != nil {
			return nil, err
		}
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
