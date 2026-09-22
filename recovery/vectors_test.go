package recovery

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// Test vectors shared with the TypeScript implementation.
//
// # Why this file exists
//
// The device seals and splits; the services store and return. That means the
// same arithmetic is written twice, in two languages, and the two have to agree
// byte for byte or an account enrolled in a browser cannot be recovered by
// anything — including the same browser after an update.
//
// This repository has already paid for that lesson once. A client
// canonicalising an identifier one way and a chain indexing it another produced
// two animals where there was one, and x/herd/keeper/identifiers.go exists
// because of it. The difference here is that the failure would be silent and
// late: everything would look correct at enrolment, and the disagreement would
// surface only when somebody who had lost their phone tried to get their
// account back — which is precisely the moment there is no way to recover from
// a bug.
//
// So neither implementation is the reference for the other. Both are checked
// against this file.
//
// # What is pinned, and what cannot be
//
// Sealing and splitting are RANDOMISED — fresh salt, fresh nonce, fresh
// polynomial — so their outputs cannot be pinned directly. What is pinned is
// the deterministic core of each: given a fixed salt, nonce and parameters, the
// ciphertext; given fixed shares, the reconstruction; given fixed inputs, the
// digest. A round trip in each language covers the randomised half, and these
// cover the half a round trip cannot see, which is the half where two
// implementations silently differ.
//
// Regenerate with:
//
//	go test ./recovery -run TestVectors -update-vectors
//
// Regenerating is a decision, not a fix. If a vector stops matching and nothing
// was deliberately changed about the format, the code is wrong and the vector
// is right.

var updateVectors = flag.Bool("update-vectors", false,
	"rewrite testdata/vectors.json from this implementation")

const vectorsPath = "testdata/vectors.json"

type vectorFile struct {
	Note string `json:"note"`

	ArgonSalt []argonSaltVector `json:"argonSalt"`
	Seal      []sealVector      `json:"seal"`
	Combine   []combineVector   `json:"combine"`
	Digest    []digestVector    `json:"digest"`
}

type argonSaltVector struct {
	Salt   string `json:"salt"`
	Expect string `json:"expect"`
}

// sealVector pins the deterministic core of Seal: everything Seal chooses at
// random is supplied instead.
type sealVector struct {
	Password string `json:"password"`
	Secret   string `json:"secret"`
	Salt     string `json:"salt"`
	Nonce    string `json:"nonce"`
	Time     uint32 `json:"time"`
	Memory   uint32 `json:"memory"`
	Threads  uint8  `json:"threads"`
	Expect   string `json:"expect"`
}

type shareVector struct {
	X byte   `json:"x"`
	Y string `json:"y"`
}

type combineVector struct {
	Shares []shareVector `json:"shares"`
	Expect string        `json:"expect"`
}

