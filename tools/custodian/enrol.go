package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"yamale/blockchain/recovery"
	"yamale/blockchain/recovery/chain"
)

// Enrolment: how a member of the public gets an account.
//
// # What this used to be, and why it is a tenth of the size
//
// Until the architecture decision of 2026-09-18 this file ran a threshold key
// generation. The device coordinated a three-party protocol across two
// services, each computing a share of a key that existed nowhere; it took three
// endpoints, a session store with a ten-minute TTL, a pool of pre-computed
// Paillier safe primes, and about nine and a half minutes of a phone's CPU.
//
// All of that existed so that several parties could sign together. Decision A
// says a deal is between a buyer and a seller and the services appear only in a
// recovery — so nothing signs together, and the entire apparatus was paying for
// a property nobody wanted.
//
// What replaces it: the device generates an ordinary secp256k1 key, seals it
// under the account holder's password, splits the sealed blob three ways, and
// posts ONE of those holdings here. One request, no session, no protocol, no
// state between calls. The arithmetic is in the recovery package.
//
// # What this service can and cannot verify
//
// It cannot verify that the person enrolling is who they say. Nobody can, over
// HTTP, without an identity document or an agent — and pretending otherwise
// would be worse than saying so. The identity question belongs to enrolment
// policy and is deliberately not decided here.
//
// What it CAN verify, and does:
//
//   - the email is not already enrolled, so an enrolment can never overwrite an
//     existing account or replace its holding;
//   - the deposit is signed by the key it names, and the signature covers the
//     holding, the account and this service's role — so nothing between the
//     device and here can substitute any of them. See recovery/deposit.go for
//     exactly what that proves and what it does not;
//   - the address is DERIVED from the verified public key rather than taken
//     from the request, so there is no address field to lie in;
//   - the password verifier is written only when all of the above hold.

var (
	ErrAlreadyEnrolled = errors.New("an account already exists for that email")
	ErrWrongRole       = errors.New("that holding was not meant for this service")
)

// Deposit is one service's half of an enrolment.
//
// Note what is absent: no address. It is derived from PubKey after the
// signature verifies, because a field the caller supplies and the service
// merely compares is a field somebody eventually forgets to compare.
type Deposit struct {
	Email    string           `json:"email"`
	Password string           `json:"password"`
	PubKey   []byte           `json:"pubkey"`
	Holding  recovery.Holding `json:"holding"`
	// Signature is over recovery.DepositDigest(index, role, pubkey, holding).
	Signature []byte `json:"signature"`
}

// Enrol validates a deposit and commits the account.
//
// The order is deliberate and every step is cheap before the expensive one: the
// existence check and the signature check both run before Argon2id is asked to
// do 256 MiB of work, so an unauthenticated caller cannot spend this service's
// memory by posting rubbish.
func (s *server) Enrol(d Deposit) (string, error) {
	email := strings.TrimSpace(d.Email)
	if email == "" || d.Password == "" {
		return "", errors.New("an email and a password are required")
	}
	if err := checkPassword(d.Password); err != nil {
		return "", err
	}
	if d.Holding.Role != s.role {
		// The device posts a different holding to each service. One that posted
		// the same holding to both would leave the account recoverable from one
		// breach and unrecoverable from the intended pair — and neither
		// symptom would appear until somebody tried to recover.
		return "", fmt.Errorf("%w: this is the %s service and that is the %s holding",
			ErrWrongRole, s.role, d.Holding.Role)
	}

	index := s.index.Of(email)

	// Refused before anything expensive happens, and this is the check that
	// makes enrolment safe to expose: without it, anybody could enrol over an
	// existing email and replace that account's holding, which does not steal
	// the money but does destroy the ability to ever recover it.
	if _, err := s.store.Get(index); err == nil {
		return "", ErrAlreadyEnrolled
	}

	pub, err := recovery.VerifyDeposit(index, s.role, d.PubKey, d.Signature, d.Holding)
	if err != nil {
		return "", err
	}
	address, err := chain.Address(pub.SerializeCompressed())
	if err != nil {
		return "", err
	}

	verifier, err := NewVerifier(d.Password)
	if err != nil {
		return "", fmt.Errorf("the password could not be prepared: %w", err)
	}

	// Re-checked after the slow work. Two deposits for one email can arrive
	// close enough together that both passed the first check, and the one that
	// commits second would otherwise overwrite the first — replacing the
	// holding of an account whose owner has already been told it exists.
	if _, err := s.store.Get(index); err == nil {
		return "", ErrAlreadyEnrolled
	}

	account := Account{
		Index:    index,
		Address:  address,
		Password: verifier,
		Created:  nowUTC(),
	}
	if err := s.store.Put(account, d.Holding); err != nil {
		return "", err
	}
	return address, nil
}

// enrolmentError maps the failures above onto status codes.
func enrolmentError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrAlreadyEnrolled):
		return http.StatusConflict, err.Error()
	case errors.Is(err, ErrWrongRole):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, recovery.ErrBadSignature), errors.Is(err, recovery.ErrBadPubKey):
		// 400 rather than 403. Nobody is being denied permission; the request
		// does not prove what it claims, and the remedy is to send a correct
		// one rather than to obtain access.
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusBadRequest, fmt.Sprint(err)
	}
}

// There is no longer any such thing as an abandoned enrolment.
//
// Worth stating because the operational runbook still refers to reaping them. A
// deposit either commits or leaves nothing behind: no session, nothing to
// expire, and no background sweeper that can quietly stop running.
