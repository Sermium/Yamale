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
		RoleGrants:    []RoleGrant{},
	}
}

// Validate checks the genesis state, and checks the invariants that the keeper
// depends on rather than only the shapes.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	// The grants are checked FIRST, and then the set of administrators is read
	// off them, because the ZZ exemption below is decided by that set. Validating
	// the aliases first would let a file whose grants are malformed be refused
	// for the identifier that depended on one of them, which sends a reader to
	// the wrong half of their own file.
	if err := gs.validateRoles(); err != nil {
		return err
	}
	administrators := gs.foundationAdministrators()

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
			// holder of ROLE_FOUNDATION_ADMINISTRATOR may hold an identifier
			// with no country behind it.
			if _, ok := administrators[a.Address]; !ok {
				return fmt.Errorf("identifier %q carries the foundation prefix but %s holds no chain-wide grant of %s",
					a.Id, a.Address, RoleName(ROLE_FOUNDATION_ADMINISTRATOR))
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

// validateRoles checks the grant registry.
//
// The rules are the handler's rules, held against a file: a role that is set
// and real, a jurisdiction that is either an assigned country or the chain-wide
// marker, and one record per (holder, role, jurisdiction). Nothing re-examines a
// grant seeded at height zero, so a rule enforced only in the handler is a rule
// a genesis file walks around — and the grant registry is the one place where
// walking around a rule hands somebody an authority nobody voted for.
//
// What it deliberately does not check: whether the holder is a group account.
// That question needs x/group's state, which genesis validation does not have,
// and inventing an answer here would be worse than leaving it to the handler and
// to whoever reads the file.
func (gs GenesisState) validateRoles() error {
	seen := make(map[string]struct{}, len(gs.RoleGrants))
	administrators := 0
	for _, g := range gs.RoleGrants {
		if err := g.Validate(); err != nil {
			return err
		}
		// RoleGrant.Validate has already refused a country scope for this role,
		// so counting the grants of it is counting the chain-wide ones.
		if ChainWideOnly(g.Role) {
			administrators++
		}
		// One record per triple. A duplicate is not harmless: the derived index
		// would carry one entry for two records, so an export would emit two and
		// the reverse view would list one, and the file would stop round-tripping
		// the moment anybody revoked either of them.
		key := g.Holder + "\x00" + RoleName(g.Role) + "\x00" + g.Jurisdiction
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%s is granted %s in %s twice",
				g.Holder, RoleName(g.Role), g.Jurisdiction)
		}
		seen[key] = struct{}{}
	}
	// The cap, held against a file for the same reason GrantRole holds it
	// against a message: nothing re-examines a grant seeded at height zero, so a
	// rule enforced only in the handler is a rule a genesis file walks around —
	// and this one bounds the single exception to every account having a
	// jurisdiction. Counted after the duplicate check above, so a triple written
	// twice cannot spend two of the eight places.
	if administrators > MaxFoundationAdministrators {
		return fmt.Errorf("at most %d accounts may hold %s, and the file grants it to %d",
			MaxFoundationAdministrators, RoleName(ROLE_FOUNDATION_ADMINISTRATOR), administrators)
	}
	return nil
}

// foundationAdministrators is the set of accounts the file exempts from the rule
// that every account carries a country.
//
// A set rather than a count, because the alias check above asks about one
// address at a time and a linear scan per identifier would make validating a
// file quadratic in the two things a large deployment has most of.
//
// It is called after validateRoles rather than instead of it, so every grant it
// reads has already been held to the handler's rules — including the one that
// makes this function's exact match on ChainWide complete rather than merely
// narrow, which is that a grant of this role naming a country is refused
// outright.
func (gs GenesisState) foundationAdministrators() map[string]struct{} {
	set := make(map[string]struct{})
	for _, g := range gs.RoleGrants {
		if g.Role == ROLE_FOUNDATION_ADMINISTRATOR && g.Jurisdiction == ChainWide {
			set[g.Holder] = struct{}{}
		}
	}
	return set
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
