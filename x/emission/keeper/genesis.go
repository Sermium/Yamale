package keeper

import (
	"context"
	"errors"

	"yamale/blockchain/x/emission/types"

	"cosmossdk.io/collections"
)

// InitGenesis initializes the module's state from a provided genesis state.
//
// EmissionState must always end up set: BeginBlocker reads it on every block,
// so a chain whose genesis omitted it would halt at height 1. When it is
// absent the schedule is seeded from the params instead, which is exactly the
// state DefaultGenesis describes — the start of the emission curve.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	state := genState.EmissionState
	if state == nil {
		state = &types.EmissionState{
			CurrentProvisionsPerBlock: genState.Params.GenesisProvisionsPerBlock,
			LastReductionPeriod:       0,
		}
	}

	return k.EmissionState.Set(ctx, *state)
}

// ExportGenesis returns the module's exported genesis.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	var err error

	genesis := types.DefaultGenesis()
	genesis.Params, err = k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	emissionState, err := k.EmissionState.Get(ctx)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return nil, err
	}
	genesis.EmissionState = &emissionState

	return genesis, nil
}
