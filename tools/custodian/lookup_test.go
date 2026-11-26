package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/recovery"
)

func lookupCall(t *testing.T, sv *service, token, email string) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(lookupRequest{Email: email})
	r := httptest.NewRequest("POST", "/v1/internal/address", bytes.NewReader(raw))
	if token != "" {
		r.Header.Set("X-Yamale-Lookup-Token", token)
	}
	w := httptest.NewRecorder()
	sv.server.handleLookup(w, r)
	var out map[string]string
	_ = json.NewDecoder(w.Body).Decode(&out)
	return w.Code, out["address"]
}

func TestTheAccountsAPIFindsTheAccountOfAnEmail(t *testing.T) {
	sv, _ := verifyingCustodian(t)
	token := strings.Repeat("t", 40)
	sv.server.lookupToken = token
	_, got := askCode(t, sv, enrolMail)
	code := sv.server.codes.code(enrolCodeSubject(enrolMail), got.Nonce, got.Issued)
	require.Equal(t, http.StatusOK, enrolWith(t, sv, enrolMail, got.Nonce, got.Issued, code))

	status, address := lookupCall(t, sv, token, enrolMail)
	require.Equal(t, http.StatusOK, status)
	require.True(t, strings.HasPrefix(address, "yml1"))

	status, _ = lookupCall(t, sv, token, "nobody@example.org")
	require.Equal(t, http.StatusNotFound, status)
}

func TestNobodyElseLooksUpAnEmail(t *testing.T) {
	sv, _ := verifyingCustodian(t)
	sv.server.lookupToken = strings.Repeat("t", 40)
	status, _ := lookupCall(t, sv, "", enrolMail)
	require.Equal(t, http.StatusNotFound, status)
	status, _ = lookupCall(t, sv, strings.Repeat("x", 40), enrolMail)
	require.Equal(t, http.StatusNotFound, status)

	// Without a configured token the endpoint answers nothing at all.
	sv.server.lookupToken = ""
	status, _ = lookupCall(t, sv, "", enrolMail)
	require.Equal(t, http.StatusNotFound, status)

	// And the recovery service never answers it.
	rs := newService(t, recovery.RoleRecovery)
	rs.server.lookupToken = strings.Repeat("t", 40)
	status, _ = lookupCall(t, rs, strings.Repeat("t", 40), enrolMail)
	require.Equal(t, http.StatusNotFound, status)
}
