package main

// The hosted ceremony, driven over a real HTTP surface.
//
// These tests run five custodians through the actual handlers on a real loopback
// socket rather than calling the state machine directly, because the properties
// that matter are properties of the HTTP layer: what a request body can carry,
// which credential opens which route, and what the server refuses. A test that
// called the methods would be testing something else.
//
// The custodian side here derives with the Go code rather than the browser's.
// That is deliberate and it is not a gap: testdata/vectors/ceremony.json holds
// both sides to the same addresses, signatures and fingerprints, so this file
// can concentrate on the server and clients/ceremony/src/rehearsal.test.ts drives
// the same surface with the real TypeScript client.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	bip39 "github.com/cosmos/go-bip39"
	"github.com/stretchr/testify/require"
)

type hostHarness struct {
	t       *testing.T
	server  *httptest.Server
	session *hostSession
	// bodies is every request body that reached the server, kept so a test can
	// assert over all of them at once rather than one call at a time.
	bodies []recordedBody
}

type recordedBody struct {
	Path string
	Body string
}

const testCoordinatorToken = "coordinator-token-for-tests"

func newHostHarness(t *testing.T) *hostHarness {
	t.Helper()
	configureAddresses()

	bundle := &bundleInfo{
		Hash:    "0000000000000000000000000000000000000000000000000000000000000000",
		Files:   map[string][]byte{"index.html": []byte("<!doctype html><div id=app></div>"), "ceremony.js": []byte("// test bundle")},
		PerFile: map[string]string{},
	}
	h := &hostHarness{t: t}
	h.session = newHostSession(t.TempDir(), mustURL(t, "http://127.0.0.1:9/ceremony/"), bundle)

	handler := newHostServer(h.session, "/ceremony/", testCoordinatorToken, "")
	h.server = httptest.NewServer(h.record(handler))
	t.Cleanup(h.server.Close)
	return h
}

// record captures every body before the handler sees it.
//
// This is how "no phrase ever reaches the coordinator" is asserted rather than
// eyeballed. Every byte of every request is kept and TestHostNeverReceivesAny
// PhraseOrPrivateKey searches all of them for the five phrases, the five private
// keys and the BIP-39 shape, after a whole ceremony has run through.
func (h *hostHarness) record(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			raw, err := io.ReadAll(r.Body)
			require.NoError(h.t, err)
			h.bodies = append(h.bodies, recordedBody{Path: r.URL.Path, Body: string(raw)})
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		next.ServeHTTP(w, r)
	})
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}

func (h *hostHarness) do(t *testing.T, method, path, token string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, h.server.URL+path, reader)
	require.NoError(t, err)
	if token != "" {
		request.Header.Set("X-Ceremony-Token", token)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := h.server.Client().Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, raw
}

func (h *hostHarness) post(t *testing.T, path, token string, body any) (int, []byte) {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	return h.do(t, http.MethodPost, path, token, body)
}

func (h *hostHarness) get(t *testing.T, path, token string) (int, []byte) {
	t.Helper()
	return h.do(t, http.MethodGet, path, token, nil)
}

// setup runs the coordinator's first screen and returns the invite links.
func (h *hostHarness) setup(t *testing.T, roster []string) hostStateView {
	t.Helper()
	status, raw := h.post(t, "/ceremony/api/coordinator/setup", testCoordinatorToken, setupRequest{
		Ceremony:     "Yamale foundation, hosted rehearsal",
		ChainID:      "yamale-1",
		Threshold:    3,
		Custodians:   roster,
		PolicySeq:    1,
		VotingPeriod: "168h0m0s",
	})
	require.Equal(t, http.StatusOK, status, string(raw))
	var view hostStateView
	require.NoError(t, json.Unmarshal(raw, &view))
	require.Len(t, view.Custodians, len(roster))
	return view
}

// currentParams reads back the parameters the coordinator settled on, including
// the ceremony id it minted.
func (h *hostHarness) currentParams(t *testing.T) ceremonyParams {
	t.Helper()
	status, raw := h.get(t, "/ceremony/api/coordinator/state", testCoordinatorToken)
	require.Equal(t, http.StatusOK, status)
	var state hostStateView
	require.NoError(t, json.Unmarshal(raw, &state))
	require.NotEmpty(t, state.Params.ID)
	return state.Params
}

