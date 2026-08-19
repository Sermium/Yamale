package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/constitution/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
//
// Validated here and not only in `genesis validate`, because the two are not
// the same gate. `genesis validate` is a command an operator may or may not run
// against the file they actually distributed; this runs in InitChain, on the
// bytes the chain is really starting from. Returning an error here stops the
// chain from starting at all, which for the module holding the seizure
// threshold and the address seized assets go to is the correct response to a
// genesis that leaves either unset.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	if err := genState.Validate(); err != nil {
		return fmt.Errorf("constitution genesis is invalid, refusing to start: %w", err)
	}

	if err := k.Invariants.Set(ctx, genState.Invariants); err != nil {
		return err
	}

	for _, amendment := range genState.Amendments {
		if err := k.Amendment.Set(ctx, amendment.Id, amendment); err != nil {
			return err
		}
		// Only a pending amendment belongs in the queue. Putting a resolved one
		// back would resolve it a second time at the height it originally
		// ended, and for an enacted amendment that means re-enacting a
		// settlement the chain may since have amended again.
		if amendment.Status == types.AMENDMENT_STATUS_PENDING {
			if err := k.AmendmentQueue.Set(ctx, collections.Join(amendment.EffectiveAtHeight, amendment.Id)); err != nil {
				return err
			}
		}
	}

	for _, ratification := range genState.Ratifications {
		if err := k.Ratification.Set(ctx, collections.Join(ratification.AmendmentId, ratification.Validator), ratification); err != nil {
			return err
		}
	}

	count := genState.AmendmentCount
	if count == 0 {
		// A genesis written before this module existed, or by hand. The first
		// amendment must still not be amendment zero.
		count = 1
	}
	return k.AmendmentSeq.Set(ctx, count)
}

// ExportGenesis returns the module's exported genesis.
//
// The queue is rebuilt on import rather than exported, because it is derived
// from the amendments entirely. Exporting it as well would make it possible for
// a hand-edited genesis to disagree with itself about when a change takes
// effect, which is the one field of an amendment nobody should be able to move
// quietly.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	invariants, err := k.GetInvariants(ctx)
	if err != nil {
		return nil, err
	}

	genesis := types.DefaultGenesis()
	genesis.Invariants = invariants
	genesis.Amendments = nil
	genesis.Ratifications = nil

	if err := k.Amendment.Walk(ctx, nil, func(_ uint64, amendment types.Amendment) (bool, error) {
		genesis.Amendments = append(genesis.Amendments, amendment)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if err := k.Ratification.Walk(ctx, nil, func(_ collections.Pair[uint64, string], ratification types.Ratification) (bool, error) {
		genesis.Ratifications = append(genesis.Ratifications, ratification)
		return false, nil
	}); err != nil {
		return nil, err
	}

	// Peeked, not consumed: exporting must not change what the next amendment
	// would be numbered.
	count, err := k.AmendmentSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	genesis.AmendmentCount = count

	return genesis, nil
}
