package keeper

import (
	"strings"

	"yamale/blockchain/x/herd/types"
)

// Making an identifier mean one animal.
//
// This file exists because of a hole in the one guarantee the registry makes
// about itself. The keeper's own comment says:
//
//	ByNationalID makes an identifier mean one animal. It is the key printed
//	on a certificate.
//
// and the code behind it did `strings.TrimSpace` and nothing else. So the
// uniqueness check caught only byte-identical duplicates, and these are each
// TWO ANIMALS to the register:
//
//	221/682/1029/L   and  221/682/001029/L
//	982 000 000 000 003  and  982000000000003
//
// Neither duplicate is visible to `AnimalByNationalID` or `AnimalByTag`,
// because the query canonicalises no more than the write did. An operator who
// typed the serial without its leading zeros gets a second record, a second
// certificate, and no indication that the first exists — on a register whose
// purpose is that a beast has one identity.
//
// Canonicalising in the client is necessary and not sufficient. A second
// client, the CLI, or anybody signing a message by hand bypasses it, and the
// chain is the thing that has to be authoritative about what an identifier
// means. So it is done here, before the index is touched.
//
// # Refusing rather than repairing, where repair would guess
//
// A serial SHORTER than the register issues is padded: `1029` is unambiguously
// `001029`, because the field has a fixed width and leading zeros carry no
// information. A serial LONGER is refused rather than truncated — seven digits
// is either a different numbering scheme or a typo, and dropping a digit would
// point the record at a different animal with a plausible-looking identifier.
// That is the one outcome worse than a rejection.
//
// The client does exactly this, in clients/herd/src/identity.ts. The two
// implementations must agree; a disagreement between the canonicalisation a
// client signs and the one the chain indexes would be a subtler version of the
// bug this file fixes.

// NationalIDSerialDigits is the width of the serial in the form the sector
// issues: 221/682/021029/L — country dialling code, flock, serial, breed.
const NationalIDSerialDigits = 6

// CanonicalNationalID returns the form an identifier is indexed under.
//
// Refuses anything it would have to guess at. The error names what is wrong
// with the identifier rather than reporting a generic rejection, because the
// person reading it is standing in a pen with a certificate in their hand.
func CanonicalNationalID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", types.ErrNoNationalID
	}

	parts := strings.Split(id, "/")
	if len(parts) != 4 {
		return "", types.ErrInvalidNationalID.Wrapf(
			"%q is not country/flock/serial/breed", id)
	}

	country := strings.TrimSpace(parts[0])
	flock := strings.TrimSpace(parts[1])
	serial := strings.TrimSpace(parts[2])
	breed := strings.ToUpper(strings.TrimSpace(parts[3]))

	if !allDigits(country) || !allDigits(flock) || !allDigits(serial) {
		return "", types.ErrInvalidNationalID.Wrapf(
			"country, flock and serial must be digits: %q", id)
	}
	if country == "" || flock == "" || serial == "" || breed == "" {
		return "", types.ErrInvalidNationalID.Wrapf("%q has an empty part", id)
	}
	if len(serial) > NationalIDSerialDigits {
		// Not truncated. See the note at the top: dropping a digit produces a
		// valid-looking identifier belonging to a different animal.
		return "", types.ErrInvalidNationalID.Wrapf(
			"serial %q is longer than the %d digits the register issues",
			serial, NationalIDSerialDigits)
	}

	return country + "/" + flock + "/" +
		strings.Repeat("0", NationalIDSerialDigits-len(serial)) + serial + "/" + breed, nil
}

// CanonicalTag returns the form an ISO 11784/11785 code is indexed under.
//
// Fifteen digits: a three-digit country or manufacturer code and a twelve-digit
// national code. Readers and operators write it with spaces, dots and hyphens
// in every combination, and every one of those is the same transponder.
//
// The empty string is returned unchanged and without error: a level-1 animal
// has no tag, and the keeper deliberately does not index an empty one.
func CanonicalTag(raw string) (string, error) {
	tag := strings.TrimSpace(raw)
	if tag == "" {
		return "", nil
	}

	var digits strings.Builder
	for _, r := range tag {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == ' ' || r == '.' || r == '-' || r == '_':
			// Separators an operator or a reader inserted. Dropped rather than
			// refused: they carry nothing, and refusing them would reject codes
			// that are correct in every way that matters.
		default:
			return "", types.ErrInvalidTag.Wrapf(
				"%q contains %q, which is not a digit or a separator", tag, string(r))
		}
	}

	out := digits.String()
	if len(out) != 15 {
		return "", types.ErrInvalidTag.Wrapf(
			"%q has %d digits; an ISO 11784/11785 code has 15", tag, len(out))
	}
	return out, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
