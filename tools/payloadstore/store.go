package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"yamale/blockchain/x/paymsg/types"
)

// store holds the envelopes and the outstanding challenges.
type store struct {
	cfg   config
	chain *chainReader

	mu         sync.Mutex
	challenges map[string]challenge
}

type challenge struct {
	address string
	nonce   []byte
	expires time.Time
}

func newStore(cfg config) (*store, error) {
	if err := os.MkdirAll(cfg.dir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot open the payload directory: %w", err)
	}
	return &store{
		cfg:        cfg,
		chain:      newChainReader(cfg.rest),
		challenges: map[string]challenge{},
	}, nil
}

// envelopePath is where one payment's envelope lives.
//
// The end-to-end id is hashed rather than used as a filename. It is
// caller-supplied text that reaches this process from the chain, so using it
// raw would make path traversal a matter of choosing a reference — and hashing
// also keeps the directory listing from being a plaintext index of every
// payment reference this participant has handled, readable by anyone who gets
// as far as the disk.
func (s *store) envelopePath(participant, endToEndID string) string {
	sum := sha256.Sum256([]byte(participant + "\x00" + endToEndID))
	return filepath.Join(s.cfg.dir, hex.EncodeToString(sum[:])+".envelope")
}

// challenge issues a proof-of-possession puzzle for one account.
//
// The nonce is sealed to the account's registered viewing key, so answering it
// requires the private half — the same key that will decrypt the payload. The
// challenge therefore grants nothing the key did not already grant, which is
// what keeps this from being a second, weaker credential sitting beside the
// real one.
func (s *store) challenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, response{
			Status: statusError,
			Detail: `POST {"address":"yml1…"} to be issued a challenge.`,
		})
		return
	}

	var req struct {
		Address string `json:"address"`
	}
	if err := readJSON(r, &req); err != nil || req.Address == "" {
		writeJSON(w, http.StatusBadRequest, response{Status: statusError, Detail: "an address is required"})
		return
	}

	key, err := s.chain.latestLiveViewingKey(req.Address)
	if err != nil {
		// An account with no key is told so plainly. It is not a failure to
		// retry: nothing can be sealed to it and nothing it holds can open a
		// payload, so the honest answer is that it has published no key.
		writeJSON(w, http.StatusNotFound, response{
			Status: statusDenied,
			Reason: reasonUnproven,
			Detail: fmt.Sprintf("%s has published no live viewing key on this chain, so there is nothing to prove possession of", req.Address),
		})
		return
	}

	nonce := make([]byte, types.ContentKeyLength)
	if _, err := rand.Read(nonce); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Status: statusError, Detail: "could not generate a challenge"})
		return
	}
	block, err := types.WrapTo(nonce, key)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Status: statusError, Detail: "could not seal the challenge to that viewing key"})
		return
	}

	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Status: statusError, Detail: "could not generate a challenge"})
		return
	}
	challengeID := hex.EncodeToString(id)
	expires := time.Now().Add(s.cfg.challengeTTL)

	s.mu.Lock()
	s.expireLocked()
	s.challenges[challengeID] = challenge{address: req.Address, nonce: nonce, expires: expires}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, response{
		Status:             statusOK,
		ChallengeID:        challengeID,
		EphemeralPublicKey: base64.StdEncoding.EncodeToString(block.EphemeralPublicKey),
		WrappedNonce:       base64.StdEncoding.EncodeToString(block.WrappedKey),
		KeyID:              base64.StdEncoding.EncodeToString(block.KeyId),
		ExpiresAt:          expires.UTC().Format(time.RFC3339),
	})
}

// expireLocked drops challenges past their deadline.
//
// Swept on issue rather than by a timer, because the map is the only thing that
// grows without bound here and the moment it grows is the moment to prune it. A
// challenge that outlived its deadline is also an answer somebody could have
// been sitting on.
func (s *store) expireLocked() {
	now := time.Now()
	for id, c := range s.challenges {
		if now.After(c.expires) {
			delete(s.challenges, id)
		}
	}
}

