package types

import (
	"encoding/hex"
	"strings"
)

// NormaliseInscription reduces an inscription to the form it is indexed by:
// upper-case letters and digits only. Separators a person types between
// groups (space, dash, dot, slash, underscore) are dropped; anything else is
// refused rather than repaired, because a laser does not engrave it.
func NormaliseInscription(s string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '.' || r == '/' || r == '_':
		default:
			return "", ErrInvalidInscription.Wrapf("%q contains %q", s, r)
		}
	}
	out := b.String()
	if len(out) < 4 || len(out) > 40 {
		return "", ErrInvalidInscription.Wrapf("%q has %d characters once separators are removed", s, len(out))
	}
	return out, nil
}

// ValidateRecord checks a record fingerprint and its location. Lower case
// only, so one file has one fingerprint string.
func ValidateRecord(hash, uri string) error {
	if len(hash) != 64 || hash != strings.ToLower(hash) {
		return ErrInvalidRecord.Wrapf("fingerprint %q", hash)
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return ErrInvalidRecord.Wrapf("fingerprint %q", hash)
	}
	if uri == "" || len(uri) > 512 {
		return ErrInvalidRecord.Wrap("location")
	}
	return nil
}
