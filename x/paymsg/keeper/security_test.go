package keeper_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

// Findings from the pre-genesis review of this module. Each test is the
// exploit, written so it fails against the code as it was.

// The record this module produces is its product: a camt.053-style statement
// entry naming the institutions that handled a payment. Nothing bound the
// signer to the participants it named, so any account could file a payment
// attributing it to two banks that had never seen it — and those banks would
// find payments they never processed recorded against their name, in the ledger
// their customers reconcile against.
//
// No funds are at risk: the transfer is from the signer's own balance. What is
// at risk is the meaning of every record in the module.
func TestAPaymentCannotNameParticipantsItHasNothingToDoWith(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, bankA := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, bankB := newParticipant(t, f, ms, "00000002", "Bank Two", true)

	// An account with no relationship to either bank.
	stranger, strangerStr := f.env.Addr(t)
	f.env.Fund(t, stranger, payCoins(1_000_000))
	_, creditorStr := f.env.Addr(t)

	_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor:                 strangerStr,
		EndToEndId:             "E2E-IMPERSONATION",
		InstructingParticipant: bankA,
		InstructedParticipant:  bankB,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "500000",
		PurposeCode:            "GDSV",
	})
	require.ErrorIs(t, err, types.ErrNotACustomer,
		"a payment must be instructed by the participant it names, not merely name one")
}

// The instructing participant paying out of its own balance is the ordinary
// case and must keep working.
func TestAParticipantMayInstructItsOwnPayment(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	bankAddr, bankA := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, bankB := newParticipant(t, f, ms, "00000002", "Bank Two", true)
	f.env.Fund(t, bankAddr, payCoins(1_000_000))
	_, creditorStr := f.env.Addr(t)

	_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor:                 bankA,
		EndToEndId:             "E2E-OWN",
		InstructingParticipant: bankA,
		InstructedParticipant:  bankB,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "500000",
		PurposeCode:            "GDSV",
	})
	require.NoError(t, err)
}

// A denom is an attacker-chosen string reaching sdk.NewCoin, which panics
// rather than erroring on an invalid one. The panic is recovered into a failed
// transaction, so this is a robustness finding rather than a halt — but a
// handler should reject its own bad input, not rely on a recover further up.
func TestAnInvalidDenomIsRejectedNotPanicked(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	bankAddr, bankA := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, bankB := newParticipant(t, f, ms, "00000002", "Bank Two", true)
	f.env.Fund(t, bankAddr, payCoins(1_000_000))
	_, creditorStr := f.env.Addr(t)

	for _, denom := range []string{"", "1nvalid", strings.Repeat("x", 200)} {
		require.NotPanics(t, func() {
			_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
				Debtor:                 bankA,
				EndToEndId:             "E2E-DENOM-" + denom,
				InstructingParticipant: bankA,
				InstructedParticipant:  bankB,
				Creditor:               creditorStr,
				Denom:                  denom,
				Amount:                 "500000",
				PurposeCode:            "GDSV",
			})
			require.Error(t, err, "denom %q must be refused", denom)
		}, "denom %q must not panic", denom)
	}
}

// The end-to-end id is a store key in one global namespace, so without a bound
// it is an attacker-chosen key of any length, kept forever. ISO 20022 caps it
// at 35 characters, which is both a natural limit and the one every system on
// the other side of these payments already enforces.
func TestPaymentTextIsBounded(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	bankAddr, bankA := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, bankB := newParticipant(t, f, ms, "00000002", "Bank Two", true)
	f.env.Fund(t, bankAddr, payCoins(100_000_000))
	_, creditorStr := f.env.Addr(t)

	payment := func(id, purpose, remittance string) error {
		_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
			Debtor:                 bankA,
			EndToEndId:             id,
			InstructingParticipant: bankA,
			InstructedParticipant:  bankB,
			Creditor:               creditorStr,
			Denom:                  payDenom,
			Amount:                 "1000",
			PurposeCode:            purpose,
			RemittanceInformation:  remittance,
		})
		return err
	}

	require.Error(t, payment(strings.Repeat("A", 100), "GDSV", ""), "an oversized end-to-end id is a permanent store key")
	require.Error(t, payment("E2E-1", strings.Repeat("A", 50), ""), "a purpose code is four characters in ISO 20022")
	require.Error(t, payment("E2E-2", "GDSV", strings.Repeat("A", 5_000)), "remittance information is capped at 140")

	// Realistic values still work.
	require.NoError(t, payment("INVOICE-2026-0042", "GDSV", "Invoice 2026-0042, 12 units"))
}

// The end-to-end id namespace is global and permanent, so an id somebody else
// intends to use can be taken first and blocked forever. ISO 20022 scopes
// uniqueness to the instructing party, which is also what makes the id
// predictable enough to squat.
func TestAnEndToEndIdCannotBeSquattedAcrossParticipants(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	squatterAddr, squatter := newParticipant(t, f, ms, "00000001", "Squatter Bank", true)
	victimAddr, victim := newParticipant(t, f, ms, "00000002", "Victim Bank", true)
	_, other := newParticipant(t, f, ms, "00000003", "Counterparty", true)
	f.env.Fund(t, squatterAddr, payCoins(10_000_000))
	f.env.Fund(t, victimAddr, payCoins(10_000_000))
	_, creditorStr := f.env.Addr(t)

	const contested = "INVOICE-2026-0001"

	_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor: squatter, EndToEndId: contested,
		InstructingParticipant: squatter, InstructedParticipant: other,
		Creditor: creditorStr, Denom: payDenom, Amount: "1", PurposeCode: "GDSV",
	})
	require.NoError(t, err)

	// The victim's own reference, which they are entitled to use.
	_, err = ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor: victim, EndToEndId: contested,
		InstructingParticipant: victim, InstructedParticipant: other,
		Creditor: creditorStr, Denom: payDenom, Amount: "1", PurposeCode: "GDSV",
	})
	require.NoError(t, err, "one participant's reference must not block another's")
}
