package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/alias/types"
)

// InitGenesis writes the genesis state, rebuilding both derived indexes from
// the records they index rather than reading them from the file.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, id := range gs.Retired {
		if err := k.Retired.Set(ctx, id); err != nil {
			return err
		}
	}
	for _, j := range gs.Jurisdictions {
		if err := k.Jurisdictions.Set(ctx, j.Address, j); err != nil {
			return err
		}
		// Derived here, and deliberately not exported.
		if err := k.Perimeter.Set(ctx, collections.Join(j.Country, j.Address)); err != nil {
			return err
		}
	}
	for _, a := range gs.Aliases {
		if err := k.Aliases.Set(ctx, a.Id, a); err != nil {
			return err
		}
		// Derived here, and deliberately not exported. Two copies of the same
		// fact in a genesis file can disagree.
		if err := k.Owners.Set(ctx, a.Address, a.Id); err != nil {
			return err
		}
	}
	return nil
}

// ExportGenesis reads the genesis state back out.
//
// Emits Aliases, Retired and Jurisdictions, and neither derived index, so that
// exporting and importing produces byte-identical state — the property upgrades
// and state migrations depend on.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	gs := types.GenesisState{
		Params:        params,
		Aliases:       []types.Alias{},
		Retired:       []string{},
		Jurisdictions: []types.Jurisdiction{},
	}

	if err := k.Aliases.Walk(ctx, nil, func(_ string, a types.Alias) (bool, error) {
		gs.Aliases = append(gs.Aliases, a)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Retired.Walk(ctx, nil, func(id string) (bool, error) {
		gs.Retired = append(gs.Retired, id)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Jurisdictions.Walk(ctx, nil, func(_ string, j types.Jurisdiction) (bool, error) {
		gs.Jurisdictions = append(gs.Jurisdictions, j)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &gs, nil
}
