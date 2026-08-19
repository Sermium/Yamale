package keeper

import (
	"context"

	"yamale/blockchain/x/land/types"
)

// InitGenesis restores the registry.
//
// The two indexes are rebuilt here rather than exported, because they are
// derivable from the parcels and a derived value that is also stored is a
// derived value that can disagree with its source. What is *not* derived is the
// pair of counters: those come from genesis, so an imported registry keeps its
// numbering and a parcel id means the same thing before and after an upgrade.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	for _, office := range genState.Authorities {
		if err := k.Authority.Set(ctx, office.Address, office); err != nil {
			return err
		}
	}

	for _, parcel := range genState.Parcels {
		if err := k.Parcel.Set(ctx, parcel.Id, parcel); err != nil {
			return err
		}
		if err := k.ByGeometry.Set(ctx, parcel.GeometryHash, parcel.Id); err != nil {
			return err
		}
		if err := k.ByRef.Set(ctx, parcel.CadastralRef, parcel.Id); err != nil {
			return err
		}
	}

	for _, transfer := range genState.Transfers {
		if err := k.Transfer.Set(ctx, transfer.Id, transfer); err != nil {
			return err
		}
	}

	// Written exactly as exported, including the withdrawn and the expired.
	// Dropping a lapsed permission would be a derivation — "live" depends on
	// the block time at import, so a filter here would make the imported state
	// differ from the exported state on nothing but when the import happened.
	for _, auth := range genState.FractionalisationAuthorities {
		if err := k.FractionalisationAuthority.Set(ctx, auth.ParcelId, auth); err != nil {
			return err
		}
	}

	if err := k.NextParcelID.Set(ctx, genState.NextParcelId); err != nil {
		return err
	}
	if err := k.NextTransferID.Set(ctx, genState.NextTransferId); err != nil {
		return err
	}

	return k.Params.Set(ctx, genState.Params)
}

// ExportGenesis writes the registry out.
//
// Everything that cannot be recomputed goes here, and nothing that can. The
// counters are written even when no record uses them yet: a counter that
// exports as zero because nothing has been registered, and then re-derives as
// zero on import, is fine — but a counter derived from the highest id present
// would silently rewind after a parcel was removed, and hand a future parcel an
// id that has already meant something else.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	genesis := types.DefaultGenesis()

	var err error
	genesis.Params, err = k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	if err := k.Authority.Walk(ctx, nil, func(_ string, v types.Authority) (bool, error) {
		genesis.Authorities = append(genesis.Authorities, v)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if err := k.Parcel.Walk(ctx, nil, func(_ uint64, v types.Parcel) (bool, error) {
		genesis.Parcels = append(genesis.Parcels, v)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if err := k.Transfer.Walk(ctx, nil, func(_ uint64, v types.Transfer) (bool, error) {
		genesis.Transfers = append(genesis.Transfers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if err := k.FractionalisationAuthority.Walk(ctx, nil,
		func(_ uint64, v types.FractionalisationAuthority) (bool, error) {
			genesis.FractionalisationAuthorities = append(
				genesis.FractionalisationAuthorities, v)
			return false, nil
		}); err != nil {
		return nil, err
	}

	if genesis.NextParcelId, err = k.NextParcelID.Peek(ctx); err != nil {
		return nil, err
	}
	if genesis.NextTransferId, err = k.NextTransferID.Peek(ctx); err != nil {
		return nil, err
	}

	return genesis, nil
}
