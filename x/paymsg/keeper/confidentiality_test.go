package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

// confidentialFixture sets up two approved participants and a funded customer,
// which is the minimum a payment needs before any of these rules are reached.
func confidentialFixture(t *testing.T) (*fixture, types.MsgServer, string, string, string, string) {
	t.Helper()

	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, instructingStr := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, instructedStr := newParticipant(t, f, ms, "00000002", "Bank Two", true)
	_, debtorStr := newCustomer(t, f, ms, instructingStr, 1_000_000)
	_, creditorStr := f.env.Addr(t)

	return f, ms, instructingStr, instructedStr, debtorStr, creditorStr
}

// A payment sends the hash and keeps the detail off-chain; the party holding
// the payload can then show the chain recorded that payload and no other.
func TestSendPaymentRecordsMetadataHashAndItVerifies(t *testing.T) {
	f, ms, instructingStr, instructedStr, debtorStr, creditorStr := confidentialFixture(t)

	payload, err := types.NewPaymentMetadata("SALA", "March salary, employee 4417")
	require.NoError(t, err)
	hash, err := payload.Hash()
	require.NoError(t, err)

	_, err = ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor:                 debtorStr,
		EndToEndId:             "E2E-META-1",
		InstructingParticipant: instructingStr,
		InstructedParticipant:  instructedStr,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "250000",
		MetadataHash:           hash,
		SettlementJurisdiction: "NG",
	})
	require.NoError(t, err)

	rec, err := f.keeper.PaymentRecord.Get(f.ctx, collections.Join(instructingStr, "E2E-META-1"))
	require.NoError(t, err)

	// Nothing readable about the customer reached the ledger.
	require.Empty(t, rec.PurposeCode)
	require.Empty(t, rec.RemittanceInformation)
	require.Equal(t, hash, rec.MetadataHash)
	require.Equal(t, "NG", rec.SettlementJurisdiction)

	require.NoError(t, types.VerifyMetadata(payload, rec.MetadataHash))
}

// The off-chain store is only worth something if a payload it hands back can be
// checked against the block. A store that returned an edited remittance line
// has to be caught by the chain's record, not trusted.
func TestAlteredPayloadFailsAgainstTheRecordedHash(t *testing.T) {
	f, ms, instructingStr, instructedStr, debtorStr, creditorStr := confidentialFixture(t)

	payload, err := types.NewPaymentMetadata("SALA", "March salary, employee 4417")
	require.NoError(t, err)
	hash, err := payload.Hash()
	require.NoError(t, err)

	_, err = ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor:                 debtorStr,
		EndToEndId:             "E2E-META-2",
		InstructingParticipant: instructingStr,
		InstructedParticipant:  instructedStr,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "1000",
		MetadataHash:           hash,
		SettlementJurisdiction: "NG",
	})
	require.NoError(t, err)

	rec, err := f.keeper.PaymentRecord.Get(f.ctx, collections.Join(instructingStr, "E2E-META-2"))
	require.NoError(t, err)

	tampered := payload
	tampered.RemittanceInformation = "March salary, employee 4418"

	err = types.VerifyMetadata(tampered, rec.MetadataHash)
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrInvalidMetadata)
}

// Sending the hash and the plaintext together is the mistake that gives the
// privacy of neither, so the chain refuses it rather than storing both.
func TestSendPaymentRefusesHashBesideItsPlaintext(t *testing.T) {
	f, ms, instructingStr, instructedStr, debtorStr, creditorStr := confidentialFixture(t)

	payload, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)
	hash, err := payload.Hash()
	require.NoError(t, err)

	_, err = ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor:                 debtorStr,
		EndToEndId:             "E2E-META-3",
		InstructingParticipant: instructingStr,
		InstructedParticipant:  instructedStr,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "1000",
		PurposeCode:            "SALA",
		RemittanceInformation:  "March salary",
		MetadataHash:           hash,
	})
	require.ErrorIs(t, err, types.ErrInvalidMetadata)

	// Refused before the transfer, not after it.
	has, err := f.keeper.PaymentRecord.Has(f.ctx, collections.Join(instructingStr, "E2E-META-3"))
	require.NoError(t, err)
	require.False(t, has)
}

