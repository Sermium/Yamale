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
	"strings"
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

	// RoleNames pins the chain's role enum for the browser, which cannot import
	// it. Same arrangement as PolicyDerivation above: the Go side generates it
	// from aliastypes.Role_name filtered by ValidRole, and the TypeScript suite
	// asserts its own hard-coded table equals this array. A role added to the
	// chain and not to the page would otherwise be a role the coordinator's form
	// silently refused; one removed from the chain and not from the page would be
	// a ceremony whose super users read a fingerprint aloud for authority the
	// chain will never grant.
	RoleNames []string `json:"role_names"`

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

	// The country-office ceremony, pinned separately and in full.
	//
	// Without it only the foundation path is covered, and the foundation path is
	// the one with a track record: its canonical bytes are the old bytes plus a
	// fixed empty tail, so it would keep agreeing across the two languages even if
	// the office encoding had been implemented differently on each side. The
	// office path is the new one, it is the one where a nil and an empty string
	// have to produce identical bytes, and it is therefore exactly the one that
	// can diverge silently — with the consequence that a country's super users and
	// their coordinator read different fingerprints and cannot tell that apart
	// from an attack.
	OfficeParams            ceremonyParams `json:"office_params"`
	OfficeParamsCanonicalHx string         `json:"office_params_canonical_hex"`
	OfficeParamsFingerprint string         `json:"office_params_fingerprint"`
	OfficeGroup             groupVector    `json:"office_group"`

	// The foundation-administrator ceremony, pinned for the same reason the office
	// is and with one extra job. The administrators marker is appended to the
	// canonical bytes ONLY when set, so these vectors are what prove two things at
	// once: that the two languages agree on the marker, and that the FOUNDATION's
	// bytes above did not move when it was added. The second is the one that
	// matters to somebody holding a paper record from a ceremony already held.
	AdminParams            ceremonyParams `json:"administrators_params"`
	AdminParamsCanonicalHx string         `json:"administrators_params_canonical_hex"`
	AdminParamsFingerprint string         `json:"administrators_params_fingerprint"`
	AdminGroup             groupVector    `json:"administrators_group"`
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
	ComputedAt    string `json:"computed_at"`
	PolicyAddress string `json:"policy_address"`
	// Label, Metadata and PolicyMetadata are the three strings recorded on chain
	// as what this group is. Pinned because the first two are inside the genesis
	// fragment the group fingerprint covers, and because they are the field a
	// human reads to find out whose group they are looking at — a country office
	// recorded as "Yamale foundation" is a lie in the one place nobody checks.
	Label          string `json:"label"`
	Metadata       string `json:"metadata"`
	PolicyMetadata string `json:"policy_metadata"`
	GenesisJSON    string `json:"genesis_json"`
	// ConstitutionJSON is the empty string for a country office. That is the
	// value under test, not an omission: Go leaves the field nil and the browser
	// holds "", and canonBytes has to produce the same four zero bytes from both
	// or the two languages compute different group fingerprints.
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
	v.RoleNames = ceremonyRoleNames()

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
	v.Group = groupVectorOf(a, v.Params)

	// The office ceremony, over the first three of the same phrases.
	//
	// A 2-of-3 rather than a 3-of-5, because that is the shape a country office
	// actually is, and over the same names so that the office's genesis fragment
	// exercises Go's HTML escaping the way the foundation's does. Two roles, given
	// in the order a coordinator would type them rather than sorted, so the
	// fixture proves the encoding sorts them.
	v.OfficeParams = ceremonyParams{
		ID:           "9WPXTM-3KZ4QC-7HB0VN-2RJDGS",
		Name:         "Senegal payments authority",
		ChainID:      "yamale-1",
		Threshold:    2,
		Custodians:   append([]string(nil), vectorNames[:3]...),
		PolicySeq:    7,
		VotingPeriod: "72h0m0s",
		Office: &officeParams{
			Country: "SN",
			Roles:   []string{"ROLE_PAYMENTS_AUTHORITY", "ROLE_ENFORCEMENT_AUTHORITY"},
		},
	}
	require.NoError(t, v.OfficeParams.validate())
	v.OfficeParamsCanonicalHx = hex.EncodeToString(v.OfficeParams.canonical())
	v.OfficeParamsFingerprint = v.OfficeParams.fingerprint()

	officeSubmissions := make([]submission, 0, 3)
	for i := range v.OfficeParams.Custodians {
		phrase := fixturePhrase(i)
		s, err := secretFromInput(phrase)
		require.NoError(t, err)
		priv, path, err := s.derive(0)
		require.NoError(t, err)
		generated, err := time.Parse(time.RFC3339, vectorTimes[i])
		require.NoError(t, err)
		id, err := identityOf(vectorNames[i], roleCustodian, priv, path, generated)
		require.NoError(t, err)
		sub, err := signSubmission(v.OfficeParams.ID, id, priv)
		require.NoError(t, err)
		officeSubmissions = append(officeSubmissions, sub)
	}

	office, err := assembleGroup(v.OfficeParams, officeSubmissions)
	require.NoError(t, err)
	// The one assertion in the generator itself, because a fixture written from a
	// run that HAD produced the fragment would pin the dangerous document rather
	// than its absence.
	require.Nil(t, office.Constitution,
		"an office's assembled group must carry no constitutional invariants fragment")
	v.OfficeGroup = groupVectorOf(office, v.OfficeParams)

	// The foundation-administrator ceremony, over the first four phrases.
	//
	// A 3-of-4 rather than a 3-of-5, so the shape differs from the foundation's
	// and a vector accidentally generated from the wrong parameters would not
	// happen to match. Deliberately the same NAME prefix as the foundation's
	// ceremony, because that is the case worth pinning: the label has to come out
	// as "… (foundation administrators)" and not as the bare foundation constant,
	// and the canonical bytes have to differ from an otherwise identical
	// foundation ceremony's.
	v.AdminParams = ceremonyParams{
		ID:             "5NQ8HD-7XVBKR-2WCT0M-9JZFPA",
		Name:           "Yamale foundation administrators",
		ChainID:        "yamale-1",
		Threshold:      3,
		Custodians:     append([]string(nil), vectorNames[:4]...),
		PolicySeq:      11,
		VotingPeriod:   "168h0m0s",
		Administrators: true,
	}
	require.NoError(t, v.AdminParams.validate())
	v.AdminParamsCanonicalHx = hex.EncodeToString(v.AdminParams.canonical())
	v.AdminParamsFingerprint = v.AdminParams.fingerprint()

	adminSubmissions := make([]submission, 0, 4)
	for i := range v.AdminParams.Custodians {
		phrase := fixturePhrase(i)
		s, err := secretFromInput(phrase)
		require.NoError(t, err)
		priv, path, err := s.derive(0)
		require.NoError(t, err)
		generated, err := time.Parse(time.RFC3339, vectorTimes[i])
		require.NoError(t, err)
		id, err := identityOf(vectorNames[i], roleCustodian, priv, path, generated)
		require.NoError(t, err)
		sub, err := signSubmission(v.AdminParams.ID, id, priv)
		require.NoError(t, err)
		adminSubmissions = append(adminSubmissions, sub)
	}

	admins, err := assembleGroup(v.AdminParams, adminSubmissions)
	require.NoError(t, err)
	// Asserted in the generator for the office's reason, and it matters more here.
	// A constitutional fragment produced for this group would say "send every
	// seized asset on this chain to the foundation administrators" — and the name
	// in it already contains the word "foundation", so it is the one such document
	// a person might splice into a genesis without blinking.
	require.Nil(t, admins.Constitution,
		"an administrator group's assembled group must carry no constitutional invariants fragment")
	v.AdminGroup = groupVectorOf(admins, v.AdminParams)

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

