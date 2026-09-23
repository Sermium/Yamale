package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"yamale/blockchain/recovery"
)

// Where the custodian's shares live, and what a breach of this file yields.
//
// The honest answer, stated first because it is the one an auditor will ask
// for: a breach of the store alone yields ciphertext and a set of blind
// indexes. It does NOT yield a signing key, because a custodian share is one of
// three and signs nothing by itself — that is the whole design and it holds
// even if everything below fails.
//
// What a breach of the store PLUS the sealing key yields is every custodian
// share. Still not a key: an attacker then needs a device share per account, one
// at a time, from the people who hold them. The point of encrypting at rest is
// not to be the last line of defence; it is to make sure a stolen disk is not
// also a stolen queue of half-compromised accounts.

// Account is everything the custodian holds about one person.
//
// Note what is NOT here: no name, no email, no phone, no document number. The
// custodian needs to find an account and decide whether to co-sign; it does not
// need to know who anybody is, and personal data it never holds is personal
// data it cannot leak or be compelled to produce.
type Account struct {
	// Index is the blind index of the email. The email itself is not stored.
	Index string `json:"index"`
	// Address is the chain account the three shares control. Public by nature.
	Address string `json:"address"`
	// Password is the Argon2id verifier, never the password.
	Password Verifier `json:"password"`
	// Holding is the custodian's holding, sealed again at rest by this store.
	// One of three, and already a share of a password-sealed ciphertext before
	// it ever reaches here — see the recovery package.
	Holding []byte `json:"holding"`
	// Factors is the account holder's own second factor, sealed. Empty until
	// they enrol one.
	//
	// Sealed rather than stored plainly because a TOTP secret IS the factor:
	// anybody who reads it can produce codes for ever, and it does not expire
	// or get noticed the way a stolen phone does.
	Factors []byte `json:"factors,omitempty"`
	// Created is when enrolment completed.
	Created time.Time `json:"created"`
	// Frozen stops this account co-signing at all, which is what a reported
	// theft turns on. It is deliberately not a deletion: an account that
	// vanishes takes its own audit trail with it.
	Frozen bool `json:"frozen,omitempty"`
	// FrozenReason is shown to whoever tries to sign.
	FrozenReason string `json:"frozen_reason,omitempty"`
}

// Store is the custodian's accounts, sealed at rest.
//
// One file per account under a directory, rather than a database, for the same
// reason tools/payloadstore does it: the operational story of "back this
// directory up" is one a ministry's own staff can follow, and a service that
// needs a database administrator before it can hold anybody's share is a
// service that will be run wrong.
type Store struct {
	dir string
	// role is the ONE share role this store may hold. Checked on every read
	// rather than trusted from whatever wrote the file, because a store holding
	// two roles is a service that can sign alone.
	role string
	aead cipher.AEAD
	mu   sync.RWMutex
}

// NewStore opens the store, sealing with a 32-byte key held outside it.
func NewStore(dir, role string, sealingKey []byte) (*Store, error) {
	if role != recovery.RoleCustodian && role != recovery.RoleRecovery {
		// The device's holding belongs on the device, and recovery.RoleDevice
		// is now a name this can refuse by. Under the sealed-then-split scheme
		// a service pair holds ciphertext, so one more holding does not by
		// itself sign — but a service holding the device's is a service holding
		// what the account holder was promised nobody else had. Refused at
		// construction rather than at the first read.
		return nil, fmt.Errorf(
			"a store may hold the %s or %s holding, never %q",
			recovery.RoleCustodian, recovery.RoleRecovery, role)
	}
	if len(sealingKey) != 32 {
		return nil, fmt.Errorf(
			"the sealing key is %d bytes; 32 are required, and it must not live in the directory it seals",
			len(sealingKey))
	}
	block, err := aes.NewCipher(sealingKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, role: role, aead: aead}, nil
}

// ErrNoAccount is returned for an index nothing is stored under.
//
// Deliberately indistinguishable at the API boundary from a wrong password. An
// endpoint that says "no such account" is an endpoint that confirms which email
// addresses are enrolled, to anybody who asks, one guess at a time.
var ErrNoAccount = errors.New("no account")

func (s *Store) path(index string) string {
	// The index is already base64url of an HMAC, so it is a safe filename and
	// carries no personal data. Checked anyway rather than trusted: a path
	// assembled from anything a caller influences is worth one line of doubt.
	return filepath.Join(s.dir, filepath.Base(index)+".json")
}

// Put writes an account, sealing the holding.
func (s *Store) Put(a Account, holding recovery.Holding) error {
	// Refused on the way in as well as on the way out. The read-side check is
	// the one that catches a bad file; this one stops the bad file existing,
	// which matters because a share written here is a share that survives a
	// restart.
	if holding.Role != s.role {
		return fmt.Errorf(
			"this is a %s store and that is a %s holding; one service holding two of the three "+
				"is one service that can attempt a recovery on its own",
			s.role, holding.Role)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	plain, err := json.Marshal(holding)
	if err != nil {
		return fmt.Errorf("serialising the holding: %w", err)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("reading a nonce: %w", err)
	}
	// The index is authenticated as additional data, so a sealed holding cannot
	// be lifted out of one account's file and dropped into another's.
	a.Holding = s.aead.Seal(nonce, nonce, plain, []byte(a.Index))

	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path(a.Index), raw)
}