func (h *hostHarness) tokenFor(t *testing.T, view hostStateView, name string) string {
	t.Helper()
	for _, custodian := range view.Custodians {
		if custodian.Name != name {
			continue
		}
		parsed, err := url.Parse(custodian.Link)
		require.NoError(t, err)
		token := parsed.Query().Get("i")
		require.NotEmpty(t, token, "the invite link for %s carries no token", name)
		return token
	}
	t.Fatalf("no invite for %q", name)
	return ""
}

// custodianKey is one custodian's side of the ceremony, held only in the test.
type custodianKey struct {
	Name   string
	Phrase string
	Priv   *secp256k1.PrivKey
	ID     identity
}

func newCustodianKey(t *testing.T, name string, index int, at time.Time) custodianKey {
	t.Helper()
	phrase := fixturePhrase(index)
	secret, err := secretFromInput(phrase)
	require.NoError(t, err)
	priv, path, err := secret.derive(0)
	require.NoError(t, err)
	id, err := identityOf(name, roleCustodian, priv, path, at)
	require.NoError(t, err)
	return custodianKey{Name: name, Phrase: phrase, Priv: priv, ID: id}
}

var testRoster = []string{"A. Okafor", "Chipo & Sons <Trust>", "Naledi Ngugi", "Bank of Yamale", "J. Mwangi"}

// runWholeCeremony is the five-custodian rehearsal, over HTTP, end to end.
func (h *hostHarness) runWholeCeremony(t *testing.T) ([]custodianKey, hostStateView) {
	t.Helper()
	view := h.setup(t, testRoster)

	var params ceremonyParams
	status, raw := h.get(t, "/ceremony/api/coordinator/state", testCoordinatorToken)
	require.Equal(t, http.StatusOK, status)
	var state hostStateView
	require.NoError(t, json.Unmarshal(raw, &state))
	params = state.Params

	keys := make([]custodianKey, 0, len(testRoster))
	for i, name := range testRoster {
		token := h.tokenFor(t, view, name)
		status, raw := h.post(t, "/ceremony/api/invite/opened", token, nil)
		require.Equal(t, http.StatusOK, status, string(raw))
		status, raw = h.post(t, "/ceremony/api/invite/generated", token, nil)
		require.Equal(t, http.StatusOK, status, string(raw))

		key := newCustodianKey(t, name, i, time.Now())
		sub, err := signSubmission(params.ID, key.ID, key.Priv)
		require.NoError(t, err)
		status, raw = h.post(t, "/ceremony/api/invite/submission", token, sub)
		require.Equal(t, http.StatusOK, status, string(raw))
		keys = append(keys, key)
	}

	// Every instance computes the group. This is the test standing in for a
	// custodian's browser doing it.
	status, raw = h.get(t, "/ceremony/api/invite", h.tokenFor(t, view, testRoster[0]))
	require.Equal(t, http.StatusOK, status)
	var invited inviteView
	require.NoError(t, json.Unmarshal(raw, &invited))
	require.Len(t, invited.Submissions, len(testRoster))

	assembledHere, err := assembleGroup(params, invited.Submissions)
	require.NoError(t, err)

	for _, key := range keys {
		token := h.tokenFor(t, view, key.Name)
		require.NoError(t, assembledHere.presence(key.ID.Address, key.ID.Fingerprint))
		att := attestation{
			CeremonyID:            params.ID,
			Name:                  key.Name,
			Address:               key.ID.Address,
			GroupFingerprint:      assembledHere.Fingerprint,
			PolicyAddress:         assembledHere.PolicyAddress,
			TranscriptionVerified: true,
			RestoreDrillPassed:    true,
			EnvelopeSealed:        true,
			SignedAt:              time.Now().UTC().Format(time.RFC3339),
		}
		signed, err := signAttestation(att, key.Priv)
		require.NoError(t, err)
		status, raw := h.post(t, "/ceremony/api/invite/attestation", token, signed)
		require.Equal(t, http.StatusOK, status, string(raw))
	}

	status, raw = h.get(t, "/ceremony/api/coordinator/state", testCoordinatorToken)
	require.Equal(t, http.StatusOK, status)
	var final hostStateView
	require.NoError(t, json.Unmarshal(raw, &final))
	return keys, final
}

