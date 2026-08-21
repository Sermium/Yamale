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
	for _, v := range gs.ViewingKeys {
		if err := k.ViewingKeys.Set(ctx, collections.Join(v.Address, v.Version), v); err != nil {
			return err
		}
	}
	for _, r := range gs.Regulators {
		if err := k.Regulators.Set(ctx, r.Country, r); err != nil {
			return err
		}
	}
	for _, g := range gs.AuditorGrants {
		if err := k.AuditorGrants.Set(ctx, g.Address, g); err != nil {
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
		ViewingKeys:   []types.ViewingKey{},
		Regulators:    []types.RegulatorAppointment{},
		AuditorGrants: []types.AuditorGrant{},
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
	// Every version, live and revoked alike. An export that dropped the
	// superseded ones would destroy the ability to read every payload sealed
	// before the last rotation, and it would do it at an upgrade — where nobody
	// is looking at payment detail. Revoked keys are kept for the same reason:
	// revocation says the private half may be in other hands, not that the
	// envelopes sealed to it stopped existing.
	if err := k.ViewingKeys.Walk(ctx, nil, func(_ collections.Pair[string, uint64], v types.ViewingKey) (bool, error) {
		gs.ViewingKeys = append(gs.ViewingKeys, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Regulators.Walk(ctx, nil, func(_ string, r types.RegulatorAppointment) (bool, error) {
		gs.Regulators = append(gs.Regulators, r)
		return false, nil
	}); err != nil {
		return nil, err
	}
	// Expired grants included. "Who could read this payment" is asked long
	// after the grant lapsed, and an export that pruned them answers "nobody".
	if err := k.AuditorGrants.Walk(ctx, nil, func(_ string, g types.AuditorGrant) (bool, error) {
		gs.AuditorGrants = append(gs.AuditorGrants, g)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &gs, nil
}
