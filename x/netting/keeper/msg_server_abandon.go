package keeper

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/netting/types"
)

// AbandonHeldSlice releases the collateral behind a slice that will not settle,
// and gives up on settling it.
//
// # Why this exists
//
// A held slice locks its participants' reserve and is retried unchanged at
// every cycle boundary, forever. That is right for a slice that might yet
// settle and unacceptable for one that never will, and nothing in this module
// can tell the two apart — which is exactly why the decision is governance's
// and not a keeper's. An independent review on 2026-08-31 named the unbounded
// hold as the defect; this is the bound's other half, the escalation event
// being the first.
//
// # What it does not do, which is the point
//
// It does not touch a single obligation. None is edited, cancelled, reassigned
// or recomputed. That restraint is the whole reason this message is safe to
// have at all: the institutions in the slice were told what they owed each
// other, and that record stands. What is abandoned is this ledger's attempt to
// settle it, not the debt.
//
// Say it the blunt way, because whoever proposes one of these should have read
// it: this hands collateral back to debtors while the creditors they owe have
// not been paid. It belongs at the end of a documented failure investigation.
func (k msgServer) AbandonHeldSlice(
	ctx context.Context, msg *types.MsgAbandonHeldSlice,
) (*types.MsgAbandonHeldSliceResponse, error) {
	if err := k.assertAuthority(msg.Authority); err != nil {
		return nil, err
	}
	if msg.Reason == "" {
		// Required, not merely encouraged. A release of somebody else's
		// collateral with no stated grounds is one nobody can argue with
		// afterwards, and the argument afterwards is the only accountability
		// this message has.
		return nil, errorsmod.Wrap(types.ErrInvalidAmount,
			"a reason is required: this releases collateral from debtors whose creditors were never paid")
	}

	key := collections.Join(msg.CycleId, msg.Denom)
	if has, err := k.HeldSlice.Has(ctx, key); err != nil {
		return nil, err
	} else if !has {
		// Deliberately refuses a slice that is merely unsettled rather than
		// held. This message must never become a way to cancel a cycle that
		// was about to work.
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount,
			"cycle %d has no held slice in %s", msg.CycleId, msg.Denom)
	}

	// Every debtor in the slice, and what each has locked behind it. Collected
	// before anything is written, because releasing inside the walk would
	// mutate the store under its own iterator.
	type release struct {
		participant string
		amount      math.Int
	}
	var releases []release
	rng := collections.NewSuperPrefixedTripleRange[uint64, string, string](msg.CycleId, msg.Denom)
	if err := k.Position.Walk(ctx, rng, func(
		pos collections.Triple[uint64, string, string], amount math.Int,
	) (bool, error) {
		if !amount.IsNegative() {
			// A creditor has nothing locked against this slice. It is owed
			// money it will not now receive here, which is the cost of this
			// message and is recorded in the event rather than silently.
			return false, nil
		}
		releases = append(releases, release{participant: pos.K3(), amount: amount.Neg()})
		return false, nil
	}); err != nil {
		return nil, err
	}

	// Sorted, because the event's contents are consensus state and a map or an
	// iterator's incidental order would differ between nodes.
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].participant < releases[j].participant
	})

	released := make([]string, 0, len(releases))
	for _, r := range releases {
		locked, err := k.GetLocked(ctx, r.participant, msg.Denom)
		if err != nil {
			return nil, err
		}
		if locked.LT(r.amount) {
			// Unreachable while every obligation passed the net debit cap, and
			// checked for the same reason settleGroup checks it: reaching it
			// means collateral was released that was never held, and the
			// response is to change nothing rather than to release what fits.
			return nil, errorsmod.Wrapf(types.ErrInvalidAmount,
				"%s has %s%s locked against a slice owing %s", r.participant, locked, msg.Denom, r.amount)
		}
		if err := k.setLocked(ctx, r.participant, msg.Denom, locked.Sub(r.amount)); err != nil {
			return nil, err
		}
		released = append(released, fmt.Sprintf("%s:%s%s", r.participant, r.amount, msg.Denom))
	}

	// The positions go, because the slice is no longer going to settle and a
	// position left behind would be retried into an unbalanced state at the
	// next boundary. The OBLIGATIONS stay, which is the distinction this whole
	// message turns on: what is discarded is the netting arithmetic, not the
	// record of who owed what.
	var keys []collections.Triple[uint64, string, string]
	if err := k.Position.Walk(ctx, rng, func(
		pos collections.Triple[uint64, string, string], _ math.Int,
	) (bool, error) {
		keys = append(keys, pos)
		return false, nil
	}); err != nil {
		return nil, err
	}
	for _, pos := range keys {
		if err := k.Position.Remove(ctx, pos); err != nil {
			return nil, err
		}
	}

	since, err := k.HeldSince.Get(ctx, key)
	if err != nil {
		since = msg.CycleId
	}
	current, err := k.CurrentCycle.Get(ctx)
	if err != nil {
		return nil, err
	}

	if err := k.HeldSlice.Remove(ctx, key); err != nil {
		return nil, err
	}
	if err := k.HeldSince.Remove(ctx, key); err != nil {
		return nil, err
	}

	if err := k.markSliceAbandoned(ctx, msg.CycleId, msg.Denom, msg.Reason); err != nil {
		return nil, err
	}

	if err := sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventHeldSliceAbandoned{
		CycleId:    msg.CycleId,
		Denom:      msg.Denom,
		CyclesHeld: current - since,
		Reason:     msg.Reason,
		Released:   released,
	}); err != nil {
		return nil, err
	}

	return &types.MsgAbandonHeldSliceResponse{Released: released}, nil
}

// markSliceAbandoned records the outcome on the cycle so a reader of history
// can tell a slice that settled from one somebody gave up on.
func (k Keeper) markSliceAbandoned(ctx context.Context, cycleID uint64, denom, reason string) error {
	cycle, err := k.Cycle.Get(ctx, cycleID)
	if err != nil {
		return err
	}
	for i := range cycle.Outcomes {
		if cycle.Outcomes[i].Denom != denom {
			continue
		}
		cycle.Outcomes[i].NetAmount = math.ZeroInt()
		// The reason is overwritten rather than appended: what matters to
		// whoever reads this later is why it was abandoned, and the original
		// settlement failure is already on the EventCycleHeld that preceded it.
		cycle.Outcomes[i].HoldReason = "abandoned by governance: " + reason
	}
	return k.Cycle.Set(ctx, cycleID, cycle)
}

// assertAuthority refuses anybody but the governance account.
func (k Keeper) assertAuthority(addr string) error {
	bz, err := k.addressCodec.StringToBytes(addr)
	if err != nil {
		return err
	}
	if !bytes.Equal(bz, k.authority) {
		return errorsmod.Wrapf(types.ErrInvalidSigner,
			"only governance may abandon a held slice; got %s", addr)
	}
	return nil
}
