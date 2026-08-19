package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

const payDenom = "uyml"

func payCoins(amount int64) sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin(payDenom, math.NewInt(amount)))
}

// newCustomer funds an account and registers it as banking with the given
// participant, which is what entitles its payments to name that participant as
// their instructing agent.
func newCustomer(t *testing.T, f *fixture, ms types.MsgServer, participant string, funding int64) (sdk.AccAddress, string) {
	t.Helper()

	addr, addrStr := f.env.NewFundedAddr(t, payCoins(funding))
	_, err := ms.RegisterCustomer(f.ctx, &types.MsgRegisterCustomer{
		Participant: participant, Customer: addrStr, Registered: true,
	})
	require.NoError(t, err)
	return addr, addrStr
}

// newParticipant files an application and, if approve is true, has governance
// approve it. It returns the participant's address.
func newParticipant(t *testing.T, f *fixture, ms types.MsgServer, code, name string, approve bool) (sdk.AccAddress, string) {
	t.Helper()

	addr, addrStr := f.env.Addr(t)
	_, err := ms.ApplyParticipant(f.ctx, &types.MsgApplyParticipant{
		Creator: addrStr, Code: code, Name: name,
	})
	require.NoError(t, err)

	if approve {
		_, err = ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
			Authority: f.env.AuthorityString(t), Participant: addrStr, Approve: true,
		})
		require.NoError(t, err)
	}
	return addr, addrStr
}

func TestApplyParticipantRecordsPendingApplication(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, addrStr := newParticipant(t, f, ms, "00000001", "Bank One", false)

	app, err := f.keeper.ParticipantApplication.Get(f.ctx, addrStr)
	require.NoError(t, err)
	require.Equal(t, types.StatusPending, app.Status)
	require.Equal(t, "00000001", app.Code)
	require.Equal(t, "Bank One", app.Name)

	has, err := f.keeper.ApprovedParticipant.Has(f.ctx, addrStr)
	require.NoError(t, err)
	require.False(t, has, "applying must not by itself confer participant status")
}

func TestApplyParticipantRejectsDuplicates(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, addrStr := newParticipant(t, f, ms, "00000001", "Bank One", false)

	_, err := ms.ApplyParticipant(f.ctx, &types.MsgApplyParticipant{
		Creator: addrStr, Code: "00000009", Name: "Bank One Again",
	})
	require.ErrorIs(t, err, types.ErrApplicationExists)
}

func TestApproveParticipantRequiresGovAuthority(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, addrStr := newParticipant(t, f, ms, "00000001", "Bank One", false)

	// The applicant cannot approve themself.
	_, err := ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: addrStr, Participant: addrStr, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	has, err := f.keeper.ApprovedParticipant.Has(f.ctx, addrStr)
	require.NoError(t, err)
	require.False(t, has)
}

func TestApproveParticipantAssignsCode(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, addrStr := newParticipant(t, f, ms, "00000001", "Bank One", true)

	approved, err := f.keeper.ApprovedParticipant.Get(f.ctx, addrStr)
	require.NoError(t, err)
	require.Equal(t, "00000001", approved.Code)
	require.Equal(t, "Bank One", approved.Name)

	app, err := f.keeper.ParticipantApplication.Get(f.ctx, addrStr)
	require.NoError(t, err)
	require.Equal(t, types.StatusApproved, app.Status)
}

func TestApproveParticipantRejection(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, addrStr := newParticipant(t, f, ms, "00000001", "Bank One", false)

	_, err := ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: f.env.AuthorityString(t), Participant: addrStr, Approve: false,
	})
	require.NoError(t, err)

	app, err := f.keeper.ParticipantApplication.Get(f.ctx, addrStr)
	require.NoError(t, err)
	require.Equal(t, types.StatusRejected, app.Status)

	has, err := f.keeper.ApprovedParticipant.Has(f.ctx, addrStr)
	require.NoError(t, err)
	require.False(t, has)

	// A decided application cannot be revisited without a fresh one.
	_, err = ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: f.env.AuthorityString(t), Participant: addrStr, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrApplicationNotPending)
}

func TestApproveParticipantUnknownApplication(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, strangerStr := f.env.Addr(t)
	_, err := ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: f.env.AuthorityString(t), Participant: strangerStr, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrApplicationNotFound)
}

