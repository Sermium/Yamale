package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/herd/types"
)

// The supervised transfer.
//
// It is not here to trace a theft afterwards. It is here to make the LAWFUL
// RESALE of a stolen animal impossible, which is what destroys its value — a
// thief can still take the beast, he simply cannot sell it with a title, and an
// animal that cannot be sold on is worth the meat rather than the pedigree.
//
// Four parties, and each one is there because removing it reopens a specific
// route:
//
//   - the SELLER proposes, so an animal cannot be assigned away from its keeper;
//   - the BUYER accepts, so a keeper cannot dump an animal on somebody who
//     never asked for it and walk away from a sanitary obligation;
//   - an ACCREDITED AGENT attests that the animal presented is the animal on the
//     record — the one claim in the whole system the chain cannot make itself;
//   - the REGISTRY holds a challenge window, so the person who actually owns a
//     stolen animal has time to say so.

// ProposeTransfer opens a handover.
func (m msgServer) ProposeTransfer(
	ctx context.Context, msg *types.MsgProposeTransfer,
) (*types.MsgProposeTransferResponse, error) {
	animal, err := m.Animal.Get(ctx, msg.AnimalId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoAnimal.Wrapf("animal %d", msg.AnimalId)
		}
		return nil, err
	}
	if animal.Holder != msg.Creator {
		return nil, types.ErrNotKeeper.Wrapf("animal %d is held by %s", animal.Id, animal.Holder)
	}
	if animal.Status != types.STATUS_ACTIVE {
		return nil, types.ErrAnimalNotActive.Wrapf("animal %d is %s", animal.Id, animal.Status)
	}
	if _, err := m.addressCodec.StringToBytes(msg.To); err != nil {
		return nil, types.ErrInvalidRecipient.Wrapf("%q", msg.To)
	}
	if msg.To == animal.Holder {
		return nil, types.ErrSelfTransfer
	}

	id, err := nextID(ctx, m.NextTransferID)
	if err != nil {
		return nil, err
	}

	transfer := types.Transfer{
		Id:         id,
		AnimalId:   animal.Id,
		From:       animal.Holder,
		To:         msg.To,
		Price:      msg.Price,
		State:      types.TRANSFER_STATE_PROPOSED,
		ProposedAt: blockTime(ctx),
	}
	if err := m.Transfer.Set(ctx, id, transfer); err != nil {
		return nil, err
	}
	if err := m.TransfersByAnimal.Set(ctx, collections.Join(animal.Id, id)); err != nil {
		return nil, err
	}

	// The animal is frozen against a second transfer. Without this a keeper
	// could open two sales in one block and complete whichever paid first,
	// which is the double-sale the registry exists to prevent.
	animal.Status = types.STATUS_TRANSFER_PENDING
	if err := m.Animal.Set(ctx, animal.Id, animal); err != nil {
		return nil, err
	}

	return &types.MsgProposeTransferResponse{Id: id}, nil
}

// AcceptTransfer is the buyer agreeing.
func (m msgServer) AcceptTransfer(
	ctx context.Context, msg *types.MsgAcceptTransfer,
) (*types.MsgAcceptTransferResponse, error) {
	transfer, err := m.openTransfer(ctx, msg.TransferId)
	if err != nil {
		return nil, err
	}
	if transfer.To != msg.Buyer {
		return nil, types.ErrNotBuyer.Wrapf("transfer %d names %s", transfer.Id, transfer.To)
	}
	if transfer.State != types.TRANSFER_STATE_PROPOSED {
		return nil, types.ErrTransferClosed.Wrapf("transfer %d is %s", transfer.Id, transfer.State)
	}

	transfer.State = types.TRANSFER_STATE_ACCEPTED
	transfer.AcceptedAt = blockTime(ctx)
	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}
	return &types.MsgAcceptTransferResponse{}, nil
}

// AttestTransfer is an accredited agent swearing the animal is the animal.
//
// This is the only claim in the system the chain cannot check for itself, which
// is why it needs a named person answering for it and why the evidence hash is
// recorded: a false attestation should be a specific claim by somebody, not a
// shrug that leaves no trace.
func (m msgServer) AttestTransfer(
	ctx context.Context, msg *types.MsgAttestTransfer,
) (*types.MsgAttestTransferResponse, error) {
	office, err := m.activeAuthority(ctx, msg.Attestor)
	if err != nil {
		return nil, err
	}

	transfer, err := m.openTransfer(ctx, msg.TransferId)
	if err != nil {
		return nil, err
	}
	if transfer.State != types.TRANSFER_STATE_ACCEPTED &&
		transfer.State != types.TRANSFER_STATE_ATTESTED {
		return nil, types.ErrNotAccepted.Wrapf("transfer %d is %s", transfer.Id, transfer.State)
	}

	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	animal, err := m.Animal.Get(ctx, transfer.AnimalId)
	if err != nil {
		return nil, err
	}

	// An attestor from the office that holds the animal's record is not
	// independent. Allowing it collapses the whole mechanism back to a single
	// office deciding, which is the arrangement the quorum exists to replace.
	if !params.SameAuthorityAttestation && office.Address == animal.Authority {
		return nil, types.ErrNotIndependent.Wrapf(
			"attestor %s answers to the same office as the record, %s",
			msg.Attestor, animal.Authority)
	}

	for _, existing := range transfer.Attestations {
		if existing.Attestor == msg.Attestor {
			return nil, types.ErrAlreadyAttested.Wrapf("%s", msg.Attestor)
		}
	}

	transfer.Attestations = append(transfer.Attestations, types.Attestation{
		Attestor: msg.Attestor,
		// Captured now rather than looked up later: an attestor can be struck
		// off, and "was this independent when it was made" has to stay
		// answerable afterwards.
		Authority:    office.Address,
		EvidenceHash: strings.TrimSpace(msg.EvidenceHash),
		AttestedAt:   blockTime(ctx),
	})

	// The challenge window runs from the moment quorum is reached, not from the
	// proposal. Otherwise a transfer could be opened quietly, left to age, and
	// completed the instant it was attested — which is a window in name only.
	if uint32(len(transfer.Attestations)) >= params.AttestationQuorum &&
		transfer.State != types.TRANSFER_STATE_ATTESTED {
		transfer.State = types.TRANSFER_STATE_ATTESTED
		transfer.AttestedAt = blockTime(ctx)
	}

	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}
	return &types.MsgAttestTransferResponse{}, nil
}

