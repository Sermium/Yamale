package main

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"yamale/blockchain/x/paymsg/types"
)

const (
	testParticipant = "yml1bankone"
	testE2E         = "E2E-STORE-1"
	testToken       = "write-token-for-tests"
	testCountry     = "NG"
)

// fakeChain stands in for the node's REST API.
//
// A fake rather than a live chain because these tests are about the store's own
// rules — entitlement, proof of possession and erasure — and a test that needed
// a running node to check them would be run once and then skipped. The
// end-to-end path against a real chain is exercised separately.
type fakeChain struct {
	debtor, creditor string
	jurisdiction     string
	regulator        string
	auditors         []string
	keys             map[string][]viewingKey
}

type viewingKey struct {
	version   uint64
	publicKey []byte
	revoked   bool
}

func (f *fakeChain) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/yamale/blockchain/paymsg/v1/payment_record_by_id", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("instructing_participant") != testParticipant ||
			r.URL.Query().Get("end_to_end_id") != testE2E {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeTestJSON(w, map[string]any{"payment_record": map[string]any{
			"debtor":                  f.debtor,
			"creditor":                f.creditor,
			"settlement_jurisdiction": f.jurisdiction,
		}})
	})

	mux.HandleFunc("/yamale/blockchain/alias/v1/viewing_keys/", func(w http.ResponseWriter, r *http.Request) {
		addr := strings.TrimPrefix(r.URL.Path, "/yamale/blockchain/alias/v1/viewing_keys/")
		out := []map[string]any{}
		for _, k := range f.keys[addr] {
			revoked := "0"
			if k.revoked {
				revoked = "42"
			}
			out = append(out, map[string]any{
				"version":           fmt.Sprint(k.version),
				"public_key":        base64.StdEncoding.EncodeToString(k.publicKey),
				"revoked_at_height": revoked,
			})
		}
		writeTestJSON(w, map[string]any{"keys": out})
	})

	mux.HandleFunc("/yamale/blockchain/alias/v1/regulator/", func(w http.ResponseWriter, r *http.Request) {
		country := strings.TrimPrefix(r.URL.Path, "/yamale/blockchain/alias/v1/regulator/")
		if country != testCountry || f.regulator == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeTestJSON(w, map[string]any{"appointment": map[string]any{"address": f.regulator}})
	})

	mux.HandleFunc("/yamale/blockchain/alias/v1/auditors", func(w http.ResponseWriter, _ *http.Request) {
		out := []map[string]any{}
		for _, a := range f.auditors {
			out = append(out, map[string]any{"grant": map[string]any{"address": a}})
		}
		writeTestJSON(w, map[string]any{"auditors": out})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeTestJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

type party struct {
	address string
	priv    *ecdh.PrivateKey
}

func newParty(t *testing.T, address string) party {
	t.Helper()
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return party{address: address, priv: priv}
}

// harness wires a store to a fake chain and returns everything a test needs.
type harness struct {
	t        *testing.T
	store    *store
	mux      *http.ServeMux
	chain    *fakeChain
	payer    party
	payee    party
	reg      party
	stranger party
	payload  types.PaymentMetadata
	envelope []byte
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	payer := newParty(t, "yml1payer")
	payee := newParty(t, "yml1payee")
	reg := newParty(t, "yml1regulator")
	stranger := newParty(t, "yml1stranger")

	chain := &fakeChain{
		debtor:       payer.address,
		creditor:     payee.address,
		jurisdiction: testCountry,
		regulator:    reg.address,
		keys:         map[string][]viewingKey{},
	}
	for _, p := range []party{payer, payee, reg, stranger} {
		chain.keys[p.address] = []viewingKey{{version: 1, publicKey: p.priv.PublicKey().Bytes()}}
	}

	srv := chain.server(t)
	s, err := newStore(config{
		participant:  testParticipant,
		rest:         srv.URL,
		dir:          t.TempDir(),
		writeToken:   testToken,
		challengeTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/challenge", s.challenge)
	mux.HandleFunc("/payloads/", s.payload)

	payload, err := types.NewPaymentMetadata("SALA", "March salary, employee 4417")
	if err != nil {
		t.Fatal(err)
	}
	env, err := types.SealPayload(payload, []types.Recipient{
		{PublicKey: payer.priv.PublicKey().Bytes()},
		{PublicKey: payee.priv.PublicKey().Bytes()},
		{PublicKey: reg.priv.PublicKey().Bytes()},
	}, types.PaymentAAD(testParticipant, testE2E))
	if err != nil {
		t.Fatal(err)
	}
	blob, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	return &harness{
		t: t, store: s, mux: mux, chain: chain,
		payer: payer, payee: payee, reg: reg, stranger: stranger,
		payload: payload, envelope: blob,
	}
}

func (h *harness) do(req *http.Request) (int, response) {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	var body response
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		h.t.Fatalf("response was not JSON: %v", err)
	}
	return rec.Code, body
}

func (h *harness) put() (int, response) {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodPut, payloadURL(testE2E), bytes.NewReader(h.envelope))
	req.Header.Set("Authorization", "Bearer "+testToken)
	return h.do(req)
}

func payloadURL(endToEndID string) string {
	return "/payloads/" + testParticipant + "/" + endToEndID
}

// authorise runs the full challenge-response for one party and returns the
// header that proves possession.
func (h *harness) authorise(p party) string {
	h.t.Helper()

	body, _ := json.Marshal(map[string]string{"address": p.address})
	code, issued := h.do(httptest.NewRequest(http.MethodPost, "/challenge", bytes.NewReader(body)))
	if code != http.StatusOK {
		h.t.Fatalf("challenge for %s: %d %s", p.address, code, issued.Detail)
	}

	ephemeral, err := base64.StdEncoding.DecodeString(issued.EphemeralPublicKey)
	if err != nil {
		h.t.Fatal(err)
	}
	wrapped, err := base64.StdEncoding.DecodeString(issued.WrappedNonce)
	if err != nil {
		h.t.Fatal(err)
	}
	keyID, err := base64.StdEncoding.DecodeString(issued.KeyID)
	if err != nil {
		h.t.Fatal(err)
	}

	nonce, err := types.UnwrapFrom(&types.RecipientBlock{
		KeyId:              keyID,
		EphemeralPublicKey: ephemeral,
		WrappedKey:         wrapped,
	}, p.priv)
	if err != nil {
		h.t.Fatalf("%s could not answer its own challenge: %v", p.address, err)
	}
	return "Yamale " + issued.ChallengeID + ":" + base64.StdEncoding.EncodeToString(nonce)
}

func (h *harness) fetch(p party, endToEndID string) (int, response) {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, payloadURL(endToEndID), nil)
	req.Header.Set("Authorization", h.authorise(p))
	return h.do(req)
}

// openFetched decrypts a served envelope with one party's key.
func (h *harness) openFetched(body response, p party) types.PaymentMetadata {
	h.t.Helper()
	blob, err := base64.StdEncoding.DecodeString(body.Envelope)
	if err != nil {
		h.t.Fatal(err)
	}
	var env types.PayloadEnvelope
	if err := env.Unmarshal(blob); err != nil {
		h.t.Fatal(err)
	}
	got, err := types.OpenPayload(env, p.priv, types.PaymentAAD(testParticipant, testE2E))
	if err != nil {
		h.t.Fatalf("%s could not open the served envelope: %v", p.address, err)
	}
	return got
}

// The three entitled parties fetch and decrypt; the fourth is refused. The
// store never sees a private key and never sees the plaintext.
func TestEntitledPartiesFetchAndDecryptAndAStrangerCannot(t *testing.T) {
	h := newHarness(t)
	if code, body := h.put(); code != http.StatusOK {
		t.Fatalf("storing the envelope: %d %s", code, body.Detail)
	}

	for _, p := range []party{h.payer, h.payee, h.reg} {
		code, body := h.fetch(p, testE2E)
		if code != http.StatusOK {
			t.Fatalf("%s was refused: %d %s %s", p.address, code, body.Reason, body.Detail)
		}
		got := h.openFetched(body, p)
		if got.RemittanceInformation != h.payload.RemittanceInformation {
			t.Fatalf("%s decrypted %q", p.address, got.RemittanceInformation)
		}
	}

	// The stranger holds a registered viewing key, so it can prove who it is.
	// What it cannot do is show the chain records it as a party. Proof of
	// possession and entitlement are separate checks, and this is the case that
	// distinguishes them.
	code, body := h.fetch(h.stranger, testE2E)
	if code != http.StatusForbidden || body.Reason != reasonNotEntitled {
		t.Fatalf("the stranger got %d %s, expected 403 %s", code, body.Reason, reasonNotEntitled)
	}
	if body.Envelope != "" {
		t.Fatal("the refusal carried an envelope")
	}
}

// Erasure is the whole point. The payload is destroyed, the chain's record is
// untouched and still verifies, and the payment reconciles exactly as before.
func TestErasureDestroysThePayloadAndLeavesTheChainRecordValid(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.put(); code != http.StatusOK {
		t.Fatal("could not store the envelope")
	}

	// What the chain recorded, captured before the erasure so the assertion
	// afterwards is against the real value rather than a recomputed one.
	recorded, err := h.payload.Hash()
	if err != nil {
		t.Fatal(err)
	}

	// It reads before.
	code, body := h.fetch(h.payee, testE2E)
	if code != http.StatusOK {
		t.Fatalf("the payee could not read before the erasure: %d", code)
	}
	before := h.openFetched(body, h.payee)
	if err := types.VerifyMetadata(before, recorded); err != nil {
		t.Fatalf("the payload did not verify before the erasure: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, payloadURL(testE2E), nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	if code, erased := h.do(req); code != http.StatusOK || erased.Reason != reasonErased {
		t.Fatalf("erasure returned %d %s", code, erased.Reason)
	}

	// It does not read after, and the answer says why. "Erased" rather than a
	// bare not-found, because a client has to render an exercised erasure as
	// detail being unavailable and must never render it as a payment with no
	// detail.
	code, after := h.fetch(h.payee, testE2E)
	if code != http.StatusNotFound {
		t.Fatalf("the payload was still served after erasure: %d", code)
	}
	if after.Status != statusUnavailable || after.Reason != reasonErased {
		t.Fatalf("erased payload reported as %s/%s", after.Status, after.Reason)
	}
	if after.Envelope != "" {
		t.Fatal("the erasure answer carried an envelope")
	}

	// And the chain's side of it is untouched: the hash is still a 32-byte
	// SHA-256 that the payload verified against a moment ago. Nothing about the
	// ledger changed, which is exactly what makes this erasure legal rather
	// than a rewrite of history.
	if len(recorded) != types.MetadataHashLength {
		t.Fatalf("the recorded hash is %d bytes", len(recorded))
	}
	if err := types.VerifyMetadata(before, recorded); err != nil {
		t.Fatalf("the chain's record stopped verifying because a payload was deleted: %v", err)
	}

	// The regulator, who was entitled and never fetched, gets the same honest
	// answer rather than a permission error that would send them to the wrong
	// place.
	code, forRegulator := h.fetch(h.reg, testE2E)
	if code != http.StatusNotFound || forRegulator.Reason != reasonErased {
		t.Fatalf("the regulator was told %d %s", code, forRegulator.Reason)
	}
}

// A payload that was never stored is a different fact from one that was erased,
// and the store must say which. One is an exercised right; the other is an
// operational gap somebody should chase.
func TestNeverStoredIsDistinguishedFromErased(t *testing.T) {
	h := newHarness(t)

	code, body := h.fetch(h.payee, testE2E)
	if code != http.StatusNotFound {
		t.Fatalf("expected not found, got %d", code)
	}
	if body.Status != statusUnavailable || body.Reason != reasonNeverStored {
		t.Fatalf("a payload that was never stored reported as %s/%s", body.Status, body.Reason)
	}
}

// A payment the chain does not record cannot have a payload here, and saying so
// is more useful than a permission error — which would send the caller looking
// for an entitlement problem that does not exist.
func TestAPaymentTheChainDoesNotRecordIsReportedAsSuch(t *testing.T) {
	h := newHarness(t)

	code, body := h.fetch(h.payee, "E2E-NOT-ON-CHAIN")
	if code != http.StatusNotFound || body.Reason != reasonNoSuchPayment {
		t.Fatalf("got %d %s", code, body.Reason)
	}
}

// The challenge is the only credential, and it is single use. A replayed answer
// is a bearer token with a two-minute expiry on it.
func TestAChallengeAnswerCannotBeReplayed(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.put(); code != http.StatusOK {
		t.Fatal("could not store the envelope")
	}

	header := h.authorise(h.payee)

	req := httptest.NewRequest(http.MethodGet, payloadURL(testE2E), nil)
	req.Header.Set("Authorization", header)
	if code, _ := h.do(req); code != http.StatusOK {
		t.Fatalf("the first use of a challenge answer failed: %d", code)
	}

	replay := httptest.NewRequest(http.MethodGet, payloadURL(testE2E), nil)
	replay.Header.Set("Authorization", header)
	code, body := h.do(replay)
	if code != http.StatusUnauthorized || body.Reason != reasonUnproven {
		t.Fatalf("a replayed challenge answer was accepted: %d %s", code, body.Reason)
	}
}

// Answering somebody else's challenge is the attack the key agreement exists to
// stop: the nonce is sealed to the named account's viewing key, so a caller who
// does not hold that private half cannot produce it.
func TestAPartyCannotAnswerAnotherPartysChallenge(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.put(); code != http.StatusOK {
		t.Fatal("could not store the envelope")
	}

	body, _ := json.Marshal(map[string]string{"address": h.payee.address})
	_, issued := h.do(httptest.NewRequest(http.MethodPost, "/challenge", bytes.NewReader(body)))

	ephemeral, _ := base64.StdEncoding.DecodeString(issued.EphemeralPublicKey)
	wrapped, _ := base64.StdEncoding.DecodeString(issued.WrappedNonce)
	keyID, _ := base64.StdEncoding.DecodeString(issued.KeyID)

	if _, err := types.UnwrapFrom(&types.RecipientBlock{
		KeyId:              keyID,
		EphemeralPublicKey: ephemeral,
		WrappedKey:         wrapped,
	}, h.stranger.priv); err == nil {
		t.Fatal("a stranger opened a challenge sealed to the payee's viewing key")
	}
}

// Retrieval without any proof at all is refused, and the refusal explains how
// to obtain one. An unauthenticated read would make every other check here
// decoration.
func TestRetrievalWithoutProofIsRefused(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.put(); code != http.StatusOK {
		t.Fatal("could not store the envelope")
	}

	code, body := h.do(httptest.NewRequest(http.MethodGet, payloadURL(testE2E), nil))
	if code != http.StatusUnauthorized || body.Reason != reasonUnproven {
		t.Fatalf("an unauthenticated read returned %d %s", code, body.Reason)
	}
	if body.Envelope != "" {
		t.Fatal("an unauthenticated read was served an envelope")
	}
}

// Storing and erasing are the participant's acts, not anybody's who can reach
// the port.
func TestWritesAndErasuresRequireTheWriteToken(t *testing.T) {
	h := newHarness(t)

	unauthorised := httptest.NewRequest(http.MethodPut, payloadURL(testE2E), bytes.NewReader(h.envelope))
	if code, _ := h.do(unauthorised); code != http.StatusUnauthorized {
		t.Fatalf("an unauthenticated write returned %d", code)
	}

	if code, _ := h.put(); code != http.StatusOK {
		t.Fatal("could not store the envelope")
	}

	wrongToken := httptest.NewRequest(http.MethodDelete, payloadURL(testE2E), nil)
	wrongToken.Header.Set("Authorization", "Bearer not-the-token")
	if code, _ := h.do(wrongToken); code != http.StatusUnauthorized {
		t.Fatalf("an erasure with the wrong token returned %d", code)
	}

	// And the payload is still there, so the refusal was a refusal rather than
	// a deletion that reported an error afterwards.
	if code, _ := h.fetch(h.payee, testE2E); code != http.StatusOK {
		t.Fatal("the payload went missing after a refused erasure")
	}
}

// A revoked key must not be handed a challenge. Sealing one to it would give
// the puzzle to whoever the revocation was declared because of, and the puzzle
// is the gate on every payload that account can read.
func TestARevokedKeyIsNotIssuedAChallenge(t *testing.T) {
	h := newHarness(t)
	h.chain.keys[h.payee.address] = []viewingKey{{
		version: 1, publicKey: h.payee.priv.PublicKey().Bytes(), revoked: true,
	}}

	body, _ := json.Marshal(map[string]string{"address": h.payee.address})
	code, issued := h.do(httptest.NewRequest(http.MethodPost, "/challenge", bytes.NewReader(body)))
	if code != http.StatusNotFound || issued.Reason != reasonUnproven {
		t.Fatalf("a revoked key was issued a challenge: %d %s", code, issued.Reason)
	}
}

// After a rotation the challenge goes to the newest live key, and the holder of
// the superseded one can no longer authenticate. The old key still opens the
// old envelopes — that is the design — but it is no longer who this account is.
func TestAChallengeIsSealedToTheNewestLiveKey(t *testing.T) {
	h := newHarness(t)
	rotated := newParty(t, h.payee.address)

	// Deliberately out of order, so a store that took the first entry rather
	// than the highest version would fail here.
	h.chain.keys[h.payee.address] = []viewingKey{
		{version: 1, publicKey: h.payee.priv.PublicKey().Bytes()},
		{version: 2, publicKey: rotated.priv.PublicKey().Bytes()},
	}

	body, _ := json.Marshal(map[string]string{"address": h.payee.address})
	code, issued := h.do(httptest.NewRequest(http.MethodPost, "/challenge", bytes.NewReader(body)))
	if code != http.StatusOK {
		t.Fatalf("challenge refused: %d", code)
	}

	ephemeral, _ := base64.StdEncoding.DecodeString(issued.EphemeralPublicKey)
	wrapped, _ := base64.StdEncoding.DecodeString(issued.WrappedNonce)
	keyID, _ := base64.StdEncoding.DecodeString(issued.KeyID)
	block := &types.RecipientBlock{KeyId: keyID, EphemeralPublicKey: ephemeral, WrappedKey: wrapped}

	if _, err := types.UnwrapFrom(block, rotated.priv); err != nil {
		t.Fatalf("the current key could not answer its own challenge: %v", err)
	}
	if _, err := types.UnwrapFrom(block, h.payee.priv); err == nil {
		t.Fatal("the superseded key answered a challenge issued after the rotation")
	}
}

// An envelope nobody could open must not be accepted, because the party who
// would discover it is the payee, long after the plaintext that could have been
// re-sealed is gone.
func TestTheStoreRefusesAnEnvelopeNobodyCouldOpen(t *testing.T) {
	h := newHarness(t)

	empty, err := (&types.PayloadEnvelope{Version: types.EnvelopeVersion}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, payloadURL(testE2E), bytes.NewReader(empty))
	req.Header.Set("Authorization", "Bearer "+testToken)
	if code, body := h.do(req); code != http.StatusBadRequest {
		t.Fatalf("an envelope with no recipients was accepted: %d %s", code, body.Detail)
	}

	notAnEnvelope := httptest.NewRequest(http.MethodPut, payloadURL(testE2E),
		bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff}))
	notAnEnvelope.Header.Set("Authorization", "Bearer "+testToken)
	if code, _ := h.do(notAnEnvelope); code != http.StatusBadRequest {
		t.Fatalf("bytes that are not an envelope were accepted: %d", code)
	}
}

