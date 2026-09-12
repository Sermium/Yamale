package types

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
)

// AcknowledgementText is the warning a holder must accept, embedded so that the
// binary and the text cannot drift apart.
//
// Embedded rather than read from disk at run time, and hashed rather than
// compared. A file on a server can be edited after somebody agreed to it; a
// hash pinned in the chain's parameters cannot, which is the whole reason the
// acknowledgement is recorded as a hash and not as a version string. Anybody
// can recompute this and check what a given holder actually accepted.
//
//go:embed acknowledgement-v1.md
var AcknowledgementText string

// AcknowledgementVersion names the text above.
const AcknowledgementVersion = "2026-09-1"

// AcknowledgementHash is the sha256 of the embedded text, hex encoded.
//
// Computed rather than written down. A constant here would be one more thing
// that can silently stop matching the file it claims to describe.
func AcknowledgementHash() string {
	sum := sha256.Sum256([]byte(normaliseAcknowledgement(AcknowledgementText)))
	return hex.EncodeToString(sum[:])
}

// normaliseAcknowledgement makes the hash independent of line endings.
//
// Not cosmetic. This repository has been bitten before by files arriving with
// CRLF on one machine and LF on another; a hash that changed with the checkout
// would refuse every acknowledgement made on the other platform, and the
// failure would look like the holder having agreed to the wrong thing.
func normaliseAcknowledgement(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// DefaultParams are a starting position, not a finding.
func DefaultParams() Params {
	return Params{
		// Forty-eight hours.
		//
		// Long enough that a phone stolen in the evening is noticed before the
		// key can be taken, and short enough that somebody who genuinely wants
		// their key is not being obstructed. A week would protect marginally
		// better and would read, correctly, as the platform making it hard to
		// leave — which is the accusation this whole pathway exists to refuse.
		DisclosureDelay: 48 * 60 * 60,

		AcknowledgementVersion: AcknowledgementVersion,
		AcknowledgementHash:    AcknowledgementHash(),

		// Thirty days. A request nobody completed is a standing authorisation
		// to take a key, waiting for the day somebody acquires the device.
		RequestExpiry: 30 * 24 * 60 * 60,
	}
}

// Validate refuses parameters that would disable the protections.
func (p Params) Validate() error {
	// Zero would let a request be made and confirmed in the same block, which
	// removes the only defence a person with a stolen phone has.
	if p.DisclosureDelay <= 0 {
		return ErrInvalidParams.Wrapf(
			"disclosure_delay must be positive; at zero a stolen device can take a key "+
				"in one block and the waiting period protects nobody, got %d", p.DisclosureDelay)
	}
	if p.RequestExpiry <= p.DisclosureDelay {
		return ErrInvalidParams.Wrapf(
			"request_expiry (%d) must exceed disclosure_delay (%d), or a request lapses "+
				"before it can ever be confirmed", p.RequestExpiry, p.DisclosureDelay)
	}
	if strings.TrimSpace(p.AcknowledgementVersion) == "" {
		return ErrInvalidParams.Wrap("an acknowledgement version is required")
	}
	// A hash that is not a hash cannot be checked against anything, and an
	// acknowledgement nobody can verify is the modal dialog this module exists
	// to replace.
	if _, err := hex.DecodeString(p.AcknowledgementHash); err != nil || len(p.AcknowledgementHash) != 64 {
		return ErrInvalidParams.Wrapf(
			"acknowledgement_hash must be a 64-character hex sha256, got %q", p.AcknowledgementHash)
	}
	return nil
}

// MatchesPublished reports whether an acknowledgement offered by a holder is
// the one this chain publishes.
func (p Params) MatchesPublished(version, hash string) error {
	if !strings.EqualFold(hash, p.AcknowledgementHash) || version != p.AcknowledgementVersion {
		return ErrWrongAcknowledgement.Wrapf(
			"this chain publishes version %s (%s); you offered version %s (%s)",
			p.AcknowledgementVersion, short(p.AcknowledgementHash), version, short(hash))
	}
	return nil
}

func short(h string) string {
	if len(h) <= 12 {
		return h
	}
	return fmt.Sprintf("%s…", h[:12])
}
