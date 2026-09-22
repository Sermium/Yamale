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

	"yamale/blockchain/recovery"
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
		store: store, index: index,
		role:       recovery.RoleCustodian,
		recoveries: recoveries, notifier: notifier,
		started: time.Now(),
		// The SAME clock the recoveries store stamps EligibleAt with. Without
		// this the handlers read wall time, and every assertion about the
		// 72-hour notice period silently stopped holding once real time passed
		// the fixture's fixed date plus 72 hours.
		now: func() time.Time { return *clock },
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/recovery/initiate", s.handleRecoveryInitiate)
	mux.HandleFunc("POST /v1/recovery/approve", s.handleRecoveryApprove)
	mux.HandleFunc("POST /v1/recovery/complete", s.handleRecoveryComplete)
	mux.HandleFunc("POST /v1/recovery/cancel", s.handleRecoveryCancel)
	mux.HandleFunc("GET /v1/recovery/statistics", s.handleRecoveryStatistics)
	mux.HandleFunc("POST /v1/recovery/claim", s.handleRecoveryClaim)

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
	}, custodianHolding(t)))
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

// TestRecoveryOverHTTPWithholdsTheHoldingAfterwards is the wiring test: every
// rule enforced through the endpoints, and the waiting period actually
// withholding something.
//
// It used to assert that a recovered account could not SIGN for 24 hours, which
// was enforced by the custodian refusing to co-sign. Nothing co-signs now, so
// that property is not merely untested — it is gone, and no test could bring it
// back. What replaced it is the same 24 hours applied to the one thing this
// service still controls: when the holding is released. The test follows.
func TestRecoveryOverHTTPWithholdsTheHoldingAfterwards(t *testing.T) {
	const email = "amara@example.org"
	const password = "correct horse battery staple"
	f := newRecoveryFixture(t)
	f.enrolAccount(t, email, password)

	// Nothing is released before a recovery exists at all.
	code := f.post(t, "/v1/recovery/claim", claimRequest{
		ID: "no-such-recovery", Email: email, Password: password,
	}, nil)
	require.Equal(t, http.StatusNotFound, code)

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

	// A pending recovery releases nothing.
	code = f.post(t, "/v1/recovery/claim", claimRequest{
		ID: rec.ID, Email: email, Password: password,
	}, nil)
	require.Equal(t, http.StatusConflict, code, "a pending recovery released the holding")

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

	// THE POINT. Completed, and the holding still does not come out — checked
	// by its reason and not only its status, because a conflict is also what
	// several other refusals on this endpoint answer with.
	code, reason := f.postWithBody(t, "/v1/recovery/claim", claimRequest{
		ID: rec.ID, Email: email, Password: password,
	})
	require.Equal(t, http.StatusConflict, code,
		"the holding came out the moment the recovery completed")
	require.Contains(t, reason, "can still cancel it")

	// And the wrong password does not get it either, at any point.
	code = f.post(t, "/v1/recovery/claim", claimRequest{
		ID: rec.ID, Email: email, Password: "not the password at all",
	}, nil)
	require.Equal(t, http.StatusUnauthorized, code)

	// The hold ends on its own, and the holding is the custodian's.
	*f.clock = f.clock.Add(PostRecoveryFreeze + time.Minute)
	var claimed claimResponse
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/claim", claimRequest{
		ID: rec.ID, Email: email, Password: password,
	}, &claimed), "the hold outlasted its window")
	require.Equal(t, recovery.RoleCustodian, claimed.Holding.Role)
	require.NotEmpty(t, claimed.Holding.Share.Y, "an empty holding was released")
	require.Equal(t, "yml1testaccount", claimed.Address)
}

// The notice sent on completion asks the account holder to speak up if this was
// not them. This is the test that they can.
//
// Before the waiting period moved to the claim, completion was the end of the
// story and a cancellation afterwards was meaningless — access had already been
// restored. Now there are 24 hours in which the holding has not moved, so a
// cancellation in that window is the whole point of sending the notice.
func TestACompletedRecoveryCanStillBeCancelledWithinItsWindow(t *testing.T) {
	const email = "fatou@example.org"
	const password = "correct horse battery staple"
	f := newRecoveryFixture(t)
	f.enrolAccount(t, email, password)

	var rec recoveryView
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/initiate", recoveryInitiateRequest{
		Email: email, Operator: "amina", Team: "support", Proof: "in person",
	}, &rec))
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/approve",
		recoveryApproveRequest{ID: rec.ID, Operator: "boubacar", Team: "risk"}, nil))
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/approve",
		recoveryApproveRequest{ID: rec.ID, Operator: "chinwe", Team: "compliance"}, nil))

	*f.clock = f.clock.Add(RecoveryDelay + time.Minute)
	require.Equal(t, http.StatusOK,
		f.post(t, "/v1/recovery/complete", recoveryIDRequest{ID: rec.ID}, nil))

	// The real owner reads the notice and says it was not them.
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/cancel", recoveryIDRequest{
		ID: rec.ID, By: "the account holder", Reason: "I did not ask for this",
	}, nil))

	// And now nothing is ever released for it, even after the window.
	*f.clock = f.clock.Add(PostRecoveryFreeze + time.Minute)
	code := f.post(t, "/v1/recovery/claim", claimRequest{
		ID: rec.ID, Email: email, Password: password,
	}, nil)
	require.Equal(t, http.StatusConflict, code,
		"a cancelled recovery still released the holding")
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
