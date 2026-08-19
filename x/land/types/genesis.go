package types

import "fmt"

// DefaultGenesis is an empty registry with the protective defaults in place.
// Empty is the right starting point: seeding who owns what is a political act
// performed by the state that adopts this, recorded as first registrations so
// the initial allocation is as auditable as everything after it.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params: DefaultParams(),
	}
}

// Validate refuses a genesis that would import a registry with the guarantees
// already broken — two titles over the same ground, or a counter that would
// hand out an id already in use.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	geometries := make(map[string]uint64, len(gs.Parcels))
	refs := make(map[string]uint64, len(gs.Parcels))
	ids := make(map[uint64]bool, len(gs.Parcels))

	for _, p := range gs.Parcels {
		if prev, seen := geometries[p.GeometryHash]; seen {
			return fmt.Errorf("parcels %d and %d share a survey hash", prev, p.Id)
		}
		geometries[p.GeometryHash] = p.Id

		if prev, seen := refs[p.CadastralRef]; seen {
			return fmt.Errorf("parcels %d and %d share cadastral reference %q",
				prev, p.Id, p.CadastralRef)
		}
		refs[p.CadastralRef] = p.Id

		if ids[p.Id] {
			return fmt.Errorf("duplicate parcel id %d", p.Id)
		}
		ids[p.Id] = true

		// A counter behind an existing id would re-issue that id to a different
		// parcel, which is the double-registration this module exists to stop —
		// arriving through the import rather than through a message.
		if p.Id >= gs.NextParcelId {
			return fmt.Errorf("next parcel id %d would re-issue existing id %d",
				gs.NextParcelId, p.Id)
		}
	}

	for _, t := range gs.Transfers {
		if t.Id >= gs.NextTransferId {
			return fmt.Errorf("next transfer id %d would re-issue existing id %d",
				gs.NextTransferId, t.Id)
		}
		if !ids[t.ParcelId] {
			return fmt.Errorf("transfer %d refers to unknown parcel %d", t.Id, t.ParcelId)
		}
	}

	// One authorisation per parcel, over a parcel that exists. A second entry
	// for the same parcel would silently lose to whichever was written last,
	// and an import cannot be allowed to choose a ceiling by list order. An
	// authorisation over no parcel is a permission x/tokenisation would honour
	// for land the registry has never heard of.
	authorised := make(map[uint64]bool, len(gs.FractionalisationAuthorities))
	for _, a := range gs.FractionalisationAuthorities {
		if !ids[a.ParcelId] {
			return fmt.Errorf("fractionalisation authority refers to unknown parcel %d",
				a.ParcelId)
		}
		if authorised[a.ParcelId] {
			return fmt.Errorf("parcel %d has more than one fractionalisation authority",
				a.ParcelId)
		}
		authorised[a.ParcelId] = true

		if a.MaxShareBps == 0 || a.MaxShareBps > 10_000 {
			return fmt.Errorf("parcel %d has a share ceiling of %d basis points",
				a.ParcelId, a.MaxShareBps)
		}
	}

	return nil
}