// groupVectorOf records one assembled group, both paths through the same
// function so that the office case cannot be recorded differently from the
// foundation one.
func groupVectorOf(a assembled, params ceremonyParams) groupVector {
	label := groupLabel(params)
	return groupVector{
		ComputedAt:       a.ComputedAt,
		PolicyAddress:    a.PolicyAddress,
		Label:            label,
		Metadata:         groupMetadata(label, a.Custodians, params.Threshold),
		PolicyMetadata:   fmt.Sprintf("%s %d-of-%d", label, params.Threshold, len(a.Custodians)),
		GenesisJSON:      string(a.Genesis),
		ConstitutionJSON: string(a.Constitution),
		CanonicalHex:     hex.EncodeToString(a.canonical()),
		Fingerprint:      a.Fingerprint,
	}
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
	require.NotEmpty(t, v.RoleNames, "no role names: the browser's table would be pinned against nothing")
	require.NotEmpty(t, v.OfficeGroup.Fingerprint, "no office group fingerprint: the country path would be unpinned")
	require.NotNil(t, v.OfficeParams.Office, "the office ceremony in the fixture has no office block")
	require.NotEmpty(t, v.AdminGroup.Fingerprint,
		"no administrator group fingerprint: the appointment path would be unpinned")
	require.True(t, v.AdminParams.Administrators,
		"the administrator ceremony in the fixture is not marked as one, so it pins the foundation's bytes twice")
	return v
}

