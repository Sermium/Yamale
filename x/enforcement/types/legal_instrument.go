package types

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// A legal instrument is the external authority a seizure is carried out under,
// and it is deliberately not the evidence fields.
//
// The distinction is the whole reason this is a separate structure rather than
// two more strings beside evidence_uri. Evidence answers "why does the chain
// believe this account did something"; an instrument answers "who, outside this
// chain, ordered that something be done about it". They are produced by
// different parties, verified by different means, and required at different
// strengths — evidence is optional for a freeze and an instrument is mandatory
// for a seizure and meaningless for a freeze.
//
// Folding them together would let a case satisfy its authority requirement by
// attaching its own investigation report, which is precisely the substitution
// the requirement exists to prevent. The validator set producing a document and
// then citing that document as its warrant to take somebody's assets is not
// oversight; it is the same body twice.
//
// There is no URI, on purpose. A link is a document somebody controls: the host
// can edit it, take it down, or never have had it, and a seizure whose only
// external anchor is a URL is anchored to whoever runs that web server. What is
// stored instead is an identifier that names the instrument in the world — the
// issuing body plus its own reference number — so verification means going to
// that body's register, and a hash that pins the content of what was served, so
// a copy produced later can be checked against what the validators voted on.
// Neither half depends on anyone keeping anything online.

// SHA256HexLength is the length of a SHA-256 digest written as lowercase hex.
const SHA256HexLength = 64

// IsZero reports whether no instrument was supplied at all.
//
// Every field, not just one. A caller that filled in an issuing authority and
// left the hash blank has supplied something, and that something has to reach
// Validate and be refused there with a message naming the missing half —
// rather than being read as "no instrument" and refused with a message about a
// field they did fill in.
func (li LegalInstrument) IsZero() bool {
	return strings.TrimSpace(li.IssuingAuthority) == "" &&
		strings.TrimSpace(li.Reference) == "" &&
		li.Kind == LEGAL_INSTRUMENT_KIND_UNSPECIFIED &&
		strings.TrimSpace(li.Hash) == "" &&
		li.IssuedAt == 0
}

// Validate checks an instrument that a seizure will rest on.
//
// blockTime is the chain's current time in Unix seconds, used to refuse an
// instrument dated in the future. Passed in rather than read from a clock
// because this runs inside consensus, where the only time that exists is the
// block's.
func (li LegalInstrument) Validate(blockTime int64) error {
	authority := strings.TrimSpace(li.IssuingAuthority)
	if authority == "" {
		return fmt.Errorf("legal_instrument must name the issuing authority; an order with no issuer is not an order")
	}
	if len(li.IssuingAuthority) > MaxIssuingAuthorityLength {
		return fmt.Errorf(
			"legal_instrument issuing_authority is %d characters, the maximum is %d",
			len(li.IssuingAuthority), MaxIssuingAuthorityLength)
	}

	// The reference is the half that makes the instrument findable by somebody
	// who does not trust this chain. Without it the hash pins a document nobody
	// can ask for, which proves the copy you were shown is the one on the case
	// and proves nothing about whether it exists.
	reference := strings.TrimSpace(li.Reference)
	if reference == "" {
		return fmt.Errorf(
			"legal_instrument must carry the issuer's own reference for it; without one the instrument cannot be looked up in the register that issued it")
	}
	if len(li.Reference) > MaxInstrumentReferenceLength {
		return fmt.Errorf(
			"legal_instrument reference is %d characters, the maximum is %d",
			len(li.Reference), MaxInstrumentReferenceLength)
	}

	switch li.Kind {
	case LEGAL_INSTRUMENT_KIND_COURT_ORDER,
		LEGAL_INSTRUMENT_KIND_REGULATORY_DIRECTION,
		LEGAL_INSTRUMENT_KIND_WARRANT:
	default:
		return fmt.Errorf(
			"legal_instrument kind must be a court order, a regulatory direction or a warrant, got %s", li.Kind)
	}

	// Checked as a digest, not merely as a non-empty string. A hash that is not
	// a hash pins nothing, and the moment anybody tries to verify it — which is
	// months later, in an argument — is the worst moment to discover that.
	if len(li.Hash) != SHA256HexLength {
		return fmt.Errorf(
			"legal_instrument hash is %d characters; a SHA-256 digest in hex is %d", len(li.Hash), SHA256HexLength)
	}
	if li.Hash != strings.ToLower(li.Hash) {
		return fmt.Errorf("legal_instrument hash must be lowercase hex, so that two records of the same instrument compare equal")
	}
	if _, err := hex.DecodeString(li.Hash); err != nil {
		return fmt.Errorf("legal_instrument hash is not hex: %w", err)
	}

	if li.IssuedAt <= 0 {
		return fmt.Errorf("legal_instrument must say when it was issued")
	}
	// An order dated tomorrow has not been issued. A case that claims one is
	// either mistaken about its own paperwork or manufacturing it, and neither
	// is a state the chain should record as a valid authority to take money.
	if blockTime > 0 && li.IssuedAt > blockTime {
		return fmt.Errorf(
			"legal_instrument is dated %d, which is after the current block time %d; an instrument cannot authorise a seizure before it exists",
			li.IssuedAt, blockTime)
	}

	return nil
}
