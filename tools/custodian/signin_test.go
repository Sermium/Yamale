package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/recovery"
)

// Signing in on a new device: what releases a holding, what does not, and the
// emailed code that two services check without either trusting the other.

type sentMail struct{ to, subject, body string }

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
	fail error
}

func (m *fakeMailer) Send(to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	m.sent = append(m.sent, sentMail{to, subject, body})
	return nil
}

func (m *fakeMailer) last(t *testing.T) sentMail {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	require.NotEmpty(t, m.sent, "no mail was sent")
	return m.sent[len(m.sent)-1]
}

var sixDigits = regexp.MustCompile(`\b\d{6}\b`)

const (
	signinEmail    = "amara@example.org"
	signinPassword = "correct horse battery staple"
)

// signinFixture is the recovery fixture with the sign-in routes, a code key
// and a mailer. The code key is what both services share.
func signinFixture(t *testing.T, codeKey []byte) (*recoveryFixture, *fakeMailer) {
	t.Helper()
	f := newRecoveryFixture(t)
	mailer := &fakeMailer{}
	f.server.mailer = mailer
	if codeKey != nil {
		f.server.codes = newSigninCodes(codeKey)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/signin/start", f.server.handleSigninStart)
	mux.HandleFunc("POST /v1/signin", f.server.handleSignin)
	mux.HandleFunc("POST /v1/recovery/initiate", f.server.handleRecoveryInitiate)
	mux.HandleFunc("POST /v1/factors/totp/begin", f.server.handleTOTPBegin)
	mux.HandleFunc("POST /v1/factors/totp/confirm", f.server.handleTOTPConfirm)
	f.srv.Close()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	f.enrolAccount(t, signinEmail, signinPassword)
	return f, mailer
}

func codeKey() []byte { return []byte(strings.Repeat("k", 32)) }

func TestAnAuthenticatorAccountSignsInWithItsCode(t *testing.T) {
	f, _ := signinFixture(t, codeKey())
	secret := f.armTOTP(t, signinEmail)

	var start signinStartResponse
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: signinPassword}, &start))
	require.Equal(t, "totp", start.Factor)
	require.Empty(t, start.Nonce, "no code is emailed to an account with an authenticator")

	var got claimResponse
	code := totpNow(secret, *f.clock)
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin", signinRequest{
		Email: signinEmail, Password: signinPassword, Factor: "totp", Answer: code,
	}, &got))
	require.Equal(t, recovery.RoleCustodian, got.Role)
	require.Equal(t, "yml1testaccount", got.Address)
	require.NotEmpty(t, got.Holding.Share.Y, "the holding is released")

	// The same code again is a replay.
	require.Equal(t, http.StatusUnauthorized, f.post(t, "/v1/signin", signinRequest{
		Email: signinEmail, Password: signinPassword, Factor: "totp", Answer: code,
	}, nil))
}

func TestAnEmailedCodeSignsInOnceAndOnlyOnce(t *testing.T) {
	f, mailer := signinFixture(t, codeKey())

	var start signinStartResponse
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: signinPassword, Lang: "fr"}, &start))
	require.Equal(t, "email", start.Factor)
	mail := mailer.last(t)
	require.Equal(t, signinEmail, mail.to)
	require.Contains(t, mail.subject, "code")
	code := sixDigits.FindString(mail.body)
	require.NotEmpty(t, code)

	req := signinRequest{Email: signinEmail, Password: signinPassword, Factor: "email", Answer: code, Nonce: start.Nonce, Issued: start.Issued, Device: "Firefox on Linux", Lang: "fr"}
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin", req, nil))
	require.Eventually(t, func() bool {
		return strings.Contains(mailer.last(t).subject, "connexion")
	}, time.Second, 10*time.Millisecond, "a new-sign-in notice follows, in the person's language")
	require.Contains(t, mailer.last(t).body, "Firefox on Linux")

	require.Equal(t, http.StatusUnauthorized, f.post(t, "/v1/signin", req, nil), "a spent code does not work twice")
}

func TestAWrongCodeIsRefusedAndCountsLikeAWrongPassword(t *testing.T) {
	f, _ := signinFixture(t, codeKey())
	var start signinStartResponse
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: signinPassword}, &start))

	for i := 0; i < 5; i++ {
		require.Equal(t, http.StatusUnauthorized, f.post(t, "/v1/signin", signinRequest{
			Email: signinEmail, Password: signinPassword, Factor: "email", Answer: "000000", Nonce: start.Nonce, Issued: start.Issued,
		}, nil))
	}
	// Five wrong answers: the next attempt waits, even with everything right.
	require.Equal(t, http.StatusTooManyRequests, f.post(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: signinPassword}, nil))
}

func TestAnExpiredCodeIsRefused(t *testing.T) {
	f, mailer := signinFixture(t, codeKey())
	var start signinStartResponse
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: signinPassword}, &start))
	code := sixDigits.FindString(mailer.last(t).body)
	*f.clock = f.clock.Add(signinCodeLifetime + time.Second)
	require.Equal(t, http.StatusUnauthorized, f.post(t, "/v1/signin", signinRequest{
		Email: signinEmail, Password: signinPassword, Factor: "email", Answer: code, Nonce: start.Nonce, Issued: start.Issued,
	}, nil))
}

