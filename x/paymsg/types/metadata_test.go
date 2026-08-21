package types_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/paymsg/types"
)

// A payload built off-chain hashes to something the chain can record, and the
// party holding that payload can show it is the one recorded.
func TestPaymentMetadataRoundTrips(t *testing.T) {
	payload, err := types.NewPaymentMetadata("SALA", "March salary, employee 4417")
	require.NoError(t, err)
	require.Len(t, payload.Salt, types.MetadataSaltLength)

	hash, err := payload.Hash()
	require.NoError(t, err)
	require.Len(t, hash, types.MetadataHashLength)

	require.NoError(t, types.VerifyMetadata(payload, hash))

	// The same payload hashes the same way every time, which is the whole basis
	// of the check: a hash that depended on anything but the payload would
	// verify for the party that computed it and for nobody else.
	again, err := payload.Hash()
	require.NoError(t, err)
	require.True(t, bytes.Equal(hash, again))
}

// Altering any part of the payload breaks the proof. This is the property the
// off-chain store is worth nothing without: whoever holds the payload must not
// be able to produce a different one and call it the record.
func TestAlteredPaymentMetadataFailsVerification(t *testing.T) {
	payload, err := types.NewPaymentMetadata("SALA", "March salary, employee 4417")
	require.NoError(t, err)

	hash, err := payload.Hash()
	require.NoError(t, err)

	altered := []struct {
		name   string
		mutate func(types.PaymentMetadata) types.PaymentMetadata
	}{
		{
			name: "remittance information rewritten",
			mutate: func(m types.PaymentMetadata) types.PaymentMetadata {
				m.RemittanceInformation = "March salary, employee 4418"
				return m
			},
		},
		{
			name: "purpose code swapped",
			mutate: func(m types.PaymentMetadata) types.PaymentMetadata {
				m.PurposeCode = "SUPP"
				return m
			},
		},
		{
			name: "remittance information emptied",
			mutate: func(m types.PaymentMetadata) types.PaymentMetadata {
				m.RemittanceInformation = ""
				return m
			},
		},
		{
			// A different salt is a different payload even when every readable
			// field matches, which is what stops one payment's payload being
			// offered as another's.
			name: "salt replaced",
			mutate: func(m types.PaymentMetadata) types.PaymentMetadata {
				other, err := types.NewPaymentMetadata(m.PurposeCode, m.RemittanceInformation)
				require.NoError(t, err)
				m.Salt = other.Salt
				return m
			},
		},
	}

	for _, tc := range altered {
		t.Run(tc.name, func(t *testing.T) {
			err := types.VerifyMetadata(tc.mutate(payload), hash)
			require.Error(t, err)
			require.ErrorIs(t, err, types.ErrInvalidMetadata)
		})
	}
}

// Two payments carrying identical detail must not produce identical hashes, or
// the ledger publishes that they are the same payment detail without anybody
// ever decrypting anything.
func TestPaymentMetadataSaltsDifferPerPayment(t *testing.T) {
	first, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)
	second, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)

	firstHash, err := first.Hash()
	require.NoError(t, err)
	secondHash, err := second.Hash()
	require.NoError(t, err)

	require.False(t, bytes.Equal(firstHash, secondHash))
}

// The chain and the client SDK must produce the same digest for the same
// payload, or a payload that verifies in a browser fails against the block it
// was recorded in.
//
// Both sides read testdata/vectors/confidentiality.json, and that is the whole
// point. This test used to pin a hex constant here while the TypeScript side
// rebuilt the wire bytes by hand: the two agreed, but neither could ever make
// the other fail, so the drift they were meant to catch would have gone through
// both. Reading one file means an implementation that moves goes red against a
// fixture it cannot quietly rewrite.
//
// The wire encoding is asserted as well as the digest. A digest alone says the
// two disagree; the encoding says where — a reordered field, a default that
// stopped being omitted — which is the difference between a fix and an
// afternoon.
func TestPaymentMetadataMatchesTheSharedVectors(t *testing.T) {
	for _, v := range loadVectors(t).Metadata {
		t.Run(v.Name, func(t *testing.T) {
			payload := types.PaymentMetadata{
				Salt:                  mustHex(t, v.SaltHex),
				PurposeCode:           v.PurposeCode,
				RemittanceInformation: v.RemittanceInformation,
			}

			wire, err := payload.Marshal()
			require.NoError(t, err)
			require.Equal(t, v.WireHex, hex.EncodeToString(wire),
				"the protobuf encoding of the payload has changed")

			hash, err := payload.Hash()
			require.NoError(t, err)
			require.Equal(t, v.HashHex, hex.EncodeToString(hash))

			require.NoError(t, types.VerifyMetadata(payload, mustHex(t, v.HashHex)))
		})
	}
}

