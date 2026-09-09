// Package dkls23spike checks whether a DKLs23 signature is usable on this
// chain, against the chain's own derivation rather than against a claim.
//
// It reads the JSON that cosmos_spike.rs prints — path in DKLS_SPIKE_JSON — and
// skips when that is unset, so `go test ./...` stays green on a machine with no
// Rust toolchain. Which is every machine here: neither host can build Rust
// without risking a stalled validator, so the Rust half runs in CI and this
// test runs against its output.
//
// What it is actually asserting, in the order the failures matter:
//
//  1. the public key parses as a secp256k1 point at all;
//  2. it derives a yml1 address through mpc/cosmos — the same code path the
//     device and the custodian both use, so a key encoded the way this
//     codebase does not expect fails here rather than at a payment;
//  3. the signature verifies against that key over the digest;
//  4. S is in the lower half of the curve order. This is the one a passing
//     "it verified" would hide: both (r, s) and (r, n-s) verify, Cosmos rejects
//     the high form, and tss-lib normalises internally. Whether sl-dkls23 does
//     is not documented and is exactly the kind of thing that works in every
//     test and fails on chain.
package dkls23spike

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	mpccosmos "yamale/blockchain/mpc/cosmos"

	_ "yamale/blockchain/app" // its init() seals the yml bech32 prefix
)

type spikeResult struct {
	Protocol    string `json:"protocol"`
	Library     string `json:"library"`
	Threshold   string `json:"threshold"`
	ChainPath   string `json:"chain_path"`
	PubKeySEC1  string `json:"pubkey_sec1"`
	Digest      string `json:"digest"`
	SignatureRS string `json:"signature_rs"`
	RecoveryID  int    `json:"recovery_id"`
}

func load(t *testing.T) spikeResult {
	t.Helper()

	path := os.Getenv("DKLS_SPIKE_JSON")
	if path == "" {
		t.Skip("DKLS_SPIKE_JSON unset: run .github/workflows/dkls-spike.yml, which produces it")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the spike output: %v", err)
	}

	var got spikeResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the spike did not print JSON:\n%s\n\n%v", raw, err)
	}
	return got
}

func decode(t *testing.T, what, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		t.Fatalf("%s is not hex: %q: %v", what, s, err)
	}
	return b
}

// TestDklsSignatureIsUsableOnThisChain is the whole point of the spike.
func TestDklsSignatureIsUsableOnThisChain(t *testing.T) {
	got := load(t)
	t.Logf("%s via %s, %s, chain path %q", got.Protocol, got.Library, got.Threshold, got.ChainPath)

	// --- 1. the public key parses
	pubBytes := decode(t, "pubkey_sec1", got.PubKeySEC1)
	t.Logf("public key: %d bytes, %s", len(pubBytes), got.PubKeySEC1)

	pub, err := secp256k1.ParsePubKey(pubBytes)
	if err != nil {
		t.Fatalf("the joint public key does not parse as a secp256k1 point (%d bytes): %v\n"+
			"a 33-byte compressed or 65-byte uncompressed SEC1 encoding is what this chain reads",
			len(pubBytes), err)
	}
	ecdsaPub := pub.ToECDSA()

	// --- 2. it derives an address through the chain's own code
	cosmosPub, err := mpccosmos.CosmosPubKey(ecdsaPub)
	if err != nil {
		t.Fatalf("mpc/cosmos refused the key: %v", err)
	}
	addr, err := mpccosmos.Address(ecdsaPub)
	if err != nil {
		t.Fatalf("mpc/cosmos could not derive an address: %v", err)
	}
	if !strings.HasPrefix(addr, "yml1") {
		t.Fatalf("derived address %q is not a yml1 address", addr)
	}
	t.Logf("derives to %s", addr)
	t.Logf("cosmos key type %T, %d bytes", cosmosPub, len(cosmosPub.Bytes()))

	// The address must come from the key and nothing else, or a device could be
	// told to fund somebody else's account.
	if len(cosmosPub.Bytes()) != 33 {
		t.Errorf("cosmos pubkey is %d bytes, want 33 compressed: an account holding "+
			"this must be indistinguishable from a self-custodied one",
			len(cosmosPub.Bytes()))
	}

	// --- 3. the signature verifies over the digest
	digest := decode(t, "digest", got.Digest)
	if len(digest) != 32 {
		t.Fatalf("digest is %d bytes, want 32", len(digest))
	}

	sig := decode(t, "signature_rs", got.SignatureRS)
	if len(sig) != 64 {
		t.Fatalf("signature is %d bytes, want 64 for R||S", len(sig))
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])

	if !ecdsa.Verify(ecdsaPub, digest, r, s) {
		t.Fatalf("the signature does not verify against the joint key over the digest\n"+
			"  key    %s\n  digest %s\n  sig    %s",
			got.PubKeySEC1, got.Digest, got.SignatureRS)
	}
	t.Log("signature verifies against the joint key")

	// --- 4. S is in the lower half, which Cosmos requires
	order := secp256k1.S256().N
	half := new(big.Int).Rsh(order, 1)
	if s.Cmp(half) > 0 {
		t.Errorf("S is in the UPPER half of the curve order, and Cosmos rejects that.\n"+
			"The signature is cryptographically valid and the chain will refuse it, so the\n"+
			"sidecar has to normalise the way mpc.normalise() already does for tss-lib:\n"+
			"  s = n - s when s > n/2\n"+
			"  S    %s\n  n/2  %s", s, half)
	} else {
		t.Log("S is in the lower half: no normalisation needed at this signature")
	}

	// One passing signature does not establish that S is always low — it is
	// roughly even odds per signature if the library does not normalise. Said
	// out loud so a single green run is not mistaken for the guarantee.
	t.Log("note: low S here is one sample, not proof the library normalises; " +
		"the sidecar should normalise unconditionally")
}
