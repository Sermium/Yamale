package main

// The cross-language test vectors for the hosted ceremony.
//
// `ceremony host` serves a page that derives its own key and computes the group
// itself, in TypeScript, in a browser this program cannot see into. That is the
// whole point of it — the coordinator must not be able to reconstruct a phrase —
// but it means there are now two implementations of the derivation, the
// canonical serialisation and the fingerprint, and nothing in either language's
// compiler notices when they stop agreeing.
//
// The failure that would cause is specific and silent. A page deriving even
// slightly differently hands five custodians addresses that look right and
// control nothing, and nobody finds out until the first seizure arrives at an
// account no three people can open. A page computing the group fingerprint over
// slightly different bytes would make the read-aloud comparison fail for
// innocent reasons, which teaches five custodians to shrug at the one check
// that defeats a hostile relay.
//
// So both suites read this one file, exactly as x/paymsg/types and clients/sdk
// share testdata/vectors/confidentiality.json: the Go tests below and
// clients/ceremony/src/wire.test.ts. Editing the fixture turns both red, and
// neither can be made green on its own.
//
// Regenerate deliberately, never to make a test pass:
//
//	CEREMONY_WRITE_VECTORS=1 go test ./tools/ceremony -run TestCeremonyVectors

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/x/group"
	groupkeeper "github.com/cosmos/cosmos-sdk/x/group/keeper"
	bip39 "github.com/cosmos/go-bip39"
	"github.com/stretchr/testify/require"
)

const vectorFile = "../../testdata/vectors/ceremony.json"

type ceremonyVectors struct {
	Note string `json:"note"`

	// PolicyDerivation pins the two constants the TypeScript side cannot import
	// from the SDK. A change to either upstream moves every policy address, and
	// without them written down here the browser would keep deriving the old
	// one while Go derived the new one.
	PolicyDerivation struct {
		Module      string `json:"module"`
		TablePrefix int    `json:"group_policy_table_prefix"`
	} `json:"policy_derivation"`

	PolicyAddresses []policyAddressVector `json:"policy_addresses"`

	// Durations pin two things the browser has to do that look trivial and are
	// not: parse the Go duration string carried in the ceremony parameters, and
	// render it back out the way protobuf JSON does. The rendered form lands
	// inside the genesis fragment, which the group fingerprint covers, so a
	// voting period of "1h30m0.5s" rendered as "5400.5s" instead of "5400.500s"
	// would give the browser and the binary different fingerprints from the same
	// five submissions — the one failure the read-aloud step cannot tell apart
	// from an attack.
	Durations []durationVector `json:"duration_vectors"`

	Params            ceremonyParams `json:"params"`
	ParamsCanonicalHx string         `json:"params_canonical_hex"`
	ParamsFingerprint string         `json:"params_fingerprint"`

	Custodians []custodianVector `json:"custodians"`

	Group groupVector `json:"group"`

	Attestations []attestationVector `json:"attestations"`
}

type policyAddressVector struct {
	Seq     uint64 `json:"seq"`
	Address string `json:"address"`
}

type durationVector struct {
	Duration    string `json:"duration"`
	WindowsJSON string `json:"windows_json"`
}

// custodianVector carries a phrase, which is the one place in this repository
// where that is correct: these five are fixed test values with no assets behind
// them, and the whole purpose of the file is to prove that a browser handed this
// phrase derives the address Go derives.
type custodianVector struct {
	Name        string `json:"name"`
	Phrase      string `json:"phrase"`
	Index       uint32 `json:"index"`
	HDPath      string `json:"hd_path"`
	Address     string `json:"address"`
	PubKey      string `json:"pubkey_base64"`
	Fingerprint string `json:"fingerprint"`
	GeneratedAt string `json:"generated_at"`

	PossessionMessageHex string `json:"possession_message_hex"`
	PossessionSignature  string `json:"possession_signature_base64"`
}

type groupVector struct {
	ComputedAt       string `json:"computed_at"`
	PolicyAddress    string `json:"policy_address"`
	GenesisJSON      string `json:"genesis_json"`
	ConstitutionJSON string `json:"constitution_json"`
	CanonicalHex     string `json:"canonical_hex"`
	Fingerprint      string `json:"fingerprint"`
}

type attestationVector struct {
	Attestation  attestation `json:"attestation"`
	CanonicalHex string      `json:"canonical_hex"`
	Signature    string      `json:"signature_base64"`
}