// TestTheFoundationsCanonicalBytesDidNotMove is the compatibility claim, asserted.
//
// The administrators marker is appended to the params canonical encoding only
// when it is set, and paramsDomain deliberately stayed at v2 rather than being
// bumped to v3 the way adding the office block did. The whole justification for
// that is this: the foundation's params fingerprint is on paper, in ink, in five
// sealed envelopes from a ceremony that has already happened, and a bump would
// leave anybody checking an old record unable to tell whether the parameters had
// changed or the tool had.
//
// So the claim needs a test, and it needs to be a test of the recorded bytes
// rather than of the current code agreeing with itself. The hex below is the
// value committed in the fixture before the administrator path existed. If a
// future change to the encoding moves it, this fails and names the reason —
// which is the moment to bump the domain deliberately rather than to discover
// later that every paper record has become uncheckable.
func TestTheFoundationsCanonicalBytesDidNotMove(t *testing.T) {
	configureAddresses()
	v := loadCeremonyVectors(t)

	const beforeAdministrators = "0000001979616d616c652d636572656d6f6e792d706172616d732d76320000001b4b345439524d2d3251" +
		"5758565a2d38483050424e2d35434a4447460000002359616d616c6520666f756e646174696f6e2c20686f7374656420726568" +
		"6561727361"
	require.True(t, strings.HasPrefix(v.ParamsCanonicalHx, beforeAdministrators),
		"the foundation's params canonical bytes have moved. Adding a field that always encodes does that, and "+
			"it invalidates every params fingerprint written on paper at a ceremony already held. If the move is "+
			"deliberate, bump paramsDomain so old values fail loudly instead of silently disagreeing")

	// The tail, specifically: a foundation ceremony must still end with the empty
	// office country and a zero role count and NOTHING after it. An eight-byte
	// tail, not a marker.
	require.True(t, strings.HasSuffix(v.ParamsCanonicalHx, "0000000000000000"),
		"a foundation ceremony's canonical bytes must end with the empty office block and nothing else")

	// And the administrator ceremony's must end with the marker, so the two are
	// genuinely distinguishable rather than accidentally equal.
	marker := hex.EncodeToString(canonField(nil, administratorsMarker))
	require.True(t, strings.HasSuffix(v.AdminParamsCanonicalHx, marker),
		"an administrator ceremony's canonical bytes must end with the administrators marker")
	require.NotEqual(t, v.ParamsFingerprint, v.AdminParamsFingerprint)
}

// TestAnAdministratorCeremonyIsNotTheFoundation pins the two facts a custodian
// could not otherwise check by looking at the page.
//
// The label, because it is recorded on chain permanently and is the one field a
// human reads to find out what a group is — and on this chain the foundation
// already exists, so two groups both called "Yamale foundation" would be
// indistinguishable in exactly the place nobody thinks to look.
//
// And the absence of the genesis and constitution documents, because an
// administrator group is created by a transaction on a running chain. A fragment
// naming it the destination of every seized asset is the most dangerous file this
// tool could produce, and its name already contains the word "foundation".
func TestAnAdministratorCeremonyIsNotTheFoundation(t *testing.T) {
	configureAddresses()
	v := loadCeremonyVectors(t)

	// Recomputed from the parameters, not just read out of the fixture. Reading it
	// only proves the fixture says what it says; a mutation that made groupLabel
	// fall back to the foundation constant survived exactly that assertion.
	require.Equal(t, v.AdminGroup.Label, groupLabel(v.AdminParams),
		"groupLabel no longer produces the label recorded in the fixture")
	require.Equal(t, "Yamale foundation administrators (foundation administrators)", v.AdminGroup.Label)
	require.NotEqual(t, foundationLabel, v.AdminGroup.Label)
	require.NotEqual(t, foundationLabel, groupLabel(v.AdminParams),
		"an administrator group must not be recorded on chain as the foundation")
	require.Contains(t, v.AdminGroup.Metadata, "foundation administrators")
	require.Empty(t, v.AdminGroup.ConstitutionJSON,
		"an administrator group must carry no constitutional invariants fragment")

	// Same parameters but for the marker: the fingerprint everybody reads aloud
	// has to differ, or a coordinator could take keys generated for the foundation
	// and stand up an administrator group with nothing any custodian saw saying so.
	asFoundation := v.AdminParams
	asFoundation.Administrators = false
	require.NotEqual(t, v.AdminParams.fingerprint(), asFoundation.fingerprint())
	require.Equal(t, foundationLabel, groupLabel(asFoundation))
}