func TestTheRecoveryServiceChecksTheCustodiansCode(t *testing.T) {
	custodian, mailer := signinFixture(t, codeKey())
	rec, recMailer := signinFixture(t, codeKey())
	rec.server.role = recovery.RoleRecovery
	rec.server.mailer = nil
	*rec.clock = *custodian.clock

	var start signinStartResponse
	require.Equal(t, http.StatusOK, custodian.post(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: signinPassword}, &start))
	code := sixDigits.FindString(mailer.last(t).body)
	req := signinRequest{Email: signinEmail, Password: signinPassword, Factor: "email", Answer: code, Nonce: start.Nonce, Issued: start.Issued}

	require.Equal(t, http.StatusOK, custodian.post(t, "/v1/signin", req, nil))
	require.Equal(t, http.StatusOK, rec.post(t, "/v1/signin", req, nil), "the same code opens the other service's holding")
	require.Empty(t, recMailer.sent, "the recovery service sends nothing")

	// A service with a different key cannot be fooled by the custodian's code.
	other, _ := signinFixture(t, []byte(strings.Repeat("x", 32)))
	*other.clock = *custodian.clock
	require.Equal(t, http.StatusUnauthorized, other.post(t, "/v1/signin", req, nil))

	// And the recovery service never starts a sign-in.
	require.Equal(t, http.StatusNotFound, rec.post(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: signinPassword}, nil))
}

func TestCodesAreNotEmailedFasterThanOnceAMinute(t *testing.T) {
	f, mailer := signinFixture(t, codeKey())
	start := signinStartRequest{Email: signinEmail, Password: signinPassword}
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin/start", start, nil))
	require.Equal(t, http.StatusTooManyRequests, f.post(t, "/v1/signin/start", start, nil))
	*f.clock = f.clock.Add(61 * time.Second)
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin/start", start, nil))
	require.Len(t, mailer.sent, 2)
}

func TestWithoutAMailerAnEmailAccountIsToldItCannotSignInHere(t *testing.T) {
	f, _ := signinFixture(t, codeKey())
	f.server.mailer = nil
	require.Equal(t, http.StatusServiceUnavailable, f.post(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: signinPassword}, nil))
}

func TestAWrongPasswordReleasesNothingAndSaysNothing(t *testing.T) {
	f, mailer := signinFixture(t, codeKey())
	wrong, wrongBody := f.postWithBody(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: "not the password at all"})
	unknown, unknownBody := f.postWithBody(t, "/v1/signin/start", signinStartRequest{Email: "nobody@example.org", Password: "not the password at all"})
	require.Equal(t, http.StatusUnauthorized, wrong)
	require.Equal(t, wrong, unknown)
	require.Equal(t, wrongBody, unknownBody, "a wrong password and an unknown email read the same")
	require.Empty(t, mailer.sent)
}

func TestSigningInWaitsWhileARecoveryIsOpen(t *testing.T) {
	f, _ := signinFixture(t, codeKey())
	secret := f.armTOTP(t, signinEmail)
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/initiate", recoveryInitiateRequest{
		Email: signinEmail, Operator: "amina", Team: "support", Proof: "in person",
	}, nil))
	require.Equal(t, http.StatusConflict, f.post(t, "/v1/signin", signinRequest{
		Email: signinEmail, Password: signinPassword, Factor: "totp", Answer: totpNow(secret, *f.clock),
	}, nil))
}

func TestAFrozenAccountCannotSignIn(t *testing.T) {
	f, _ := signinFixture(t, codeKey())
	secret := f.armTOTP(t, signinEmail)
	require.NoError(t, f.server.store.Freeze(f.server.index.Of(signinEmail), "reported stolen"))
	require.Equal(t, http.StatusForbidden, f.post(t, "/v1/signin", signinRequest{
		Email: signinEmail, Password: signinPassword, Factor: "totp", Answer: totpNow(secret, *f.clock),
	}, nil))
}

func TestAnAuthenticatorAccountCannotBeDowngradedToAnEmailedCode(t *testing.T) {
	f, mailer := signinFixture(t, codeKey())
	f.armTOTP(t, signinEmail)
	nonce := strings.Repeat("n", 24)
	issued := f.clock.Unix()
	code := f.server.codes.code(signinEmail, nonce, issued)
	require.Equal(t, http.StatusBadRequest, f.post(t, "/v1/signin", signinRequest{
		Email: signinEmail, Password: signinPassword, Factor: "email", Answer: code, Nonce: nonce, Issued: issued,
	}, nil))
	require.Empty(t, mailer.sent)
}

func TestTheDeviceCanChooseTheAuthenticatorSecretForBothServices(t *testing.T) {
	a, _ := signinFixture(t, codeKey())
	b, _ := signinFixture(t, codeKey())
	*b.clock = *a.clock
	secret, err := NewTOTPSecret()
	require.NoError(t, err)
	chosen := base32Secret(secret)

	for _, f := range []*recoveryFixture{a, b} {
		var begun totpBegin
		require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/totp/begin", factorRequest{Email: signinEmail, Password: signinPassword, Secret: chosen}, &begun))
		require.Equal(t, chosen, begun.Secret, "the service keeps the device's secret")
		require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/totp/confirm", factorRequest{Email: signinEmail, Password: signinPassword, Code: totpNow(secret, *f.clock)}, nil))
	}
	fresh, _ := signinFixture(t, codeKey())
	require.Equal(t, http.StatusBadRequest, fresh.post(t, "/v1/factors/totp/begin", factorRequest{Email: signinEmail, Password: signinPassword, Secret: "TOOSHORT"}, nil))
}