// fixturePhrase values are generated from fixed entropy rather than written out, so the
// twenty-four words in the fixture are demonstrably real BIP-39 phrases with
// valid checksums rather than something a hand-edit produced.
func fixturePhrase(i int) string {
	entropy := make([]byte, entropyBits/8)
	for j := range entropy {
		entropy[j] = byte(i*32 + j)
	}
	phrase, err := bip39.NewMnemonic(entropy)
	if err != nil {
		panic(err)
	}
	return phrase
}

// vectorNames deliberately include a name with an ampersand, an angle bracket
// and a non-ASCII letter.
//
// The group fingerprint covers the raw bytes of the genesis fragment, and those
// bytes come from Go's JSON encoder, which escapes the three HTML-significant
// characters into their backslash-u escapes while leaving other non-ASCII
// alone. A TypeScript side using
// JSON.stringify does none of that. Without a name in the fixture that exercises
// it, the two would agree on every plain-ASCII roster and diverge the first time
// a custodian's institution had an ampersand in it — on the day, over a phone
// call, with no way to tell an innocent mismatch from an attack.
var vectorNames = []string{
	"A. Okafor",
	"Chipo & Sons <Trust>",
	"Naledi Ngũgĩ",
	"Bank of Yamale",
	"J. Mwangi",
}

var vectorTimes = []string{
	"2026-03-02T09:15:00Z",
	"2026-03-02T09:41:00Z",
	"2026-03-02T10:03:00Z",
	"2026-03-02T10:27:00Z",
	"2026-03-02T11:02:00Z",
}

func buildVectors(t *testing.T) ceremonyVectors {
	t.Helper()

	v := ceremonyVectors{
		Note: "Cross-language vectors for the hosted key ceremony. Read by tools/ceremony " +
			"(vectors_test.go) and clients/ceremony (src/wire.test.ts): the browser derives the " +
			"keys and computes the group fingerprint itself, so both implementations are pinned " +
			"to this one file. Regenerate with CEREMONY_WRITE_VECTORS=1 go test ./tools/ceremony " +
			"-run TestCeremonyVectors, and only when you mean to change what five custodians read " +
			"aloud to each other.",
	}
	v.PolicyDerivation.Module = group.ModuleName
	v.PolicyDerivation.TablePrefix = int(groupkeeper.GroupPolicyTablePrefix)

	for _, seq := range []uint64{0, 1, 2, 4096} {
		address, err := policyAddress(seq)
		require.NoError(t, err)
		v.PolicyAddresses = append(v.PolicyAddresses, policyAddressVector{Seq: seq, Address: address})
	}

	registry := codectypes.NewInterfaceRegistry()
	group.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	for _, spec := range []string{"168h0m0s", "1h30m0.5s", "0s", "72h", "1.000000001s", "336h0m0s"} {
		d, err := time.ParseDuration(spec)
		require.NoError(t, err)
		windows, err := cdc.MarshalJSON(&group.DecisionPolicyWindows{VotingPeriod: d})
		require.NoError(t, err)
		v.Durations = append(v.Durations, durationVector{Duration: spec, WindowsJSON: string(windows)})
	}

	v.Params = ceremonyParams{
		ID:           "K4T9RM-2QWXVZ-8H0PBN-5CJDGF",
		Name:         "Yamale foundation, hosted rehearsal",
		ChainID:      "yamale-1",
		Threshold:    3,
		Custodians:   append([]string(nil), vectorNames...),
		PolicySeq:    1,
		VotingPeriod: "168h0m0s",
	}
	require.NoError(t, v.Params.validate())
	v.ParamsCanonicalHx = hex.EncodeToString(v.Params.canonical())
	v.ParamsFingerprint = v.Params.fingerprint()

	submissions := make([]submission, 0, len(vectorNames))
	keys := make([]*secp256k1.PrivKey, 0, len(vectorNames))
	for i, name := range vectorNames {
		phrase := fixturePhrase(i)
		s, err := secretFromInput(phrase)
		require.NoError(t, err)
		priv, path, err := s.derive(0)
		require.NoError(t, err)
		generated, err := time.Parse(time.RFC3339, vectorTimes[i])
		require.NoError(t, err)

		id, err := identityOf(name, roleCustodian, priv, path, generated)
		require.NoError(t, err)
		sub, err := signSubmission(v.Params.ID, id, priv)
		require.NoError(t, err)

		v.Custodians = append(v.Custodians, custodianVector{
			Name:                 name,
			Phrase:               phrase,
			Index:                0,
			HDPath:               path,
			Address:              id.Address,
			PubKey:               id.PubKey.Key,
			Fingerprint:          id.Fingerprint,
			GeneratedAt:          id.GeneratedAt,
			PossessionMessageHex: hex.EncodeToString(possessionMessage(v.Params.ID, id)),
			PossessionSignature:  sub.Possession,
		})
		submissions = append(submissions, sub)
		keys = append(keys, priv)
	}

	a, err := assembleGroup(v.Params, submissions)
	require.NoError(t, err)
	v.Group = groupVector{
		ComputedAt:       a.ComputedAt,
		PolicyAddress:    a.PolicyAddress,
		GenesisJSON:      string(a.Genesis),
		ConstitutionJSON: string(a.Constitution),
		CanonicalHex:     hex.EncodeToString(a.canonical()),
		Fingerprint:      a.Fingerprint,
	}

	for i, name := range vectorNames {
		var own identity
		for _, custodian := range a.Custodians {
			if custodian.Name == name {
				own = custodian
			}
		}
		att := attestation{
			CeremonyID:            v.Params.ID,
			Name:                  name,
			Address:               own.Address,
			GroupFingerprint:      a.Fingerprint,
			PolicyAddress:         a.PolicyAddress,
			TranscriptionVerified: true,
			RestoreDrillPassed:    true,
			EnvelopeSealed:        i%2 == 0,
			Virtualised:           false,
			SignedAt:              "2026-03-02T11:30:00Z",
		}
		signed, err := signAttestation(att, keys[i])
		require.NoError(t, err)
		v.Attestations = append(v.Attestations, attestationVector{
			Attestation:  att,
			CanonicalHex: hex.EncodeToString(att.canonical()),
			Signature:    signed.Signature,
		})
	}
	return v
}