// TestACeremonyCannotBeBothAnOfficeAndAnAdministrator is the contradiction.
//
// An administrator's authority is chain-wide and its identifier carries the code
// that marks the ABSENCE of a national perimeter; an office holds authority inside
// one. Refused rather than resolved, because resolving it would mean this tool
// deciding which of the two a room full of people meant.
func TestACeremonyCannotBeBothAnOfficeAndAnAdministrator(t *testing.T) {
	configureAddresses()
	p := ceremonyParams{
		ID: "5NQ8HD-7XVBKR-2WCT0M-9JZFPA", Name: "Confused", ChainID: "yamale-1",
		Threshold: 2, Custodians: []string{"A", "B", "C"}, PolicySeq: 1, VotingPeriod: "168h0m0s",
		Office:         &officeParams{Country: "SN", Roles: []string{"ROLE_PAYMENTS_AUTHORITY"}},
		Administrators: true,
	}
	err := p.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "opposites")

	// And each alone is fine, so the refusal is about the pair and not about
	// either field.
	office := p
	office.Administrators = false
	require.NoError(t, office.validate())
	admin := p
	admin.Office = nil
	require.NoError(t, admin.validate())
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

	require.Equal(t, want.RoleNames, built.RoleNames,
		"the chain's role enum has moved; the browser's own table is pinned against this array and would now be wrong")

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

	require.Equal(t, want.OfficeParams, built.OfficeParams)
	require.Equal(t, want.OfficeParamsCanonicalHx, built.OfficeParamsCanonicalHx,
		"the office params canonical encoding has moved, so the fingerprint a country's super users read aloud before generating is not the one in the fixture")
	require.Equal(t, want.OfficeParamsFingerprint, built.OfficeParamsFingerprint)
	require.Equal(t, want.OfficeGroup, built.OfficeGroup,
		"the office group has drifted; a country's super users and their coordinator would compute different fingerprints from the same submissions")

	// The administrator vectors, compared as well as generated.
	//
	// A mutation pass is why these four lines exist. The fields and the generator
	// went in and the comparison did not, so the fixture was being WRITTEN and
	// never CHECKED — and the mutation that made groupLabel fall back to the bare
	// foundation constant for an administrator ceremony survived the whole suite.
	// That is the exact failure the label exists to prevent: a group recorded on
	// chain as "Yamale foundation" when it is not, in the one field a human reads
	// to find out what a group is, on a chain where the real foundation already
	// exists.
	require.Equal(t, want.AdminParams, built.AdminParams)
	require.Equal(t, want.AdminParamsCanonicalHx, built.AdminParamsCanonicalHx,
		"the administrator params canonical encoding has moved, so the fingerprint the custodians read aloud before generating is not the one in the fixture — and the foundation's own bytes may have moved with it")
	require.Equal(t, want.AdminParamsFingerprint, built.AdminParamsFingerprint)
	require.Equal(t, want.AdminGroup, built.AdminGroup,
		"the administrator group has drifted; its label, metadata and fingerprint are what distinguish it from the foundation's own group")
}

