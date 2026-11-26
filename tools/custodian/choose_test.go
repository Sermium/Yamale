package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Choosing the factor: the emailed code, an authenticator, or a PIN. The rule
// these tests hold is the one that keeps a choice from being a downgrade
// attack: nothing changes without the current factor answered.

func chooseFixture(t *testing.T) *recoveryFixture {
	t.Helper()
	f := newRecoveryFixture(t)
	f.server.mailer = &fakeMailer{}
	f.server.codes = newSigninCodes(codeKey())
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/signin/start", f.server.handleSigninStart)
	mux.HandleFunc("POST /v1/signin", f.server.handleSignin)
	mux.HandleFunc("POST /v1/factors", f.server.handleFactors)
	mux.HandleFunc("POST /v1/factors/choose", f.server.handleFactorChoose)
	mux.HandleFunc("POST /v1/factors/totp/begin", f.server.handleTOTPBegin)
	mux.HandleFunc("POST /v1/factors/totp/confirm", f.server.handleTOTPConfirm)
	f.srv.Close()
	f.srv = httptest.NewServer(mux)
	f.enrolAccount(t, signinEmail, signinPassword)
	return f
}

// emailProof is an emailed code as the custodian would have sent it.
func emailProof(f *recoveryFixture) *factorProof {
	nonce := strings.Repeat("p", 24)
	issued := f.clock.Unix()
	return &factorProof{Factor: "email", Answer: f.server.codes.code(signinEmail, nonce, issued), Nonce: nonce, Issued: issued}
}

func TestChoosingAPINNeedsTheCurrentFactor(t *testing.T) {
	f := chooseFixture(t)
	// The password alone is what a thief has.
	require.Equal(t, http.StatusForbidden, f.post(t, "/v1/factors/choose", factorRequest{
		Email: signinEmail, Password: signinPassword, Factor: "pin", PIN: "482913",
	}, nil))
	// A wrong emailed code is not the current factor answered.
	bad := emailProof(f)
	bad.Answer = "000000"
	require.Equal(t, http.StatusUnauthorized, f.post(t, "/v1/factors/choose", factorRequest{
		Email: signinEmail, Password: signinPassword, Factor: "pin", PIN: "482913", Proof: bad,
	}, nil))

	var state factorState
	require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/choose", factorRequest{
		Email: signinEmail, Password: signinPassword, Factor: "pin", PIN: "482913", Proof: emailProof(f),
	}, &state))
	require.Equal(t, "pin", state.Current)
	require.True(t, state.PIN)
}

func TestAPINIsWhatSignInAsksFor(t *testing.T) {
	f := chooseFixture(t)
	require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/choose", factorRequest{
		Email: signinEmail, Password: signinPassword, Factor: "pin", PIN: "482913", Proof: emailProof(f),
	}, nil))

	var started signinStartResponse
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin/start", signinStartRequest{Email: signinEmail, Password: signinPassword}, &started))
	require.Equal(t, "pin", started.Factor)
	require.Empty(t, started.Nonce, "no code is emailed to an account that chose a PIN")

	require.Equal(t, http.StatusUnauthorized, f.post(t, "/v1/signin", signinRequest{
		Email: signinEmail, Password: signinPassword, Factor: "pin", Answer: "111111",
	}, nil))
	*f.clock = f.clock.Add(10 * time.Minute) // past the back-off a wrong answer earns
	require.Equal(t, http.StatusOK, f.post(t, "/v1/signin", signinRequest{
		Email: signinEmail, Password: signinPassword, Factor: "pin", Answer: "482913",
	}, nil))

	// An emailed code is not accepted in place of the PIN the holder chose.
	require.Equal(t, http.StatusBadRequest, f.post(t, "/v1/signin", signinRequest{
		Email: signinEmail, Password: signinPassword, Factor: "email", Answer: emailProof(f).Answer,
		Nonce: emailProof(f).Nonce, Issued: emailProof(f).Issued,
	}, nil))
}

func TestAPINIsSixToTwelveDigits(t *testing.T) {
	f := chooseFixture(t)
	for _, pin := range []string{"12345", "1234567890123", "12a456", ""} {
		require.Equal(t, http.StatusBadRequest, f.post(t, "/v1/factors/choose", factorRequest{
			Email: signinEmail, Password: signinPassword, Factor: "pin", PIN: pin, Proof: emailProof(f),
		}, nil), pin)
	}
}

func TestLeavingTheAuthenticatorNeedsItsCode(t *testing.T) {
	f := chooseFixture(t)
	secret := f.armTOTP(t, signinEmail)
	// An emailed code does not prove the authenticator.
	require.Equal(t, http.StatusForbidden, f.post(t, "/v1/factors/choose", factorRequest{
		Email: signinEmail, Password: signinPassword, Factor: "email", Proof: emailProof(f),
	}, nil))
	var state factorState
	require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/choose", factorRequest{
		Email: signinEmail, Password: signinPassword, Factor: "email",
		Proof: &factorProof{Factor: "totp", Answer: totpNow(secret, *f.clock)},
	}, &state))
	require.Equal(t, "email", state.Current)
	require.False(t, state.TOTP, "the old authenticator is dropped, so returning to it starts from a fresh secret")
}

func TestAnAuthenticatorBecomesTheChoiceWithAProof(t *testing.T) {
	f := chooseFixture(t)
	require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/choose", factorRequest{
		Email: signinEmail, Password: signinPassword, Factor: "pin", PIN: "482913", Proof: emailProof(f),
	}, nil))
	var begun totpBegin
	require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/totp/begin", factorRequest{Email: signinEmail, Password: signinPassword}, &begun))
	secret := decodeBase32(t, begun.Secret)

	// Confirmed without a proof, the authenticator does not displace the PIN the holder chose.
	var state factorState
	require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/totp/confirm", factorRequest{
		Email: signinEmail, Password: signinPassword, Code: totpNow(secret, *f.clock),
	}, &state))
	require.Equal(t, "pin", state.Current)

	// With the PIN as the proof, it becomes the choice and the PIN is dropped.
	require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/totp/confirm", factorRequest{
		Email: signinEmail, Password: signinPassword, Code: totpNow(secret, *f.clock),
		Proof: &factorProof{Factor: "pin", Answer: "482913"},
	}, &state))
	require.Equal(t, "totp", state.Current)
	require.False(t, state.PIN)
}
