package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/land/types"
)

// ProposeTransfer opens a transfer. Only the holder may.
//
// An authority cannot start the sale of somebody's land, which is the first of
// the four separations: the person losing the asset has to move first.
func (m msgServer) ProposeTransfer(
	ctx context.Context, msg *types.MsgProposeTransfer,
) (*types.MsgProposeTransferResponse, error) {
	parcel, err := m.Parcel.Get(ctx, msg.ParcelId)
	if err != nil {
		return nil, types.ErrNoParcel
	}
	if parcel.Holder != msg.Creator {
		return nil, types.ErrNotHolder
	}
	// FROZEN and DISPUTED are stops, not warnings. TRANSFER_PENDING prevents a
	// second, competing transfer — the on-chain form of selling twice.
	if parcel.Status != types.STATUS_REGISTERED {
		return nil, types.ErrParcelNotTransferable.Wrapf("status %s", parcel.Status)
	}
	if _, err := m.addressCodec.StringToBytes(msg.To); err != nil {
		return nil, types.ErrInvalidRecipient
	}
	if msg.To == parcel.Holder {
		return nil, types.ErrSelfTransfer
	}

	id, err := m.NextTransferID.Next(ctx)
	if err != nil {
		return nil, err
	}

	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	if err := m.Transfer.Set(ctx, id, types.Transfer{
		Id:         id,
		ParcelId:   parcel.Id,
		From:       parcel.Holder,
		To:         msg.To,
		Price:      msg.Price,
		ProposedAt: height,
	}); err != nil {
		return nil, err
	}

	parcel.Status = types.STATUS_TRANSFER_PENDING
	if err := m.Parcel.Set(ctx, parcel.Id, parcel); err != nil {
		return nil, err
	}

	return &types.MsgProposeTransferResponse{TransferId: id}, nil
}

// ValidateTransfer records the jurisdiction's own validation.
//
// Only the office that holds the parcel's file may do this: it is the one that
// can check the seller is who they claim to be, against paper the chain cannot
// see.
func (m msgServer) ValidateTransfer(
	ctx context.Context, msg *types.MsgValidateTransfer,
) (*types.MsgValidateTransferResponse, error) {
	transfer, parcel, err := m.pending(ctx, msg.TransferId)
	if err != nil {
		return nil, err
	}
	if _, err := m.activeAuthority(ctx, msg.Creator); err != nil {
		return nil, err
	}
	if msg.Creator != parcel.Authority {
		return nil, types.ErrWrongJurisdiction
	}
	if transfer.Validated {
		return nil, types.ErrAlreadyValidated
	}

	transfer.Validated = true
	transfer.ValidatedBy = msg.Creator
	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}
	return &types.MsgValidateTransferResponse{}, nil
}

// AttestTransfer adds one independent attestation.
//
// The independence rule is the whole mechanism, so it is enforced here rather
// than trusted: an attestor from the office that holds the parcel is not
// independent, and permitting it would collapse a quorum of many offices back
// into a single bribe. Governance can relax it (`same_authority_attestation`),
// which is a decision that should be visible and argued, not a default.
func (m msgServer) AttestTransfer(
	ctx context.Context, msg *types.MsgAttestTransfer,
) (*types.MsgAttestTransferResponse, error) {
	transfer, parcel, err := m.pending(ctx, msg.TransferId)
	if err != nil {
		return nil, err
	}
	if _, err := m.activeAuthority(ctx, msg.Creator); err != nil {
		return nil, err
	}

	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if !params.SameAuthorityAttestation && msg.Creator == parcel.Authority {
		return nil, types.ErrNotIndependent
	}
	// One office, one attestation. Without this the quorum is satisfiable by a
	// single signer sending the same message three times.
	for _, existing := range transfer.Attestors {
		if existing == msg.Creator {
			return nil, types.ErrAlreadyAttested
		}
	}

	transfer.Attestors = append(transfer.Attestors, msg.Creator)

	// The challenge window runs from quorum, not from proposal: the public
	// clock should start only once the transfer is real enough to object to.
	if uint32(len(transfer.Attestors)) >= params.AttestationQuorum && transfer.QuorumAt == 0 {
		transfer.QuorumAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	}

	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}
	return &types.MsgAttestTransferResponse{
		Attestations: uint32(len(transfer.Attestors)),
		Quorum:       params.AttestationQuorum,
	}, nil
}

// Object halts a transfer. Open to anyone, deliberately.
//
// Requiring standing would exclude exactly the people this protects: somebody
// whose land is being sold from under them usually has no official
// relationship to prove. One objection is enough — the chain stops and a court
// decides, because deciding is not the chain's job.
func (m msgServer) Object(
	ctx context.Context, msg *types.MsgObject,
) (*types.MsgObjectResponse, error) {
	transfer, parcel, err := m.pending(ctx, msg.TransferId)
	if err != nil {
		return nil, err
	}
	if msg.Reason == "" {
		return nil, types.ErrNoReason
	}

	transfer.ObjectedBy = msg.Creator
	transfer.ObjectionReason = msg.Reason
	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}

	parcel.Status = types.STATUS_DISPUTED
	if err := m.Parcel.Set(ctx, parcel.Id, parcel); err != nil {
		return nil, err
	}
	return &types.MsgObjectResponse{}, nil
}

// CompleteTransfer applies a transfer that has met every condition.
//
// Callable by anyone, on purpose. If only an official could finalise, an
// official could refuse to — and a refusal that costs the owner their sale is
// leverage worth paying to remove. Here the last step is arithmetic: the
// conditions either hold or they do not, and nobody has a discretionary say.
func (m msgServer) CompleteTransfer(
	ctx context.Context, msg *types.MsgCompleteTransfer,
) (*types.MsgCompleteTransferResponse, error) {
	transfer, parcel, err := m.pending(ctx, msg.TransferId)
	if err != nil {
		return nil, err
	}

	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	if !transfer.Validated {
		return nil, types.ErrNotValidated
	}
	if uint32(len(transfer.Attestors)) < params.AttestationQuorum {
		return nil, types.ErrNoQuorum.Wrapf(
			"%d of %d", len(transfer.Attestors), params.AttestationQuorum)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if transfer.QuorumAt == 0 || now < transfer.QuorumAt+params.ChallengeWindow {
		return nil, types.ErrChallengeWindowOpen
	}

	parcel.Holder = transfer.To
	parcel.Status = types.STATUS_REGISTERED
	if err := m.Parcel.Set(ctx, parcel.Id, parcel); err != nil {
		return nil, err
	}

	transfer.CompletedAt = now
	// Kept, not deleted. The record of who signed what and when is the receipt a
	// dispossessed owner does not currently get, and it is worth more than the
	// bytes it costs.
	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}
	return &types.MsgCompleteTransferResponse{}, nil
}

// pending loads a transfer that is still open, with its parcel.
func (m msgServer) pending(
	ctx context.Context, id uint64,
) (types.Transfer, types.Parcel, error) {
	transfer, err := m.Transfer.Get(ctx, id)
	if err != nil {
		return types.Transfer{}, types.Parcel{}, types.ErrNoTransfer
	}
	if transfer.CompletedAt != 0 {
		return types.Transfer{}, types.Parcel{}, types.ErrTransferClosed
	}
	if transfer.ObjectedBy != "" {
		return types.Transfer{}, types.Parcel{}, types.ErrTransferDisputed
	}
	parcel, err := m.Parcel.Get(ctx, transfer.ParcelId)
	if err != nil {
		return types.Transfer{}, types.Parcel{}, types.ErrNoParcel
	}
	return transfer, parcel, nil
}
