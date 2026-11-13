package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTheBusinessSetsItsLookAndAnyoneReadsIt(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	logo := pngBytes
	require.Equal(t, http.StatusCreated, serve(s, sign(t, request("POST", "/v1/files", logo), biz, "file", hashHex(logo), testNow.Unix())).Code)

	body := []byte(`{"logo":"` + hashHex(logo) + `","colour":"#12253f","accent":"#a87b3c"}`)
	w := serve(s, sign(t, request("PUT", "/v1/jewelers/1/profile", body), biz, "profile", "1."+hashHex(body), testNow.Unix()))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := serve(s, request("GET", "/v1/jewelers/1/profile", nil))
	require.Equal(t, http.StatusOK, got.Code)
	require.JSONEq(t, string(body), got.Body.String())
}

func TestOnlyTheBusinessSetsTheLook(t *testing.T) {
	biz, staff := testKey("biz"), testKey("staff")
	s := testServer(t, activeJeweler("1", biz, staff))
	body := []byte(`{"logo":"","colour":"#12253f","accent":"#a87b3c"}`)
	require.Equal(t, http.StatusForbidden, serve(s, sign(t, request("PUT", "/v1/jewelers/1/profile", body), staff, "profile", "1."+hashHex(body), testNow.Unix())).Code)
	// Signed for jeweler 2, sent to jeweler 1.
	require.Equal(t, http.StatusUnauthorized, serve(s, sign(t, request("PUT", "/v1/jewelers/1/profile", body), biz, "profile", "2."+hashHex(body), testNow.Unix())).Code)
}

func TestAProfileIsChecked(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	for _, body := range []string{
		`{"logo":"","colour":"navy","accent":"#a87b3c"}`,
		`{"logo":"","colour":"#12253F","accent":"#a87b3c"}`,
		`{"logo":"` + hashHex([]byte("never stored")) + `","colour":"#12253f","accent":"#a87b3c"}`,
		`{"logo":"","colour":"#12253f","accent":"#a87b3c","font":"Comic"}`,
	} {
		b := []byte(body)
		require.Equal(t, http.StatusBadRequest, serve(s, sign(t, request("PUT", "/v1/jewelers/1/profile", b), biz, "profile", "1."+hashHex(b), testNow.Unix())).Code, body)
	}
	require.Equal(t, http.StatusNotFound, serve(s, request("GET", "/v1/jewelers/1/profile", nil)).Code)
}
