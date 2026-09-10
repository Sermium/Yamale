package types

import "fmt"

// DefaultGenesis returns an empty registry with working parameters.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params: DefaultParams(),
	}
}

// Validate refuses a genesis the module could not have produced.
//
// The counter checks are the ones that matter. A counter sitting at or below an
// id already present would hand out that id again on the next enrolment, and
// the second record would overwrite the first — silently, because a write to an
// existing key is not an error. That is how a registry loses a herd.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	seenHerdRef := map[string]uint64{}
	for _, h := range gs.Herds {
		if h.Id == 0 {
			return fmt.Errorf("herd 0 is never issued; a zero id is indistinguishable from an unset field")
		}
		if h.Id >= gs.NextHerdId {
			return fmt.Errorf(
				"herd %d exists but next_herd_id is %d, so the next enrolment would overwrite it",
				h.Id, gs.NextHerdId)
		}
		if prev, ok := seenHerdRef[h.Reference]; ok {
			return fmt.Errorf("herds %d and %d share the reference %q", prev, h.Id, h.Reference)
		}
		seenHerdRef[h.Reference] = h.Id
	}

	herds := map[uint64]bool{}
	for _, h := range gs.Herds {
		herds[h.Id] = true
	}

	seenNationalID := map[string]uint64{}
	seenTag := map[string]uint64{}
	animals := map[uint64]Animal{}
	for _, a := range gs.Animals {
		if a.Id == 0 {
			return fmt.Errorf("animal 0 is never issued")
		}
		if a.Id >= gs.NextAnimalId {
			return fmt.Errorf(
				"animal %d exists but next_animal_id is %d, so the next record would overwrite it",
				a.Id, gs.NextAnimalId)
		}
		if !herds[a.HerdId] {
			return fmt.Errorf(
				"animal %d belongs to herd %d, which is not in this genesis; an animal with no "+
					"herd has no keeper to answer for it", a.Id, a.HerdId)
		}
		if prev, ok := seenNationalID[a.NationalId]; ok {
			return fmt.Errorf("animals %d and %d share the national identifier %q",
				prev, a.Id, a.NationalId)
		}
		seenNationalID[a.NationalId] = a.Id
		if a.Tag != "" {
			if prev, ok := seenTag[a.Tag]; ok {
				return fmt.Errorf("animals %d and %d carry the same tag %q", prev, a.Id, a.Tag)
			}
			seenTag[a.Tag] = a.Id
		}
		animals[a.Id] = a
	}

	// A pedigree pointing at an animal that is not here would import a line
	// nobody can walk, and the walk would stop without saying why.
	for _, a := range gs.Animals {
		for _, parent := range []struct {
			id   uint64
			what string
		}{{a.SireId, "sire"}, {a.DamId, "dam"}} {
			if parent.id == 0 {
				continue
			}
			if _, ok := animals[parent.id]; !ok {
				return fmt.Errorf("animal %d names %s %d, which is not in this genesis",
					a.Id, parent.what, parent.id)
			}
			if parent.id == a.Id {
				return fmt.Errorf("animal %d is its own %s", a.Id, parent.what)
			}
		}
	}

	for _, e := range gs.HealthEvents {
		if e.Id >= gs.NextHealthEventId {
			return fmt.Errorf(
				"health event %d exists but next_health_event_id is %d", e.Id, gs.NextHealthEventId)
		}
		if !herds[e.HerdId] {
			return fmt.Errorf("health event %d names herd %d, which is not in this genesis",
				e.Id, e.HerdId)
		}
	}

	for _, t := range gs.Transfers {
		if t.Id >= gs.NextTransferId {
			return fmt.Errorf(
				"transfer %d exists but next_transfer_id is %d", t.Id, gs.NextTransferId)
		}
		if _, ok := animals[t.AnimalId]; !ok {
			return fmt.Errorf("transfer %d names animal %d, which is not in this genesis",
				t.Id, t.AnimalId)
		}
	}

	return nil
}
