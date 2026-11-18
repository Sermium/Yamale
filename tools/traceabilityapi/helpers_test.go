package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"
)

const testChain = "yamale-test-1"

var testNow = time.Unix(1_790_000_000, 0)

type fakeChain struct {
	js  []Jeweler
	err error
}

func (f fakeChain) Jewelers(context.Context) ([]Jeweler, error) { return f.js, f.err }

func testServer(t *testing.T, js ...Jeweler) *server {
	t.Helper()
	return &server{dir: t.TempDir(), chainID: testChain, chain: fakeChain{js: js}, now: func() time.Time { return testNow }}
}

func testKey(label string) *secp256k1.PrivKey {
	sum := sha256.Sum256([]byte("traceabilityapi-test:" + label))
	return &secp256k1.PrivKey{Key: sum[:]}
}

func addrOf(k *secp256k1.PrivKey) string {
	s, err := bech32.ConvertAndEncode("yml", k.PubKey().Address())
	if err != nil {
		panic(err)
	}
	return s
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sign(t *testing.T, r *http.Request, k *secp256k1.PrivKey, purpose, subject string, issued int64) *http.Request {
	t.Helper()
	sig, err := k.Sign(signBytes(purpose, testChain, subject, issued))
	require.NoError(t, err)
	r.Header.Set(headerKey, base64.StdEncoding.EncodeToString(k.PubKey().Bytes()))
	r.Header.Set(headerSignature, base64.StdEncoding.EncodeToString(sig))
	r.Header.Set(headerIssued, strconv.FormatInt(issued, 10))
	return r
}

func request(method, path string, body []byte) *http.Request {
	return httptest.NewRequest(method, path, bytes.NewReader(body))
}

func serve(s *server, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	return w
}

func activeJeweler(id string, business *secp256k1.PrivKey, staff ...*secp256k1.PrivKey) Jeweler {
	j := Jeweler{ID: id, Name: "Maison " + id, Business: addrOf(business), Status: "JEWELER_ACTIVE"}
	for _, k := range staff {
		j.Staff = append(j.Staff, addrOf(k))
	}
	return j
}
