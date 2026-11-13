package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Shared with clients/sdk/src/loyaltyauth.ts. Regenerate with:
//
//	go test ./tools/loyaltyapi -run TestVectors -update-vectors
var updateVectors = flag.Bool("update-vectors", false, "rewrite testdata/vectors.json")

type requestVector struct {
	PrivKey   string `json:"priv_key"`
	PubKey    string `json:"pub_key"`
	Address   string `json:"address"`
	Purpose   string `json:"purpose"`
	ChainID   string `json:"chain_id"`
	Subject   string `json:"subject"`
	Issued    int64  `json:"issued"`
	SignBytes string `json:"sign_bytes"`
	Signature string `json:"signature"`
}

func TestVectors(t *testing.T) {
	var built []requestVector
	for _, c := range []struct{ label, purpose, subject string }{
		{"staff", "file", hashHex([]byte("photo"))},
		{"business", "profile", "12." + hashHex([]byte(`{"logo":""}`))},
		{"owner", "sponsor", ""},
		{"applicant", "application", hashHex([]byte(`{}`))},
	} {
		k := testKey(c.label)
		subject := c.subject
		if c.purpose == "sponsor" {
			subject = addrOf(k)
		}
		msg := signBytes(c.purpose, "yamale-local-1", subject, 1_790_000_000)
		sig, err := k.Sign(msg)
		require.NoError(t, err)
		built = append(built, requestVector{
			PrivKey: hex.EncodeToString(k.Key), PubKey: hex.EncodeToString(k.PubKey().Bytes()), Address: addrOf(k),
			Purpose: c.purpose, ChainID: "yamale-local-1", Subject: subject, Issued: 1_790_000_000,
			SignBytes: string(msg), Signature: hex.EncodeToString(sig),
		})
	}
	path := filepath.Join("testdata", "vectors.json")
	if *updateVectors {
		out, err := json.MarshalIndent(built, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, append(out, '\n'), 0o644))
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "run once with -update-vectors")
	var stored []requestVector
	require.NoError(t, json.Unmarshal(raw, &stored))
	require.Equal(t, built, stored)
}
