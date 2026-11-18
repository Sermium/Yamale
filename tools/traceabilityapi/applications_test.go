package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func applicationBody(business string) []byte {
	return []byte(`{"business":"` + business + `","name":"Maison Diop","contact":"Awa Diop",` +
		`"email":"awa@example.sn","phone":"+221 77 000 00 00","country":"SN","message":"Gold and silver since 1987."}`)
}

func TestAnApplicationFromItsBusinessAccountIsSpooled(t *testing.T) {
	s := testServer(t)
	s.operatorToken = strings.Repeat("t", 32)
	k := testKey("applicant")
	body := applicationBody(addrOf(k))
	w := serve(s, sign(t, request("POST", "/v1/applications", body), k, "application", hashHex(body), testNow.Unix()))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	entries, err := os.ReadDir(filepath.Join(s.dir, "applications"))
	require.NoError(t, err)
	require.Len(t, entries, 1)

	r := request("GET", "/v1/applications", nil)
	r.Header.Set("Authorization", "Bearer "+s.operatorToken)
	list := serve(s, r)
	require.Equal(t, http.StatusOK, list.Code)
	var got []struct {
		Application struct{ Name string } `json:"application"`
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &got))
	require.Equal(t, "Maison Diop", got[0].Application.Name)
}

func TestAnApplicationIsSignedByTheBusinessItNames(t *testing.T) {
	s := testServer(t)
	k := testKey("applicant")
	body := applicationBody(addrOf(testKey("somebody-else")))
	require.Equal(t, http.StatusForbidden, serve(s, sign(t, request("POST", "/v1/applications", body), k, "application", hashHex(body), testNow.Unix())).Code)
}

func TestAnApplicationIsChecked(t *testing.T) {
	s := testServer(t)
	k := testKey("applicant")
	a := addrOf(k)
	for _, body := range []string{
		strings.Replace(string(applicationBody(a)), `"SN"`, `"Senegal"`, 1),
		strings.Replace(string(applicationBody(a)), `awa@example.sn`, `awa`, 1),
		strings.Replace(string(applicationBody(a)), `"name":"Maison Diop"`, `"name":""`, 1),
		strings.Replace(string(applicationBody(a)), `}`, `,"extra":1}`, 1),
	} {
		b := []byte(body)
		require.Equal(t, http.StatusBadRequest, serve(s, sign(t, request("POST", "/v1/applications", b), k, "application", hashHex(b), testNow.Unix())).Code, body)
	}
}

func TestTheListNeedsTheOperatorToken(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusNotFound, serve(s, request("GET", "/v1/applications", nil)).Code)
	s.operatorToken = strings.Repeat("t", 32)
	require.Equal(t, http.StatusUnauthorized, serve(s, request("GET", "/v1/applications", nil)).Code)
	r := request("GET", "/v1/applications", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	require.Equal(t, http.StatusUnauthorized, serve(s, r).Code)
	bare := request("GET", "/v1/applications", nil)
	bare.Header.Set("Authorization", s.operatorToken)
	require.Equal(t, http.StatusUnauthorized, serve(s, bare).Code)
}
