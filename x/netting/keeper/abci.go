package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/netting/types"
)

// EndBlocker closes the netting window when one falls due, settles it, and
// opens the next.
//
// In EndBlock rather than BeginBlock so that an obligation submitted in the
// last block of a window is in that window. Closing at the start of a block
// would settle a window that was still accepting traffic a moment earlier,
// which is a rule no participant's back office could reconcile against.
func (k Keeper) EndBlocker(ctx context.Context) error {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}

	// The guard in front of the modulo, and the reason it is here rather than
	// only in Params.Validate(). cycle_blocks is a divisor. A zero reaching
	// this line panics, and a panic in an end blocker is not a failed
	// transaction — every validator halts on the same block and the chain does
	// not restart until somebody edits state. This chain has been stopped that
	// way before. Validate() runs on a governance proposal and on `genesis
	// validate`; both are gates an operator can go around, and this one is not.
	if !params.NettingEnabled() {
		return nil
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()
	if height <= 0 {
		return nil
	}
	if height%int64(params.CycleBlocks) != 0 { //nolint:gosec // NettingEnabled bounds CycleBlocks well inside int64
		return nil
	}

	// Retried before the current window closes, so a slice that has been stuck
	// clears at the first opportunity rather than waiting behind a window that
	// may itself be about to get stuck.
	if err := k.retryHeldSlices(ctx); err != nil {
		return err
	}
	if err := k.closeCycle(ctx, height); err != nil {
		return err
	}
	return k.openCycle(ctx, height+1)
}

// closeCycle settles every currency in the open window, one currency at a
// time, and records what happened to each.
//
// Currencies are independent. A euro slice that cannot settle does not stop a
// naira slice that can, because there is no cross-currency netting on this
// chain and therefore no arithmetic that ties them together. Holding all of
// them because one failed would turn a single fault into a system-wide stop.
func (k Keeper) closeCycle(ctx context.Context, height int64) error {
	cycleID, err := k.CurrentCycle.Get(ctx)
	if err != nil {
		return err
	}
	cycle, err := k.Cycle.Get(ctx, cycleID)
	if err != nil {
		return err
	}

	groups, err := k.collectPositions(ctx, cycleID)
	if err != nil {
		return err
	}

	cycle.ClosedAtHeight = height
	cycle.Status = types.CYCLE_STATUS_SETTLED

	for _, group := range groups {
		settled, netAmount, reason := k.trySettle(ctx, cycleID, group)

		outcome := findOutcome(&cycle, group.denom)
		if settled {
			outcome.Status = types.DENOM_STATUS_SETTLED
			outcome.NetAmount = netAmount
			outcome.HoldReason = ""
			if err := k.emitSettled(ctx, cycleID, group, *outcome, false); err != nil {
				return err
			}
			continue
		}

		outcome.Status = types.DENOM_STATUS_HELD
		outcome.NetAmount = math.ZeroInt()
		outcome.HoldReason = reason
		cycle.Status = types.CYCLE_STATUS_HELD
		if err := k.HeldSlice.Set(ctx, collections.Join(cycleID, group.denom)); err != nil {
			return err
		}
		if err := sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventCycleHeld{
			CycleId: cycleID,
			Denom:   group.denom,
			Reason:  reason,
		}); err != nil {
			return err
		}
	}

	// A currency that was recorded when its first obligation arrived but whose
	// positions all cancelled out has nothing to settle and is closed as
	// settled with a net of zero. That is not a degenerate case, it is the best
	// possible outcome: total compression.
	for i := range cycle.Outcomes {
		if cycle.Outcomes[i].Status == types.DENOM_STATUS_OPEN {
			cycle.Outcomes[i].Status = types.DENOM_STATUS_SETTLED
			cycle.Outcomes[i].NetAmount = math.ZeroInt()
		}
	}

	return k.Cycle.Set(ctx, cycleID, cycle)
}

// openCycle starts the next window.
func (k Keeper) openCycle(ctx context.Context, openedAt int64) error {
	id, err := k.CycleSeq.Next(ctx)
	if err != nil {
		return err
	}
	if err := k.Cycle.Set(ctx, id, types.Cycle{
		Id:             id,
		OpenedAtHeight: openedAt,
		Status:         types.CYCLE_STATUS_OPEN,
	}); err != nil {
		return err
	}
	return k.CurrentCycle.Set(ctx, id)
}

