package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
)

// The rolling window is one ledger and no counters.
//
// Two implementations of a rolling cap are obvious and both are wrong. Summing
// every seizure the chain has ever carried out gets slower forever, and the
// thing it eventually makes late is a block. Resetting a running total on a
// fixed boundary is not rolling at all: it lets twice the cap through by
// putting half the seizures either side of the reset, and whoever discovered
// that would not discover it by accident.
//
// What is here instead is a ledger keyed by (height, case id) and summed by a
// range scan from the window's start height. That is exactly the entries inside
// the window and nothing else, and the number of them is bounded by the cap
// itself — a chain that permits five seizures a week has at most five records
// to add up. The cap pays for its own enforcement.
//
// Nothing is forgotten, either. The sum's lower bound is computed from the
// current height every time it is asked, so correctness does not depend on
// pruning having run. Pruning exists only to stop the store growing forever,
// and if it ever fell behind the window would be measured correctly anyway.

// maxPrunedPerBlock bounds how many expired ledger records one block clears.
//
// The bound is belt and braces: the cap already limits how many records can
// enter a window, so the number leaving one at any height is limited too.
// It is here for the case that limit was lowered by governance while a larger
// set of records was already in flight — leaving some for the next block makes
// the window very slightly conservative, which refuses seizures rather than
// admitting them, which is the direction that does not lose anybody's money.
const maxPrunedPerBlock = 256

// seizureWindow sums what has been seized inside the current window and counts
// the seizures it took.
func (k Keeper) seizureWindow(ctx context.Context, params types.Params, height int64) (sdk.Coins, uint64, error) {
	start := params.WindowStartHeight(height)

	total := sdk.NewCoins()
	count := uint64(0)

	// From the window's start to the end of the ledger. Height is the first
	// component of the key, so this touches only the records inside the window.
	rng := new(collections.Range[collections.Pair[int64, uint64]]).
		StartInclusive(collections.Join(start, uint64(0)))

	if err := k.SeizureLedger.Walk(ctx, rng, func(_ collections.Pair[int64, uint64], record types.SeizureRecord) (bool, error) {
		total = total.Add(record.Amount...)
		count++
		return false, nil
	}); err != nil {
		return nil, 0, err
	}

	return total, count, nil
}

// admitSeizure decides whether a seizure of this value may be carried out now.
//
// It returns the height at which there could be room if the answer is no, and a
// sentence saying which limit refused it. Both are put on the event rather than
// left for a reader to reconstruct from the parameters, because a frozen
// account waiting on a case that will not execute deserves a better answer than
// "not yet".
func (k Keeper) admitSeizure(ctx context.Context, params types.Params, height int64, value sdk.Coins) (bool, int64, string, error) {
	total, count, err := k.seizureWindow(ctx, params, height)
	if err != nil {
		return false, 0, "", err
	}

	// The count limit first, because it is the one that binds every
	// denomination — including a currency issued the day after the value cap
	// was last set, which the value cap would not mention at all.
	if count >= params.MaxSeizuresPerWindow {
		retry, err := k.nextWindowOpening(ctx, params, height)
		if err != nil {
			return false, 0, "", err
		}
		return false, retry, fmt.Sprintf(
			"%d seizures have already been carried out in this window and the maximum is %d",
			count, params.MaxSeizuresPerWindow), nil
	}

	for _, cap := range params.SeizureWindowCap {
		want := value.AmountOf(cap.Denom)
		if want.IsZero() {
			continue
		}
		if total.AmountOf(cap.Denom).Add(want).GT(cap.Amount) {
			retry, err := k.nextWindowOpening(ctx, params, height)
			if err != nil {
				return false, 0, "", err
			}
			return false, retry, fmt.Sprintf(
				"seizing %s%s would take this window to %s against a cap of %s",
				want, cap.Denom, total.AmountOf(cap.Denom).Add(want), cap), nil
		}
	}

	return true, 0, "", nil
}

// nextWindowOpening is the earliest height at which the window could have room:
// one block after the oldest record inside it falls out.
//
// When the ledger is empty and a seizure was still refused, the seizure is
// larger than the whole window's budget and no amount of waiting helps. The
// retry is set a full window ahead anyway, so the case keeps saying so, once a
// window, on the record — rather than being silently dropped or silently
// retried every block. Governance raising the cap or reversing the case are the
// two ways out, and both need somebody to know it is stuck.
func (k Keeper) nextWindowOpening(ctx context.Context, params types.Params, height int64) (int64, error) {
	start := params.WindowStartHeight(height)
	window := params.WindowBlocks()

	rng := new(collections.Range[collections.Pair[int64, uint64]]).
		StartInclusive(collections.Join(start, uint64(0)))

	oldest := int64(-1)
	if err := k.SeizureLedger.Walk(ctx, rng, func(key collections.Pair[int64, uint64], _ types.SeizureRecord) (bool, error) {
		oldest = key.K1()
		// Keys come back in order, so the first one is the oldest and there is
		// nothing to learn from the rest.
		return true, nil
	}); err != nil {
		return 0, err
	}

	var retry int64
	if oldest < 0 {
		retry = height + int64(window)
	} else {
		retry = oldest + int64(window) + 1
	}

	// A retry at or before the current height would be re-checked in the same
	// end blocker that scheduled it, or in the next one, forever. The window
	// guard in WindowBlocks already refuses a zero, and this refuses whatever
	// arithmetic still managed to point backwards.
	if retry <= height {
		retry = height + 1
	}
	return retry, nil
}

// recordSeizure writes an executed seizure into the window's ledger.
//
// The amount recorded is the larger, per denomination, of what the case was
// assessed at when it was decided and what execution actually moved. The
// assessment is normally the larger of the two, because it counts stake that
// arrives weeks later — but a deposit landing in the target's account during
// the hold would otherwise be taken without ever being counted against the cap,
// and a cap that can be stepped over by paying the target is not one.
func (k Keeper) recordSeizure(ctx context.Context, caseID uint64, height int64, assessed, collected sdk.Coins) error {
	amount := assessed.Max(collected)
	return k.SeizureLedger.Set(ctx, collections.Join(height, caseID), types.SeizureRecord{
		CaseId: caseID,
		Height: height,
		Amount: amount,
	})
}

// pruneSeizureLedger clears records that have fallen out of the window.
//
// Correctness does not depend on this running: every sum computes its own lower
// bound from the current height, so a record left behind is not counted. This
// only stops the store growing without limit.
func (k Keeper) pruneSeizureLedger(ctx context.Context, params types.Params, height int64) error {
	start := params.WindowStartHeight(height)
	if start <= 0 {
		return nil
	}

	stale := make([]collections.Pair[int64, uint64], 0)
	rng := new(collections.Range[collections.Pair[int64, uint64]]).
		EndExclusive(collections.Join(start, uint64(0)))

	if err := k.SeizureLedger.Walk(ctx, rng, func(key collections.Pair[int64, uint64], _ types.SeizureRecord) (bool, error) {
		stale = append(stale, key)
		return len(stale) >= maxPrunedPerBlock, nil
	}); err != nil {
		return err
	}

	// Collected first, then removed: deleting under an iterator is how a walk
	// silently skips half its work without ever failing.
	for _, key := range stale {
		if err := k.SeizureLedger.Remove(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