func TestHostedCeremonyRunsFiveCustodiansToAttestation(t *testing.T) {
	h := newHostHarness(t)
	keys, final := h.runWholeCeremony(t)

	require.True(t, final.Ready)
	require.Len(t, final.Attestations, len(testRoster))
	require.NotNil(t, final.Assembled)
	for _, custodian := range final.Custodians {
		require.Equal(t, phaseAttested, custodian.Phase, custodian.Name)
		require.True(t, custodian.Proved, "%s's row must rest on a signature, not on a self-report", custodian.Name)
	}
	for _, key := range keys {
		require.NoError(t, final.Assembled.presence(key.ID.Address, key.ID.Fingerprint))
	}
}

// TestHostNeverReceivesAnyPhraseOrPrivateKey is the property the whole command
// exists for, asserted over every byte of every request rather than reasoned
// about.
func TestHostNeverReceivesAnyPhraseOrPrivateKey(t *testing.T) {
	h := newHostHarness(t)
	keys, _ := h.runWholeCeremony(t)
	require.NotEmpty(t, h.bodies, "no request bodies were recorded, so this test would pass vacuously")

	for _, recorded := range h.bodies {
		lower := strings.ToLower(recorded.Body)
		for _, key := range keys {
			require.NotContains(t, lower, strings.ToLower(key.Phrase),
				"%s's recovery phrase appeared in a request to %s", key.Name, recorded.Path)
			require.NotContains(t, lower, strings.ToLower(fmt.Sprintf("%x", key.Priv.Key)),
				"%s's private key appeared in hex in a request to %s", key.Name, recorded.Path)
			require.NotContains(t, recorded.Body, base64Of(key.Priv.Key),
				"%s's private key appeared in base64 in a request to %s", key.Name, recorded.Path)
		}
		require.Empty(t, longestMnemonicRun(recorded.Body), "a run of BIP-39 words reached %s", recorded.Path)
		for _, forbidden := range []string{"mnemonic", "\"phrase\"", "private_key", "privkey", "entropy", "seed_hex"} {
			require.NotContains(t, lower, forbidden,
				"the field %q appeared in a request to %s", forbidden, recorded.Path)
		}
	}
}

// TestNoHostRouteAcceptsAPhrase drives every route with a phrase in the body.
//
// Iterating hostRoutes rather than a list written here, so a route added later is
// covered without anybody remembering to add it — a forgotten route would be
// precisely the one with the hole in it. The expectation is a refusal, not a
// silent ignore: DisallowUnknownFields means the coordinator cannot end up
// holding a phrase in memory or in an access log even if a modified page tried to
// send one.
func TestNoHostRouteAcceptsAPhrase(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)
	token := h.tokenFor(t, view, testRoster[0])

	phrase := fixturePhrase(0)
	tested := 0
	for _, route := range h.session.routes() {
		credential := testCoordinatorToken
		if route.Audience == audienceCustodian {
			credential = token
		}
		body := map[string]any{"phrase": phrase, "mnemonic": phrase}
		status, raw := h.post(t, "/ceremony/"+route.Path, credential, body)
		if status == http.StatusMethodNotAllowed {
			// A GET-only route cannot carry a body to a handler at all, which is
			// the same guarantee by a different mechanism.
			tested++
			continue
		}
		require.Equal(t, http.StatusBadRequest, status,
			"%s accepted a body containing a recovery phrase: %s", route.Path, raw)
		require.NotContains(t, string(raw), phrase,
			"%s echoed the phrase back in its error, which would put it in a browser's console", route.Path)
		tested++
	}
	require.Equal(t, len(h.session.routes()), tested, "not every route was exercised")
	require.Greater(t, tested, 5, "the route table looks empty, so this test would pass vacuously")
}

func TestHostRefusesASubmissionWhoseAddressWasSwapped(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)

	status, raw := h.get(t, "/ceremony/api/coordinator/state", testCoordinatorToken)
	require.Equal(t, http.StatusOK, status)
	var state hostStateView
	require.NoError(t, json.Unmarshal(raw, &state))

	honest := newCustodianKey(t, testRoster[0], 0, time.Now())
	attacker := newCustodianKey(t, testRoster[1], 1, time.Now())

	sub, err := signSubmission(state.Params.ID, honest.ID, honest.Priv)
	require.NoError(t, err)
	// The one attack a coordinator relay could try without breaking a signature:
	// keep the honest custodian's public key and name, change the address field
	// to one it controls. Every later check reads the address, so if it were
	// trusted the forgery would be consistent all the way to the record.
	sub.Identity.Address = attacker.ID.Address

	token := h.tokenFor(t, view, testRoster[0])
	status, raw = h.post(t, "/ceremony/api/invite/submission", token, sub)
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, string(raw), "claims address")

	// And nothing was recorded, so a refused submission does not block the
	// honest one that follows.
	status, raw = h.post(t, "/ceremony/api/invite/submission", token, mustSign(t, state.Params.ID, honest))
	require.Equal(t, http.StatusOK, status, string(raw))
}