// retryHeldSlices attempts every slice that previously refused to settle.
//
// Held obligations are never recomputed, never reassigned and never cancelled;
// they are simply tried again, unchanged, until they clear. That is the whole
// failure policy, and its cost is that a participant's collateral stays
// committed while a slice is stuck — which is the honest price of refusing to
// alter what institutions were told they owed.
func (k Keeper) retryHeldSlices(ctx context.Context) error {
	type slice struct {
		cycleID uint64
		denom   string
	}

	// Collected before acting, because settling a slice removes it from the
	// set being iterated, and mutating a store under its own iterator is how a
	// module skips half its work without ever failing.
	pending := make([]slice, 0)
	if err := k.HeldSlice.Walk(ctx, nil, func(key collections.Pair[uint64, string]) (bool, error) {
		pending = append(pending, slice{cycleID: key.K1(), denom: key.K2()})
		return false, nil
	}); err != nil {
		return err
	}

	for _, held := range pending {
		groups, err := k.collectPositions(ctx, held.cycleID)
		if err != nil {
			return err
		}
		var group *positionGroup
		for i := range groups {
			if groups[i].denom == held.denom {
				group = &groups[i]
				break
			}
		}
		if group == nil {
			// No positions left in a held slice means there is nothing to
			// settle. Recorded as settled rather than left in the queue
			// forever, so the operational alarm stays meaningful.
			if err := k.markRetried(ctx, held.cycleID, held.denom, math.ZeroInt(), nil); err != nil {
				return err
			}
			continue
		}

		settled, netAmount, _ := k.trySettle(ctx, held.cycleID, *group)
		if !settled {
			continue
		}
		if err := k.markRetried(ctx, held.cycleID, held.denom, netAmount, group); err != nil {
			return err
		}
	}

	return nil
}

// markRetried records that a held slice finally cleared.
func (k Keeper) markRetried(ctx context.Context, cycleID uint64, denom string, netAmount math.Int, group *positionGroup) error {
	if err := k.HeldSlice.Remove(ctx, collections.Join(cycleID, denom)); err != nil {
		return err
	}

	cycle, err := k.Cycle.Get(ctx, cycleID)
	if err != nil {
		return err
	}
	outcome := findOutcome(&cycle, denom)
	outcome.Status = types.DENOM_STATUS_SETTLED
	outcome.NetAmount = netAmount
	outcome.HoldReason = ""

	// The cycle stops being held only once nothing in it is.
	cycle.Status = types.CYCLE_STATUS_SETTLED
	for _, existing := range cycle.Outcomes {
		if existing.Status == types.DENOM_STATUS_HELD {
			cycle.Status = types.CYCLE_STATUS_HELD
			break
		}
	}
	if err := k.Cycle.Set(ctx, cycleID, cycle); err != nil {
		return err
	}

	if group == nil {
		return nil
	}
	return k.emitSettled(ctx, cycleID, *group, *outcome, true)
}

func (k Keeper) emitSettled(ctx context.Context, cycleID uint64, group positionGroup, outcome types.DenomOutcome, retried bool) error {
	return sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventCycleSettled{
		CycleId:          cycleID,
		Denom:            group.denom,
		GrossAmount:      outcome.GrossAmount,
		NetAmount:        outcome.NetAmount,
		ObligationCount:  outcome.ObligationCount,
		ParticipantCount: uint64(len(group.entries)),
		Retried:          retried,
	})
}

// findOutcome returns the cycle's record for a currency, creating it if the
// currency has none.
//
// A currency can hold positions with no outcome only if state was imported
// that way; a running chain writes the outcome when the first obligation
// arrives. Creating one here rather than failing means such a state still
// settles and still reports, with a gross figure of zero — visibly wrong in the
// compression number rather than invisibly absent from the record.
func findOutcome(cycle *types.Cycle, denom string) *types.DenomOutcome {
	for i := range cycle.Outcomes {
		if cycle.Outcomes[i].Denom == denom {
			return &cycle.Outcomes[i]
		}
	}
	cycle.Outcomes = append(cycle.Outcomes, types.DenomOutcome{
		Denom:       denom,
		Status:      types.DENOM_STATUS_OPEN,
		GrossAmount: math.ZeroInt(),
		NetAmount:   math.ZeroInt(),
	})
	return &cycle.Outcomes[len(cycle.Outcomes)-1]
}

// positionEntry is one participant's signed position in a currency.
type positionEntry struct {
	participant string
	amount      math.Int
}

// positionGroup is every position in one currency of one cycle, in store order.
type positionGroup struct {
	denom   string
	entries []positionEntry
}

