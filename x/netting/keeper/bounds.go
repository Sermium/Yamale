package keeper

import (
	"context"
	"strconv"
	"strings"

	"cosmossdk.io/collections"
)

// maxSlicesPerBlock bounds how many held slices one block will work through.
//
// The end blocker walked every entry in HeldSlice and, for each, walked every
// position in that cycle; escalateOldHolds walked all of HeldSince. Neither had
// a bound, and consensus max_gas on this chain is -1, so nothing else supplied
// one. Held slices accumulate whenever settlement refuses — across every
// denomination and every cycle — and are cleared only by settling, so the work
// per block grows with the number of things that have gone wrong. An error
// returned from an end blocker halts the chain rather than failing a message,
// which is what makes an unbounded loop here worth a bound rather than a note.
//
// x/enforcement's pruneSeizureLedger takes 256 a block for the same reason.
const maxSlicesPerBlock = 256

// nextSliceRange is where this block picks up.
//
// A bound alone would be a liveness bug: a fixed set of 256 slices would be
// retried forever and everything behind them never. So the cursor advances past
// the last slice looked at and wraps when the walk runs out, which makes the
// pass round-robin rather than truncated. Nothing depends on where a given
// block starts, so losing the cursor costs at most one extra sweep.
func nextSliceRange(cursor string) *collections.Range[collections.Pair[uint64, string]] {
	rng := new(collections.Range[collections.Pair[uint64, string]])
	if cursor == "" {
		return rng
	}
	cycleID, denom, ok := decodeSliceCursor(cursor)
	if !ok {
		return rng
	}
	return rng.StartExclusive(collections.Join(cycleID, denom))
}

// encodeSliceCursor renders a (cycle, denom) key as one string.
//
// A vertical bar separates them because a bank denomination cannot contain one:
// the SDK's denom charset is letters, digits and "/:._-", so no denom can forge
// a cursor that points somewhere else.
func encodeSliceCursor(cycleID uint64, denom string) string {
	return strconv.FormatUint(cycleID, 10) + "|" + denom
}

func decodeSliceCursor(cursor string) (uint64, string, bool) {
	cycle, denom, found := strings.Cut(cursor, "|")
	if !found {
		return 0, "", false
	}
	id, err := strconv.ParseUint(cycle, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return id, denom, true
}

// advanceCursor stores where the next block should start, or clears it when the
// sweep reached the end.
func (k Keeper) advanceCursor(ctx context.Context, cursor collections.Item[string], last string, exhausted bool) error {
	if exhausted {
		return cursor.Set(ctx, "")
	}
	return cursor.Set(ctx, last)
}

// readCursor is nil-safe: a chain upgraded into this code has no cursor stored,
// and starting from the beginning is exactly right.
func (k Keeper) readCursor(ctx context.Context, cursor collections.Item[string]) string {
	v, err := cursor.Get(ctx)
	if err != nil {
		return ""
	}
	return v
}
