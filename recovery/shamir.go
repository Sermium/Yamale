// Package recovery splits a secret so that losing a device does not lose an
// account, and holding a service does not gain one.
//
// # Why this replaces the threshold-signature machinery
//
// The platform's decision of 2026-09-18 is that a deal is between a buyer and a
// seller, and the custodian services appear only in a recovery. The device
// signs alone.
//
// That one sentence removes the reason for threshold ECDSA. GG18 exists so that
// several parties can produce a signature without any of them holding the key —
// which is exactly what is NOT wanted here, because the device holding the key
// is the point. What remains is the ability to put a lost key back together,
// and that is secret sharing, which is this file.
//
// What goes away with it: the tss-lib dependency, the Paillier safe primes, the
// **nine and a half minutes** an enrolment took in a browser (measured, in a
// worker, on a desktop), and the requirement that key generation run off the
// main thread at all. Generating a secp256k1 key is microseconds.
//
// # The construction, and the property it keeps
//
//  1. The device generates an ordinary secp256k1 private key.
//  2. It is SEALED first — encrypted under a key derived from the account
//     holder's password — and only then split.
//  3. The sealed blob is split 2-of-3: one share stays with the device as its
//     own backup, one goes to the custodian, one to the recovery service.
//
// Sealing BEFORE splitting is the whole design. With plain Shamir over the raw
// key, the custodian and the recovery service between them hold two shares and
// can reconstruct it — so "no single service can move your money" would be true
// while "our two services together cannot" was false, and the second is the one
// that matters when both are run by the same operator.
//
// Sealed first, two colluding services reconstruct a ciphertext they cannot
// open. What stands between them and the account is then the password, and
// Argon2id is what makes that expensive. That is an honest weakening compared
// with a threshold scheme — an offline guessing attack becomes possible in the
// both-services-breached case, where GG18 offered none — and it is the trade
// the architecture asked for: under decision A a stolen device and password is
// already a stolen account, because nothing co-signs and nothing can freeze.
//
// Splitting rather than handing each service the whole ciphertext is not
// ceremony: one breached service then holds a share, which is information
// theoretically nothing at all, rather than a blob somebody can grind offline
// at their leisure.
//
// # The arithmetic
//
// Shamir over GF(2^8), byte by byte, which is the standard construction for
// sharing an opaque blob rather than a single number. Each byte of the secret
// becomes the constant term of an independent random polynomial; a share is
// that polynomial evaluated at the share's own x. Any `threshold` shares
// interpolate back to the constant term; any fewer reveal nothing — every
// secret remains exactly as likely as it was before, which is why this is
// information-theoretic rather than computational.
package recovery

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
)

// Share is one part of a split secret.
//
// `X` is the evaluation point and must be non-zero: the polynomial's value at
// zero IS the secret, so a share at x=0 would be the secret itself handed out
// under a name that suggests otherwise.
type Share struct {
	X byte
	Y []byte
}

var (
	ErrThreshold = errors.New(
		"the threshold must be at least 2 and no more than the number of shares")
	ErrShareCount   = errors.New("not enough shares to reconstruct")
	ErrShareLengths = errors.New("the shares are not all the same length")
	ErrShareX       = errors.New("a share's x is zero, which would be the secret itself")
	ErrDuplicateX   = errors.New("two shares have the same x and carry the same information")
	ErrEmptySecret  = errors.New("there is nothing to split")
)

/* --------------------------------------------------- GF(2^8) arithmetic */

// The field is GF(2^8) modulo x^8 + x^4 + x^3 + x + 1 — AES's polynomial, used
// because it is the one every reviewer has already seen.
const polynomial = 0x11b

// Log and exponential tables over the generator 3, which is primitive for this
// polynomial. Built once so multiplication and division are lookups: the
// alternative is a loop per byte per share, and this runs over every byte of
// every secret.
var (
	logTable [256]byte
	expTable [512]byte
)

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		expTable[i] = byte(x)
		expTable[i+255] = byte(x)
		logTable[byte(x)] = byte(i)
		// Multiply by the generator 3, i.e. x + 1.
		x ^= x << 1
		if x&0x100 != 0 {
			x ^= polynomial
		}
	}
	// log(0) is undefined; mul and div special-case zero rather than reading it.
	logTable[0] = 0
}