type digestVector struct {
	Index   string      `json:"index"`
	Role    string      `json:"role"`
	PubKey  string      `json:"pubkey"`
	Salt    string      `json:"salt"`
	Nonce   string      `json:"nonce"`
	Time    uint32      `json:"time"`
	Memory  uint32      `json:"memory"`
	Threads uint8       `json:"threads"`
	Share   shareVector `json:"share"`
	Expect  string      `json:"expect"`
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// holdingOf rebuilds a Holding from a digest vector, so the digest is computed
// over the same structure the caller would pass.
func (d digestVector) holding(t *testing.T) Holding {
	t.Helper()
	return Holding{
		Role:    d.Role,
		Salt:    mustHex(t, d.Salt),
		Nonce:   mustHex(t, d.Nonce),
		Time:    d.Time,
		Memory:  d.Memory,
		Threads: d.Threads,
		Share:   Share{X: d.Share.X, Y: mustHex(t, d.Share.Y)},
	}
}

// sealCiphertext is the deterministic seal: Seal with every random choice made
// by the caller. Not exported, because a caller who supplies their own nonce is
// a caller who can repeat one.
func sealCiphertext(t *testing.T, v sealVector) []byte {
	t.Helper()
	s := &Sealed{
		Salt:    mustHex(t, v.Salt),
		Nonce:   mustHex(t, v.Nonce),
		Time:    v.Time,
		Memory:  v.Memory,
		Threads: v.Threads,
	}
	aead, err := s.aead(v.Password)
	if err != nil {
		t.Fatalf("aead: %v", err)
	}
	return aead.Seal(nil, s.Nonce, mustHex(t, v.Secret), nil)
}

func TestVectors(t *testing.T) {
	if *updateVectors {
		writeVectors(t)
		return
	}

	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("reading vectors (regenerate with -update-vectors): %v", err)
	}
	var f vectorFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parsing vectors: %v", err)
	}

	if len(f.ArgonSalt) == 0 || len(f.Seal) == 0 || len(f.Combine) == 0 || len(f.Digest) == 0 {
		t.Fatal("the vector file is missing a section; an empty section passes vacuously")
	}

	for i, v := range f.ArgonSalt {
		got := hex.EncodeToString(argonSalt(mustHex(t, v.Salt)))
		if got != v.Expect {
			t.Errorf("argonSalt[%d] = %s, want %s", i, got, v.Expect)
		}
		if len(got) != 32 {
			t.Errorf("argonSalt[%d] is %d hex chars; libsodium requires exactly 16 bytes",
				i, len(got))
		}
	}

	for i, v := range f.Seal {
		if v.Threads != 1 {
			t.Errorf("seal[%d] pins threads=%d, which no browser can reproduce", i, v.Threads)
		}
		got := hex.EncodeToString(sealCiphertext(t, v))
		if got != v.Expect {
			t.Errorf("seal[%d] = %s, want %s", i, got, v.Expect)
		}
	}

	for i, v := range f.Combine {
		shares := make([]Share, 0, len(v.Shares))
		for _, s := range v.Shares {
			shares = append(shares, Share{X: s.X, Y: mustHex(t, s.Y)})
		}
		out, err := Combine(shares)
		if err != nil {
			t.Errorf("combine[%d]: %v", i, err)
			continue
		}
		if got := hex.EncodeToString(out); got != v.Expect {
			t.Errorf("combine[%d] = %s, want %s", i, got, v.Expect)
		}
	}

	for i, v := range f.Digest {
		got := DepositDigest(v.Index, v.Role, mustHex(t, v.PubKey), v.holding(t))
		if h := hex.EncodeToString(got[:]); h != v.Expect {
			t.Errorf("digest[%d] = %s, want %s", i, h, v.Expect)
		}
	}
}