func TestHostRefusesASubmissionUnderAnotherCustodiansLink(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)

	status, raw := h.get(t, "/ceremony/api/coordinator/state", testCoordinatorToken)
	require.Equal(t, http.StatusOK, status)
	var state hostStateView
	require.NoError(t, json.Unmarshal(raw, &state))

	// A link that leaked into a group chat, used to submit a key under somebody
	// else's name. Refused because the name comes from the token, not the body.
	stolen := h.tokenFor(t, view, testRoster[0])
	other := newCustodianKey(t, testRoster[1], 1, time.Now())
	status, raw = h.post(t, "/ceremony/api/invite/submission", stolen, mustSign(t, state.Params.ID, other))
	require.Equal(t, http.StatusForbidden, status)
	require.Contains(t, string(raw), "A link speaks for one custodian only")
}

func TestHostRefusesAnAttestationWhenTheSignerIsNotInTheGroup(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)

	status, raw := h.get(t, "/ceremony/api/coordinator/state", testCoordinatorToken)
	require.Equal(t, http.StatusOK, status)
	var state hostStateView
	require.NoError(t, json.Unmarshal(raw, &state))
	params := state.Params

	keys := make([]custodianKey, 0, len(testRoster))
	for i, name := range testRoster {
		token := h.tokenFor(t, view, name)
		key := newCustodianKey(t, name, i, time.Now())
		status, raw := h.post(t, "/ceremony/api/invite/submission", token, mustSign(t, params.ID, key))
		require.Equal(t, http.StatusOK, status, string(raw))
		keys = append(keys, key)
	}

	status, raw = h.get(t, "/ceremony/api/invite", h.tokenFor(t, view, testRoster[0]))
	require.Equal(t, http.StatusOK, status)
	var invited inviteView
	require.NoError(t, json.Unmarshal(raw, &invited))

	// The group as everybody computes it, minus one custodian. This is what a
	// relay that dropped somebody would produce: an internally consistent group
	// of four that the excluded custodian must refuse.
	fourOfFive := make([]submission, 0, 4)
	for _, sub := range invited.Submissions {
		if sub.Identity.Name != keys[2].Name {
			fourOfFive = append(fourOfFive, sub)
		}
	}
	shortParams := params
	shortParams.Custodians = nil
	for _, name := range params.Custodians {
		if name != keys[2].Name {
			shortParams.Custodians = append(shortParams.Custodians, name)
		}
	}
	substituted, err := assembleGroup(shortParams, fourOfFive)
	require.NoError(t, err)

	// The excluded custodian's own presence check refuses first: this is the
	// check the page runs before it will let the button be pressed.
	require.ErrorContains(t, substituted.presence(keys[2].ID.Address, keys[2].ID.Fingerprint),
		"YOUR KEY IS NOT IN THIS GROUP")

	// And if a modified page signed it anyway, the server refuses too, naming
	// both fingerprints rather than saying "invalid".
	att := attestation{
		CeremonyID:       params.ID,
		Name:             keys[2].Name,
		Address:          keys[2].ID.Address,
		GroupFingerprint: substituted.Fingerprint,
		PolicyAddress:    substituted.PolicyAddress,
		SignedAt:         time.Now().UTC().Format(time.RFC3339),
	}
	signed, err := signAttestation(att, keys[2].Priv)
	require.NoError(t, err)
	status, raw = h.post(t, "/ceremony/api/invite/attestation", h.tokenFor(t, view, keys[2].Name), signed)
	require.Equal(t, http.StatusConflict, status)
	require.Contains(t, string(raw), "different groups")
}