// ObjectTransfer stops one.
//
// Open to anybody, deliberately. The person best placed to notice a stolen
// animal being sold is rarely a party to the sale — it is the neighbour who
// recognises the beast, or the real owner who has been looking for it. A
// registry that only heard from the parties to a transaction would hear
// nothing from the one person the window exists for.
//
// The chain does not adjudicate. It records the objection, stops the transfer
// and preserves the evidence, which is where a court starts rather than where
// it finishes.
func (m msgServer) ObjectTransfer(
	ctx context.Context, msg *types.MsgObjectTransfer,
) (*types.MsgObjectTransferResponse, error) {
	transfer, err := m.openTransfer(ctx, msg.TransferId)
	if err != nil {
		return nil, err
	}

	// Only a live transfer can be objected to. Without this, objecting again to
	// something already stopped would overwrite the first objector's name with
	// the second's — so the cheapest way to erase an inconvenient objection
	// from the record would be to object to the same transfer yourself.
	switch transfer.State {
	case types.TRANSFER_STATE_PROPOSED,
		types.TRANSFER_STATE_ACCEPTED,
		types.TRANSFER_STATE_ATTESTED:
		// Objectable.
	default:
		return nil, types.ErrTransferClosed.Wrapf(
			"transfer %d is %s", transfer.Id, transfer.State)
	}

	transfer.State = types.TRANSFER_STATE_OBJECTED
	transfer.ObjectedBy = msg.Objector
	transfer.ObjectionReason = msg.Reason
	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}

	animal, err := m.Animal.Get(ctx, transfer.AnimalId)
	if err != nil {
		return nil, err
	}
	animal.Status = types.STATUS_DISPUTED
	if err := m.Animal.Set(ctx, animal.Id, animal); err != nil {
		return nil, err
	}
	return &types.MsgObjectTransferResponse{}, nil
}

// CompleteTransfer moves the animal.
//
// Permissionless on purpose. Every condition has already been checked by the
// chain, so who pays the gas is irrelevant — and requiring a particular signer
// would let that signer sit on a completed sale indefinitely, which is a veto
// nobody voted to give them.
func (m msgServer) CompleteTransfer(
	ctx context.Context, msg *types.MsgCompleteTransfer,
) (*types.MsgCompleteTransferResponse, error) {
	transfer, err := m.openTransfer(ctx, msg.TransferId)
	if err != nil {
		return nil, err
	}

	switch transfer.State {
	case types.TRANSFER_STATE_OBJECTED:
		return nil, types.ErrTransferObjected.Wrapf(
			"transfer %d was objected to by %s", transfer.Id, transfer.ObjectedBy)
	case types.TRANSFER_STATE_ATTESTED:
		// The only state from which a transfer completes.
	default:
		return nil, types.ErrQuorumNotMet.Wrapf("transfer %d is %s", transfer.Id, transfer.State)
	}

	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if elapsed := blockTime(ctx) - transfer.AttestedAt; elapsed < params.ChallengeWindow {
		return nil, types.ErrWindowOpen.Wrapf(
			"%d seconds of %d have passed since the last attestation",
			elapsed, params.ChallengeWindow)
	}

	animal, err := m.Animal.Get(ctx, transfer.AnimalId)
	if err != nil {
		return nil, err
	}
	animal.Holder = transfer.To
	animal.Status = types.STATUS_ACTIVE
	if err := m.Animal.Set(ctx, animal.Id, animal); err != nil {
		return nil, err
	}

	transfer.State = types.TRANSFER_STATE_COMPLETED
	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}
	return &types.MsgCompleteTransferResponse{}, nil
}

// openTransfer fetches a transfer that is not already finished.
func (m msgServer) openTransfer(ctx context.Context, id uint64) (types.Transfer, error) {
	transfer, err := m.Transfer.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return types.Transfer{}, types.ErrNoTransfer.Wrapf("transfer %d", id)
		}
		return types.Transfer{}, err
	}
	if transfer.State == types.TRANSFER_STATE_COMPLETED {
		return types.Transfer{}, types.ErrTransferClosed.Wrapf("transfer %d", id)
	}
	return transfer, nil
}