// writeVectors regenerates the file. Split out so the checking path above reads
// as what it is: an assertion, not a round trip through the generator.
func writeVectors(t *testing.T) {
	t.Helper()

	f := vectorFile{
		Note: "Shared by recovery/vectors_test.go and the TypeScript port. " +
			"Regenerate with: go test ./recovery -run TestVectors -update-vectors. " +
			"A vector that stops matching means the code changed, not the vector.",
	}

	for _, salt := range []string{
		"00000000000000000000000000000000",
		"0102030405060708090a0b0c0d0e0f10",
		"ffffffffffffffffffffffffffffffff",
	} {
		f.ArgonSalt = append(f.ArgonSalt, argonSaltVector{
			Salt:   salt,
			Expect: hex.EncodeToString(argonSalt(mustHex(t, salt))),
		})
	}

	// Few, because each one costs a 128 MiB Argon2id derivation in both
	// languages. Two is enough to catch a parameter that is ignored: they
	// differ in password, secret, salt and nonce.
	seals := []sealVector{
		{
			Password: "seven horses eating apples slowly",
			// 32 bytes, the length of a secp256k1 private key.
			Secret:  "4c9f1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7",
			Salt:    "0102030405060708090a0b0c0d0e0f10",
			Nonce:   "000102030405060708090a0b0c0d0e0f1011121314151617",
			Time:    sealTime,
			Memory:  sealMemory,
			Threads: sealThreads,
		},
		{
			Password: "une phrase de passe beaucoup plus longue",
			Secret:   "00112233445566778899aabbccddeeff",
			Salt:     "aabbccddeeff00112233445566778899",
			Nonce:    "ffeeddccbbaa99887766554433221100aabbccddeeff0011",
			Time:     sealTime,
			Memory:   sealMemory,
			Threads:  sealThreads,
		},
	}
	for _, v := range seals {
		v.Expect = hex.EncodeToString(sealCiphertext(t, v))
		f.Seal = append(f.Seal, v)
	}

	// Combine is the deterministic half of the sharing. Real shares, produced
	// once by Split and then frozen, so the vector exercises interpolation over
	// values nobody chose to be convenient.
	secret := mustHex(t, "596120726563757065726174696f6e207465737420766563746f722e2e2e2e2e")
	shares, err := Split(secret, 3, 2)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	for _, pair := range [][2]int{{0, 1}, {0, 2}, {1, 2}} {
		f.Combine = append(f.Combine, combineVector{
			Shares: []shareVector{
				{X: shares[pair[0]].X, Y: hex.EncodeToString(shares[pair[0]].Y)},
				{X: shares[pair[1]].X, Y: hex.EncodeToString(shares[pair[1]].Y)},
			},
			Expect: hex.EncodeToString(secret),
		})
	}
	// And all three, which a two-share-only implementation gets wrong.
	all := make([]shareVector, 0, 3)
	for _, s := range shares {
		all = append(all, shareVector{X: s.X, Y: hex.EncodeToString(s.Y)})
	}
	f.Combine = append(f.Combine, combineVector{Shares: all, Expect: hex.EncodeToString(secret)})

	digests := []digestVector{
		{
			Index:   "Zm9vYmFyLWJsaW5kLWluZGV4",
			Role:    RoleCustodian,
			PubKey:  "02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5",
			Salt:    "0102030405060708090a0b0c0d0e0f10",
			Nonce:   "000102030405060708090a0b0c0d0e0f1011121314151617",
			Time:    sealTime,
			Memory:  sealMemory,
			Threads: sealThreads,
			Share:   shareVector{X: 2, Y: "deadbeefcafe0123456789abcdef0123"},
		},
		{
			// Same everything, different role — the pair that proves the role is
			// actually bound rather than merely present.
			Index:   "Zm9vYmFyLWJsaW5kLWluZGV4",
			Role:    RoleRecovery,
			PubKey:  "02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5",
			Salt:    "0102030405060708090a0b0c0d0e0f10",
			Nonce:   "000102030405060708090a0b0c0d0e0f1011121314151617",
			Time:    sealTime,
			Memory:  sealMemory,
			Threads: sealThreads,
			Share:   shareVector{X: 2, Y: "deadbeefcafe0123456789abcdef0123"},
		},
	}
	for _, d := range digests {
		sum := DepositDigest(d.Index, d.Role, mustHex(t, d.PubKey), d.holding(t))
		d.Expect = hex.EncodeToString(sum[:])
		f.Digest = append(f.Digest, d)
	}

	if err := os.MkdirAll(filepath.Dir(vectorsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vectorsPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", vectorsPath)
}

// The two vectors above must not be the same digest, or the role is not bound
// and both would pass against an implementation that ignored it.
func TestTheDigestVectorsActuallyDiffer(t *testing.T) {
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Skip("no vectors yet")
	}
	var f vectorFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Digest) < 2 {
		t.Fatal("fewer than two digest vectors, so nothing is being contrasted")
	}
	if f.Digest[0].Expect == f.Digest[1].Expect {
		t.Fatal("two vectors differing only in role produced the same digest")
	}
}
