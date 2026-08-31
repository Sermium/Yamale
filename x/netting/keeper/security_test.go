package keeper_test

import (
	_ "embed"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

// Parameters decide what nets and how long a window is open, so the only signer
// that may change them is governance.
func TestUpdateParamsIsGovernanceOnly(t *testing.T) {
	f := initFixture(t)
	_, impostor := f.env.Addr(t)

	_, err := f.ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: impostor,
		Params:    types.Params{CycleBlocks: 10},
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	_, err = f.ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.env.AuthorityString(t),
		Params:    types.Params{CycleBlocks: 10},
	})
	require.NoError(t, err)
}

// Governance cannot install parameters the module would then have to cope with.
// A window length beyond the bound is one the chain would never close, which
// would lock every participant's reserve against positions that never settle.
func TestUpdateParamsRefusesAnUnclosableWindow(t *testing.T) {
	f := initFixture(t)

	_, err := f.ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.env.AuthorityString(t),
		Params:    types.Params{CycleBlocks: types.MaxCycleBlocks + 1},
	})
	require.ErrorContains(t, err, "exceeds the maximum")
}

// An institution removed from the rail must still be able to take back
// collateral that is not committed to anything. Stranding it would make removal
// a confiscation, which is a power this module was never meant to hold.
func TestARemovedParticipantMayStillWithdrawUncommittedReserve(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	f.postReserve(t, bankA, coins(eur, 1_000))
	require.NoError(t, f.trySubmit(bankA, bankB, eur, 400, "a owes b"))

	f.participants.approved[bankA] = false

	require.ErrorIs(t, f.trySubmit(bankA, bankB, eur, 1, "after removal"),
		types.ErrNotApprovedParticipant)

	_, err := f.ms.WithdrawReserve(f.ctx, &types.MsgWithdrawReserve{
		Participant: bankA, Amount: coins(eur, 600),
	})
	require.NoError(t, err)

	// And the obligation it took on while it was admitted still settles. It was
	// funded when it was accepted; cancelling it because the counterparty's
	// licence lapsed would be exactly the retroactive rewriting this module
	// refuses to do anywhere else.
	f.endBlockAt(t, 10)
	require.Equal(t, math.NewInt(0).String(), f.reserve(t, bankA, eur).String())
	require.Equal(t, math.NewInt(400).String(), f.reserve(t, bankB, eur).String())
	f.requireCustodyBalances(t, eur)
}

// The reserve must not be spendable from the participant's own account once
// posted. This is why it moves into the module account rather than being
// recorded as a lien: a lien is a promise, and a promise can be broken with an
// ordinary bank send.
func TestPostedReserveCannotBeSpentByItsOwner(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000))
	_, other := f.env.Addr(t)
	f.postReserve(t, bankA, coins(eur, 1_000))

	require.True(t, f.env.Balance(mustAddr(t, f, bankA), eur).IsZero())

	otherAddr, err := f.env.AddressCodec.StringToBytes(other)
	require.NoError(t, err)
	require.Error(t, f.env.BankKeeper.SendCoins(f.ctx, mustAddr(t, f, bankA), otherAddr, coins(eur, 1)),
		"the posted reserve has left the participant's account entirely")
}

// Coins that arrive at the module account without a reserve record behind them
// are unattributable and unreachable forever: no message pays them out, because
// every payout path consults the reserve, and a module account has no key to
// sign with. That the account is on the chain's blocked list is asserted in
// app/blocked_accounts_test.go, which is where the list lives; asserted here is
// the invariant it protects — the module must never hold more than it has
// recorded.
func TestRecordedReservesAlwaysEqualCustody(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000))
	bankB := f.newParticipant(t, coins(eur, 1_000))
	f.postReserve(t, bankA, coins(eur, 700))
	f.postReserve(t, bankB, coins(eur, 700))
	f.requireCustodyBalances(t, eur)

	require.NoError(t, f.trySubmit(bankA, bankB, eur, 500, "a owes b"))
	f.requireCustodyBalances(t, eur)

	f.endBlockAt(t, 10)
	f.requireCustodyBalances(t, eur)

	_, err := f.ms.WithdrawReserve(f.ctx, &types.MsgWithdrawReserve{
		Participant: bankB, Amount: coins(eur, 1_200),
	})
	require.NoError(t, err)
	f.requireCustodyBalances(t, eur)
}

// Settlement sends nothing, so nobody can stall a cycle.
//
// Raised by an independent review on 2026-08-31, which could not tell from the
// code it read whether a frozen participant could move value through this
// module. The answer has two halves and this is the stronger one: the netted
// path moves no coins at all. It rearranges claims inside a module account
// whose balance does not change, so there is no send for anything to refuse —
// which is also why a frozen CREDITOR cannot stall a cycle, and neither can a
// blocked address nor a participant whose approval lapsed this morning.
//
// Asserted structurally rather than behaviourally, because the property is the
// ABSENCE of a call and no amount of passing test cases demonstrates that. A
// later change that helpfully moved coins here would reintroduce every way an
// external account can refuse, and the failure would surface as a stuck cycle
// in production rather than as a red test.
//
// The other half — that the GROSS path is covered, because the freeze is a bank
// send restriction rather than an ante decorator — is asserted in app, where
// the wiring lives. An ante decorator would only see messages arriving as
// transactions and a module calling SendCoins directly would walk past it,
// which is the same bypass this project already documents for interchain
// accounts.
func TestSettlementSendsNothingSoNobodyCanStallACycle(t *testing.T) {
	for _, forbidden := range []string{
		"SendCoins",
		"SendCoinsFromModuleToAccount",
		"SendCoinsFromAccountToModule",
	} {
		require.NotContains(t, abciSource, forbidden,
			"settlement moves coins now, so a frozen or blocked account can stall a whole cycle")
	}
}

// Embedded rather than read at run time, so the assertion holds wherever the
// test binary is executed from.
//
//go:embed abci.go
var abciSource string
