package keeper

import (
	"context"

	"yamale/blockchain/x/stablecoin/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	for _, elem := range genState.IssuerApplicationMap {
		if err := k.IssuerApplication.Set(ctx, elem.Denom, elem); err != nil {
			return err
		}
	}
	for _, elem := range genState.ApprovedIssuerMap {
		if err := k.ApprovedIssuer.Set(ctx, elem.Denom, elem); err != nil {
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
	if err := k.IssuerApplication.Walk(ctx, nil, func(_ string, val types.IssuerApplication) (stop bool, err error) {
		genesis.IssuerApplicationMap = append(genesis.IssuerApplicationMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.ApprovedIssuer.Walk(ctx, nil, func(_ string, val types.ApprovedIssuer) (stop bool, err error) {
		genesis.ApprovedIssuerMap = append(genesis.ApprovedIssuerMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return genesis, nil
}