// TestTwoInstancesComputeTheSameFingerprint is the comparison the whole
// distributed design rests on.
//
// Two independent assemblies from the same relayed submissions, in an order that
// differs, must produce one fingerprint. If they could differ, the read-aloud
// check would fail for innocent reasons and five custodians would learn to shrug
// at the only thing that catches a hostile relay.
func TestTwoInstancesComputeTheSameFingerprint(t *testing.T) {
	h := newHostHarness(t)
	_, final := h.runWholeCeremony(t)
	require.NotNil(t, final.Assembled)

	forwards := append([]submission(nil), final.Submissions...)
	backwards := append([]submission(nil), final.Submissions...)
	for i, j := 0, len(backwards)-1; i < j; i, j = i+1, j-1 {
		backwards[i], backwards[j] = backwards[j], backwards[i]
	}

	first, err := assembleGroup(final.Params, forwards)
	require.NoError(t, err)
	second, err := assembleGroup(final.Params, backwards)
	require.NoError(t, err)

	require.Equal(t, first.Fingerprint, second.Fingerprint)
	require.Equal(t, string(first.Genesis), string(second.Genesis))
	require.Equal(t, final.Assembled.Fingerprint, first.Fingerprint)
}

func TestInviteTokenIsSingleUseForGeneration(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)
	token := h.tokenFor(t, view, testRoster[0])

	status, raw := h.post(t, "/ceremony/api/invite/generated", token, nil)
	require.Equal(t, http.StatusOK, status, string(raw))
	status, raw = h.post(t, "/ceremony/api/invite/generated", token, nil)
	require.Equal(t, http.StatusGone, status)
	require.Contains(t, string(raw), "already been generated")
	// The message has to say what to do, not only that it refused: the custodian
	// with their words on paper carries on, and the one without them needs a
	// reissue and needs to know it costs a key.
	require.Contains(t, string(raw), "reissue")
}

func TestReissuingRevokesTheOldLinkAndNotesTheAbandonedKey(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)
	old := h.tokenFor(t, view, testRoster[0])

	status, _ := h.post(t, "/ceremony/api/invite/generated", old, nil)
	require.Equal(t, http.StatusOK, status)

	status, raw := h.post(t, "/ceremony/api/coordinator/reissue", testCoordinatorToken,
		reissueRequest{Name: testRoster[0], Reason: "closed the tab"})
	require.Equal(t, http.StatusOK, status, string(raw))

	var state hostStateView
	require.NoError(t, json.Unmarshal(raw, &state))
	require.Contains(t, strings.Join(state.Notes, " "), "abandoned",
		"an abandoned key must reach the record; a record claiming five keys were generated once each would be false")

	status, raw = h.get(t, "/ceremony/api/invite", old)
	require.Equal(t, http.StatusForbidden, status)
	require.Contains(t, string(raw), "withdrawn and replaced")
	require.Contains(t, string(raw), "destroy that sheet")

	fresh := h.tokenFor(t, state, testRoster[0])
	require.NotEqual(t, old, fresh)
	status, _ = h.post(t, "/ceremony/api/invite/generated", fresh, nil)
	require.Equal(t, http.StatusOK, status)
}

func TestReissueIsRefusedOnceAKeyIsInTheGroup(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)

	status, raw := h.get(t, "/ceremony/api/coordinator/state", testCoordinatorToken)
	require.Equal(t, http.StatusOK, status)
	var state hostStateView
	require.NoError(t, json.Unmarshal(raw, &state))

	token := h.tokenFor(t, view, testRoster[0])
	key := newCustodianKey(t, testRoster[0], 0, time.Now())
	status, raw = h.post(t, "/ceremony/api/invite/submission", token, mustSign(t, state.Params.ID, key))
	require.Equal(t, http.StatusOK, status, string(raw))

	status, raw = h.post(t, "/ceremony/api/coordinator/reissue", testCoordinatorToken,
		reissueRequest{Name: testRoster[0]})
	require.Equal(t, http.StatusConflict, status)
	require.Contains(t, string(raw), "two keys for one custodian")
}

func TestACustodianTokenCannotReachTheCoordinator(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)
	token := h.tokenFor(t, view, testRoster[0])

	status, _ := h.get(t, "/ceremony/api/coordinator/state", token)
	require.Equal(t, http.StatusForbidden, status)
	status, _ = h.post(t, "/ceremony/api/coordinator/reissue", token, reissueRequest{Name: testRoster[1]})
	require.Equal(t, http.StatusForbidden, status)
	status, _ = h.get(t, "/ceremony/api/invite", "not-a-token")
	require.Equal(t, http.StatusForbidden, status)
	status, _ = h.get(t, "/ceremony/api/invite", "")
	require.Equal(t, http.StatusForbidden, status)
}

