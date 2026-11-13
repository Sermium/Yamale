package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{7}, 64)...)

func TestStaffStoreAPhotoAndAnyoneReadsItBack(t *testing.T) {
	biz, staff := testKey("biz"), testKey("staff")
	s := testServer(t, activeJeweler("1", biz, staff))
	w := serve(s, sign(t, request("POST", "/v1/files", pngBytes), staff, "file", hashHex(pngBytes), testNow.Unix()))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.JSONEq(t, `{"hash":"`+hashHex(pngBytes)+`"}`, w.Body.String())

	got := serve(s, request("GET", "/v1/files/"+hashHex(pngBytes), nil))
	require.Equal(t, http.StatusOK, got.Code)
	require.Equal(t, pngBytes, got.Body.Bytes())
	require.Equal(t, "image/png", got.Header().Get("Content-Type"))
	require.Equal(t, "public, max-age=31536000, immutable", got.Header().Get("Cache-Control"))
}

func TestARecordIsServedAsJSON(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	rec := []byte(`{"version":1,"title":"Ring"}`)
	require.Equal(t, http.StatusCreated, serve(s, sign(t, request("POST", "/v1/files", rec), biz, "file", hashHex(rec), testNow.Unix())).Code)
	require.Equal(t, "application/json", serve(s, request("GET", "/v1/files/"+hashHex(rec), nil)).Header().Get("Content-Type"))
}

func TestStrangersAndSuspendedJewelersCannotStore(t *testing.T) {
	biz, stranger, susp := testKey("biz"), testKey("stranger"), testKey("susp")
	suspended := activeJeweler("2", susp)
	suspended.Status = "JEWELER_SUSPENDED"
	s := testServer(t, activeJeweler("1", biz), suspended)
	require.Equal(t, http.StatusForbidden, serve(s, sign(t, request("POST", "/v1/files", pngBytes), stranger, "file", hashHex(pngBytes), testNow.Unix())).Code)
	require.Equal(t, http.StatusForbidden, serve(s, sign(t, request("POST", "/v1/files", pngBytes), susp, "file", hashHex(pngBytes), testNow.Unix())).Code)
}

func TestTheSignatureMustCoverTheBody(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	other := []byte(`{"version":1}`)
	require.Equal(t, http.StatusUnauthorized, serve(s, sign(t, request("POST", "/v1/files", pngBytes), biz, "file", hashHex(other), testNow.Unix())).Code)
}

func TestUnsupportedAndOversizedFilesAreRefused(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	txt := []byte("just some words")
	require.Equal(t, http.StatusUnsupportedMediaType, serve(s, sign(t, request("POST", "/v1/files", txt), biz, "file", hashHex(txt), testNow.Unix())).Code)
	big := append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{1}, maxFileBytes)...)
	require.Equal(t, http.StatusRequestEntityTooLarge, serve(s, sign(t, request("POST", "/v1/files", big), biz, "file", hashHex(big), testNow.Unix())).Code)
}

func TestAFileIsNamedByItsFingerprint(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusBadRequest, serve(s, request("GET", "/v1/files/ZZ", nil)).Code)
	require.Equal(t, http.StatusNotFound, serve(s, request("GET", "/v1/files/"+hashHex([]byte("absent")), nil)).Code)
}
