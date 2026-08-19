package types

import "fmt"

// DefaultGenesis returns the default genesis state.
//
// There is no valid default. DefaultInvariants leaves the recovery destination
// empty and Validate refuses it, so this function returns a template that a
// genesis ceremony has to complete and a chain started from it unmodified will
// not start at all. That is the intended behaviour and it is the same shape
// x/enforcement already uses: the alternative is a chain that boots with a
// constitution nobody wrote.
//
// The amendment count starts at one because amendments are numbered from one.
// An id of zero is indistinguishable from an unset field in every client that
// reads one, and "ratified amendment 0" would be a record nobody could look up.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Invariants:     DefaultInvariants(),
		AmendmentCount: 1,
	}
}

// Validate performs basic genesis state validation.
func (gs GenesisState) Validate() error {
	if err := gs.Invariants.Validate(); err != nil {
		return err
	}

	amendments := make(map[uint64]Amendment, len(gs.Amendments))
	for _, a := range gs.Amendments {
		if a.Id == 0 {
			return fmt.Errorf("amendment with id 0; amendments are numbered from 1")
		}
		if _, seen := amendments[a.Id]; seen {
			return fmt.Errorf("duplicate amendment %d", a.Id)
		}
		if a.Id >= gs.AmendmentCount {
			return fmt.Errorf("amendment %d is at or beyond amendment_count %d, so the next amendment opened would collide with it", a.Id, gs.AmendmentCount)
		}
		if a.Status == AMENDMENT_STATUS_UNSPECIFIED {
			return fmt.Errorf("amendment %d has an unspecified status", a.Id)
		}
		// A pending amendment is the one thing in this genesis that will still
		// act. An import that carried one with no effective height would enact
		// it in the first block after the import, which is the outcome the
		// public delay exists to make impossible.
		if a.Status == AMENDMENT_STATUS_PENDING {
			if a.EffectiveAtHeight <= 0 {
				return fmt.Errorf("amendment %d is pending but has no effective height, so it would take effect in the first block after the import", a.Id)
			}
			if err := a.Proposed.Validate(); err != nil {
				return fmt.Errorf("amendment %d is pending and would enact an invalid settlement: %w", a.Id, err)
			}
		}
		amendments[a.Id] = a
	}

	seen := make(map[string]bool, len(gs.Ratifications))
	tallied := make(map[uint64]int64, len(gs.Amendments))
	for _, r := range gs.Ratifications {
		if _, ok := amendments[r.AmendmentId]; !ok {
			return fmt.Errorf("ratification by %s of amendment %d, which does not exist", r.Validator, r.AmendmentId)
		}
		key := fmt.Sprintf("%d/%s", r.AmendmentId, r.Validator)
		if seen[key] {
			return fmt.Errorf("validator %s ratified amendment %d twice", r.Validator, r.AmendmentId)
		}
		seen[key] = true
		tallied[r.AmendmentId] += r.Power
	}

	// The running total and the ratifications behind it must agree. They are
	// stored separately — the total so that a tally cannot be changed by
	// changing the set, the individual records so that a validator is
	// answerable for what it agreed to — and a genesis in which they disagree
	// is one where the chain would enact a change on a tally nobody signed.
	for _, a := range gs.Amendments {
		if a.RatifiedPower != tallied[a.Id] {
			return fmt.Errorf(
				"amendment %d records %d ratified power but the ratifications in this genesis add up to %d",
				a.Id, a.RatifiedPower, tallied[a.Id],
			)
		}
	}

	return nil
}