// prove consumes a challenge answer and returns the address it authenticates.
//
// One use per challenge. A reusable answer is a bearer token with a two-minute
// name on it: anything that observed one reply could replay it for as long as
// the challenge lived, which is exactly the credential the key agreement was
// supposed to avoid handing out.
func (s *store) prove(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	const prefix = "Yamale "
	if !strings.HasPrefix(header, prefix) {
		return "", errors.New("no challenge answer presented")
	}
	id, answer, ok := strings.Cut(strings.TrimPrefix(header, prefix), ":")
	if !ok {
		return "", errors.New("challenge answer must be <challenge-id>:<base64 nonce>")
	}
	given, err := base64.StdEncoding.DecodeString(answer)
	if err != nil {
		return "", errors.New("challenge answer is not base64")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	c, found := s.challenges[id]
	if !found {
		return "", errors.New("no such challenge, or it has already been answered")
	}
	delete(s.challenges, id)

	if time.Now().After(c.expires) {
		return "", errors.New("that challenge has expired")
	}
	// Constant time, because a comparison that returns early tells an attacker
	// how many leading bytes they guessed, and the whole nonce is only 32.
	if subtle.ConstantTimeCompare(given, c.nonce) != 1 {
		return "", errors.New("that is not the answer to this challenge")
	}
	return c.address, nil
}

// payload routes the three things that can be done to one stored envelope.
func (s *store) payload(w http.ResponseWriter, r *http.Request) {
	participant, endToEndID, ok := parsePayloadPath(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusBadRequest, response{
			Status: statusError,
			Detail: "path must be /payloads/{instructing-participant}/{end-to-end-id}",
		})
		return
	}
	// A store serves the payments of one participant. Serving another's would
	// mean resolving entitlement against records this operator has no
	// relationship with, and would let any participant's store be used as a
	// front for any other's.
	if participant != s.cfg.participant {
		writeJSON(w, http.StatusNotFound, response{
			Status: statusUnavailable,
			Reason: reasonNoSuchPayment,
			Detail: fmt.Sprintf("this store serves payments instructed by %s", s.cfg.participant),
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.get(w, r, participant, endToEndID)
	case http.MethodPut:
		s.put(w, r, participant, endToEndID)
	case http.MethodDelete:
		s.erase(w, r, participant, endToEndID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, response{Status: statusError, Detail: "GET, PUT or DELETE"})
	}
}

// parsePayloadPath splits /payloads/{participant}/{end-to-end-id}.
func parsePayloadPath(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, "/payloads/")
	if rest == path {
		return "", "", false
	}
	participant, endToEndID, ok := strings.Cut(rest, "/")
	if !ok || participant == "" || endToEndID == "" || strings.Contains(endToEndID, "/") {
		return "", "", false
	}
	p, err := unescape(participant)
	if err != nil {
		return "", "", false
	}
	e, err := unescape(endToEndID)
	if err != nil {
		return "", "", false
	}
	return p, e, true
}

// get serves one envelope to a party the chain says is entitled to it.
func (s *store) get(w http.ResponseWriter, r *http.Request, participant, endToEndID string) {
	caller, err := s.prove(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, response{
			Status: statusDenied,
			Reason: reasonUnproven,
			Detail: err.Error() + ". POST /challenge with your address, decrypt the nonce with your viewing key, and present it as `Authorization: Yamale <challenge-id>:<base64 nonce>`.",
		})
		return
	}

	record, err := s.chain.paymentRecord(participant, endToEndID)
	if err != nil {
		// The chain has no such payment, so there is nothing this store could
		// legitimately be holding for it. Reported as unavailable rather than
		// as an error, because the caller's next move is the same either way
		// and "the chain does not record this payment" is the useful half.
		writeJSON(w, http.StatusNotFound, response{
			Status: statusUnavailable,
			Reason: reasonNoSuchPayment,
			Detail: "the chain records no payment with that reference from this participant",
		})
		return
	}

	entitled, err := s.chain.entitled(caller, record)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, response{
			Status: statusError,
			Detail: "could not resolve entitlement against the chain: " + err.Error(),
		})
		return
	}
	if !entitled {
		// Deliberately the same answer whether or not the payload is here. A
		// store that said "denied" for a payment it holds and "unavailable" for
		// one it does not would let anyone with a viewing key enumerate which
		// payments this participant has detail for.
		writeJSON(w, http.StatusForbidden, response{
			Status: statusDenied,
			Reason: reasonNotEntitled,
			Detail: "the chain does not record you as a party to this payment, its settlement regulator, or a live auditor",
		})
		return
	}

	blob, err := os.ReadFile(s.envelopePath(participant, endToEndID))
	if err != nil {
		// The payment exists and the caller is entitled, so this is the erasure
		// path: the payload has been destroyed, or was never stored here. Both
		// are reported as unavailable with the reason attached, and a client
		// must render either as detail being unavailable — never as a payment
		// whose detail was empty, which is a different and false statement
		// about somebody's money.
		reason := reasonNeverStored
		if s.wasErased(participant, endToEndID) {
			reason = reasonErased
		}
		writeJSON(w, http.StatusNotFound, response{
			Status: statusUnavailable,
			Reason: reason,
			Detail: "the payment record on the chain is unaffected and still verifies; only the payload it commits to is gone",
		})
		return
	}

	writeJSON(w, http.StatusOK, response{
		Status:   statusOK,
		Envelope: base64.StdEncoding.EncodeToString(blob),
	})
}