// A live auditor reads across accounts it has no relationship with. That is the
// role's whole purpose and the reason it is capped and time-boxed on-chain.
func TestALiveAuditorMayRead(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.put(); code != http.StatusOK {
		t.Fatal("could not store the envelope")
	}

	// Not entitled before the grant.
	if code, _ := h.fetch(h.stranger, testE2E); code != http.StatusForbidden {
		t.Fatal("an account with no grant was served")
	}

	h.chain.auditors = []string{h.stranger.address}
	code, body := h.fetch(h.stranger, testE2E)
	if code != http.StatusOK {
		t.Fatalf("a live auditor was refused: %d %s", code, body.Detail)
	}

	// It can fetch, and it still cannot read: this envelope was sealed before
	// the grant existed, so no block is addressed to it. Entitlement and
	// decryptability are separate, and conflating them is how a design ends up
	// claiming a grant retroactively opens history.
	blob, err := base64.StdEncoding.DecodeString(body.Envelope)
	if err != nil {
		t.Fatal(err)
	}
	var env types.PayloadEnvelope
	if err := env.Unmarshal(blob); err != nil {
		t.Fatal(err)
	}
	if _, err := types.OpenPayload(env, h.stranger.priv, types.PaymentAAD(testParticipant, testE2E)); err == nil {
		t.Fatal("an auditor granted after the payment opened an envelope it was never sealed to")
	}
}

// This store serves one participant's payments. Answering for another's would
// resolve entitlement against records this operator has no relationship with.
func TestAStoreDoesNotServeAnotherParticipantsPayments(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/payloads/yml1banktwo/"+testE2E, nil)
	code, body := h.do(req)
	if code != http.StatusNotFound || body.Reason != reasonNoSuchPayment {
		t.Fatalf("got %d %s", code, body.Reason)
	}
}
