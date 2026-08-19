package keeper

import (
	"context"

	"yamale/blockchain/x/amm/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	for _, elem := range genState.PoolList {
		if err := k.Pool.Set(ctx, elem.Id, elem); err != nil {
			return err
		}
	}

	// Never zero: see DefaultGenesis. A genesis predating the convention is
	// lifted rather than trusted verbatim.
	poolCount := genState.PoolCount
	if poolCount == 0 {
		poolCount = 1
	}
	if err := k.PoolSeq.Set(ctx, poolCount); err != nil {
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
	err = k.Pool.Walk(ctx, nil, func(key uint64, elem types.Pool) (bool, error) {
		genesis.PoolList = append(genesis.PoolList, elem)
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	genesis.PoolCount, err = k.PoolSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}

	return genesis, nil
}