func TestSendPaymentMovesFundsAndRecordsStatement(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, instructingStr := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, instructedStr := newParticipant(t, f, ms, "00000002", "Bank Two", true)

	debtor, debtorStr := newCustomer(t, f, ms, instructingStr, 1_000_000)
	creditor, creditorStr := f.env.Addr(t)

	f.env.Ctx = f.env.Ctx.WithBlockHeight(42)

	_, err := ms.SendPayment(f.env.Ctx, &types.MsgSendPayment{
		Debtor:                 debtorStr,
		EndToEndId:             "E2E-0001",
		InstructingParticipant: instructingStr,
		InstructedParticipant:  instructedStr,
		Creditor:               creditorStr,
		Denom:                  payDenom,
		Amount:                 "250000",
		PurposeCode:            "SALA",
		RemittanceInformation:  "March salary",
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(750_000), f.env.Balance(debtor, payDenom))
	require.Equal(t, math.NewInt(250_000), f.env.Balance(creditor, payDenom))

	rec, err := f.keeper.PaymentRecord.Get(f.env.Ctx, collections.Join(instructingStr, "E2E-0001"))
	require.NoError(t, err)
	require.Equal(t, "E2E-0001", rec.EndToEndId)
	require.Equal(t, instructingStr, rec.InstructingParticipant)
	require.Equal(t, instructedStr, rec.InstructedParticipant)
	require.Equal(t, debtorStr, rec.Debtor)
	require.Equal(t, creditorStr, rec.Creditor)
	require.Equal(t, payDenom, rec.Denom)
	require.Equal(t, "250000", rec.Amount)
	require.Equal(t, "SALA", rec.PurposeCode)
	require.Equal(t, "March salary", rec.RemittanceInformation)
	require.Equal(t, uint64(42), rec.BlockHeight)
}

func TestSendPaymentRequiresBothParticipantsApproved(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, approvedStr := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, pendingStr := newParticipant(t, f, ms, "00000002", "Bank Two", false)

	debtor, debtorStr := newCustomer(t, f, ms, approvedStr, 1_000_000)
	_, creditorStr := f.env.Addr(t)

	base := func(instructing, instructed, e2e string) *types.MsgSendPayment {
		return &types.MsgSendPayment{
			Debtor: debtorStr, EndToEndId: e2e,
			InstructingParticipant: instructing, InstructedParticipant: instructed,
			Creditor: creditorStr, Denom: payDenom, Amount: "1000",
		}
	}

	t.Run("instructing side not approved", func(t *testing.T) {
		_, err := ms.SendPayment(f.ctx, base(pendingStr, approvedStr, "E2E-A"))
		require.ErrorIs(t, err, types.ErrNotApprovedParticipant)
	})

	t.Run("instructed side not approved", func(t *testing.T) {
		_, err := ms.SendPayment(f.ctx, base(approvedStr, pendingStr, "E2E-B"))
		require.ErrorIs(t, err, types.ErrNotApprovedParticipant)
	})

	t.Run("participant never applied", func(t *testing.T) {
		_, strangerStr := f.env.Addr(t)
		_, err := ms.SendPayment(f.ctx, base(approvedStr, strangerStr, "E2E-C"))
		require.ErrorIs(t, err, types.ErrNotApprovedParticipant)
	})

	// No funds moved and no statement entries were written for any of them.
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(debtor, payDenom))
	for _, id := range []string{"E2E-A", "E2E-B", "E2E-C"} {
		has, err := f.keeper.PaymentRecord.Has(f.ctx, collections.Join(approvedStr, id))
		require.NoError(t, err)
		require.False(t, has)
	}
}

