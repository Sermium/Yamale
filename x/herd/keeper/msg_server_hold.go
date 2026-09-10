package keeper

import (
	"context"
	"strings"

	"yamale/blockchain/x/herd/types"
)

// The way out.
//
// Every stop in this module is deliberately cheap to make. A keeper opens a
// transfer with one message; anybody at all — not a party, not accredited, not
// known to the registry — can stop it with another. That openness is not an
// oversight, it is the point: the person best placed to recognise a stolen
// animal is the neighbour who has seen it every day, and a registry that only
// listened to the parties to a sale would never hear from them.
//
// But a stop that nothing can lift is not a protection, it is a weapon. An
// objection costs one transaction's gas and, until this file existed, froze an
// animal in STATUS_DISPUTED permanently — no message could return it to ACTIVE.
// One adversary iterating over open transfers could have taken the national
// herd out of lawful circulation for a few thousand francs, and every honest
// party would have been left with a record saying their animal was disputed and
// no way on earth to say by whom it had been cleared.
//
// So each stop here has a counterpart, each counterpart is an office rather
// than an individual — a registry office is a group account, so it is already
// an M-of-N decision — and each one records who decided and why. The chain
// still does not adjudicate ownership; it cannot, and pretending otherwise
// would be the worse failure. It decides only whether this sale proceeds, and
// it makes that decision attributable.

// CancelTransfer withdraws a proposal the buyer never took up.
//
// The second liveness hole, and a quieter one than the objection. A keeper
// proposes a sale, the animal is frozen against a second transfer so that it
// cannot be sold twice, and then the buyer simply never answers — they change
// their mind, they lose the phone, they were never serious. Without this
// message that animal is unsellable for ever and its keeper has done nothing
// wrong at all.
//
// Only before an attestation. Once an accredited agent has travelled out and
// sworn the animal is the animal, the buyer has relied on the sale and so has
// the agent; a seller who could still walk away at that point would hold
// exactly the veto that CompleteTransfer being permissionless exists to deny.
func (m msgServer) CancelTransfer(
	ctx context.Context, msg *types.MsgCancelTransfer,
) (*types.MsgCancelTransferResponse, error) {
	transfer, err := m.openTransfer(ctx, msg.TransferId)
	if err != nil {
		return nil, err
	}
	if transfer.From != msg.Seller {
		return nil, types.ErrNotSeller.Wrapf("transfer %d was opened by %s", transfer.Id, transfer.From)
	}

	switch transfer.State {
	case types.TRANSFER_STATE_PROPOSED, types.TRANSFER_STATE_ACCEPTED:
		// Withdrawable.
	case types.TRANSFER_STATE_ATTESTED:
		return nil, types.ErrTransferAttested.Wrapf("transfer %d", transfer.Id)
	default:
		return nil, types.ErrTransferClosed.Wrapf("transfer %d is %s", transfer.Id, transfer.State)
	}

	transfer.State = types.TRANSFER_STATE_CANCELLED
	transfer.ResolutionReason = strings.TrimSpace(msg.Reason)
	transfer.ResolvedBy = msg.Seller
	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}

	// The record stays and the animal goes back into circulation. A sale
	// somebody opened and abandoned is part of an animal's provenance, so the
	// transfer is not deleted — it is closed.
	return &types.MsgCancelTransferResponse{}, m.setAnimalStatus(
		ctx, transfer.AnimalId, types.STATUS_ACTIVE)
}

// ResolveObjection is an office answering an objection.
//
// Dismissing restarts the challenge window rather than resuming it, and that is
// the detail worth defending. The obvious implementation puts the transfer back
// exactly as it was, which means an objection raised on the last day of the
// window is dismissed and the sale completes in the same minute — with nobody
// left able to object, because the window they would have used has run. An
// objection that has just been overruled is precisely the moment when somebody
// else is most likely to have something to add, and the cost of the restart
// falls on a sale that has already been contested once.
func (m msgServer) ResolveObjection(
	ctx context.Context, msg *types.MsgResolveObjection,
) (*types.MsgResolveObjectionResponse, error) {
	reason := strings.TrimSpace(msg.Reason)
	if reason == "" {
		return nil, types.ErrNoReason
	}

	transfer, err := m.Transfer.Get(ctx, msg.TransferId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoTransfer.Wrapf("transfer %d", msg.TransferId)
		}
		return nil, err
	}
	if transfer.State != types.TRANSFER_STATE_OBJECTED {
		return nil, types.ErrNotObjected.Wrapf("transfer %d is %s", transfer.Id, transfer.State)
	}

	animal, err := m.Animal.Get(ctx, transfer.AnimalId)
	if err != nil {
		return nil, err
	}
	office, err := m.resolvingOffice(ctx, msg.Authority, animal)
	if err != nil {
		return nil, err
	}

	transfer.ResolvedBy = office.Address
	transfer.ResolutionReason = reason
	transfer.ObjectionUpheld = msg.Uphold

	if msg.Uphold {
		// The objection had substance. The sale is dead and the animal is held
		// rather than returned to circulation, because whatever the objector
		// saw is now the registry's problem and not the buyer's. FROZEN rather
		// than DISPUTED: a freeze is a state an office owns and can lift, which
		// is what stops this from being a confiscation.
		transfer.State = types.TRANSFER_STATE_CANCELLED
		if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
			return nil, err
		}
		return &types.MsgResolveObjectionResponse{}, m.setAnimalStatus(
			ctx, animal.Id, types.STATUS_FROZEN)
	}

	// Dismissed: back to where the transfer was, judged by what has actually
	// happened to it rather than by a remembered state — the attestations are
	// on the record, so they are the truth about how far it had got.
	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	switch {
	case uint32(len(transfer.Attestations)) >= params.AttestationQuorum:
		transfer.State = types.TRANSFER_STATE_ATTESTED
		transfer.AttestedAt = blockTime(ctx)
	case transfer.To != "" && transfer.AcceptedAt != 0:
		transfer.State = types.TRANSFER_STATE_ACCEPTED
	default:
		transfer.State = types.TRANSFER_STATE_PROPOSED
	}
	if err := m.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
		return nil, err
	}
	return &types.MsgResolveObjectionResponse{}, m.setAnimalStatus(
		ctx, animal.Id, types.STATUS_TRANSFER_PENDING)
}