// TestTheCoordinatorLinkIsNeverSentToACustodian guards the one field in the
// state view that is a credential.
func TestTheCoordinatorLinkIsNeverSentToACustodian(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)
	token := h.tokenFor(t, view, testRoster[0])

	status, raw := h.get(t, "/ceremony/api/invite", token)
	require.Equal(t, http.StatusOK, status)
	body := string(raw)
	require.NotContains(t, body, testCoordinatorToken)
	for _, custodian := range view.Custodians {
		require.NotContains(t, body, custodian.Link,
			"%s's own page was sent another custodian's invitation link", custodian.Name)
	}
}

func TestTheServedPageCarriesAContentSecurityPolicyThatPinsConnections(t *testing.T) {
	h := newHostHarness(t)
	response, err := h.server.Client().Get(h.server.URL + "/ceremony/")
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()

	policy := response.Header.Get("Content-Security-Policy")
	// connect-src 'self' is what stops an altered page posting a phrase to
	// another origin. script-src without 'unsafe-inline' is what stops one being
	// injected into the page at all.
	require.Contains(t, policy, "connect-src 'self'")
	require.Contains(t, policy, "script-src 'self'")
	require.NotContains(t, policy, "script-src 'unsafe-inline'")
	require.Contains(t, response.Header.Get("Cache-Control"), "no-store")
}

// TestTheEmbeddedBundleUsesNoBrowserStorage is the Go half of the storage claim.
//
// clients/ceremony/src/storage.test.ts asserts the same thing from the
// JavaScript side; this one runs with no npm installed, so the guard survives a
// CI job that only has Go. A page that stored anything would make the private
// window the custodian was told to open the only thing standing between the
// ceremony and a shared laptop.
func TestTheEmbeddedBundleUsesNoBrowserStorage(t *testing.T) {
	bundle, err := hostedBundle()
	require.NoError(t, err)
	require.NotEmpty(t, bundle.Files)

	forbidden := []string{"localStorage", "sessionStorage", "indexedDB", "document.cookie", "caches.open", "openDatabase"}
	found := false
	for name, data := range bundle.Files {
		if !strings.HasSuffix(name, ".js") && !strings.HasSuffix(name, ".html") {
			continue
		}
		found = true
		for _, api := range forbidden {
			require.NotContains(t, string(data), api,
				"%s uses %s; this page must store nothing, because a custodian who forgot to open a private "+
					"window has to be left with nothing worth finding", name, api)
		}
	}
	require.True(t, found, "no script was checked, so this test would pass vacuously")
}

// TestHostRefusesATimestampThatWouldSplitTheFingerprint is the guard on the one
// value a hostile submission can choose that the two implementations read
// differently.
//
// The latest generated_at becomes the timestamp inside the genesis fragment, and
// the group fingerprint covers those bytes. This binary parses and normalises;
// the browser compares the strings. Both are right for one format and only one
// format, so anything else is refused rather than accepted and quietly rewritten.
func TestHostRefusesATimestampThatWouldSplitTheFingerprint(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)
	params := h.currentParams(t)
	token := h.tokenFor(t, view, testRoster[0])

	for _, spelling := range []string{
		"2026-03-02T11:15:00+02:00", // the same instant as an earlier Z value, sorting after it
		"2026-03-02T09:15:00.500Z",  // fractional seconds
		"2026-03-02T09:15:00-00:00", // a zero offset spelled as an offset
	} {
		key := newCustodianKey(t, testRoster[0], 0, time.Now())
		key.ID.GeneratedAt = spelling
		// Signed over the altered timestamp, so this is not a forgery: it is what
		// a custodian on a device with an unusual clock format would genuinely
		// send, and it has to be refused anyway.
		status, raw := h.post(t, "/ceremony/api/invite/submission", token, mustSign(t, params.ID, key))
		require.Equal(t, http.StatusBadRequest, status, spelling)
		require.Contains(t, string(raw), "UTC, whole seconds, trailing Z", spelling)
	}

	// And the canonical form still goes through, so the check is not simply
	// refusing everything.
	key := newCustodianKey(t, testRoster[0], 0, time.Now())
	status, raw := h.post(t, "/ceremony/api/invite/submission", token, mustSign(t, params.ID, key))
	require.Equal(t, http.StatusOK, status, string(raw))
}

