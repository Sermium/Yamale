package types

import "fmt"

// MaxFoundationAdministrators caps the exemption list.
//
// The cap is not about storage. It is there so that widening the one rule the
// whole perimeter rests on cannot happen by accident: a governance proposal
// that appends a hundred addresses fails outright rather than passing because
// nobody scrolled. Eight is more than a foundation board needs and small enough
// that the whole list fits in a proposal somebody will actually read.
const MaxFoundationAdministrators = 8

// DefaultParams returns the module's default parameters.
//
// The administrator list is empty, and empty is the safe state: with nobody
// named, the exemption from the jurisdiction rule grants nothing at all. It is
// left nil rather than an empty slice because that is what protobuf gives back
// for an absent repeated field, and a genesis export that does not round-trip
// byte-for-byte breaks upgrades.
func DefaultParams() Params {
	return Params{PayloadLength: PayloadLength}
}

// NewParams constructs Params.
func NewParams(payloadLength uint32, foundationAdministrators ...string) Params {
	return Params{
		PayloadLength:            payloadLength,
		FoundationAdministrators: foundationAdministrators,
	}
}

// Validate bounds the identifier length and the exemption list.
//
// Below eight characters the space stops being large enough to be unguessable;
// above sixteen nobody can read one aloud, which is the entire point of the
// module. Both ends are refused rather than clamped: a governance proposal that
// silently became something other than what was voted on is worse than one that
// fails.
//
// The administrator list is bounded and deduplicated for a harder reason. It is
// the single exception to "every account has a jurisdiction", so every way it
// could grow without anyone noticing has to be closed. A duplicate is refused
// because a list that reads as seven names and grants six is a list nobody can
// audit by looking at it.
func (p Params) Validate() error {
	if p.PayloadLength < MinPayloadLen || p.PayloadLength > MaxPayloadLen {
		return fmt.Errorf("payload_length must be between %d and %d, got %d",
			MinPayloadLen, MaxPayloadLen, p.PayloadLength)
	}

	if len(p.FoundationAdministrators) > MaxFoundationAdministrators {
		return fmt.Errorf("at most %d foundation administrators may be named, got %d",
			MaxFoundationAdministrators, len(p.FoundationAdministrators))
	}
	seen := make(map[string]struct{}, len(p.FoundationAdministrators))
	for _, addr := range p.FoundationAdministrators {
		if addr == "" {
			return fmt.Errorf("foundation_administrators contains an empty address")
		}
		if _, dup := seen[addr]; dup {
			return fmt.Errorf("foundation administrator %s is named twice", addr)
		}
		seen[addr] = struct{}{}
	}
	return nil
}

// IsFoundationAdministrator reports whether an address holds the exemption.
//
// Exact equality against the named list, and nothing else. No prefix match, no
// pattern, no "is it a module account" shortcut — every one of those is a way
// for the exemption to end up covering an account nobody voted to exempt.
func (p Params) IsFoundationAdministrator(addr string) bool {
	for _, a := range p.FoundationAdministrators {
		if a == addr {
			return true
		}
	}
	return false
}
