package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/constitution/types"
)

// EndBlocker enacts the amendments whose public delay has run out, and lapses
// the ones that ran out without enough ratified power behind them.
//
// It is a queue walk bounded by what falls due at this height, not a scan of
// every amendment ever opened. There is no division by a stored value anywhere
// in it: the effective height was computed when the amendment opened, and the
// threshold divides by the basis-point constant. A block producer must never be
// able to halt this chain by proposing a settlement.
func (k Keeper) EndBlocker(ctx context.Context) error {
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()

	due := make([]uint64, 0)
	iter, err := k.AmendmentQueue.Iterate(ctx, collections.NewPrefixUntilPairRange[int64, uint64](height))
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

	// Collected first, then acted on: resolving an amendment removes it from
	// the queue being iterated, and mutating a store under its own iterator is
	// how a module skips half its work without ever failing.
	for _, id := range due {
		amendment, err := k.Amendment.Get(ctx, id)
		if err != nil {
			return err
		}
		if amendment.Status != types.AMENDMENT_STATUS_PENDING {
			// Already withdrawn; the queue entry is stale.
			if err := k.AmendmentQueue.Remove(ctx, collections.Join(amendment.EffectiveAtHeight, id)); err != nil {
				return err
			}
			continue
		}

		current, err := k.GetInvariants(ctx)
		if err != nil {
			return err
		}

		// Measured against the power recorded when the amendment opened, not
		// against the power bonded now. A threshold measured against the set
		// that remains is passed by jailing everyone who would have refused.
		if amendment.RatifiedPower < current.RequiredPower(amendment.SnapshotPower) {
			if err := k.resolve(ctx, &amendment, types.AMENDMENT_STATUS_LAPSED); err != nil {
				return err
			}
			continue
		}

		// Written before the amendment is marked enacted, so that the event
		// resolve emits carries the threshold from the settlement that was in
		// force while the amendment was running — the one it actually had to
		// clear.
		if err := k.resolve(ctx, &amendment, types.AMENDMENT_STATUS_ENACTED); err != nil {
			return err
		}
		if err := k.Invariants.Set(ctx, amendment.Proposed); err != nil {
			return err
		}
	}

	return nil
}