// TestARevokedInviteCannotSubmitFromUnderTheGuard covers the window between the
// guard resolving a token and the handler acting on it.
//
// hostGuard checks Revoked while it resolves the token and then releases the
// lock, so a reissue landing in that instant would let one already-authorised
// request through — and the request it would let through is a submission of the
// exact key the coordinator just abandoned. The handler therefore re-checks under
// the lock it mutates under.
//
// Driven by handing the handler a revoked invite directly, because the race
// cannot be scheduled reliably over HTTP: the guard would refuse first. This is a
// test of the second check, and the second check is the only thing that closes
// the window.
func TestARevokedInviteCannotSubmitFromUnderTheGuard(t *testing.T) {
	h := newHostHarness(t)
	h.setup(t, testRoster)
	params := h.currentParams(t)

	h.session.mu.Lock()
	stale := h.session.live[testRoster[0]]
	h.session.mu.Unlock()
	require.NotNil(t, stale)

	// Whatever the guard admitted a moment ago, held in a request context.
	key := newCustodianKey(t, testRoster[0], 0, time.Now())
	body, err := json.Marshal(mustSign(t, params.ID, key))
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/ceremony/api/invite/submission", bytes.NewReader(body))
	request = request.WithContext(withInvite(request.Context(), stale))

	// The coordinator reissues in the meantime.
	status, _ := h.post(t, "/ceremony/api/coordinator/reissue", testCoordinatorToken,
		reissueRequest{Name: testRoster[0], Reason: "closed the tab"})
	require.Equal(t, http.StatusOK, status)

	recorder := httptest.NewRecorder()
	h.session.handleHostSubmission(recorder, request)
	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "withdrawn while this request was on its way")

	h.session.mu.Lock()
	_, recorded := h.session.submissions[testRoster[0]]
	h.session.mu.Unlock()
	require.False(t, recorded, "the abandoned key was recorded anyway")
}

func TestHostRefusesAKeyDerivedOnAnotherChainsPath(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)
	params := h.currentParams(t)

	key := newCustodianKey(t, testRoster[0], 0, time.Now())
	key.ID.HDPath = "m/44'/60'/0'/0/0"
	status, raw := h.post(t, "/ceremony/api/invite/submission",
		h.tokenFor(t, view, testRoster[0]), mustSign(t, params.ID, key))
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, string(raw), "this chain's accounts live under")
}

// TestParametersAreBoundedWhereTheBrowserCannotFollow covers the two parameter
// values that could make the page and the binary disagree rather than refuse.
func TestParametersAreBoundedWhereTheBrowserCannotFollow(t *testing.T) {
	base := ceremonyParams{
		ID:           "K4T9RM-2QWXVZ-8H0PBN-5CJDGF",
		Name:         "bounds",
		ChainID:      "yamale-1",
		Threshold:    3,
		Custodians:   testRoster,
		PolicySeq:    1,
		VotingPeriod: "168h0m0s",
	}
	require.NoError(t, base.validate())

	// Above 2^53 a JavaScript number is no longer exact, so the browser would
	// derive a different policy address — the recovery destination — while every
	// other check agreed.
	huge := base
	huge.PolicySeq = 1 << 60
	require.ErrorContains(t, huge.validate(), "past the range a browser holds exactly")

	// ThresholdDecisionPolicy.ValidateBasic refuses zero and permits negative,
	// and x/group's genesis validation never looks deeper.
	negative := base
	negative.VotingPeriod = "-1h"
	require.ErrorContains(t, negative.validate(), "no window to vote in")
	zero := base
	zero.VotingPeriod = "0s"
	require.ErrorContains(t, zero.validate(), "no window to vote in")
}

// TestSetupIsRefusedOnceSomebodyHasWordsOnPaper guards the interval between a
// custodian being shown their phrase and submitting it.
//
// Re-running setup mints a new ceremony id, so that custodian would be holding a
// key on paper for a ceremony that no longer exists — and the first anybody would
// know of it is a submission refused for the wrong id.
func TestSetupIsRefusedOnceSomebodyHasWordsOnPaper(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)
	status, _ := h.post(t, "/ceremony/api/invite/generated", h.tokenFor(t, view, testRoster[0]), nil)
	require.Equal(t, http.StatusOK, status)

	status, raw := h.post(t, "/ceremony/api/coordinator/setup", testCoordinatorToken, setupRequest{
		Ceremony: "second thoughts", ChainID: "yamale-1", Threshold: 3,
		Custodians: testRoster, PolicySeq: 1, VotingPeriod: "168h0m0s",
	})
	require.Equal(t, http.StatusConflict, status)
	require.Contains(t, string(raw), "already been shown twenty-four words")
}

