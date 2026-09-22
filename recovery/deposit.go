package recovery

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// Proving that a deposited holding belongs to the account it claims.
//
// # What the old protocol checked, and what was lost with it
//
// Under threshold key generation each service ran its own party, computed the
// joint public key itself, and compared the address to the one the device
// claimed. A device talking through something that rewrote the traffic produced
// a different key at the service and was refused. That check came free with the
// protocol.
//
// A single deposit has no such protocol, so the check has to be paid for
// explicitly, and without it the service would be storing an address on a
// stranger's say-so.
//
// # What the signature proves, stated exactly
//
// It proves that whoever assembled this deposit held the private key for the
// public key in it, at that moment, and that they meant THIS holding to go to
// THIS service for THIS account. Change any of those and the signature fails.
//
// It does NOT prove that the holding decrypts to that key. Nothing can: opening
// it needs the password, and a service that could check would be a service that
// could grind the password. So the residual risk is a device that deposits a
// well-formed holding of the wrong key — which loses that person their own
// account at recovery time and is not a route to anybody else's.
//
// That asymmetry is why this is worth having rather than worth perfecting: the
// attack it closes is substitution by a third party, and the one it leaves open
// is self-harm by a broken client.
//
// # Why the digest is defined here
//
// Because the device computes it too, and the two must agree byte for byte. A
// signing side and a verifying side that each build their own digest is the
// same class of fault as a client and a chain that each canonicalise an
// identifier their own way — see x/herd/keeper/identifiers.go, which exists
// because that happened.

const depositDomain = "yamale/recovery/deposit/v1"

var (
	ErrBadSignature = errors.New("the deposit is not signed by the key it claims")
	ErrBadPubKey    = errors.New("that is not a compressed secp256k1 public key")
)

// SignatureLen is the fixed r||s form, 32 bytes each.
//
// Fixed-width rather than DER because the other side of this is JavaScript, and
// DER's length prefixes are a well-worn source of parsers that accept two
// different encodings of one signature.
const SignatureLen = 64

// DepositDigest is what the device signs.
//
// Every field is length-prefixed. Plain concatenation would let one deposit's
// fields be re-cut into another's — an index of "ab" with a role of "c" hashing
// the same as an index of "a" with a role of "bc" — and the whole point of this
// digest is that it binds a specific set of values together.
func DepositDigest(index, role string, pubKey []byte, h Holding) [32]byte {
	sum := sha256.New()
	write := func(b []byte) {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(b)))
		sum.Write(n[:])
		sum.Write(b)
	}

	write([]byte(depositDomain))
	write([]byte(index))
	write([]byte(role))
	write(pubKey)

	// The header, so a rewritten salt or a downgraded cost parameter cannot be
	// slipped past a signature made over the share alone.
	write(h.Salt)
	write(h.Nonce)
	var params [9]byte
	binary.BigEndian.PutUint32(params[0:], h.Time)
	binary.BigEndian.PutUint32(params[4:], h.Memory)
	params[8] = h.Threads
	write(params[:])

	write([]byte{h.Share.X})
	write(h.Share.Y)

	var out [32]byte
	copy(out[:], sum.Sum(nil))
	return out
}

// SignDeposit produces the proof. Used by the device, and by the tests that
// stand in for one.
func SignDeposit(key *secp256k1.PrivateKey, index, role string, h Holding) ([]byte, error) {
	if key == nil {
		return nil, errors.New("no key to sign with")
	}
	pub := key.PubKey().SerializeCompressed()
	digest := DepositDigest(index, role, pub, h)

	sig := ecdsa.Sign(key, digest[:])
	r, s := sig.R(), sig.S()

	out := make([]byte, SignatureLen)
	rb, sb := r.Bytes(), s.Bytes()
	copy(out[:32], rb[:])
	copy(out[32:], sb[:])
	return out, nil
}

// VerifyDeposit checks the proof.
//
// Returns the parsed public key so the caller derives the address from what was
// actually verified, rather than from a second copy of it that might differ.
// Handing back only a boolean is how a check ends up guarding one value while
// the code stores another.
func VerifyDeposit(index, role string, pubKey, signature []byte, h Holding) (*secp256k1.PublicKey, error) {
	if len(pubKey) != secp256k1.PubKeyBytesLenCompressed {
		return nil, ErrBadPubKey
	}
	pub, err := secp256k1.ParsePubKey(pubKey)
	if err != nil {
		return nil, ErrBadPubKey
	}
	if len(signature) != SignatureLen {
		return nil, ErrBadSignature
	}

	var r, s secp256k1.ModNScalar
	// Overflow means the value is not a scalar. Refused rather than reduced:
	// reducing would accept a second encoding of the same signature, and a
	// signature with two valid encodings is a signature that can be replayed
	// somewhere that recorded the first one as spent.
	if r.SetByteSlice(signature[:32]) {
		return nil, ErrBadSignature
	}
	if s.SetByteSlice(signature[32:]) {
		return nil, ErrBadSignature
	}
	if r.IsZero() || s.IsZero() {
		return nil, ErrBadSignature
	}

	digest := DepositDigest(index, role, pubKey, h)
	if !ecdsa.NewSignature(&r, &s).Verify(digest[:], pub) {
		return nil, ErrBadSignature
	}
	return pub, nil
}
