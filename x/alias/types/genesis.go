package types

import "fmt"

// DefaultGenesis returns the default genesis state: parameters and nothing
// else. Identifiers are issued by transaction, never seeded.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:        DefaultParams(),
		Aliases:       []Alias{},
		Retired:       []string{},
		Jurisdictions: []Jurisdiction{},
	}
}

// Validate checks the genesis state, and checks the invariants that the keeper
// depends on rather than only the shapes.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	seenID := make(map[string]struct{}, len(gs.Aliases))
	seenAddr := make(map[string]struct{}, len(gs.Aliases))
	retired := make(map[string]struct{}, len(gs.Retired))
	country := make(map[string]string, len(gs.Jurisdictions))

	for _, id := range gs.Retired {
		// Either shape is accepted here and nowhere else. The prefixless ones
		// are the tombstones the v1-to-v2 migration left behind, and refusing
		// them would push an operator to delete them to make the file load —
		// which would free a handle somebody memorised to be issued to a
		// stranger.
		if !Valid(id) && !ValidLegacy(id) {
			return fmt.Errorf("retired identifier %q is malformed", id)
		}
		if _, dup := retired[id]; dup {
			return fmt.Errorf("retired identifier %q appears twice", id)
		}
		retired[id] = struct{}{}
	}

	for _, j := range gs.Jurisdictions {
		if j.Address == "" {
			return fmt.Errorf("a jurisdiction record has an empty address")
		}
		if !AssignedCountry(j.Country) {
			// FoundationCountry fails this deliberately. It marks the absence
			// of a perimeter; recorded as one it would be a perimeter no
			// authority holds, handed out to an ordinary account.
			return fmt.Errorf("account %s is recorded in %q, which is not an assigned country code",
				j.Address, j.Country)
		}
		if _, dup := country[j.Address]; dup {
			return fmt.Errorf("account %s is recorded in two jurisdictions", j.Address)
		}
		country[j.Address] = j.Country
	}

	for _, a := range gs.Aliases {
		if !Valid(a.Id) {
			return fmt.Errorf("identifier %q is malformed", a.Id)
		}
		if a.Address == "" {
			return fmt.Errorf("identifier %q resolves to an empty address", a.Id)
		}
		// One identifier, one address. Both directions are unique, and the
		// keeper's reverse index would silently lose a record if they were not.
		if _, dup := seenID[a.Id]; dup {
			return fmt.Errorf("identifier %q is bound twice", a.Id)
		}
		if _, dup := seenAddr[a.Address]; dup {
			return fmt.Errorf("address %s holds more than one identifier", a.Address)
		}
		// A live identifier that is also tombstoned would resolve and be refused
		// depending on which check ran first.
		if _, dead := retired[a.Id]; dead {
			return fmt.Errorf("identifier %q is both live and retired", a.Id)
		}

		// The prefix must be true at height zero as well. Nothing later
		// re-examines an identifier seeded by genesis, so a file that bound
		// NG-… to an account recorded in GH would put a lying marker on the
		// chain that no check would ever catch.
		switch recorded, ok := country[a.Address]; {
		case ok:
			if Country(a.Id) != recorded {
				return fmt.Errorf("identifier %q claims %s but account %s is recorded in %s",
					a.Id, Country(a.Id), a.Address, recorded)
			}
		case Country(a.Id) == FoundationCountry:
			// The one exemption, and it is checked rather than assumed: only a
			// named foundation administrator may hold an identifier with no
			// country behind it.
			if !gs.Params.IsFoundationAdministrator(a.Address) {
				return fmt.Errorf("identifier %q carries the foundation prefix but %s is not a foundation administrator",
					a.Id, a.Address)
			}
		default:
			return fmt.Errorf("identifier %q is bound to %s, which has no recorded jurisdiction",
				a.Id, a.Address)
		}

		seenID[a.Id] = struct{}{}
		seenAddr[a.Address] = struct{}{}
	}
	return nil
}
