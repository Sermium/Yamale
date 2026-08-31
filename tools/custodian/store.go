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

	"yamale/blockchain/mpc"
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
	// Share is the custodian's share, sealed. One of three.
	Share []byte `json:"share"`
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
	dir  string
	aead cipher.AEAD
	mu   sync.RWMutex
}

// NewStore opens the store, sealing with a 32-byte key held outside it.
func NewStore(dir string, sealingKey []byte) (*Store, error) {
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
	return &Store{dir: dir, aead: aead}, nil
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

// Put writes an account, sealing the share.
func (s *Store) Put(a Account, share mpc.Share) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	plain, err := json.Marshal(share)
	if err != nil {
		return fmt.Errorf("serialising the share: %w", err)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("reading a nonce: %w", err)
	}
	// The index is authenticated as additional data, so a sealed share cannot
	// be lifted out of one account's file and dropped into another's.
	a.Share = s.aead.Seal(nonce, nonce, plain, []byte(a.Index))

	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path(a.Index), raw)
}

// Get reads an account without unsealing its share.
//
// Separate from Share() on purpose: authentication needs the verifier and the
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

// Share unseals the custodian's share for one account.
func (s *Store) Share(a Account) (mpc.Share, error) {
	if len(a.Share) < s.aead.NonceSize() {
		return mpc.Share{}, errors.New("the stored share is truncated")
	}
	nonce, sealed := a.Share[:s.aead.NonceSize()], a.Share[s.aead.NonceSize():]
	plain, err := s.aead.Open(nil, nonce, sealed, []byte(a.Index))
	if err != nil {
		// Wrong key, tampering, or a share moved between accounts. All three
		// are the same refusal and none of them says which.
		return mpc.Share{}, errors.New("the stored share does not open")
	}
	var share mpc.Share
	if err := json.Unmarshal(plain, &share); err != nil {
		return mpc.Share{}, fmt.Errorf("the unsealed share does not decode: %w", err)
	}
	if share.Role != mpc.RoleCustodian {
		// A custodian holding a device share would be a custodian able to sign
		// alone, so this is checked on every read rather than trusted from
		// whatever wrote the file.
		return mpc.Share{}, fmt.Errorf(
			"this store holds a %s share, which a custodian must never have", share.Role)
	}
	return share, nil
}

// Freeze stops an account co-signing.
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