func mul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return expTable[int(logTable[a])+int(logTable[b])]
}

func div(a, b byte) byte {
	if b == 0 {
		// Only reachable from a duplicate x, which Combine refuses before it
		// gets here. Returning zero rather than panicking keeps a library
		// function from taking down a node, and the caller's own check is what
		// actually prevents it.
		return 0
	}
	if a == 0 {
		return 0
	}
	return expTable[int(logTable[a])-int(logTable[b])+255]
}

/* --------------------------------------------------------------- split */

// Split shares `secret` so that any `threshold` of `shares` can rebuild it and
// any fewer learn nothing.
//
// The x values are 1..shares, which is fine because they are public: a share's
// secrecy is entirely in its y bytes. Making them unpredictable would suggest
// otherwise and buy nothing.
func Split(secret []byte, shares, threshold int) ([]Share, error) {
	if len(secret) == 0 {
		return nil, ErrEmptySecret
	}
	if threshold < 2 || threshold > shares || shares > 255 {
		return nil, ErrThreshold
	}

	out := make([]Share, shares)
	for i := range out {
		out[i] = Share{X: byte(i + 1), Y: make([]byte, len(secret))}
	}

	// One independent polynomial per byte. Sharing every byte with the same
	// polynomial would leak the secret's structure: two identical bytes would
	// produce identical share bytes at every x.
	coefficients := make([]byte, threshold-1)
	for b, value := range secret {
		if _, err := rand.Read(coefficients); err != nil {
			return nil, fmt.Errorf("no randomness for the polynomial: %w", err)
		}
		for i := range out {
			out[i].Y[b] = evaluate(value, coefficients, out[i].X)
		}
	}
	return out, nil
}

// evaluate computes the polynomial whose constant term is `constant` at `x`,
// by Horner's method from the top coefficient down.
func evaluate(constant byte, coefficients []byte, x byte) byte {
	result := byte(0)
	for i := len(coefficients) - 1; i >= 0; i-- {
		result = mul(result, x) ^ coefficients[i]
	}
	return mul(result, x) ^ constant
}

/* ------------------------------------------------------------- combine */

// Combine rebuilds the secret from any sufficient set of shares.
//
// It cannot tell how many shares were needed: a threshold is not recorded in a
// share, because recording it would tell anybody holding one how many more they
// have to steal. Given too few, this returns a value — the wrong one — with no
// error, and there is nothing in the mathematics that could detect it. That is
// why the SEALED blob is what gets split: its authentication tag fails, loudly,
// and the caller learns that the reconstruction was wrong rather than
// proceeding with plausible rubbish. See Seal.
func Combine(shares []Share) ([]byte, error) {
	if len(shares) < 2 {
		return nil, ErrShareCount
	}

	length := len(shares[0].Y)
	seen := map[byte]bool{}
	for _, s := range shares {
		if len(s.Y) != length {
			return nil, ErrShareLengths
		}
		if s.X == 0 {
			return nil, ErrShareX
		}
		if seen[s.X] {
			return nil, ErrDuplicateX
		}
		seen[s.X] = true
	}

	secret := make([]byte, length)
	for b := 0; b < length; b++ {
		// Lagrange interpolation at x = 0, which is where the secret lives.
		var value byte
		for i, si := range shares {
			basis := byte(1)
			for j, sj := range shares {
				if i == j {
					continue
				}
				// (0 - xj) / (xi - xj); subtraction is xor in this field, so
				// 0 - xj is xj.
				basis = mul(basis, div(sj.X, si.X^sj.X))
			}
			value ^= mul(si.Y[b], basis)
		}
		secret[b] = value
	}
	return secret, nil
}

// Equal reports whether two secrets match, in constant time.
//
// Here rather than left to the caller because the obvious `bytes.Equal` returns
// early on the first differing byte, and the callers of this package are
// comparing recovered key material.
func Equal(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