// A commitment the chain does not verify must not be quietly accepted: a client
// built on top of it would tell somebody their amount was hidden while it sat
// in plaintext in the same message.
func TestSendPaymentRefusesUnverifiedCommitments(t *testing.T) {
	f, ms, instructingStr, instructedStr, debtorStr, creditorStr := confidentialFixture(t)

	_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor:                 debtorStr,
		EndToEndId:             "E2E-META-4",
		InstructingParticipant: instructingStr,
		InstructedParticipant:  instructedStr,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "1000",
		AmountCommitment:       []byte("pretend pedersen commitment"),
	})
	require.ErrorIs(t, err, types.ErrConfidentialAmountUnavailable)
}

// Existing senders name no jurisdiction, and the chain still accepts them while
// the parameter is off. This is the case that would halt a node syncing from
// block 0 if the requirement were compiled in.
func TestSendPaymentAcceptsNoJurisdictionWhileOptional(t *testing.T) {
	f, ms, instructingStr, instructedStr, debtorStr, creditorStr := confidentialFixture(t)

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	require.False(t, params.RequireSettlementJurisdiction, "the default must not refuse payments that predate the field")

	_, err = ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor:                 debtorStr,
		EndToEndId:             "E2E-JUR-1",
		InstructingParticipant: instructingStr,
		InstructedParticipant:  instructedStr,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "1000",
		PurposeCode:            "SALA",
		RemittanceInformation:  "legacy plaintext",
	})
	require.NoError(t, err)
}

// Once governance turns the parameter on, a payment with no jurisdiction is
// refused — because by then the payload is encrypted to the declared
// regulator's key, and one sent without a jurisdiction is one no regulator can
// ever open.
func TestSendPaymentRequiresJurisdictionOnceParamIsOn(t *testing.T) {
	f, ms, instructingStr, instructedStr, debtorStr, creditorStr := confidentialFixture(t)

	require.NoError(t, f.keeper.Params.Set(f.ctx, types.Params{RequireSettlementJurisdiction: true}))

	send := func(e2e, jurisdiction string) error {
		_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
			Debtor:                 debtorStr,
			EndToEndId:             e2e,
			InstructingParticipant: instructingStr,
			InstructedParticipant:  instructedStr,
			Creditor:               creditorStr,
			Denom:                  payDenom,
			Amount:                 "1000",
			SettlementJurisdiction: jurisdiction,
		})
		return err
	}

	require.ErrorIs(t, send("E2E-JUR-2", ""), types.ErrInvalidSettlementJurisdiction)
	require.NoError(t, send("E2E-JUR-3", "NG"))

	for _, bad := range []string{"nga", "N", "NGA", "ng"} {
		require.ErrorIs(t, send("E2E-JUR-"+bad, bad), types.ErrInvalidSettlementJurisdiction)
	}
}

// A malformed jurisdiction is refused whether or not the parameter is on: an
// unparseable perimeter is worse than an absent one, because it reads on the
// record as though an authority was named.
func TestSendPaymentRefusesMalformedJurisdictionWhileOptional(t *testing.T) {
	f, ms, instructingStr, instructedStr, debtorStr, creditorStr := confidentialFixture(t)

	_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor:                 debtorStr,
		EndToEndId:             "E2E-JUR-9",
		InstructingParticipant: instructingStr,
		InstructedParticipant:  instructedStr,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "1000",
		SettlementJurisdiction: "nga",
	})
	require.ErrorIs(t, err, types.ErrInvalidSettlementJurisdiction)
}

// Payments written before metadata_hash existed keep their plaintext where it
// is. Nothing rewrites them, because a statement entry that changes after the
// fact is not a statement entry.
func TestExistingPlaintextPaymentsAreUnchanged(t *testing.T) {
	f, ms, instructingStr, instructedStr, debtorStr, creditorStr := confidentialFixture(t)

	_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor:                 debtorStr,
		EndToEndId:             "E2E-LEGACY",
		InstructingParticipant: instructingStr,
		InstructedParticipant:  instructedStr,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "500",
		PurposeCode:            "SUPP",
		RemittanceInformation:  "invoice 88213",
	})
	require.NoError(t, err)

	rec, err := f.keeper.PaymentRecord.Get(f.ctx, collections.Join(instructingStr, "E2E-LEGACY"))
	require.NoError(t, err)
	require.Equal(t, "SUPP", rec.PurposeCode)
	require.Equal(t, "invoice 88213", rec.RemittanceInformation)
	require.Empty(t, rec.MetadataHash)
	require.Equal(t, math.NewInt(500).String(), rec.Amount)
}