// FreezeAnimal stops an animal moving while an inquiry runs.
func (m msgServer) FreezeAnimal(
	ctx context.Context, msg *types.MsgFreezeAnimal,
) (*types.MsgFreezeAnimalResponse, error) {
	reason := strings.TrimSpace(msg.Reason)
	if reason == "" {
		return nil, types.ErrNoReason
	}

	animal, err := m.Animal.Get(ctx, msg.AnimalId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoAnimal.Wrapf("animal %d", msg.AnimalId)
		}
		return nil, err
	}
	if _, err := m.resolvingOffice(ctx, msg.Authority, animal); err != nil {
		return nil, err
	}

	// A retired animal is not frozen, it is gone, and pretending a freeze means
	// something on a dead animal would put a hold on the register that no
	// release could ever sensibly lift.
	if animal.Status == types.STATUS_RETIRED {
		return nil, types.ErrAnimalNotActive.Wrapf("animal %d is retired", animal.Id)
	}

	animal.Status = types.STATUS_FROZEN
	return &types.MsgFreezeAnimalResponse{}, m.Animal.Set(ctx, animal.Id, animal)
}

// ReleaseAnimal ends a hold.
func (m msgServer) ReleaseAnimal(
	ctx context.Context, msg *types.MsgReleaseAnimal,
) (*types.MsgReleaseAnimalResponse, error) {
	reason := strings.TrimSpace(msg.Reason)
	if reason == "" {
		return nil, types.ErrNoReason
	}

	animal, err := m.Animal.Get(ctx, msg.AnimalId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoAnimal.Wrapf("animal %d", msg.AnimalId)
		}
		return nil, err
	}
	if _, err := m.resolvingOffice(ctx, msg.Authority, animal); err != nil {
		return nil, err
	}

	// Only a hold is released. Releasing a TRANSFER_PENDING animal would
	// silently unfreeze it out from under a transfer that is still open,
	// leaving the sale live against an animal the register says is free — which
	// is the double-sale this module exists to prevent, reintroduced through
	// the door marked exit.
	switch animal.Status {
	case types.STATUS_FROZEN, types.STATUS_DISPUTED:
		// Releasable.
	default:
		return nil, types.ErrNotHeld.Wrapf("animal %d is %s", animal.Id, animal.Status)
	}

	animal.Status = types.STATUS_ACTIVE
	return &types.MsgReleaseAnimalResponse{}, m.Animal.Set(ctx, animal.Id, animal)
}

// resolvingOffice refuses anybody but the active registry office that holds the
// animal's record.
//
// Not any office. An animal's record belongs to one office's jurisdiction, and
// letting a second office lift the first one's hold would make the weakest
// office in the country the effective authority over every animal in it — a
// seller with a captured office in one region could clear objections raised
// anywhere.
//
// This does concentrate the decision: the office that holds a record is also
// the office that answers objections against sales from it, so an office
// dismissing every objection raised against one favoured seller is a pattern
// this code cannot refuse. It can only make it visible, which is why the
// decision and its author are written onto the transfer rather than merely
// applied to it. That is a supervision problem, and pretending a state machine
// solves it would be worse than saying so here.
func (m msgServer) resolvingOffice(
	ctx context.Context, signer string, animal types.Animal,
) (types.Authority, error) {
	office, err := m.activeAuthority(ctx, signer)
	if err != nil {
		return types.Authority{}, err
	}
	if office.Address != animal.Authority {
		return types.Authority{}, types.ErrWrongJurisdiction.Wrapf(
			"animal %d is registered with %s", animal.Id, animal.Authority)
	}
	return office, nil
}

// setAnimalStatus writes one status, leaving everything else alone.
func (m msgServer) setAnimalStatus(ctx context.Context, id uint64, status types.Status) error {
	animal, err := m.Animal.Get(ctx, id)
	if err != nil {
		return err
	}
	animal.Status = status
	return m.Animal.Set(ctx, id, animal)
}
