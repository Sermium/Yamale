package keeper

import (
	"context"

	"yamale/blockchain/x/jewel/types"
)

// InitGenesis writes the register in.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	return k.Params.Set(ctx, gs.Params)
}

// ExportGenesis writes the register out.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.GenesisState{Params: p}, nil
}