// Get reads an account without unsealing its holding.
//
// Separate from Holding() on purpose: authentication needs the verifier and the
// frozen flag and has no business touching key material, so the common path
// never decrypts anything.
func (s *Store) Get(index string) (Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, err := os.ReadFile(s.path(index))
	if errors.Is(err, os.ErrNotExist) {
		return Account{}, ErrNoAccount
	}
	if err != nil {
		return Account{}, err
	}
	var a Account
	if err := json.Unmarshal(raw, &a); err != nil {
		return Account{}, fmt.Errorf("reading an account: %w", err)
	}
	return a, nil
}

// Holding unseals this service's holding for one account.
//
// Called only during a recovery. Nothing else in this service has any reason to
// touch it: under the architecture decision of 2026-09-18 the device signs
// alone, so there is no co-signing path that needs key material at all.
func (s *Store) Holding(a Account) (recovery.Holding, error) {
	if len(a.Holding) < s.aead.NonceSize() {
		return recovery.Holding{}, errors.New("the stored holding is truncated")
	}
	nonce, sealed := a.Holding[:s.aead.NonceSize()], a.Holding[s.aead.NonceSize():]
	plain, err := s.aead.Open(nil, nonce, sealed, []byte(a.Index))
	if err != nil {
		// Wrong key, tampering, or a holding moved between accounts. All three
		// are the same refusal and none of them says which.
		return recovery.Holding{}, errors.New("the stored holding does not open")
	}
	var holding recovery.Holding
	if err := json.Unmarshal(plain, &holding); err != nil {
		return recovery.Holding{}, fmt.Errorf("the unsealed holding does not decode: %w", err)
	}
	if holding.Role != s.role {
		// Checked on every read rather than trusted from whatever wrote the
		// file — including a file written by an earlier version of this
		// program, or restored from the wrong backup.
		return recovery.Holding{}, fmt.Errorf(
			"this is a %s store and that is a %s holding; one service holding two of the three "+
				"is one service that can attempt a recovery on its own",
			s.role, holding.Role)
	}
	return holding, nil
}

// factorsAAD binds a sealed factor blob to its account AND to its purpose.
//
// The index alone would let a holding be written into the factors field, or the
// reverse: both are sealed by the same key with the same index as additional
// data, so both would open. They would then fail to decode, which is a worse
// error than a refusal and arrives further from the cause.
func factorsAAD(index string) []byte { return []byte(index + "|factors") }

// Factors unseals the second factor this account has enrolled.
//
// An account with none returns the zero value and no error: not having enrolled
// a factor is the ordinary state, and Strongest() answers FactorEmail for it.
func (s *Store) Factors(a Account) (Enrolled, error) {
	if len(a.Factors) == 0 {
		return Enrolled{}, nil
	}
	if len(a.Factors) < s.aead.NonceSize() {
		return Enrolled{}, errors.New("the stored factors are truncated")
	}
	nonce, sealed := a.Factors[:s.aead.NonceSize()], a.Factors[s.aead.NonceSize():]
	plain, err := s.aead.Open(nil, nonce, sealed, factorsAAD(a.Index))
	if err != nil {
		return Enrolled{}, errors.New("the stored factors do not open")
	}
	var out Enrolled
	if err := json.Unmarshal(plain, &out); err != nil {
		return Enrolled{}, fmt.Errorf("the unsealed factors do not decode: %w", err)
	}
	return out, nil
}

// SetFactors writes the account holder's enrolled factors.
//
// Read-modify-write under the store's lock, because the alternative is a caller
// holding an Account across a slow operation and writing back a record that has
// lost whatever else changed in the meantime.
func (s *Store) SetFactors(index string, factors Enrolled) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path(index))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoAccount
	}
	if err != nil {
		return err
	}
	var a Account
	if err := json.Unmarshal(raw, &a); err != nil {
		return fmt.Errorf("reading an account: %w", err)
	}

	plain, err := json.Marshal(factors)
	if err != nil {
		return fmt.Errorf("serialising the factors: %w", err)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("reading a nonce: %w", err)
	}
	a.Factors = s.aead.Seal(nonce, nonce, plain, factorsAAD(a.Index))

	out, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path(index), out)
}

// Freeze marks an account as reported stolen.
//
// It no longer stops a signature — nothing co-signs, so nothing can be stopped
// at signing time. What it stops is a RECOVERY, which is the one thing this
// service still gates, and it is the right thing to stop: somebody who has
// taken over an email address uses a recovery to move the account to a device
// they hold.
func (s *Store) Freeze(index, reason string) error {
	a, err := s.Get(index)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a.Frozen = true
	a.FrozenReason = reason
	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path(index), raw)
}

// Count reports how many accounts exist, for an operator's status line.
func (s *Store) Count() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			n++
		}
	}
	return n, nil
}

// writeFileAtomic writes through a temporary file and a rename.
//
// A share half-written because the process died mid-write is an account that
// can never be signed for again, and the person holding the other share has no
// way to tell that from a service outage.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
