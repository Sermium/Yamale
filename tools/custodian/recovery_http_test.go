package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/mpc"
)

// The recovery endpoints end to end, including the part that is easy to build
// and forget to connect: the post-recovery freeze has to stop a SIGNATURE, not
// merely appear in a JSON field.

type recoveryFixture struct {
	t        *testing.T
	srv      *httptest.Server
	server   *server
	notifier *fakeNotifier
	clock    *time.Time
}

func newRecoveryFixture(t *testing.T) *recoveryFixture {
	t.Helper()
	store, _ := testStore(t)
	index, err := NewBlindIndex(strings.Repeat("p", 32))
	require.NoError(t, err)

	recoveries, err := NewRecoveries(t.TempDir())
	require.NoError(t, err)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock := &now
	recoveries.now = func() time.Time { return *clock }

	notifier := &fakeNotifier{}
	s := &server{
		store: store, sessions: NewSessions(), index: index,
		role: mpc.RoleCustodian, enrolments: NewEnrolments(),
		recoveries: recoveries, notifier: notifier,
		// Above zero deliberately. Zero means "a second factor is always
		// required", which refuses every signature with 403 — the same status
		// the post-recovery freeze uses, so this test would have passed while
		// proving nothing about the freeze.
		secondFactorAbove: 1_000_000,
		started:           time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/recovery/initiate", s.handleRecoveryInitiate)
	mux.HandleFunc("POST /v1/recovery/approve", s.handleRecoveryApprove)
	mux.HandleFunc("POST /v1/recovery/complete", s.handleRecoveryComplete)
	mux.HandleFunc("POST /v1/recovery/cancel", s.handleRecoveryCancel)
	mux.HandleFunc("GET /v1/recovery/statistics", s.handleRecoveryStatistics)
	mux.HandleFunc("POST /v1/sign/start", s.handleStart)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &recoveryFixture{t: t, srv: srv, server: s, notifier: notifier, clock: clock}
}

func (f *recoveryFixture) enrolAccount(t *testing.T, email, password string) {
	t.Helper()
	verifier, err := NewVerifier(password)
	require.NoError(t, err)
	require.NoError(t, f.server.store.Put(Account{
		Index:    f.server.index.Of(email),
		Address:  "yml1testaccount",
		Password: verifier,
		Created:  nowUTC(),
	}, custodianShare(t)))
}

// postWithBody is post, keeping the response body so a refusal can be checked
// by its reason rather than only by its status.
func (f *recoveryFixture) postWithBody(t *testing.T, path string, body any) (int, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(f.srv.URL+path, "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	defer resp.Body.Close()
	out := new(bytes.Buffer)
	_, err = out.ReadFrom(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, out.String()
}

func (f *recoveryFixture) post(t *testing.T, path string, body any, into any) int {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(f.srv.URL+path, "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	defer resp.Body.Close()
	if into != nil && resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(into))
	}
	return resp.StatusCode
}

// TestRecoveryOverHTTPHoldsPaymentsAfterwards is the wiring test: every rule
// enforced through the endpoints, and the freeze actually refusing a signature.
func TestRecoveryOverHTTPHoldsPaymentsAfterwards(t *testing.T) {
	const email = "amara@example.org"
	const password = "correct horse battery staple"
	f := newRecoveryFixture(t)
	f.enrolAccount(t, email, password)

	// Signing works before any of this.
	var start map[string]any
	digest := make([]byte, 32)
	code := f.post(t, "/v1/sign/start", map[string]any{
		"email": email, "password": password,
		"digest": base64Of(digest), "amount": 0,
	}, &start)
	require.NotEqual(t, http.StatusForbidden, code,
		"the account was already refusing signatures before a recovery")

	var rec recoveryView
	code = f.post(t, "/v1/recovery/initiate", recoveryInitiateRequest{
		Email: email, Operator: "amina", Team: "support", Proof: "in person at an agent",
	}, &rec)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, string(RecoveryPending), rec.State)
	require.Equal(t, 1, f.notifier.started, "the account holder was not notified")
	require.Contains(t, rec.Blocking, "notice period has not passed")

	// An unknown email must not reveal itself as unknown by succeeding here.
	code = f.post(t, "/v1/recovery/initiate", recoveryInitiateRequest{
		Email: "nobody@example.org", Operator: "amina", Team: "support", Proof: "x",
	}, nil)
	require.Equal(t, http.StatusNotFound, code)

	// The rules, through the endpoints.
	code = f.post(t, "/v1/recovery/approve",
		recoveryApproveRequest{ID: rec.ID, Operator: "amina", Team: "support"}, nil)
	require.Equal(t, http.StatusConflict, code, "the initiator approved their own recovery")

	code = f.post(t, "/v1/recovery/approve",
		recoveryApproveRequest{ID: rec.ID, Operator: "boubacar", Team: "support"}, nil)
	require.Equal(t, http.StatusConflict, code, "two approvals came from one team")

	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/approve",
		recoveryApproveRequest{ID: rec.ID, Operator: "boubacar", Team: "risk"}, nil))
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/approve",
		recoveryApproveRequest{ID: rec.ID, Operator: "chinwe", Team: "compliance"}, nil))

	// Approved, and still too early.
	code = f.post(t, "/v1/recovery/complete", recoveryIDRequest{ID: rec.ID}, nil)
	require.Equal(t, http.StatusConflict, code, "a recovery completed before its delay")

	*f.clock = f.clock.Add(RecoveryDelay + time.Minute)
	var done recoveryView
	require.Equal(t, http.StatusOK,
		f.post(t, "/v1/recovery/complete", recoveryIDRequest{ID: rec.ID}, &done))
	require.Equal(t, string(RecoveryCompleted), done.State)
	require.NotNil(t, done.FrozenUntil)
	require.Equal(t, 1, f.notifier.completed, "no notice was sent on completion")

	// THE POINT. Access is restored and money still cannot leave — and the
	// refusal is checked by its reason, not only its status, because 403 is
	// also what a missing second factor and a frozen account answer with.
	code, reason := f.postWithBody(t, "/v1/sign/start", map[string]any{
		"email": email, "password": password,
		"digest": base64Of(digest), "amount": 0,
	})
	require.Equal(t, http.StatusForbidden, code,
		"an account signed immediately after being recovered")
	require.Contains(t, reason, "recovered recently")

	// And the hold ends on its own.
	*f.clock = f.clock.Add(PostRecoveryFreeze + time.Minute)
	code = f.post(t, "/v1/sign/start", map[string]any{
		"email": email, "password": password,
		"digest": base64Of(digest), "amount": 0,
	}, nil)
	require.NotEqual(t, http.StatusForbidden, code, "the hold outlasted its window")
}

// TestStatisticsExposeNoIdentity: the endpoint is meant to be publishable.
func TestStatisticsExposeNoIdentity(t *testing.T) {
	const email = "bakary@example.org"
	f := newRecoveryFixture(t)
	f.enrolAccount(t, email, "a long enough passphrase")

	var rec recoveryView
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/initiate", recoveryInitiateRequest{
		Email: email, Operator: "amina", Team: "support", Proof: "in person",
	}, &rec))

	resp, err := http.Get(f.srv.URL + "/v1/recovery/statistics")
	require.NoError(t, err)
	defer resp.Body.Close()
	body := new(bytes.Buffer)
	_, err = body.ReadFrom(resp.Body)
	require.NoError(t, err)

	require.NotContains(t, body.String(), email)
	require.NotContains(t, body.String(), f.server.index.Of(email))
	require.NotContains(t, body.String(), "amina")
	require.Contains(t, body.String(), "pending")

	// The per-recovery view is for operators and still carries no identity.
	raw, err := json.Marshal(rec)
	require.NoError(t, err)
	require.NotContains(t, string(raw), email)
	require.NotContains(t, string(raw), f.server.index.Of(email))
}

// base64Of is the encoding the sign endpoint expects for a digest.
func base64Of(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
