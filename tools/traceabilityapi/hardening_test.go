package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// meteredBody serves zeros forever and counts what was read from it.
type meteredBody struct{ read int }

func (m *meteredBody) Read(p []byte) (int, error) { m.read += len(p); return len(p), nil }

type failingTail struct{ r io.Reader }

func (f failingTail) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if err == io.EOF {
		return n, errors.New("connection reset")
	}
	return n, err
}

type countingChain struct {
	js    []Jeweler
	calls int
}

func (c *countingChain) Jewelers(context.Context) ([]Jeweler, error) { c.calls++; return c.js, nil }

func TestStrangersAreRefusedBeforeTheBodyIsRead(t *testing.T) {
	biz, stranger := testKey("biz"), testKey("stranger")
	s := testServer(t, activeJeweler("1", biz))

	m := &meteredBody{}
	r := httptest.NewRequest("POST", "/v1/files", m)
	require.Equal(t, http.StatusUnauthorized, serve(s, r).Code)
	require.Zero(t, m.read, "an unsigned upload must not be read")

	m = &meteredBody{}
	r = sign(t, httptest.NewRequest("POST", "/v1/files", m), stranger, "file", "whatever", testNow.Unix())
	require.Equal(t, http.StatusForbidden, serve(s, r).Code)
	require.Zero(t, m.read, "a stranger's upload must not be read")

	m = &meteredBody{}
	r = sign(t, httptest.NewRequest("POST", "/v1/files", m), biz, "file", "x", testNow.Unix()-3600)
	require.Equal(t, http.StatusUnauthorized, serve(s, r).Code)
	require.Zero(t, m.read, "a stale signature must not be read")
}

func TestOversizedIs413ButAnIncompleteUploadIs400(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))

	r := sign(t, httptest.NewRequest("POST", "/v1/files", &meteredBody{}), biz, "file", "x", testNow.Unix())
	require.Equal(t, http.StatusRequestEntityTooLarge, serve(s, r).Code)

	r = sign(t, httptest.NewRequest("POST", "/v1/files", failingTail{bytes.NewReader(pngBytes)}), biz, "file", hashHex(pngBytes), testNow.Unix())
	require.Equal(t, http.StatusBadRequest, serve(s, r).Code)
}

func TestNoTemporaryFileSurvivesAFailedUpload(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	txt := []byte("just some words")
	require.Equal(t, http.StatusUnsupportedMediaType, serve(s, sign(t, request("POST", "/v1/files", txt), biz, "file", hashHex(txt), testNow.Unix())).Code)
	require.Equal(t, http.StatusUnauthorized, serve(s, sign(t, request("POST", "/v1/files", pngBytes), biz, "file", "wrong", testNow.Unix())).Code)
	left, err := filepath.Glob(filepath.Join(s.dir, "files", ".upload-*"))
	require.NoError(t, err)
	require.Empty(t, left)
	entries, _ := os.ReadDir(filepath.Join(s.dir, "files"))
	require.Empty(t, entries)
}

func TestTheJewelersAreReadOncePerTTL(t *testing.T) {
	biz := testKey("biz")
	cc := &countingChain{js: []Jeweler{activeJeweler("1", biz)}}
	now := testNow
	s := testServer(t)
	s.chain, s.now = cc, func() time.Time { return now }

	for i := 0; i < 3; i++ {
		r := sign(t, request("POST", "/v1/files", pngBytes), biz, "file", hashHex(pngBytes), now.Unix())
		require.Equal(t, http.StatusCreated, serve(s, r).Code)
	}
	require.Equal(t, 1, cc.calls)

	now = now.Add(jewelersTTL + time.Second)
	r := sign(t, request("POST", "/v1/files", pngBytes), biz, "file", hashHex(pngBytes), now.Unix())
	require.Equal(t, http.StatusCreated, serve(s, r).Code)
	require.Equal(t, 2, cc.calls)
}

func TestAChainErrorIsNotCached(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	s.chain = fakeChain{err: errors.New(`Get "http://10.0.0.7:1317/blockchain/jewel": down`)}
	r := sign(t, request("POST", "/v1/files", pngBytes), biz, "file", hashHex(pngBytes), testNow.Unix())
	got := serve(s, r)
	require.Equal(t, http.StatusBadGateway, got.Code)
	require.NotContains(t, got.Body.String(), "10.0.0.7", "the node's address stays in the log")
	s.chain = fakeChain{js: []Jeweler{activeJeweler("1", biz)}}
	r = sign(t, request("POST", "/v1/files", pngBytes), biz, "file", hashHex(pngBytes), testNow.Unix())
	require.Equal(t, http.StatusCreated, serve(s, r).Code)
}

func TestAFullServerAnswers503WithRetryAfter(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	s.slots = make(chan struct{}, 1)
	s.slots <- struct{}{} // the only slot is taken
	s.slotWait = 10 * time.Millisecond
	w := serve(s, sign(t, request("POST", "/v1/files", pngBytes), biz, "file", hashHex(pngBytes), testNow.Unix()))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.NotEmpty(t, w.Header().Get("Retry-After"))

	<-s.slots // freed: the same upload now goes through
	w = serve(s, sign(t, request("POST", "/v1/files", pngBytes), biz, "file", hashHex(pngBytes), testNow.Unix()))
	require.Equal(t, http.StatusCreated, w.Code)
	require.Len(t, s.slots, 0, "the slot is released after each upload")
}

// A slow upload must still be able to write its answer after the server-wide
// WriteTimeout has passed, because the write deadline starts at the headers.
func TestASlowUploadOutlivesTheServerWriteTimeout(t *testing.T) {
	biz := testKey("biz")
	s := testServer(t, activeJeweler("1", biz))
	ts := httptest.NewUnstartedServer(s.routes())
	ts.Config.WriteTimeout = 300 * time.Millisecond
	ts.Start()
	defer ts.Close()

	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write(pngBytes[:32])
		time.Sleep(700 * time.Millisecond)
		_, _ = pw.Write(pngBytes[32:])
		pw.Close()
	}()
	req, err := http.NewRequest("POST", ts.URL+"/v1/files", pr)
	require.NoError(t, err)
	sign(t, req, biz, "file", hashHex(pngBytes), testNow.Unix())
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
}