func loadCeremonyVectors(t *testing.T) ceremonyVectors {
	t.Helper()
	blob, err := os.ReadFile(filepath.Clean(vectorFile))
	require.NoError(t, err, "the shared cross-language vectors must be readable from this package")

	var v ceremonyVectors
	decoder := json.NewDecoder(bytes.NewReader(blob))
	// Strict, so a field renamed on one side of the fixture is a failure here
	// rather than a zero value that quietly passes every comparison below.
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&v))

	// Emptiness checks, because a fixture that resolved to the wrong path or
	// failed to populate would otherwise let every case below iterate over
	// nothing and pass vacuously — which is the exact failure a shared file
	// exists to prevent.
	require.NotEmpty(t, v.Custodians, "no custodian vectors: every case below would pass vacuously")
	require.NotEmpty(t, v.PolicyAddresses, "no policy address vectors")
	require.NotEmpty(t, v.Attestations, "no attestation vectors")
	require.NotEmpty(t, v.Group.Fingerprint, "no group fingerprint")
	require.NotEmpty(t, v.Durations, "no duration vectors")
	return v
}

// TestCeremonyVectors is both the generator and the check.
//
// One function rather than two, so the bytes written by the generator are the
// bytes the assertions then read: a generator that produced the fixture through
// a different code path from the one under test would agree with itself and
// prove nothing.
func TestCeremonyVectors(t *testing.T) {
	configureAddresses()

	built := buildVectors(t)

	if os.Getenv("CEREMONY_WRITE_VECTORS") != "" {
		blob, err := json.MarshalIndent(built, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Clean(vectorFile), append(blob, '\n'), 0o644))
		t.Log("wrote", vectorFile)
	}

	want := loadCeremonyVectors(t)

	require.Equal(t, want.PolicyDerivation.Module, built.PolicyDerivation.Module,
		"x/group's module name has changed; every policy address this ceremony has ever derived moves with it")
	require.Equal(t, want.PolicyDerivation.TablePrefix, built.PolicyDerivation.TablePrefix,
		"x/group's policy table prefix has changed upstream; the address in every genesis this tool produced is now wrong")

	require.Equal(t, want.PolicyAddresses, built.PolicyAddresses)
	require.Equal(t, want.Durations, built.Durations,
		"protobuf JSON's duration rendering has moved; every genesis fragment this tool has produced now hashes differently")
	require.Equal(t, want.ParamsCanonicalHx, built.ParamsCanonicalHx,
		"the params canonical encoding has moved, so the fingerprint five custodians read aloud before generating is not the one in the fixture")
	require.Equal(t, want.ParamsFingerprint, built.ParamsFingerprint)

	require.Equal(t, len(want.Custodians), len(built.Custodians))
	for i := range want.Custodians {
		require.Equal(t, want.Custodians[i], built.Custodians[i],
			"custodian vector %d has drifted: a phrase that derives a different address is five keys that look right and control nothing", i)
	}

	require.Equal(t, want.Group, built.Group,
		"the assembled group has drifted from the fixture; the browser and this binary would compute different fingerprints from the same five submissions")
	require.Equal(t, want.Attestations, built.Attestations)
}

