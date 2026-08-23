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
	if err := g.RequiredShape.Validate(); err != nil {
		return fmt.Errorf("the grant of %s to %s: %w", RoleName(g.Role), g.Holder, err)
	}
	return nil
}

// MaxOfficeMembers is the largest office whose shape this module will check.
//
// It exists because x/group's member query pages: asked with no page request it
// returns at most a hundred members, so a larger group would be silently
// undercounted and every action by it refused for the wrong reason. Asking for
// one more than this cap turns a group past it into an explicit refusal that
// names the cap, which is a state an operator can act on — the same arrangement
// x/constitution's ante gate uses for the foundation's own group, and the same
// reasoning: refuse rather than count a page of it.
//
// Fifty rather than five. The offices this is built for are two to five people,
// but a commission with thirty members is a real thing and a cap that refused it
// would be this module deciding how big a country's authority may be.
const MaxOfficeMembers = 50

// Validate checks a recorded shape requirement is one the keeper can hold an
// office to.
//
// Nil is valid and means no requirement. That is the state of every grant made
// before required_shape existed, and it is a nil POINTER rather than a zero
// value on purpose: proto3 gives a message field real presence, so this method
// can tell "nobody asked for a shape" from "somebody asked for a shape of zero"
// — and it refuses the second, because a requirement that requires nothing reads
// on a record as though it covered something.
//
// A method on the pointer, and safe to call on nil, so that every caller checks
// the same way and none of them has to remember to guard first. A rule whose
// callers each decide whether to apply it is a rule with as many exceptions as
// callers.
func (s *OfficeShape) Validate() error {
	if s == nil {
		return nil
	}
	if s.Signatures == 0 {
		return fmt.Errorf(
			"a required shape of %s asks for no signatures; omit required_shape entirely to record no requirement",
			s.Rule())
	}
	if s.Members < s.Signatures {
		return fmt.Errorf(
			"a required shape of %s asks for more signatures than members, which no office could ever satisfy",
			s.Rule())
	}
	if s.Members > MaxOfficeMembers {
		return fmt.Errorf(
			"a required shape of %s asks for more than the %d members this module can read a group's shape from",
			s.Rule(), MaxOfficeMembers)
	}
	return nil
}

// Rule renders a shape the way the people who agreed it say it out loud.
//
// "3-of-5", because that is what is written on a ceremony record and said in the
// room, and an error message that used the field names instead would be an error
// message a custodian has to translate. Nil renders as words rather than as
// "0-of-0": a grant with no requirement is a different thing from one requiring
// nothing, and the whole point of the pointer is that those two never read the
// same.
func (s *OfficeShape) Rule() string {
	if s == nil {
		return "no required shape"
	}
	return fmt.Sprintf("%d-of-%d", s.Signatures, s.Members)
}

// Satisfies reports whether an office that takes signatures members to act, and
// has members members, still meets this requirement.
//
// Floors on both numbers, so a bigger office passes and a smaller one does not.
// See the proto comment on OfficeShape for why growth is allowed and shrinkage
// is not, and why this does not object to an office tightening its own threshold
// beyond what was asked.
//
// Nil satisfies everything, which is the same statement as "no requirement was
// recorded". It is written here rather than left to each caller for the reason
// Validate is: the permissive branch of a rule belongs in one place, where it can
// be read and tested, not in every caller's nil check.
//
// It is also asked requirement-against-requirement, by the keeper's ratchet on a
// re-grant: "does this new requirement meet the floor the old one set". The
// arithmetic is the same >= on both numbers, and reusing it means the ratchet and
// the perimeter cannot come to disagree about which way the comparison runs.
func (s *OfficeShape) Satisfies(signatures, members uint32) bool {
	if s == nil {
		return true
	}
	return signatures >= s.Signatures && members >= s.Members
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
