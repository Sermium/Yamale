package types

import "fmt"

const (
	// ViewingKeyLength is the size of an X25519 public key.
	//
	// Checked on the way in rather than assumed, because nothing later can
	// check it. A sender wrapping a content key to a 31-byte value produces an
	// envelope that is well-formed, stores cleanly and opens for nobody — and
	// the party who discovers it is the payee, months afterwards, with no way
	// to tell a truncated registration from a corrupted payload.
	ViewingKeyLength = 32

	// MaxLiveAuditorGrants caps how many accounts may hold the cross-account
	// reading role at once.
	//
	// The cap is not about storage. This is the one role that reads the payment
	// detail of people who never dealt with the holder, and every envelope is
	// sealed to every live grant — so an unbounded list is both an unbounded
	// audience and an unbounded per-payment cost paid by the sender. Four is
	// more than a supervisory programme needs and small enough that adding one
	// is visible.
	MaxLiveAuditorGrants = 4
)

// ValidateViewingKey checks a published public key.
//
// An all-zero key is refused separately from a wrong-length one. Thirty-two
// zero bytes is what an uninitialised buffer looks like, it is a low-order
// X25519 point whose every agreement is the identity, and a client that sent it
// by accident would publish a key that appears to work: envelopes seal, and
// every party who holds the same zero "secret" can open them.
func ValidateViewingKey(key []byte) error {
	if len(key) != ViewingKeyLength {
		return fmt.Errorf("viewing key must be %d bytes of X25519, got %d", ViewingKeyLength, len(key))
	}
	for _, b := range key {
		if b != 0 {
			return nil
		}
	}
	return fmt.Errorf("viewing key is all zero, which is not a public key")
}

// Live reports whether a key may still be sealed to.
//
// Revocation blocks future wrapping and nothing else. The envelopes already
// sealed to this version stay sealed to it, because ciphertext that has been
// distributed cannot be recalled — so this answers "may I use it", never "can
// it still be read".
//
// It reads the boolean and never the height. Deriving liveness from
// revoked_at_height != 0 is what this field replaced: proto3 gives back zero
// for an unset int64 and zero is also a real block height, so a genesis-seeded
// revocation read back as live and senders went on sealing to a compromised
// key.
func (k ViewingKey) Live() bool { return !k.Revoked }

// Live reports whether a grant still holds at a height.
//
// Strictly less than, so expires_at_height is the first height at which the
// grant is gone. An inclusive comparison would leave a grant alive for one
// block past the height a governance vote named, which is the kind of
// off-by-one that only ever shows up in an argument about what was read.
func (g AuditorGrant) Live(height int64) bool { return height < g.ExpiresAtHeight }
