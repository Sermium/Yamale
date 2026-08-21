package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

// The threshold is what keeps the exposures nobody would want deferred out of
// the deferred window. It is a boundary, so it is tested as one: at the
// threshold settles gross, one below it nets.
func TestThresholdBoundaryDecidesGrossOrNet(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	bankB := f.newParticipant(t, coins(eur, 0))
	f.postReserve(t, bankA, coins(eur, 2_000_000))

	atThreshold := f.submit(t, bankA, bankB, eur, 1_000_000)
	require.Equal(t, types.SETTLEMENT_MODE_GROSS, atThreshold.Mode,
		"an amount at the threshold is not below it")
	require.Equal(t, math.NewInt(1_000_000).String(), f.env.Balance(mustAddr(t, f, bankB), eur).String(),
		"gross settlement moves the money in the same block")

	belowThreshold := f.submit(t, bankA, bankB, eur, 999_999)
	require.Equal(t, types.SETTLEMENT_MODE_NET, belowThreshold.Mode,
		"one unit below the threshold nets")
	require.Equal(t, math.NewInt(1_000_000).String(), f.env.Balance(mustAddr(t, f, bankB), eur).String(),
		"a netted obligation moves nothing yet")

	cycle := f.currentCycle(t)
	require.Equal(t, math.NewInt(-999_999).String(), f.position(t, cycle, eur, bankA).String())
	require.Equal(t, math.NewInt(999_999).String(), f.position(t, cycle, eur, bankB).String())
}

// A currency governance has never enabled must not start netting because
// somebody used it. Gross is the safe direction: the money moves now and
// nobody is left carrying an exposure they did not agree to.
func TestUnconfiguredCurrencySettlesGross(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(ngn, 5_000_000))
	bankB := f.newParticipant(t, coins(ngn, 0))
	f.postReserve(t, bankA, coins(ngn, 2_000_000))

	res := f.submit(t, bankA, bankB, ngn, 10)
	require.Equal(t, types.SETTLEMENT_MODE_GROSS, res.Mode)
	require.Equal(t, math.NewInt(10).String(), f.env.Balance(mustAddr(t, f, bankB), ngn).String())
	require.True(t, f.position(t, f.currentCycle(t), ngn, bankA).IsZero(),
		"a gross obligation must take no part in the window's positions")
}

// Netting switched off chain-wide has to mean off everywhere, not "netting with
// an unreachable close". A window that never closes would lock every
// participant's reserve against positions that never settle.
func TestNettingDisabledSettlesEverythingGross(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 0, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	bankB := f.newParticipant(t, coins(eur, 0))

	res := f.submit(t, bankA, bankB, eur, 5)
	require.Equal(t, types.SETTLEMENT_MODE_GROSS, res.Mode)
	require.Equal(t, math.NewInt(5).String(), f.env.Balance(mustAddr(t, f, bankB), eur).String())
}

// The net debit cap is the whole reason settlement cannot fail. It is enforced
// synchronously, against the position after the obligation, and the rejection
// is an ordinary transaction error rather than a discovery made in an end
// blocker where nothing can be refused.
func TestNetDebitCapRefusesAnUnfundedObligation(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	bankB := f.newParticipant(t, coins(eur, 0))
	f.postReserve(t, bankA, coins(eur, 100))

	require.NoError(t, f.trySubmit(bankA, bankB, eur, 100, "exactly the reserve"))
	require.Equal(t, math.NewInt(100).String(), f.locked(t, bankA, eur).String())

	err := f.trySubmit(bankA, bankB, eur, 1, "one beyond it")
	require.ErrorIs(t, err, types.ErrNetDebitCapExceeded)

	// Nothing was written by the refused message.
	require.Equal(t, math.NewInt(-100).String(), f.position(t, f.currentCycle(t), eur, bankA).String())
	require.Equal(t, math.NewInt(100).String(), f.locked(t, bankA, eur).String())
}

// The cap is measured against the net position, not the gross flow. A bank that
// is owed 900 and now owes 1000 has an exposure of 100, and requiring it to
// collateralise 1000 would make netting cost more liquidity than gross
// settlement — which is the opposite of the reason to do it.
func TestCapIsMeasuredNetNotGross(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	bankB := f.newParticipant(t, coins(eur, 5_000_000))
	f.postReserve(t, bankA, coins(eur, 100))
	f.postReserve(t, bankB, coins(eur, 100_000))

	// B owes A 900 first, so A is a net creditor and has committed nothing.
	require.NoError(t, f.trySubmit(bankB, bankA, eur, 900, "b owes a"))
	require.True(t, f.locked(t, bankA, eur).IsZero())

	// A now owes B 1000. Its net position is -100, well inside a reserve of
	// 100, even though the gross figure is ten times it.
	require.NoError(t, f.trySubmit(bankA, bankB, eur, 1000, "a owes b"))
	require.Equal(t, math.NewInt(100).String(), f.locked(t, bankA, eur).String())
	require.Equal(t, math.NewInt(-100).String(), f.position(t, f.currentCycle(t), eur, bankA).String())
}

