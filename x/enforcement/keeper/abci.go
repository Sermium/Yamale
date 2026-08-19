package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
)

// EndBlocker resolves the cases whose voting period has ended and lifts the
// provisional freezes that have run out.
//
// Both are queue walks bounded by what falls due at this height, not scans of
// every case ever opened. A module that gets slower as its history grows would
// eventually make blocks late, and the thing it would be late doing is giving
// somebody their account back.
func (k Keeper) EndBlocker(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()

	if err := k.resolveDueCases(ctx, height); err != nil {
		return err
	}
	if err := k.executeDueSeizures(ctx, height); err != nil {
		return err
	}
	if err := k.expireFreezes(ctx, height); err != nil {
		return err
	}

	// Read once, here, rather than inside the prune: a parameter fetch that
	// fails must not be the reason a block does not end.
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}
	return k.pruneSeizureLedger(ctx, params, height)
}

// executeDueSeizures carries out the seizures whose delay has expired.
//
// It runs after resolveDueCases and never in the same block as the vote that
// decided a case, because the shortest delay any parameter set may carry is one
// block and a delay of zero is refused where the parameters are validated and
// again where they are used. A seizure that executed in the block it was agreed
// would leave the ombudsman's veto with nowhere to land.
func (k Keeper) executeDueSeizures(ctx context.Context, height int64) error {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}

	// The whole key, not just the case id. Removing by the key the iterator
	// actually produced is what keeps this exact: a case that was deferred is
	// queued at its retry height rather than at the height its delay expired,
	// and reconstructing that from the case would be a second source of truth
	// for where the entry is.
	due := make([]collections.Pair[int64, uint64], 0)
	iter, err := k.ExecutionQueue.Iterate(ctx, collections.NewPrefixUntilPairRange[int64, uint64](height))
	if err != nil {
		return err
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		key, err := iter.Key()
		if err != nil {
			return err
		}
		due = append(due, key)
	}

	// Collected first, then acted on, for the same reason resolveDueCases does
	// it: executing a case removes its entry from the queue being iterated, and
	// a deferral adds a new one.
	for _, key := range due {
		if err := k.ExecutionQueue.Remove(ctx, key); err != nil {
			return err
		}

		enforcementCase, err := k.Case.Get(ctx, key.K2())
		if err != nil {
			return err
		}
		// Anything that is no longer held was stopped after it was queued — a
		// veto, a reversal, an emergency release. The entry has already been
		// removed above, which is all that was left to do; carrying the case
		// out would be seizing from an account that has been given back.
		if enforcementCase.Status != types.CASE_STATUS_HELD {
			continue
		}
		if err := k.executeSeizure(ctx, &enforcementCase, params, height); err != nil {
			return err
		}
	}

	return nil
}

// resolveDueCases closes every case whose voting period ended at or before this
// height. A case that reached the threshold is carried out; one that did not
// expires, which is a different outcome from being rejected: nobody having
// voted is not a finding of innocence, and the record should not read as one.
func (k Keeper) resolveDueCases(ctx context.Context, height int64) error {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}

	due := make([]uint64, 0)
	iter, err := k.VotingQueue.Iterate(ctx, collections.NewPrefixUntilPairRange[int64, uint64](height))
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

	// Collected first, then acted on: resolving a case removes it from the
	// queue being iterated, and mutating a store under its own iterator is how
	// a module skips half its work without ever failing.
	for _, id := range due {
		enforcementCase, err := k.Case.Get(ctx, id)
		if err != nil {
			return err
		}
		if enforcementCase.Status != types.CASE_STATUS_VOTING {
			// Already resolved early by a vote; the queue entry is stale.
			if err := k.dequeueVoting(ctx, enforcementCase); err != nil {
				return err
			}
			continue
		}

		if enforcementCase.YesPower >= params.RequiredPower(enforcementCase.TotalPowerAtOpen) {
			if err := k.passCase(ctx, &enforcementCase); err != nil {
				return err
			}
			continue
		}
		if err := k.rejectCase(ctx, &enforcementCase, types.CASE_STATUS_EXPIRED); err != nil {
			return err
		}
	}

	return nil
}

// expireFreezes lifts the provisional freezes that have reached their expiry
// height.
//
// This is the limit on what one validator can do alone. Without it, opening a
// case and then doing nothing would freeze an account permanently, and the
// power to do that quietly is precisely what this module must not hand anybody.
func (k Keeper) expireFreezes(ctx context.Context, height int64) error {
	expired := make([]string, 0)
	iter, err := k.FreezeExpiryQueue.Iterate(ctx, collections.NewPrefixUntilPairRange[int64, string](height))
	if err != nil {
		return err
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		key, err := iter.Key()
		if err != nil {
			return err
		}
		expired = append(expired, key.K2())
	}

	for _, addr := range expired {
		freeze, found, err := k.FreezeOf(ctx, addr)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if err := k.unfreeze(ctx, addr); err != nil {
			return err
		}
		if err := sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventFreezeLifted{
			Address: addr,
			CaseId:  freeze.CaseId,
			Status:  types.CASE_STATUS_EXPIRED,
		}); err != nil {
			return err
		}
	}

	return nil
}