// TestVectorSignaturesVerifyWithTheProductionPath checks the fixture's
// signatures through the same functions a submission from a browser goes
// through, rather than only comparing bytes.
//
// Byte equality alone would be satisfied by a fixture full of values this test
// file happened to produce. verifySubmission and verifyAttestation are the
// functions the host actually gates on, so they are what the fixture has to
// satisfy.
func TestVectorSignaturesVerifyWithTheProductionPath(t *testing.T) {
	configureAddresses()
	v := loadCeremonyVectors(t)

	subs := make([]submission, 0, len(v.Custodians))
	for _, c := range v.Custodians {
		sub := submission{
			CeremonyID: v.Params.ID,
			Identity: identity{
				Name:        c.Name,
				Role:        roleCustodian,
				Address:     c.Address,
				PubKey:      pubKeyJSON{Type: secp256k1PubKeyType, Key: c.PubKey},
				Fingerprint: c.Fingerprint,
				HDPath:      c.HDPath,
				GeneratedAt: c.GeneratedAt,
			},
			Possession: c.PossessionSignature,
		}
		derived, err := verifySubmission(v.Params, sub)
		require.NoError(t, err, "%s's possession signature from the fixture must verify", c.Name)
		require.Equal(t, c.Address, derived.Address)
		require.Equal(t, c.Fingerprint, derived.Fingerprint)

		// The negative half, and it is not decoration. A test that only asserted
		// the fixture's signatures verify would pass just as happily against a
		// verifySubmission that had stopped checking signatures at all — which is
		// exactly what a mutation run found. Flipping one bit of the signature
		// has to be a refusal, or "verified" means nothing.
		corrupted := sub
		signatureBytes, err := base64.StdEncoding.DecodeString(sub.Possession)
		require.NoError(t, err)
		signatureBytes[0] ^= 0x01
		corrupted.Possession = base64.StdEncoding.EncodeToString(signatureBytes)
		_, err = verifySubmission(v.Params, corrupted)
		require.ErrorContains(t, err, "proof of possession does not verify",
			"%s's submission verified with a corrupted signature", c.Name)

		subs = append(subs, sub)
	}

	a, err := assembleGroup(v.Params, subs)
	require.NoError(t, err)
	require.Equal(t, v.Group.Fingerprint, a.Fingerprint)
	require.Equal(t, v.Group.PolicyAddress, a.PolicyAddress)
	require.JSONEq(t, v.Group.GenesisJSON, string(a.Genesis))
	require.Equal(t, v.Group.GenesisJSON, string(a.Genesis),
		"byte-for-byte, not just equivalent JSON: the group fingerprint is taken over these exact bytes")

	for _, av := range v.Attestations {
		signature, err := base64.StdEncoding.DecodeString(av.Signature)
		require.NoError(t, err)
		var pub string
		for _, c := range v.Custodians {
			if c.Name == av.Attestation.Name {
				pub = c.PubKey
			}
		}
		require.NotEmpty(t, pub, "attestation for %q has no matching custodian vector", av.Attestation.Name)
		require.NoError(t, verifyAttestation(signedAttestation{
			Attestation: av.Attestation,
			PubKey:      pubKeyJSON{Type: secp256k1PubKeyType, Key: pub},
			Signature:   base64.StdEncoding.EncodeToString(signature),
		}), "%s's attestation from the fixture must verify", av.Attestation.Name)
	}
}

// TestVectorNamesExerciseJSONEscaping fails if somebody simplifies the roster.
//
// The escaping case is not decoration: Go's encoder turns & < > into &
// < > inside the genesis fragment, and the group fingerprint covers
// those bytes. A roster of plain ASCII names would let a TypeScript
// implementation using JSON.stringify pass every test here and then disagree
// with Go the first time a custodian's name had an ampersand in it.
func TestVectorNamesExerciseJSONEscaping(t *testing.T) {
	v := loadCeremonyVectors(t)
	joined := fmt.Sprint(v.Params.Custodians)
	for _, needed := range []string{"&", "<", ">"} {
		require.Contains(t, joined, needed,
			"the vector roster must keep a name containing %q so the JSON escaping stays pinned", needed)
	}
	require.Contains(t, v.Group.GenesisJSON, "\\u0026",
		"the genesis fragment in the fixture no longer exercises Go's HTML escaping")
	nonASCII := false
	for _, r := range joined {
		if r > 127 {
			nonASCII = true
		}
	}
	require.True(t, nonASCII, "the vector roster must keep a non-ASCII name")
}