// End-to-end ids are the settlement system's idempotency key: replaying one
// must not move funds a second time.
func TestSendPaymentRejectsDuplicateEndToEndId(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, instructingStr := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, instructedStr := newParticipant(t, f, ms, "00000002", "Bank Two", true)

	debtor, debtorStr := newCustomer(t, f, ms, instructingStr, 1_000_000)
	creditor, creditorStr := f.env.Addr(t)

	msg := &types.MsgSendPayment{
		Debtor: debtorStr, EndToEndId: "E2E-DUP",
		InstructingParticipant: instructingStr, InstructedParticipant: instructedStr,
		Creditor: creditorStr, Denom: payDenom, Amount: "100000",
	}

	_, err := ms.SendPayment(f.ctx, msg)
	require.NoError(t, err)

	_, err = ms.SendPayment(f.ctx, msg)
	require.ErrorIs(t, err, types.ErrPaymentExists)

	// Exactly one transfer happened.
	require.Equal(t, math.NewInt(900_000), f.env.Balance(debtor, payDenom))
	require.Equal(t, math.NewInt(100_000), f.env.Balance(creditor, payDenom))
}

func TestSendPaymentRequiresSufficientBalance(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, instructingStr := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, instructedStr := newParticipant(t, f, ms, "00000002", "Bank Two", true)

	debtor, debtorStr := newCustomer(t, f, ms, instructingStr, 1_000)
	creditor, creditorStr := f.env.Addr(t)

	_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
		Debtor: debtorStr, EndToEndId: "E2E-BROKE",
		InstructingParticipant: instructingStr, InstructedParticipant: instructedStr,
		Creditor: creditorStr, Denom: payDenom, Amount: "1001",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient funds")

	require.Equal(t, math.NewInt(1_000), f.env.Balance(debtor, payDenom))
	require.True(t, f.env.Balance(creditor, payDenom).IsZero())

	// A failed transfer must not leave a statement entry behind, or the id
	// would be permanently unusable for a retry.
	has, err := f.keeper.PaymentRecord.Has(f.ctx, collections.Join(instructingStr, "E2E-BROKE"))
	require.NoError(t, err)
	require.False(t, has)
}

func TestSendPaymentRejectsBadInput(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, instructingStr := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, instructedStr := newParticipant(t, f, ms, "00000002", "Bank Two", true)

	_, debtorStr := newCustomer(t, f, ms, instructingStr, 1_000_000)
	_, creditorStr := f.env.Addr(t)

	testCases := []struct {
		name    string
		debtor  string
		credit  string
		amount  string
		errStr  string
		e2eName string
	}{
		{name: "invalid debtor", debtor: "nope", credit: creditorStr, amount: "1", errStr: "invalid debtor address", e2eName: "E2E-1"},
		{name: "invalid creditor", debtor: debtorStr, credit: "nope", amount: "1", errStr: "invalid creditor address", e2eName: "E2E-2"},
		{name: "zero amount", debtor: debtorStr, credit: creditorStr, amount: "0", errStr: "invalid amount", e2eName: "E2E-3"},
		{name: "negative amount", debtor: debtorStr, credit: creditorStr, amount: "-1", errStr: "invalid amount", e2eName: "E2E-4"},
		{name: "non-numeric amount", debtor: debtorStr, credit: creditorStr, amount: "lots", errStr: "invalid amount", e2eName: "E2E-5"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
				Debtor: tc.debtor, EndToEndId: tc.e2eName,
				InstructingParticipant: instructingStr, InstructedParticipant: instructedStr,
				Creditor: tc.credit, Denom: payDenom, Amount: tc.amount,
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errStr)
		})
	}
}

// The same pair of participants may settle any number of distinct payments.
func TestSendPaymentMultiplePayments(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, instructingStr := newParticipant(t, f, ms, "00000001", "Bank One", true)
	_, instructedStr := newParticipant(t, f, ms, "00000002", "Bank Two", true)

	debtor, debtorStr := newCustomer(t, f, ms, instructingStr, 1_000_000)
	creditor, creditorStr := f.env.Addr(t)

	for _, id := range []string{"E2E-1", "E2E-2", "E2E-3"} {
		_, err := ms.SendPayment(f.ctx, &types.MsgSendPayment{
			Debtor: debtorStr, EndToEndId: id,
			InstructingParticipant: instructingStr, InstructedParticipant: instructedStr,
			Creditor: creditorStr, Denom: payDenom, Amount: "100000",
		})
		require.NoError(t, err, id)
	}

	require.Equal(t, math.NewInt(700_000), f.env.Balance(debtor, payDenom))
	require.Equal(t, math.NewInt(300_000), f.env.Balance(creditor, payDenom))
}
