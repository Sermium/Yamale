package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/herd/types"
)

// InitGenesis writes a registry in, rebuilding every index from the records.
//
// The indexes are derived rather than exported. They are pure functions of the
// records, so exporting them would be exporting the same fact twice and
// inviting the two copies to disagree — and a disagreement between a record and
// its index is the kind of defect that surfaces as a missing animal years
// later.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}

	for _, a := range gs.Authorities {
		if err := k.Authority.Set(ctx, a.Address, a); err != nil {
			return err
		}
	}
	for _, a := range gs.Agents {
		if err := k.Agent.Set(ctx, a.Address, a); err != nil {
			return err
		}
	}
	for _, h := range gs.Herds {
		if err := k.Herd.Set(ctx, h.Id, h); err != nil {
			return err
		}
		if err := k.ByHerdRef.Set(ctx, h.Reference, h.Id); err != nil {
			return err
		}
	}
	for _, a := range gs.Animals {
		if err := k.Animal.Set(ctx, a.Id, a); err != nil {
			return err
		}
		if err := k.ByNationalID.Set(ctx, a.NationalId, a.Id); err != nil {
			return err
		}
		// A retired animal releases its tag, so the index is rebuilt from the
		// live ones only. Indexing a retired animal's tag would refuse the
		// legitimate refitting of a recovered transponder.
		if a.Tag != "" && a.Status != types.STATUS_RETIRED {
			if err := k.ByTag.Set(ctx, a.Tag, a.Id); err != nil {
				return err
			}
		}
		if err := k.AnimalsByHerd.Set(ctx, collections.Join(a.HerdId, a.Id)); err != nil {
			return err
		}
	}
	for _, e := range gs.HealthEvents {
		if err := k.HealthEvent.Set(ctx, e.Id, e); err != nil {
			return err
		}
		if err := k.HealthByHerd.Set(ctx, collections.Join(e.HerdId, e.Id)); err != nil {
			return err
		}
	}
	for _, t := range gs.Transfers {
		if err := k.Transfer.Set(ctx, t.Id, t); err != nil {
			return err
		}
		if err := k.TransfersByAnimal.Set(ctx, collections.Join(t.AnimalId, t.Id)); err != nil {
			return err
		}
	}

	for _, s := range []struct {
		seq collections.Sequence
		to  uint64
	}{
		{k.NextHerdID, gs.NextHerdId},
		{k.NextAnimalID, gs.NextAnimalId},
		{k.NextHealthEventID, gs.NextHealthEventId},
		{k.NextTransferID, gs.NextTransferId},
	} {
		if err := s.seq.Set(ctx, s.to); err != nil {
			return err
		}
	}
	return nil
}

// ExportGenesis reads the registry out.
//
// The counters are exported explicitly. A counter derived from the highest id
// present changes behaviour after an export whenever the highest-numbered
// record has been retired, and an imported registry that renumbers is a
// different registry.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	gs := types.GenesisState{Params: params}

	if err := k.Authority.Walk(ctx, nil, func(_ string, a types.Authority) (bool, error) {
		gs.Authorities = append(gs.Authorities, a)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Agent.Walk(ctx, nil, func(_ string, a types.Agent) (bool, error) {
		gs.Agents = append(gs.Agents, a)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Herd.Walk(ctx, nil, func(_ uint64, h types.Herd) (bool, error) {
		gs.Herds = append(gs.Herds, h)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Animal.Walk(ctx, nil, func(_ uint64, a types.Animal) (bool, error) {
		gs.Animals = append(gs.Animals, a)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.HealthEvent.Walk(ctx, nil, func(_ uint64, e types.HealthEvent) (bool, error) {
		gs.HealthEvents = append(gs.HealthEvents, e)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Transfer.Walk(ctx, nil, func(_ uint64, t types.Transfer) (bool, error) {
		gs.Transfers = append(gs.Transfers, t)
		return false, nil
	}); err != nil {
		return nil, err
	}

	for _, s := range []struct {
		seq collections.Sequence
		to  *uint64
	}{
		{k.NextHerdID, &gs.NextHerdId},
		{k.NextAnimalID, &gs.NextAnimalId},
		{k.NextHealthEventID, &gs.NextHealthEventId},
		{k.NextTransferID, &gs.NextTransferId},
	} {
		v, err := s.seq.Peek(ctx)
		if err != nil {
			return nil, err
		}
		*s.to = v
	}
	return &gs, nil
}