// collectPositions reads a cycle's positions into memory, grouped by currency.
//
// Read into slices before anything is settled, for two reasons. Settlement
// writes to the reserve and locked stores, and collecting first keeps every
// write outside the iterator that produced the work. And the slices preserve
// the store's key order — cycle, then denom, then participant — which is byte
// order, which is the same on every validator.
//
// Nothing here may become a Go map, and it is worth being exact about which
// failure that rule prevents, because the obvious one is not the real one.
// Settling a slice is commutative — each participant's reserve is read and
// written once, and the debit and credit totals are sums — so reordering the
// entries within a currency does not change the resulting state. What a map
// would break is the *grouping*: the loop below relies on a currency's
// positions arriving contiguously, which the key order guarantees and a map
// does not. A denom split across two groups is two slices that each fail the
// zero-sum check, so a whole day's netting is held for a reason that exists
// nowhere in the data. And a map keyed by a collections.Pair is worse again —
// a Pair holds pointers, so it silently never matches itself and entries
// vanish rather than merely reorder.
func (k Keeper) collectPositions(ctx context.Context, cycleID uint64) ([]positionGroup, error) {
	groups := make([]positionGroup, 0)

	rng := collections.NewPrefixedTripleRange[uint64, string, string](cycleID)
	err := k.Position.Walk(ctx, rng, func(key collections.Triple[uint64, string, string], amount math.Int) (bool, error) {
		denom := key.K2()
		if len(groups) == 0 || groups[len(groups)-1].denom != denom {
			groups = append(groups, positionGroup{denom: denom})
		}
		group := &groups[len(groups)-1]
		group.entries = append(group.entries, positionEntry{participant: key.K3(), amount: amount})
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return groups, nil
}

// trySettle discharges one currency slice, all of it or none of it.
//
// Atomicity is the point, and in an end blocker it has to be built rather than
// assumed. There is no transaction here to roll back: an error returned from
// EndBlock halts the chain, and a write already made before the error stays
// made. So the whole slice runs against a cached branch of the store, and that
// branch is committed only if every leg succeeded. A partial settlement would
// leave institutions holding exposures to each other that neither of them
// agreed to and that no message in this module can undo.
//
// Nothing here moves coins. Every debit is covered by reserve this module is
// already custodying, so settlement is a rearrangement of claims on an account
// whose balance does not change — which means no external call can refuse it.
// A frozen creditor, a blocked address, a participant whose approval lapsed
// this morning: none of them can stop a cycle, because none of them is asked
// for anything. That is the second half of why settlement cannot fail, and it
// is worth more than the first: the net debit cap makes the money available,
// and settling inside the module account makes it unrefusable.
func (k Keeper) trySettle(ctx context.Context, cycleID uint64, group positionGroup) (settled bool, netAmount math.Int, reason string) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	branch, commit := sdkCtx.CacheContext()

	total, err := k.settleGroup(branch, group)
	if err != nil {
		return false, math.ZeroInt(), err.Error()
	}

	commit()
	return true, total, ""
}

// settleGroup does the arithmetic of one currency slice against a branch of
// the store.
func (k Keeper) settleGroup(ctx sdk.Context, group positionGroup) (math.Int, error) {
	debits := math.ZeroInt()
	credits := math.ZeroInt()
	for _, entry := range group.entries {
		if entry.amount.IsNegative() {
			debits = debits.Add(entry.amount.Neg())
			continue
		}
		credits = credits.Add(entry.amount)
	}

	// The invariant, checked before anything moves. Positions in a currency are
	// built by adding and subtracting the same figure, so they can only fail to
	// sum to zero if state has been corrupted or imported inconsistently — and
	// in that case settling the part that looks right would hand one
	// institution money another never owed. Refusing the whole slice leaves
	// every obligation in it intact and every participant in a state they can
	// still reason about.
	if !debits.Equal(credits) {
		return math.ZeroInt(), fmt.Errorf("%w: %s debits against %s credits in %s",
			types.ErrPositionsUnbalanced, debits, credits, group.denom)
	}

	for _, entry := range group.entries {
		if !entry.amount.IsNegative() {
			continue
		}
		owed := entry.amount.Neg()
		reserve, err := k.GetReserve(ctx, entry.participant, group.denom)
		if err != nil {
			return math.ZeroInt(), err
		}
		if reserve.LT(owed) {
			// Unreachable while every obligation went through the net debit
			// cap, which is exactly why it is checked. Defence in depth against
			// a state this module did not build itself — an imported genesis, a
			// migration, a bug in the cap — and the response to finding it is
			// to settle nothing, not to settle what fits.
			return math.ZeroInt(), fmt.Errorf("%w: %s owes %s%s against a reserve of %s",
				types.ErrInsufficientReserve, entry.participant, owed, group.denom, reserve)
		}
		if err := k.setReserve(ctx, entry.participant, group.denom, reserve.Sub(owed)); err != nil {
			return math.ZeroInt(), err
		}

		locked, err := k.GetLocked(ctx, entry.participant, group.denom)
		if err != nil {
			return math.ZeroInt(), err
		}
		if locked.LT(owed) {
			return math.ZeroInt(), fmt.Errorf("locked %s%s for %s is below the %s it owes",
				locked, group.denom, entry.participant, owed)
		}
		if err := k.setLocked(ctx, entry.participant, group.denom, locked.Sub(owed)); err != nil {
			return math.ZeroInt(), err
		}
	}

	for _, entry := range group.entries {
		if !entry.amount.IsPositive() {
			continue
		}
		reserve, err := k.GetReserve(ctx, entry.participant, group.denom)
		if err != nil {
			return math.ZeroInt(), err
		}
		// Credited into the reserve rather than paid out to the account. A
		// creditor's money is then available to fund its own debits in the next
		// window without a round trip through its balance, which is most of
		// where a netting system's liquidity saving actually comes from — and
		// it keeps the settlement step free of any transfer that could be
		// refused. Taking it out is a withdrawal, which is an ordinary
		// transaction with ordinary error handling.
		if err := k.setReserve(ctx, entry.participant, group.denom, reserve.Add(entry.amount)); err != nil {
			return math.ZeroInt(), err
		}
	}

	return debits, nil
}
