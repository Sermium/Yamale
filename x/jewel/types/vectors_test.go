package types_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// Test vectors shared with clients/sdk/src/jewel.ts.
//
// The app signs claims and vouchers in a browser, and the chain checks them in
// Go. A difference of one byte between the two makes every claim fail on the
// chain, and nothing short of a real claim would show it. So neither
// implementation is the reference for the other: both are checked against
// this file. The same arrangement already guards recovery/ and the wallet.
//
// Regenerate with:
//
//	go test ./x/jewel/types -run TestVectors -update-vectors
//
// Regenerating is a decision, not a fix. If a vector stops matching and
// nobody meant to change a format, the code is wrong and the vector is right.
var updateVectors = flag.Bool("update-vectors", false, "rewrite testdata/vectors.json from this implementation")

type jewelVectors struct {
	Comment      string              `json:"comment"`
	Inscriptions []inscriptionVector `json:"inscriptions"`
	Claims       []claimVector       `json:"claims"`
	Vouchers     []voucherVector     `json:"vouchers"`
}

type inscriptionVector struct {
	In  string `json:"in"`
	Out string `json:"out"` // empty when the chain refuses the input
}

type claimVector struct {
	PrivKey        string `json:"priv_key"`
	PubKey         string `json:"pub_key"`
	Address        string `json:"address"`
	ChainID        string `json:"chain_id"`
	JewelID        string `json:"jewel_id"`
	OfferExpiresAt string `json:"offer_expires_at"`
	Claimer        string `json:"claimer"`
	SignBytes      string `json:"sign_bytes"`
	Signature      string `json:"signature"`
}

type voucherVector struct {
	PrivKey   string `json:"priv_key"`
	PubKey    string `json:"pub_key"`
	Address   string `json:"address"`
	ChainID   string `json:"chain_id"`
	BenefitID string `json:"benefit_id"`
	JewelID   string `json:"jewel_id"`
	Nonce     string `json:"nonce"`
	ExpiresAt string `json:"expires_at"`
	SignBytes string `json:"sign_bytes"`
	Signature string `json:"signature"`
}

func vectorKey(label string) *secp256k1.PrivKey {
	sum := sha256.Sum256([]byte("yamale-jewel-vector:" + label))
	return &secp256k1.PrivKey{Key: sum[:]}
}

func ymlAddress(t *testing.T, k *secp256k1.PrivKey) string {
	s, err := bech32.ConvertAndEncode("yml", k.PubKey().Address())
	require.NoError(t, err)
	return s
}

func sign(t *testing.T, k *secp256k1.PrivKey, msg []byte) string {
	sig, err := k.Sign(msg)
	require.NoError(t, err)
	return hex.EncodeToString(sig)
}

func buildJewelVectors(t *testing.T) jewelVectors {
	v := jewelVectors{Comment: "Shared by x/jewel/types and clients/sdk/src/jewel.ts. " +
		"Regenerate with: go test ./x/jewel/types -run TestVectors -update-vectors"}

	for _, in := range []string{
		"MD-2026-0001", "md 2026/0001", "a.b_c-d 1/2", "abc", "ring é", "straße-1",
		"ıstanbul-9", "ſeal-77", "K-1234", "ring\t01",
		"0123456789012345678901234567890123456789",
		"0123456789012345678901234567890123456789X",
	} {
		out, err := types.NormaliseInscription(in)
		if err != nil {
			out = ""
		}
		v.Inscriptions = append(v.Inscriptions, inscriptionVector{In: in, Out: out})
	}

	claimer := ymlAddress(t, vectorKey("claimer"))
	for i, c := range []struct {
		chain   string
		jewel   uint64
		expires int64
	}{{"yamale-local-1", 1, 1790000000}, {"yamale-devnet-2", 18446744073709551615, 9223372036854775807}} {
		k := vectorKey("claim-" + strconv.Itoa(i))
		msg := types.ClaimSignBytes(c.chain, c.jewel, c.expires, claimer)
		v.Claims = append(v.Claims, claimVector{
			PrivKey: hex.EncodeToString(k.Key), PubKey: hex.EncodeToString(k.PubKey().Bytes()),
			Address: ymlAddress(t, k), ChainID: c.chain, JewelID: strconv.FormatUint(c.jewel, 10),
			OfferExpiresAt: strconv.FormatInt(c.expires, 10), Claimer: claimer, SignBytes: string(msg), Signature: sign(t, k, msg),
		})
	}

	for i, w := range []types.Voucher{
		{BenefitId: 1, JewelId: 1, Nonce: "00112233445566778899aabbccddeeff", ExpiresAt: 1790000000},
		{BenefitId: 42, JewelId: 9001, Nonce: "n", ExpiresAt: 1},
	} {
		k := vectorKey("voucher-" + strconv.Itoa(i))
		msg := types.VoucherSignBytes("yamale-local-1", w)
		v.Vouchers = append(v.Vouchers, voucherVector{
			PrivKey: hex.EncodeToString(k.Key), PubKey: hex.EncodeToString(k.PubKey().Bytes()),
			Address: ymlAddress(t, k), ChainID: "yamale-local-1",
			BenefitID: strconv.FormatUint(w.BenefitId, 10), JewelID: strconv.FormatUint(w.JewelId, 10),
			Nonce: w.Nonce, ExpiresAt: strconv.FormatInt(w.ExpiresAt, 10),
			SignBytes: string(msg), Signature: sign(t, k, msg),
		})
	}
	return v
}

func TestVectors(t *testing.T) {
	path := filepath.Join("testdata", "vectors.json")
	built := buildJewelVectors(t)
	if *updateVectors {
		out, err := json.MarshalIndent(built, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, append(out, '\n'), 0o644))
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "run once with -update-vectors to create the file")
	var stored jewelVectors
	require.NoError(t, json.Unmarshal(raw, &stored))
	require.Equal(t, built, stored)
}
