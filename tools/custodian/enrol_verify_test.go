package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/recovery"
)

// A registration proves its email with a code sent to it, so nobody can
// register an address they do not own before its owner does.

const enrolMail = "kofi@example.org"
const enrolPassword = "seven horses eating apples slowly"

func verifyingCustodian(t *testing.T) (*service, *fakeMailer) {
	t.Helper()
	sv := newService(t, recovery.RoleCustodian)
	m := &fakeMailer{}
	sv.server.mailer = m
	sv.server.codes = newSigninCodes(codeKey())
	sv.server.enrolNeedsEmailCode = true
	return sv, m
}

// askCode calls /v1/enrol/code directly on the handler.
func askCode(t *testing.T, sv *service, email string) (int, enrolCodeResponse) {
	t.Helper()
	raw, _ := json.Marshal(enrolCodeRequest{Email: email, Lang: "fr"})
	w := httptest.NewRecorder()
	sv.server.handleEnrolCode(w, httptest.NewRequest("POST", "/v1/enrol/code", bytes.NewReader(raw)))
	var out enrolCodeResponse
	if w.Code == http.StatusOK {
		require.NoError(t, json.NewDecoder(w.Body).Decode(&out))
	}
	return w.Code, out
}

// enrolWith posts a valid deposit for email with the given code fields.
func enrolWith(t *testing.T, sv *service, email, nonce string, issued int64, code string) int {
	t.Helper()
	key, err := secp256k1.GeneratePrivateKey()
	require.NoError(t, err)
	holdings, err := recovery.Protect(key.Serialize(), enrolPassword)
	require.NoError(t, err)
	var h recovery.Holding
	for _, x := range holdings {
		if x.Role == recovery.RoleCustodian {
			h = x
		}
	}
	sig, err := recovery.SignDeposit(key, email, recovery.RoleCustodian, h)
	require.NoError(t, err)
	return sv.call(t, "/v1/enrol", enrolRequest{
		Deposit: Deposit{Email: email, Password: enrolPassword, PubKey: key.PubKey().SerializeCompressed(), Holding: h, Signature: sig},
		Nonce:   nonce, Issued: issued, Code: code,
	}, nil)
}

func TestARegistrationNeedsTheCodeSentToItsEmail(t *testing.T) {
	sv, mailer := verifyingCustodian(t)
	require.Equal(t, http.StatusUnauthorized, enrolWith(t, sv, enrolMail, "", 0, ""))

	status, got := askCode(t, sv, enrolMail)
	require.Equal(t, http.StatusOK, status)
	require.Len(t, mailer.sent, 1)
	code := sv.server.codes.code(enrolCodeSubject(enrolMail), got.Nonce, got.Issued)
	require.Contains(t, mailer.sent[0].body, code)

	require.Equal(t, http.StatusUnauthorized, enrolWith(t, sv, enrolMail, got.Nonce, got.Issued, "000000"))
	require.Equal(t, http.StatusOK, enrolWith(t, sv, enrolMail, got.Nonce, got.Issued, code))
}

func TestACodeForOneEmailDoesNotRegisterAnother(t *testing.T) {
	sv, _ := verifyingCustodian(t)
	_, got := askCode(t, sv, enrolMail)
	code := sv.server.codes.code(enrolCodeSubject(enrolMail), got.Nonce, got.Issued)
	require.Equal(t, http.StatusUnauthorized, enrolWith(t, sv, "someone.else@example.org", got.Nonce, got.Issued, code))
}

func TestASignInCodeIsNotARegistrationCode(t *testing.T) {
	sv, _ := verifyingCustodian(t)
	nonce := strings.Repeat("s", 24)
	issued := sv.clock.Unix()
	signin := sv.server.codes.code(enrolMail, nonce, issued)
	require.Equal(t, http.StatusUnauthorized, enrolWith(t, sv, enrolMail, nonce, issued, signin))
}

func TestARegisteredEmailGetsNoRegistrationCode(t *testing.T) {
	sv, mailer := verifyingCustodian(t)
	_, got := askCode(t, sv, enrolMail)
	code := sv.server.codes.code(enrolCodeSubject(enrolMail), got.Nonce, got.Issued)
	require.Equal(t, http.StatusOK, enrolWith(t, sv, enrolMail, got.Nonce, got.Issued, code))
	status, _ := askCode(t, sv, enrolMail)
	require.Equal(t, http.StatusConflict, status)
	require.Len(t, mailer.sent, 1)
}
