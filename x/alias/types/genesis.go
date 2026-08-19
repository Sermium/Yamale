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
		ViewingKeys:   []ViewingKey{},
		Regulators:    []RegulatorAppointment{},
		AuditorGrants: []AuditorGrant{},
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

	return gs.validateViewing()
}

// validateViewing checks the confidentiality registries.
//
// Separated only for length; every rule here is the same class as the ones
// above — a shape the keeper depends on and no later check re-examines, because
// nothing re-examines a record seeded at height zero.
func (gs GenesisState) validateViewing() error {
	// Versions per account, so a file cannot bind two keys to one version. An
	// envelope naming a duplicated version resolves to whichever record loaded
	// last, and the party holding the other half sees an authentication failure
	// that looks exactly like a corrupted payload.
	seenVersion := make(map[string]map[uint64]struct{}, len(gs.ViewingKeys))
	for _, v := range gs.ViewingKeys {
		if v.Address == "" {
			return fmt.Errorf("a viewing key has an empty address")
		}
		// Version zero is refused because the keeper issues from one, so a zero
		// is either an unset field or a record from something that is not this
		// module. Both read as a key nobody can name in an envelope.
		if v.Version == 0 {
			return fmt.Errorf("viewing key for %s has version 0; versions start at 1", v.Address)
		}
		if err := ValidateViewingKey(v.PublicKey); err != nil {
			return fmt.Errorf("viewing key %s/%d: %w", v.Address, v.Version, err)
		}
		// The two revocation fields are only meaningful together, so a file
		// that sets one without the other is refused rather than interpreted. A
		// height with no flag reads as live to Live() and as revoked to a human
		// looking at the export, which is the worst of both.
		if !v.Revoked && v.RevokedAtHeight != 0 {
			return fmt.Errorf("viewing key %s/%d carries a revocation height but is not marked revoked",
				v.Address, v.Version)
		}
		if v.Revoked && v.RevokedAtHeight < v.RegisteredAtHeight {
			return fmt.Errorf("viewing key %s/%d is revoked at height %d, before it was registered at %d",
				v.Address, v.Version, v.RevokedAtHeight, v.RegisteredAtHeight)
		}
		versions, ok := seenVersion[v.Address]
		if !ok {
			versions = map[uint64]struct{}{}
			seenVersion[v.Address] = versions
		}
		if _, dup := versions[v.Version]; dup {
			return fmt.Errorf("account %s has two viewing keys at version %d", v.Address, v.Version)
		}
		versions[v.Version] = struct{}{}
	}

	seenCountry := make(map[string]struct{}, len(gs.Regulators))
	for _, r := range gs.Regulators {
		if !AssignedCountry(r.Country) {
			return fmt.Errorf("a regulator is appointed for %q, which is not an assigned country code", r.Country)
		}
		if r.Address == "" {
			return fmt.Errorf("the regulator of %s has an empty address", r.Country)
		}
		// One per country. Two appointed regulators would reintroduce exactly
		// the contest over standing that the single settlement declaration
		// exists to end — and at genesis, silently.
		if _, dup := seenCountry[r.Country]; dup {
			return fmt.Errorf("%s has two appointed regulators", r.Country)
		}
		seenCountry[r.Country] = struct{}{}
	}

	seenAuditor := make(map[string]struct{}, len(gs.AuditorGrants))
	for _, g := range gs.AuditorGrants {
		if g.Address == "" {
			return fmt.Errorf("an auditor grant has an empty address")
		}
		// Zero would read as "no expiry" to anyone who did not check, which is
		// the one thing a time-boxed role must not be able to become by
		// omission. Grants that have already expired are accepted, because they
		// are the record of who could read what and pruning them is what makes
		// that question unanswerable.
		if g.ExpiresAtHeight <= 0 {
			return fmt.Errorf("auditor grant for %s has no expiry height", g.Address)
		}
		if _, dup := seenAuditor[g.Address]; dup {
			return fmt.Errorf("account %s holds two auditor grants", g.Address)
		}
		seenAuditor[g.Address] = struct{}{}
	}
	return nil
}
