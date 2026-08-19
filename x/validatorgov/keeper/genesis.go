package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/validatorgov/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
//
// Rotations are restored exactly as exported, and the two indexes over them are
// rebuilt rather than carried. That asymmetry is deliberate: the rotation
// records are the state, and the pending index and the completion queue are
// derived from them entirely. Exporting the indexes as well would make it
// possible for a hand-edited genesis to disagree with itself, and rebuilding
// state that genesis had written explicitly is how an import stops matching
// what was exported.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	// Validated here and not only in `genesis validate`, because the two are
	// not the same gate: one is a command an operator may run against the file
	// they meant to distribute, this runs in InitChain on the bytes the chain
	// is really starting from. A genesis whose declarations cannot be grouped
	// is one where the concentration ceilings are computed over nothing.
	if err := genState.Validate(); err != nil {
		return fmt.Errorf("validatorgov genesis is invalid, refusing to start: %w", err)
	}

	for _, elem := range genState.ValidatorApplicationMap {
		if err := k.ValidatorApplication.Set(ctx, elem.Candidate, elem); err != nil {
			return err
		}
	}
	for _, elem := range genState.ApprovedValidatorMap {
		if err := k.ApprovedValidator.Set(ctx, elem.Candidate, elem); err != nil {
			return err
		}
	}

	for _, rotation := range genState.OperatorRotations {
		if err := k.Rotation.Set(ctx, rotation.Id, rotation); err != nil {
			return err
		}
		if rotation.Status != types.ROTATION_STATUS_PENDING {
			continue
		}
		if err := k.PendingRotation.Set(ctx, rotation.CurrentOperator, rotation.Id); err != nil {
			return err
		}
		// A recovery still waiting on the admission quorum has no completion
		// height and belongs in no queue. Queueing it at height zero would
		// complete it in the first block after the import, which is the one
		// outcome the challenge window exists to make impossible.
		if rotation.CompletesAtHeight > 0 {
			if err := k.RotationQueue.Set(ctx, collections.Join(rotation.CompletesAtHeight, rotation.Id)); err != nil {
				return err
			}
		}
	}

	// A demotion is restored on its own record, so it has to survive an export.
	// Nothing else in this genesis implies one: the validator it names is
	// jailed, contributes no power, and recomputing the ceilings from the
	// exported state would find no breach and conclude there was never
	// anything to hold down.
	for _, demotion := range genState.Demotions {
		if err := k.Demotion.Set(ctx, demotion.Operator, demotion); err != nil {
			return err
		}
	}

	count := genState.RotationCount
	if count == 0 {
		// A genesis written before this module had rotations, or by hand. The
		// first rotation must still not be rotation zero.
		count = 1
	}
	if err := k.RotationSeq.Set(ctx, count); err != nil {
		return err
	}

	return k.Params.Set(ctx, genState.Params)
}

// ExportGenesis returns the module's exported genesis.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	var err error

	genesis := types.DefaultGenesis()
	genesis.Params, err = k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := k.ValidatorApplication.Walk(ctx, nil, func(_ string, val types.ValidatorApplication) (stop bool, err error) {
		genesis.ValidatorApplicationMap = append(genesis.ValidatorApplicationMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.ApprovedValidator.Walk(ctx, nil, func(_ string, val types.ApprovedValidator) (stop bool, err error) {
		genesis.ApprovedValidatorMap = append(genesis.ApprovedValidatorMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Rotation.Walk(ctx, nil, func(_ uint64, val types.OperatorRotation) (stop bool, err error) {
		genesis.OperatorRotations = append(genesis.OperatorRotations, val)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Demotion.Walk(ctx, nil, func(_ string, val types.Demotion) (stop bool, err error) {
		genesis.Demotions = append(genesis.Demotions, val)
		return false, nil
	}); err != nil {
		return nil, err
	}

	// Peeked, not consumed: exporting must not change what the next rotation
	// would be numbered.
	genesis.RotationCount, err = k.RotationSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}

	return genesis, nil
}
