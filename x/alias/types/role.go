package types

import "fmt"

// ChainWide is the scope no border bounds.
//
// A single character, and deliberately one that cannot collide with anything
// else this field accepts: every other legal value is two uppercase letters
// from the ISO 3166-1 alpha-2 assigned list, so there is no country code an
// operator could mistype into the chain-wide scope and none that could be
// mistaken for it when read.
const ChainWide = "*"

// ValidRole reports whether a role is one this chain has, and is set.
//
// Two conditions, and the second is the one that matters. The zero value is
// reserved as unspecified and refused everywhere a grant is written or read,
// because proto3 cannot tell a zero from a field nobody filled in — so a role
// numbered zero would make "grant this account the first role in the list" and
// "grant this account whatever the default is" the same message. The enum
// therefore starts at one and this function refuses zero explicitly rather than
// relying on the table.
func ValidRole(role Role) bool {
	if role == ROLE_UNSPECIFIED {
		return false
	}
	_, known := Role_name[int32(role)]
	return known
}

// ValidGrantScope reports whether a string may be the *where* of a grant.
//
// Assigned country codes and the chain-wide marker, and nothing else. Note in
// particular that the foundation's reserved code is refused: it marks the
// absence of a national perimeter, so a grant naming it would confer authority
// over nowhere while reading to a human like authority over everywhere. Those
// two must never be spellable as the same thing, because one is an error and
// the other is the highest privilege on the chain.
func ValidGrantScope(scope string) bool {
	return scope == ChainWide || AssignedCountry(scope)
}

// NormaliseScope uppercases a country code and leaves the chain-wide marker
// alone.
//
// The marker is passed through rather than folded so that normalisation cannot
// invent it: no case-folding of any two-letter code produces "*", so an
// operator cannot arrive at chain-wide authority by mistyping a country.
func NormaliseScope(scope string) string {
	if scope == ChainWide {
		return ChainWide
	}
	return NormaliseCountry(scope)
}

// RoleName renders a role for an error message or an event attribute.
//
// The enum's own name, so a refusal names the role the same way the grant that
// would have permitted it does. An unknown value renders as its number rather
// than as an empty string: a message reading "holds no grant of role" with a
// blank where the role belongs is a message that sends an operator looking for
// the wrong bug.
func RoleName(role Role) string {
	if name, known := Role_name[int32(role)]; known {
		return name
	}
	return fmt.Sprintf("ROLE(%d)", int32(role))
}

// Validate checks a grant is one the keeper can act on.
//
// Used by genesis and by the message handler, so a grant seeded at height zero
// is held to the rule a grant made by transaction is held to. Nothing
// re-examines a record written at genesis, so a rule enforced only in the
// handler is a rule a genesis file can walk around.
func (g RoleGrant) Validate() error {
	if g.Holder == "" {
		return fmt.Errorf("a role grant has an empty holder")
	}
	if !ValidRole(g.Role) {
		return fmt.Errorf("the grant to %s names %s, which is not a role that can be held",
			g.Holder, RoleName(g.Role))
	}
	if !ValidGrantScope(g.Jurisdiction) {
		return fmt.Errorf("the grant of %s to %s names %q, which is neither an assigned country code nor %q",
			RoleName(g.Role), g.Holder, g.Jurisdiction, ChainWide)
	}
	return nil
}

// There is deliberately no Covers(country) helper on RoleGrant.
//
// It would read as the natural place to ask "does this grant reach that
// country", and it would be a second implementation of the question the keeper
// answers by two exact store lookups. The day the two disagree — a prefix match
// creeping into one of them, a case-folding difference, a chain-wide marker
// spelled differently — the permissive one wins, because the permissive one is
// the one nothing fails on. The keeper's assertGranted is the only place that
// decides.