func TestExportDoesNotClaimTheCeremonyFinishedEarly(t *testing.T) {
	h := newHostHarness(t)
	view := h.setup(t, testRoster)
	params := h.currentParams(t)

	// Every key submitted and nobody attested, which is the shape where this
	// mattered. A record short of the full roster is refused by renderRecord for
	// its own reasons — no policy address to put on it — so the only case that
	// reaches the completion flag is this one: the group exists, and not one
	// custodian has yet said they hold their share of it.
	for i, name := range testRoster {
		status, raw := h.post(t, "/ceremony/api/invite/submission",
			h.tokenFor(t, view, name), mustSign(t, params.ID, newCustodianKey(t, name, i, time.Now())))
		require.Equal(t, http.StatusOK, status, string(raw))
	}

	status, raw := h.post(t, "/ceremony/api/coordinator/export", testCoordinatorToken,
		exportRequest{Location: "a desk"})
	require.Equal(t, http.StatusOK, status, string(raw))

	var result struct {
		Complete bool `json:"complete"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	require.False(t, result.Complete,
		"a record exported before anybody attested must not mark the ceremony complete")
}

func TestHostRefusesAWildcardBind(t *testing.T) {
	_, err := listenLoopback("0.0.0.0", 0)
	require.ErrorContains(t, err, "not a loopback address")
	_, err = listenLoopback("pay.example.com", 0)
	require.ErrorContains(t, err, "not an IP address")
}

func TestPublicBaseRefusesPlainHTTPForARemoteHost(t *testing.T) {
	_, err := publicBase("http://pay.example.com/ceremony/", "127.0.0.1", 8787, "/ceremony/")
	require.ErrorContains(t, err, "is not https")

	_, err = publicBase("https://pay.example.com/other/", "127.0.0.1", 8787, "/ceremony/")
	require.ErrorContains(t, err, "disagree")

	base, err := publicBase("https://pay.example.com/ceremony/", "127.0.0.1", 8787, "/ceremony/")
	require.NoError(t, err)
	require.Equal(t, "https://pay.example.com/ceremony/", base.String())
}

func TestHostHeaderMustNameThePublishedSite(t *testing.T) {
	require.True(t, hostAllowed("127.0.0.1:8787", "pay.example.com"))
	require.True(t, hostAllowed("pay.example.com", "pay.example.com"))
	require.True(t, hostAllowed("pay.example.com", "pay.example.com:443"))
	require.False(t, hostAllowed("evil.example.com", "pay.example.com"))
	require.False(t, hostAllowed("evil.example.com", ""))
}

func mustSign(t *testing.T, ceremonyID string, key custodianKey) submission {
	t.Helper()
	sub, err := signSubmission(ceremonyID, key.ID, key.Priv)
	require.NoError(t, err)
	return sub
}

func base64Of(raw []byte) string {
	return base64.StdEncoding.EncodeToString(raw)
}

// bip39Words is the English wordlist, as a set.
//
// Taken from the same library the ceremony generates with, so a wordlist change
// upstream cannot leave this search looking for words the tool no longer uses.
var bip39Words = func() map[string]bool {
	words := map[string]bool{}
	for _, word := range bip39.WordList {
		words[word] = true
	}
	return words
}()

// longestMnemonicRun reports a run of six or more consecutive BIP-39 words.
//
// Six, not one: single words from the list — "art", "gather", "record" — occur
// in ordinary prose and an error message containing one is not a leak. Six in a
// row is not something prose produces and is something a leaked phrase always
// contains, so this catches a partial phrase as well as a whole one.
func longestMnemonicRun(body string) string {
	fields := strings.FieldsFunc(strings.ToLower(body), func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	run := make([]string, 0, 24)
	for _, field := range fields {
		if bip39Words[field] {
			run = append(run, field)
			if len(run) >= 6 {
				return strings.Join(run, " ")
			}
			continue
		}
		run = run[:0]
	}
	return ""
}
