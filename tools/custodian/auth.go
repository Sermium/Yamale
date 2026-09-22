package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/argon2"

	"yamale/blockchain/recovery"
)

// Authentication, which is what this service decides with.
//
// It is tempting to think the custodian's job is holding a share. It is not,
// and since 2026-09-18 it is even less so: the holding is a share of a
// ciphertext, it opens nothing on its own, and nothing here co-signs a payment.
//
// What this service is FOR is deciding whether to let a RECOVERY proceed — to
// hand back the piece that, with the other service's piece and the account
// holder's password, rebuilds their key. That decision is the only thing
// standing between somebody who has taken over an email address and somebody
// else's account, and every rule in this file exists because of it.

// Argon2id parameters.
//
// Tuned rather than copied: the defaults in most examples are the RFC's
// *minimum* interactive figures, and a password database is worth more than
// 64 MB of somebody's server. These cost roughly 90 ms per verification on the
// hardware this runs on, which is slow enough to make offline cracking
// expensive and fast enough that nobody notices signing in.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// Verifier is what is stored for a password: a salt and a hash, never the
// password and never anything reversible into it.
//
// Encryption would be the wrong tool here and the mistake is common enough to
// name. Encryption is reversible, so a breach that takes the key takes every
// password — and people reuse passwords, so that breach spends beyond this
// system entirely.
type Verifier struct {
	Salt []byte `json:"salt"`
	Hash []byte `json:"hash"`
}

// NewVerifier hashes a password for storage.
func NewVerifier(password string) (Verifier, error) {
	if err := passwordSane(password); err != nil {
		return Verifier{}, err
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return Verifier{}, fmt.Errorf("reading randomness for a salt: %w", err)
	}
	return Verifier{
		Salt: salt,
		Hash: argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen),
	}, nil
}

// Verify reports whether a password matches.
//
// Constant time, because a comparison that returns early tells an attacker how
// many leading bytes they got right, and enough of those answers is the hash.
func (v Verifier) Verify(password string) bool {
	if len(v.Salt) == 0 || len(v.Hash) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), v.Salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return subtle.ConstantTimeCompare(got, v.Hash) == 1
}

// passwordSane refuses what is obviously not going to protect anything.
//
// A length floor and nothing else. Composition rules — a digit, a symbol, a
// capital — measurably push people towards Passw0rd! and away from length,
// which is the only property that actually matters.
func passwordSane(password string) error {
	if len([]rune(password)) < 10 {
		return errors.New("a password of fewer than 10 characters is not one")
	}
	if len(password) > 1024 {
		// Not a policy about strength; a bound on how much Argon2 work one
		// unauthenticated request can ask for.
		return errors.New("that password is implausibly long")
	}
	return nil
}

// BlindIndex is how an account is found by email without storing the email in a
// form anybody can search.
//
// The obvious wrong answers, in the order people reach for them:
//
//   - Store the email. A dump is then a mailing list of everybody who uses a
//     national payments system, which is a different kind of disaster from a
//     financial one.
//   - Encrypt it deterministically. Equal emails produce equal ciphertext, so
//     the column is a frequency-analysable index of exactly the same thing.
//   - Hash it. This is the one that feels safe and is not: email addresses are
//     low-entropy and enumerable, so an attacker with a list and a CPU tests
//     every guess offline and recovers the membership the hash was supposed to
//     hide. `clients/app` does exactly this today with SHA-256 and calls the
//     result a blind index; it is not one.
//
// The answer is a keyed hash whose PEPPER LIVES OUTSIDE THE DATABASE. Then a
// dump alone yields neither the addresses nor any way to test a guess at them,
// because testing requires the pepper, which was never in the thing that leaked.
type BlindIndex struct{ pepper []byte }

// NewBlindIndex takes the pepper from configuration, not from the store it
// protects. Refuses a short one: a pepper an attacker can guess is a hash.
func NewBlindIndex(pepper string) (*BlindIndex, error) {
	if len(pepper) < 32 {
		return nil, fmt.Errorf(
			"the pepper is %d bytes; at least 32 are needed, and it must come from somewhere "+
				"the account store does not", len(pepper))
	}
	return &BlindIndex{pepper: []byte(pepper)}, nil
}

// Of returns the lookup key for an address.
//
// Normalised first, because Alice@Example.COM and alice@example.com are one
// person and two keys, and the second one silently becomes an account nobody
// can sign in to.
func (b *BlindIndex) Of(email string) string {
	mac := hmac.New(sha256.New, b.pepper)
	// recovery.CanonicalEmail, not strings.ToLower, and the difference matters.
	// The device signs its deposit over the canonical email, so the spelling
	// this indexes under and the spelling that was signed have to be produced
	// by the same function — in two languages. See the note on CanonicalEmail.
	mac.Write([]byte(recovery.CanonicalEmail(email)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// There is no SecondFactorRequired any more.
//
// It answered "does this payment need more than a password?", and it answered
// it at the moment of signing rather than at login — which was the right place,
// because an attacker holding the password and the device share does not need
// to log in again, they need to sign.
//
// Nothing asks the question now. The device signs alone, so no service sees a
// payment before it happens and none can demand anything about one. The second
// factor moved to RECOVERY, where this service is still a participant and can
// still refuse; see factors.go.

// MinPasswordLength is the only password rule this service imposes.
//
// Length, and nothing else. Composition rules — an uppercase, a digit, a
// symbol — are known to push people toward "Password1!" and its neighbours,
// which is a smaller search space than a long phrase they can actually
// remember. NIST dropped them in 2017 and the reasoning has not changed.
//
// Twelve rather than eight because this password is one of the two things
// standing between a stolen phone and somebody's money: the thief already holds
// the device share, so the password is not one factor among several here, it is
// the factor.
const MinPasswordLength = 12

// checkPassword refuses what cannot be allowed, and explains why in terms of
// what to do instead.
func checkPassword(password string) error {
	// Counted in runes, not bytes. Twelve characters of Amharic or Arabic is
	// twelve characters; measuring bytes would quietly demand a third as many
	// from some alphabets as from others.
	if n := len([]rune(password)); n < MinPasswordLength {
		return fmt.Errorf(
			"a password needs at least %d characters and this has %d — several unrelated words are "+
				"easier to remember and harder to guess than a short one with symbols in it",
			MinPasswordLength, n)
	}
	// An upper bound only because Argon2id will happily spend the memory on a
	// megabyte of input, which is a denial of service anybody can send.
	if len(password) > 1024 {
		return errors.New("that password is longer than 1024 bytes")
	}
	return nil
}

// nowUTC is the timestamp written into account files.
//
// UTC, always. An operator moving a store between hosts in different zones
// should not find accounts that appear to have been created in the future.
func nowUTC() time.Time { return time.Now().UTC() }
