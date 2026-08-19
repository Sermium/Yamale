package types_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// vectorFile is the one file both language implementations read.
//
// It replaces an arrangement where the Go side pinned a hex digest and the
// TypeScript side rebuilt the wire bytes by hand. Those two agreed, but they
// agreed by construction: editing either one left the other green, so the
// "change one and the other goes red" contract the comments claimed was not
// true of the code. A single fixture makes it true — neither suite can move
// without moving the file, and moving the file moves both.
const vectorFile = "../../../testdata/vectors/confidentiality.json"

type metadataVector struct {
	Name                  string `json:"name"`
	SaltHex               string `json:"salt_hex"`
	PurposeCode           string `json:"purpose_code"`
	RemittanceInformation string `json:"remittance_information"`
	WireHex               string `json:"wire_hex"`
	HashHex               string `json:"hash_hex"`
}

type envelopeVector struct {
	Name                   string        `json:"name"`
	InstructingParticipant string        `json:"instructing_participant"`
	EndToEndID             string        `json:"end_to_end_id"`
	AADHex                 string        `json:"aad_hex"`
	EnvelopeHex            string        `json:"envelope_hex"`
	SaltHex                string        `json:"salt_hex"`
	PurposeCode            string        `json:"purpose_code"`
	RemittanceInformation  string        `json:"remittance_information"`
	HashHex                string        `json:"hash_hex"`
	Readers                []vectorParty `json:"readers"`
	NonReaders             []vectorParty `json:"non_readers"`
}

type vectorParty struct {
	Role          string `json:"role"`
	PrivateKeyHex string `json:"private_key_hex"`
	PublicKeyHex  string `json:"public_key_hex"`
	KeyIDHex      string `json:"key_id_hex"`
}

type vectors struct {
	Metadata  []metadataVector `json:"payment_metadata"`
	Envelopes []envelopeVector `json:"payload_envelope"`
}

// loadVectors reads the shared file and refuses an empty one.
//
// The emptiness check is the point of the helper. A fixture that failed to
// parse, or a path that silently resolved to a file with no cases in it, would
// let every vector test pass by iterating over nothing — which is precisely the
// vacuous green the shared file was introduced to eliminate.
func loadVectors(t *testing.T) vectors {
	t.Helper()

	blob, err := os.ReadFile(filepath.Clean(vectorFile))
	require.NoError(t, err, "the shared cross-language vectors must be readable from this package")

	var v vectors
	require.NoError(t, json.Unmarshal(blob, &v))
	require.NotEmpty(t, v.Metadata, "the metadata vectors are empty, so every case below would pass vacuously")
	require.NotEmpty(t, v.Envelopes, "the envelope vectors are empty, so every case below would pass vacuously")
	return v
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}