// TestTheTwoPathsAreDistinguishable is the check that the office block actually
// reaches everything it is supposed to reach.
//
// Every assertion here would still pass if officeParams were carried around and
// then dropped before the digest — which is precisely the bug that would let a
// coordinator take keys generated "for Senegal" and use them for an office
// granted authority over somewhere else. So each value that is supposed to depend
// on the office is asserted to DIFFER from the foundation's, and the constitution
// is asserted to be absent rather than merely different.
func TestTheTwoPathsAreDistinguishable(t *testing.T) {
	v := loadCeremonyVectors(t)

	require.NotEqual(t, v.ParamsCanonicalHx, v.OfficeParamsCanonicalHx)
	require.NotEqual(t, v.ParamsFingerprint, v.OfficeParamsFingerprint)
	require.NotEqual(t, v.Group.Fingerprint, v.OfficeGroup.Fingerprint)

	require.Equal(t, "Yamale foundation", v.Group.Label,
		"the foundation's label must not move: its metadata bytes are inside a fingerprint five custodians have read aloud")
	require.Contains(t, v.Group.Metadata, "Yamale foundation, 3 of 5: ")
	require.Equal(t, "Senegal payments authority (SN)", v.OfficeGroup.Label)
	require.Contains(t, v.OfficeGroup.Metadata, "Senegal payments authority (SN), 2 of 3: ")
	require.NotContains(t, v.OfficeGroup.Metadata, "Yamale foundation",
		"a country office recorded as the foundation is a lie in the one field a human reads to find out what a group is")
	require.NotContains(t, v.OfficeGroup.PolicyMetadata, "Yamale foundation")
	require.Contains(t, v.OfficeGroup.GenesisJSON, "Senegal payments authority (SN)",
		"the label has to be inside the genesis fragment, which is what the group fingerprint covers")

	require.NotEmpty(t, v.Group.ConstitutionJSON,
		"the foundation still needs its constitutional invariants fragment")
	require.Empty(t, v.OfficeGroup.ConstitutionJSON,
		"an office must not be handed a ready-to-splice document saying every seized asset on the chain goes to it")

	// The nil-versus-empty equivalence, asserted on the bytes rather than trusted.
	//
	// Go leaves assembled.Constitution nil and the browser holds "". canonBytes
	// has to turn both into the same four zero bytes, and this is the single most
	// likely place for the two languages to part company — a null on one side and
	// a "null" or "{}" on the other would give a country's super users a different
	// group fingerprint from their coordinator's, which is the one failure the
	// read-aloud step cannot tell apart from an attack.
	raw, err := hex.DecodeString(v.OfficeGroup.CanonicalHex)
	require.NoError(t, err)
	require.Equal(t, []byte{0, 0, 0, 0}, raw[len(raw)-4:],
		"an office's canonical bytes must end in a bare zero-length constitution")
	require.Equal(t, canonBytes(nil, nil), []byte{0, 0, 0, 0},
		"canonBytes must encode nil as four zero bytes, which is what the browser's empty string produces")
	require.Equal(t, canonBytes(nil, nil), canonBytes(nil, []byte("")),
		"a nil constitution and an empty one must encode identically, or Go and the browser diverge")
}

// TestOfficeParametersRefuseWhatTheChainWouldRefuse checks the office half of
// validate() at the boundary, not in the middle.
//
// Each of these is a value that reaches the chain and is refused there, after the
// super users have generated keys, read a fingerprint aloud and signed
// attestations. Refusing at setup is the difference between a coordinator
// retyping a field and a country's whole ceremony being held again.
func TestOfficeParametersRefuseWhatTheChainWouldRefuse(t *testing.T) {
	base := func() ceremonyParams {
		return ceremonyParams{
			ID:           "9WPXTM-3KZ4QC-7HB0VN-2RJDGS",
			Name:         "Senegal payments authority",
			ChainID:      "yamale-1",
			Threshold:    2,
			Custodians:   []string{"A", "B", "C"},
			PolicySeq:    7,
			VotingPeriod: "72h0m0s",
			Office:       &officeParams{Country: "SN", Roles: []string{"ROLE_PAYMENTS_AUTHORITY"}},
		}
	}
	require.NoError(t, base().validate())

	// No office at all is the foundation ceremony, and it must stay legal.
	foundation := base()
	foundation.Office = nil
	require.NoError(t, foundation.validate())

	for _, tc := range []struct {
		name    string
		office  officeParams
		message string
	}{
		{"lowercase country", officeParams{Country: "sn", Roles: []string{"ROLE_SUPERVISOR"}}, "two uppercase letters"},
		{"chain-wide", officeParams{Country: "*", Roles: []string{"ROLE_SUPERVISOR"}}, "chain-wide scope"},
		{"foundation code", officeParams{Country: "ZZ", Roles: []string{"ROLE_SUPERVISOR"}}, "ABSENCE of a national perimeter"},
		{"unassigned", officeParams{Country: "QK", Roles: []string{"ROLE_SUPERVISOR"}}, "not an assigned ISO 3166-1"},
		{"one letter", officeParams{Country: "S", Roles: []string{"ROLE_SUPERVISOR"}}, "exactly two uppercase letters"},
		{"three letters", officeParams{Country: "SEN", Roles: []string{"ROLE_SUPERVISOR"}}, "exactly two uppercase letters"},
		{"no roles", officeParams{Country: "SN"}, "holds no roles"},
		{"unknown role", officeParams{Country: "SN", Roles: []string{"ROLE_TREASURER"}}, "not a role this chain has"},
		{"unspecified role", officeParams{Country: "SN", Roles: []string{"ROLE_UNSPECIFIED"}}, "unset default"},
		{"lowercase role", officeParams{Country: "SN", Roles: []string{"role_supervisor"}}, "not written the way this chain spells it"},
		{
			"duplicate role",
			officeParams{Country: "SN", Roles: []string{"ROLE_SUPERVISOR", "ROLE_SUPERVISOR"}},
			"listed twice",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := base()
			office := tc.office
			params.Office = &office
			err := params.validate()
			require.Error(t, err, "%+v was accepted", tc.office)
			require.ErrorContains(t, err, tc.message)
		})
	}
}