// An offsetting obligation has to release the creditor's collateral at the
// moment it arrives. Holding it until close would keep reserve committed
// against an exposure that no longer exists, which is exactly the liquidity
// netting is supposed to save.
func TestOffsettingObligationReleasesCollateralImmediately(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	bankB := f.newParticipant(t, coins(eur, 5_000_000))
	f.postReserve(t, bankA, coins(eur, 1000))
	f.postReserve(t, bankB, coins(eur, 1000))

	require.NoError(t, f.trySubmit(bankA, bankB, eur, 1000, "a owes b"))
	require.Equal(t, math.NewInt(1000).String(), f.locked(t, bankA, eur).String())

	require.NoError(t, f.trySubmit(bankB, bankA, eur, 700, "b owes a"))
	require.Equal(t, math.NewInt(300).String(), f.locked(t, bankA, eur).String(),
		"A now owes only 300 net and should have 700 of collateral back")
	require.True(t, f.locked(t, bankB, eur).IsZero(),
		"B is a net creditor and has committed nothing")
}

// Withdrawing the collateral behind an obligation already submitted would be a
// default engineered in one block. The uncommitted part must still be free to
// leave, or posting reserve would be a one-way door.
func TestWithdrawCannotTakeCommittedReserve(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	bankB := f.newParticipant(t, coins(eur, 0))
	f.postReserve(t, bankA, coins(eur, 1000))
	require.NoError(t, f.trySubmit(bankA, bankB, eur, 600, "a owes b"))

	_, err := f.ms.WithdrawReserve(f.ctx, &types.MsgWithdrawReserve{
		Participant: bankA, Amount: coins(eur, 401),
	})
	require.ErrorIs(t, err, types.ErrReserveCommitted)

	_, err = f.ms.WithdrawReserve(f.ctx, &types.MsgWithdrawReserve{
		Participant: bankA, Amount: coins(eur, 400),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(600).String(), f.reserve(t, bankA, eur).String())
	f.requireCustodyBalances(t, eur)
}

// Both sides of an obligation must be on the rail. An institution that is not
// admitted cannot owe and cannot be owed, because there would be nobody the
// chain could hold to either half of it.
func TestBothSidesMustBeApprovedParticipants(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	f.postReserve(t, bankA, coins(eur, 1000))
	_, outsider := f.env.Addr(t)

	err := f.trySubmit(bankA, outsider, eur, 10, "to an outsider")
	require.ErrorIs(t, err, types.ErrNotApprovedParticipant)

	f.participants.approved[bankA] = false
	f.participants.approved[outsider] = true
	err = f.trySubmit(bankA, outsider, eur, 10, "from a removed participant")
	require.ErrorIs(t, err, types.ErrNotApprovedParticipant)
}

// A self-obligation nets to nothing while inflating the gross figure the cycle
// reports — which is the number the compression claim is computed from. Free,
// unlimited, and it would make the system look better than it is.
func TestSelfObligationIsRefused(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	f.postReserve(t, bankA, coins(eur, 1000))

	require.ErrorIs(t, f.trySubmit(bankA, bankA, eur, 10, "self"), types.ErrSelfObligation)
}

// The batch hash is the only link from an interbank figure back to the items it
// summarises. Without it, netting trades away auditability rather than only
// throughput.
func TestBatchHashIsRequiredAndFixedLength(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	bankB := f.newParticipant(t, coins(eur, 0))
	f.postReserve(t, bankA, coins(eur, 1000))

	for name, hash := range map[string][]byte{
		"absent":    nil,
		"empty":     {},
		"too short": make([]byte, 31),
		"too long":  make([]byte, 33),
	} {
		_, err := f.ms.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
			FromParticipant: bankA, ToParticipant: bankB, Denom: eur,
			Amount: math.NewInt(10), BatchHash: hash,
		})
		require.ErrorIs(t, err, types.ErrInvalidBatchHash, "a %s batch hash must be refused", name)
	}
}

// A negative amount would be the opposite obligation in disguise, and would let
// one participant move another's position in the direction it chose.
func TestNonPositiveAmountsAreRefused(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	bankB := f.newParticipant(t, coins(eur, 0))
	f.postReserve(t, bankA, coins(eur, 1000))

	for _, amount := range []math.Int{math.ZeroInt(), math.NewInt(-1)} {
		_, err := f.ms.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
			FromParticipant: bankA, ToParticipant: bankB, Denom: eur,
			Amount: amount, BatchHash: batchHash("bad amount"),
		})
		require.ErrorIs(t, err, types.ErrInvalidAmount, "amount %s must be refused", amount)
	}
}

// Posting reserve must move the coins into the module account, not merely
// record an intention. A balance that is only notionally committed can be spent
// by an ordinary bank send, and the module would find out at settlement.
func TestPostReserveTakesRealCustody(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 5_000_000))
	f.postReserve(t, bankA, coins(eur, 2_000_000))

	require.Equal(t, math.NewInt(3_000_000).String(), f.env.Balance(mustAddr(t, f, bankA), eur).String())
	require.Equal(t, math.NewInt(2_000_000).String(), f.moduleHoldings(eur).String())
	f.requireCustodyBalances(t, eur)
}

func mustAddr(t *testing.T, f *fixture, bech string) sdk.AccAddress {
	t.Helper()
	addr, err := f.env.AddressCodec.StringToBytes(bech)
	require.NoError(t, err)
	return addr
}
