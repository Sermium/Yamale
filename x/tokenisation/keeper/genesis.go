package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/tokenisation/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, c := range gs.Collections {
		if err := k.Collections.Set(ctx, c.Id, c); err != nil {
			return err
		}
	}
	for _, a := range gs.Assets {
		if err := k.Assets.Set(ctx, a.Id, a); err != nil {
			return err
		}
		// The denom index is derived, but it is written here rather than
		// rebuilt lazily: the send restriction reads it on every bank transfer
		// on the chain, and a lookup that is sometimes absent is a rule that is
		// sometimes not enforced.
		if a.FractionDenom != "" {
			if err := k.ByDenom.Set(ctx, a.FractionDenom, a.Id); err != nil {
				return err
			}
			// Derived for the same reason and written the same way: the land
			// registry's ceiling is enforced against this total, and a total
			// that is absent after an import is a ceiling that has quietly
			// reset to zero on land that has already been sold in shares.
			//
			// Nothing is written for a parcel with no issuance, so an export of
			// the reimported state is unchanged — this map is never exported,
			// and a zero written where genesis wrote nothing would be state the
			// export could not account for.
			if a.ParcelId != 0 {
				if err := k.recordIssuedShare(ctx, a.ParcelId, a.HolderShareBps); err != nil {
					return err
				}
			}
		}
	}
	for _, v := range gs.Vaults {
		if err := k.Vaults.Set(ctx, v.AssetId, v); err != nil {
			return err
		}
	}
	for _, p := range gs.Positions {
		if err := k.Positions.Set(ctx, collections.Join(p.AssetId, p.Holder), p); err != nil {
			return err
		}
	}
	for _, s := range gs.SaleReports {
		if err := k.Sales.Set(ctx, s.AssetId, s); err != nil {
			return err
		}
	}
	// Set explicitly, never derived from the asset list. A counter that
	// recomputes itself writes zero where InitGenesis wrote nothing, and the
	// export stops matching the import byte for byte.
	return k.NextAssetID.Set(ctx, gs.NextAssetId)
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	gs := types.DefaultGenesis()

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	gs.Params = params

	if err := k.Collections.Walk(ctx, nil, func(_ string, c types.Collection) (bool, error) {
		gs.Collections = append(gs.Collections, c)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Assets.Walk(ctx, nil, func(_ uint64, a types.Asset) (bool, error) {
		gs.Assets = append(gs.Assets, a)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Vaults.Walk(ctx, nil, func(_ uint64, v types.Vault) (bool, error) {
		gs.Vaults = append(gs.Vaults, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Positions.Walk(ctx, nil, func(_ collections.Pair[uint64, string], p types.Position) (bool, error) {
		gs.Positions = append(gs.Positions, p)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Sales.Walk(ctx, nil, func(_ uint64, s types.SaleReport) (bool, error) {
		gs.SaleReports = append(gs.SaleReports, s)
		return false, nil
	}); err != nil {
		return nil, err
	}

	next, err := k.NextAssetID.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.NextAssetId = next

	return gs, nil
}