// TestTheOfficeEncodingDependsOnTheSetNotTheOrder is the reason the roles are
// sorted on a copy.
//
// Two coordinators typing the same two roles in different orders must produce the
// same fingerprint, or the read-aloud check fails for a reason that has nothing
// to do with an attacker — which is how five people learn to shrug at it.
func TestTheOfficeEncodingDependsOnTheSetNotTheOrder(t *testing.T) {
	v := loadCeremonyVectors(t)
	require.NotNil(t, v.OfficeParams.Office)

	reversed := v.OfficeParams
	roles := append([]string(nil), v.OfficeParams.Office.Roles...)
	for i, j := 0, len(roles)-1; i < j; i, j = i+1, j-1 {
		roles[i], roles[j] = roles[j], roles[i]
	}
	reversed.Office = &officeParams{Country: v.OfficeParams.Office.Country, Roles: roles}
	require.NotEqual(t, v.OfficeParams.Office.Roles, roles, "the fixture's roles must not already be sorted")
	require.Equal(t, v.OfficeParams.fingerprint(), reversed.fingerprint())

	// And it must depend on the set: dropping one role has to move the value the
	// super users read aloud, because that role is authority over a country.
	fewer := v.OfficeParams
	fewer.Office = &officeParams{Country: v.OfficeParams.Office.Country, Roles: roles[:1]}
	require.NotEqual(t, v.OfficeParams.fingerprint(), fewer.fingerprint())

	// Changing the country must move it too. This is the whole reason the office
	// is in the parameters: five keys generated "for Senegal" cannot be silently
	// reused for an office granted authority over Nigeria.
	elsewhere := v.OfficeParams
	elsewhere.Office = &officeParams{Country: "NG", Roles: v.OfficeParams.Office.Roles}
	require.NotEqual(t, v.OfficeParams.fingerprint(), elsewhere.fingerprint())
}

// TestOverLongGroupMetadataIsRefused closes a latent bug rather than describing
// one.
//
// x/group's keeper refuses metadata over 255 bytes, and nothing on the genesis
// path checks it: GroupInfo.ValidateBasic does not, and x/group's
// GenesisState.Validate never goes deeper than ValidateBasic. So an over-long
// label produced a genesis fragment that imported perfectly happily — the
// foundation path would never have noticed — while a country office, whose group
// is created by MsgCreateGroupWithPolicy on a running chain, would have failed
// the transaction after the ceremony was over and the keys were on paper.
func TestOverLongGroupMetadataIsRefused(t *testing.T) {
	configureAddresses()
	people := custodians(t, 4)

	long := strings.Repeat("Autorité nationale de régulation des paiements ", 8)
	_, err := buildGroup(people, groupPurpose{Label: long, OnChain: true}, 2, time.Hour, 7, testTime())
	require.Error(t, err)
	require.ErrorContains(t, err, "x/group refuses anything over 255")
	require.ErrorContains(t, err, "Shorten the office name")

	// And the boundary is not off by one: the longest metadata x/group accepts
	// must still build.
	fitting, err := buildGroup(people, foundationPurpose(), 2, time.Hour, 7, testTime())
	require.NoError(t, err)
	require.LessOrEqual(t, len(groupMetadata(foundationLabel, people, 2)), maxGroupMetadata)
	require.NotEmpty(t, fitting.genesis)
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