// A recorded value that is not a SHA-256 digest is reported as that, not as a
// payload mismatch. bytes.Equal would refuse it either way, so the check only
// earns its place by saying which side is wrong: a party whose store returned a
// truncated hash would otherwise spend the afternoon looking for a tampered
// remittance line that was never tampered with.
func TestVerifyMetadataDistinguishesAMalformedRecordedHash(t *testing.T) {
	payload, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)
	hash, err := payload.Hash()
	require.NoError(t, err)

	err = types.VerifyMetadata(payload, hash[:16])
	require.ErrorIs(t, err, types.ErrInvalidMetadata)
	require.Contains(t, err.Error(), "recorded hash must be")

	// And the payload-mismatch path still reports the other thing.
	tampered := payload
	tampered.PurposeCode = "SUPP"
	err = types.VerifyMetadata(tampered, hash)
	require.ErrorIs(t, err, types.ErrInvalidMetadata)
	require.Contains(t, err.Error(), "does not hash to the value recorded")
}

// The ISO 20022 limits follow the detail off-chain. A payload the receiving
// system cannot hold is worth failing on while the sender is still there to be
// told, rather than at delivery when they are not.
func TestPaymentMetadataEnforcesISOLimits(t *testing.T) {
	payload, err := types.NewPaymentMetadata("SALARY", "fine")
	require.NoError(t, err)
	_, err = payload.Hash()
	require.ErrorIs(t, err, types.ErrInvalidMetadata)
	require.Contains(t, err.Error(), "purpose_code")

	long := make([]byte, types.MaxRemittanceLength+1)
	for i := range long {
		long[i] = 'x'
	}
	payload, err = types.NewPaymentMetadata("SALA", string(long))
	require.NoError(t, err)
	_, err = payload.Hash()
	require.ErrorIs(t, err, types.ErrInvalidMetadata)
	require.Contains(t, err.Error(), "remittance_information")
}

// A payload with no salt is refused rather than hashed, because a hash of a
// four-character purpose code with nothing else in it is a lookup table.
func TestPaymentMetadataRefusesUnsaltedPayload(t *testing.T) {
	_, err := types.PaymentMetadata{PurposeCode: "SALA"}.Hash()
	require.ErrorIs(t, err, types.ErrInvalidMetadata)

	short := types.PaymentMetadata{Salt: []byte("too short"), PurposeCode: "SALA"}
	_, err = short.Hash()
	require.ErrorIs(t, err, types.ErrInvalidMetadata)
}

func TestValidateSettlementJurisdiction(t *testing.T) {
	testCases := []struct {
		name     string
		country  string
		required bool
		valid    bool
	}{
		{name: "NG", country: "NG", required: false, valid: true},
		{name: "NG when required", country: "NG", required: true, valid: true},
		{name: "GH", country: "GH", required: true, valid: true},

		// Lowercase matches no authority in the registry, so it settles with
		// nobody while reading on the record as though somebody was named.
		{name: "nga", country: "nga", required: false, valid: false},
		{name: "ng", country: "ng", required: false, valid: false},

		// alpha-3 is the other ISO 3166-1 list, and the one a caller reaches
		// for by mistake.
		{name: "NGA", country: "NGA", required: false, valid: false},

		{name: "N", country: "N", required: false, valid: false},
		{name: "N1", country: "N1", required: false, valid: false},
		{name: "N ", country: "N ", required: false, valid: false},

		// Two bytes, one rune: the length check alone would let this through.
		{name: "non-ascii", country: "Ñ", required: false, valid: false},

		{name: "empty when optional", country: "", required: false, valid: true},
		{name: "empty when required", country: "", required: true, valid: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := types.ValidateSettlementJurisdiction(tc.country, tc.required)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorIs(t, err, types.ErrInvalidSettlementJurisdiction)
		})
	}
}

func TestValidateConfidentiality(t *testing.T) {
	payload, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)
	hash, err := payload.Hash()
	require.NoError(t, err)

	t.Run("hash alone is accepted", func(t *testing.T) {
		require.NoError(t, types.ValidateConfidentiality(nil, nil, hash, "", ""))
	})

	t.Run("plaintext alone is accepted", func(t *testing.T) {
		require.NoError(t, types.ValidateConfidentiality(nil, nil, nil, "SALA", "March salary"))
	})

	t.Run("hash beside the plaintext it replaces is refused", func(t *testing.T) {
		err := types.ValidateConfidentiality(nil, nil, hash, "SALA", "")
		require.ErrorIs(t, err, types.ErrInvalidMetadata)

		err = types.ValidateConfidentiality(nil, nil, hash, "", "March salary")
		require.ErrorIs(t, err, types.ErrInvalidMetadata)
	})

	t.Run("a hash of the wrong length is refused", func(t *testing.T) {
		err := types.ValidateConfidentiality(nil, nil, []byte("not a sha256 digest"), "", "")
		require.ErrorIs(t, err, types.ErrInvalidMetadata)
	})

	t.Run("commitments are refused while unverified", func(t *testing.T) {
		err := types.ValidateConfidentiality([]byte{0x01}, nil, nil, "", "")
		require.ErrorIs(t, err, types.ErrConfidentialAmountUnavailable)

		err = types.ValidateConfidentiality(nil, []byte{0x02}, nil, "", "")
		require.ErrorIs(t, err, types.ErrConfidentialAmountUnavailable)
	})
}
