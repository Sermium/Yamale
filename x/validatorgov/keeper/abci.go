package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/validatorgov/types"
)

// EndBlocker completes the rotations whose delay or challenge window has run
// out.
//
// It is a queue walk bounded by what falls due at this height, not a scan of
// every rotation ever opened. There is no arithmetic here at all — no division,
// no modulus, and no parameter used as a divisor — because the heights were
// computed when the rotation was opened, by a path that substitutes a default
// for a zero parameter. A block producer must never be able to halt this chain
// by proposing a Params value.
func (k Keeper) EndBlocker(ctx context.Context) error {
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()

	due := make([]uint64, 0)
	iter, err := k.RotationQueue.Iterate(ctx, collections.NewPrefixUntilPairRange[int64, uint64](height))
	if err != nil {
		return err
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		key, err := iter.Key()
		if err != nil {
			return err
		}
		due = append(due, key.K2())
	}

	// Collected first, then acted on: completing a rotation removes it from the
	// queue being iterated, and mutating a store under its own iterator is how
	// a module skips half its work without ever failing.
	for _, id := range due {
		rotation, err := k.Rotation.Get(ctx, id)
		if err != nil {
			return err
		}
		if rotation.Status != types.ROTATION_STATUS_PENDING {
			// Already vetoed or cancelled; the queue entry is stale.
			if err := k.RotationQueue.Remove(ctx, collections.Join(rotation.CompletesAtHeight, id)); err != nil {
				return err
			}
			continue
		}
		if err := k.completeRotation(ctx, &rotation); err != nil {
			return err
		}
	}

	return nil
}