// put stores an envelope, on the participant's own authority.
func (s *store) put(w http.ResponseWriter, r *http.Request, participant, endToEndID string) {
	if subtle.ConstantTimeCompare([]byte(bearer(r)), []byte(s.cfg.writeToken)) != 1 {
		writeJSON(w, http.StatusUnauthorized, response{
			Status: statusDenied,
			Detail: "storing a payload is the instructing participant's act",
		})
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Status: statusError, Detail: "envelope is too large or unreadable"})
		return
	}

	// Parsed before it is written. An envelope that does not decode is one no
	// reader will ever open, and the party who finds out is the payee months
	// later — by which time the plaintext that could have been re-sealed is
	// gone. Refusing it here costs the participant a retry while they still
	// hold the payload.
	var env types.PayloadEnvelope
	if err := env.Unmarshal(body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Status: statusError, Detail: "that is not a PayloadEnvelope"})
		return
	}
	if env.Version != types.EnvelopeVersion {
		writeJSON(w, http.StatusBadRequest, response{
			Status: statusError,
			Detail: fmt.Sprintf("envelope version %d, this store writes and reads version %d", env.Version, types.EnvelopeVersion),
		})
		return
	}
	if len(env.Recipients) == 0 {
		writeJSON(w, http.StatusBadRequest, response{
			Status: statusError,
			Detail: "this envelope has no recipient blocks, so nobody could ever open it",
		})
		return
	}

	path := s.envelopePath(participant, endToEndID)
	// Written to a temporary file and renamed, so a reader never sees a partial
	// envelope. A truncated envelope fails to open exactly like a tampered one,
	// and telling those apart afterwards is impossible.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Status: statusError, Detail: "could not store the envelope"})
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		writeJSON(w, http.StatusInternalServerError, response{Status: statusError, Detail: "could not store the envelope"})
		return
	}
	// A payload stored again after an erasure is a new payload, not the
	// restoration of a deleted one, so the tombstone goes. Leaving it would
	// have the store report an envelope it is holding as erased.
	_ = os.Remove(s.tombstonePath(participant, endToEndID))

	log.Printf("payloadstore: stored the envelope for %s/%s", participant, endToEndID)
	writeJSON(w, http.StatusOK, response{Status: statusOK})
}

// erase destroys one payload and leaves a tombstone.
//
// This is the act the whole design exists to make possible. The chain's record
// is untouched and still verifies — the hash, the amount, the parties and the
// height are all exactly as they were, and reconciliation over them is
// unaffected. What is gone is the only copy of the salted preimage, and with it
// any route from the hash back to what the payment said. That is erasure under
// the NDPA, Ghana's DPA, POPIA and the GDPR, and it is not available for
// anything ever written into the ledger itself.
//
// The tombstone is kept deliberately. It records that a payload was destroyed
// and when, and it contains no part of the payload. Without it the store cannot
// tell a payment whose detail was erased from one whose detail was never
// stored, and the two are entitled to different answers: the first is an
// exercised right, the second is an operational gap somebody should chase.
func (s *store) erase(w http.ResponseWriter, r *http.Request, participant, endToEndID string) {
	if subtle.ConstantTimeCompare([]byte(bearer(r)), []byte(s.cfg.writeToken)) != 1 {
		writeJSON(w, http.StatusUnauthorized, response{
			Status: statusDenied,
			Detail: "erasing a payload is the instructing participant's act, as the controller that received the request",
		})
		return
	}

	err := os.Remove(s.envelopePath(participant, endToEndID))
	switch {
	case err == nil, errors.Is(err, os.ErrNotExist):
		// Idempotent. An erasure request resubmitted after a timeout must not
		// fail: an operator answering a data subject needs to know the payload
		// is gone, not whether this particular call was the one that removed it.
	default:
		writeJSON(w, http.StatusInternalServerError, response{Status: statusError, Detail: "could not erase the payload"})
		return
	}

	stamp := time.Now().UTC().Format(time.RFC3339)
	if writeErr := os.WriteFile(s.tombstonePath(participant, endToEndID), []byte(stamp+"\n"), 0o600); writeErr != nil {
		log.Printf("payloadstore: erased %s/%s but could not record the tombstone: %v", participant, endToEndID, writeErr)
	}

	log.Printf("payloadstore: erased the payload for %s/%s", participant, endToEndID)
	writeJSON(w, http.StatusOK, response{
		Status: statusOK,
		Reason: reasonErased,
		Detail: "the payload is destroyed; the chain's record of the payment is unchanged and still verifies",
	})
}

func (s *store) tombstonePath(participant, endToEndID string) string {
	return s.envelopePath(participant, endToEndID) + ".erased"
}

func (s *store) wasErased(participant, endToEndID string) bool {
	_, err := os.Stat(s.tombstonePath(participant, endToEndID))
	return err == nil
}
