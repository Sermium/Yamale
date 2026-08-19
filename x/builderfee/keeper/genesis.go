package keeper

import (
	"context"

	"yamale/blockchain/x/builderfee/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	for _, elem := range genState.BuilderApplicationMap {
		if err := k.BuilderApplication.Set(ctx, elem.MsgTypeUrl, elem); err != nil {
			return err
		}
	}
	for _, elem := range genState.ApprovedBuilderMap {
		if err := k.ApprovedBuilder.Set(ctx, elem.MsgTypeUrl, elem); err != nil {
			return err
		}
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
	if err := k.BuilderApplication.Walk(ctx, nil, func(_ string, val types.BuilderApplication) (stop bool, err error) {
		genesis.BuilderApplicationMap = append(genesis.BuilderApplicationMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.ApprovedBuilder.Walk(ctx, nil, func(_ string, val types.ApprovedBuilder) (stop bool, err error) {
		genesis.ApprovedBuilderMap = append(genesis.ApprovedBuilderMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return genesis, nil
}
