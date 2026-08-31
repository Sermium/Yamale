package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

// Structuring: the attack an independent review found on 2026-08-31, and the
// ceiling that answers it.
//
// The per-obligation gross threshold exists so that high-value flows settle
// immediately and never sit inside the deferred window, where a settlement
// disruption could reach them. Looking at one obligation at a time, it is
// defeated by splitting a payment — which is the same manoeuvre as structuring
// under an AML threshold and the first thing a supervisor will test.
//
// It was never a solvency hole. The net debit cap still binds. What it defeats
// is systemic-risk containment, which is less visible and no less real.

// aggregatePolicy is a currency that nets below `single` per obligation and
// stops netting once one participant has put `total` into the window.
func aggregatePolicy(denom string, single, total int64) types.DenomPolicy {
	return types.DenomPolicy{
		Denom:                   denom,
		GrossThreshold:          math.NewInt(single),
		AggregateGrossThreshold: math.NewInt(total),
	}
}

func TestSplittingAPaymentNoLongerDefersIt(t *testing.T) {
	f := initFixture(t)
	// Anything at or above 1,000 settles gross; and no participant may put more
	// than 2,500 into the window in this currency in one cycle.
	f.setParams(t, 10, aggregatePolicy(eur, 1_000, 2_500))

	payer := f.newParticipant(t, coins(eur, 500_000))
	payee := f.newParticipant(t, coins(eur, 500_000))
	f.postReserve(t, payer, coins(eur, 100_000)) // leaves 400,000 free for anything forced gross

	// Three payments of 999 each: every one is below the per-obligation
	// threshold, and each on its own nets exactly as before.
	for i := 0; i < 2; i++ {
		res := f.submit(t, payer, payee, eur, 999)
		require.Equal(t, types.SETTLEMENT_MODE_NET, res.Mode,
			"obligation %d should still net; the ceiling has not been reached", i)
	}

	// The third crosses the aggregate ceiling — 999 + 999 + 999 = 2,997 — and
	// is forced gross even though it is individually small.
	res := f.submit(t, payer, payee, eur, 999)
	require.Equal(t, types.SETTLEMENT_MODE_GROSS, res.Mode,
		"a payment split into pieces put more than the ceiling into the deferred window")
}

// Forced gross, not refused. The participant is entitled to make the payment;
// what they are not entitled to is deferring it. Refusing would turn a
// systemic-risk control into an availability problem for somebody whose money
// is good.
func TestCrossingTheCeilingSettlesRatherThanFails(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, aggregatePolicy(eur, 1_000, 1_500))

	payer := f.newParticipant(t, coins(eur, 500_000))
	payee := f.newParticipant(t, coins(eur, 500_000))
	f.postReserve(t, payer, coins(eur, 100_000)) // leaves 400,000 free for anything forced gross

	f.submit(t, payer, payee, eur, 900)
	before := f.env.Balance(mustAddr(t, f, payee), eur)

	res := f.submit(t, payer, payee, eur, 900) // 1,800 > 1,500
	require.Equal(t, types.SETTLEMENT_MODE_GROSS, res.Mode)

	after := f.env.Balance(mustAddr(t, f, payee), eur)
	require.True(t, after.GT(before),
		"the obligation was pushed to gross but no money moved")
}

// The counter follows the debtor, not the creditor.
//
// A creditor's incoming obligations create no deferred exposure for it — the
// exposure belongs to whoever owes — so counting them would push honest
// recipients to gross settlement for other people's payments.
func TestTheCeilingFollowsTheDebtorNotTheCreditor(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, aggregatePolicy(eur, 1_000, 1_500))

	a := f.newParticipant(t, coins(eur, 500_000))
	b := f.newParticipant(t, coins(eur, 500_000))
	f.postReserve(t, a, coins(eur, 100_000))
	f.postReserve(t, b, coins(eur, 100_000))

	// a owes b 900, twice: a is now at 1,800 and over its ceiling.
	f.submit(t, a, b, eur, 900)
	require.Equal(t, types.SETTLEMENT_MODE_GROSS, f.submit(t, a, b, eur, 900).Mode)

	// b has received plenty and has netted nothing of its own, so b still nets.
	require.Equal(t, types.SETTLEMENT_MODE_NET, f.submit(t, b, a, eur, 900).Mode,
		"a creditor was charged for exposure that belongs to whoever owes")
}

// An obligation pushed to gross must not also consume the allowance, or one
// large payment would spend it twice.
func TestAGrossObligationDoesNotConsumeTheAllowance(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, aggregatePolicy(eur, 1_000, 5_000))

	payer := f.newParticipant(t, coins(eur, 500_000))
	payee := f.newParticipant(t, coins(eur, 500_000))
	f.postReserve(t, payer, coins(eur, 100_000)) // leaves 400,000 free for anything forced gross

	// Above the per-obligation threshold, so gross, and outside the window.
	require.Equal(t, types.SETTLEMENT_MODE_GROSS, f.submit(t, payer, payee, eur, 4_000).Mode)

	// The window is still empty, so a small payment still nets.
	require.Equal(t, types.SETTLEMENT_MODE_NET, f.submit(t, payer, payee, eur, 900).Mode,
		"a gross obligation consumed the netting allowance it never used")
}

// A chain configured before the field existed keeps its previous behaviour.
func TestNoCeilingMeansTheOldBehaviour(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000)) // no aggregate set

	payer := f.newParticipant(t, coins(eur, 2_000_000))
	payee := f.newParticipant(t, coins(eur, 2_000_000))
	f.postReserve(t, payer, coins(eur, 1_000_000))

	for i := 0; i < 6; i++ {
		require.Equal(t, types.SETTLEMENT_MODE_NET, f.submit(t, payer, payee, eur, 999).Mode,
			"an unset ceiling changed behaviour on obligation %d", i)
	}
}

// The allowance is per cycle, so a new window starts clean. Otherwise a
// participant would be pushed to gross settlement for ever after one busy day.
func TestTheAllowanceResetsWithTheCycle(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, aggregatePolicy(eur, 1_000, 1_500))

	payer := f.newParticipant(t, coins(eur, 500_000))
	payee := f.newParticipant(t, coins(eur, 500_000))
	f.postReserve(t, payer, coins(eur, 100_000)) // leaves 400,000 free for anything forced gross

	f.submit(t, payer, payee, eur, 900)
	require.Equal(t, types.SETTLEMENT_MODE_GROSS, f.submit(t, payer, payee, eur, 900).Mode)

	f.endBlockAt(t, 10) // close the window, open the next

	require.Equal(t, types.SETTLEMENT_MODE_NET, f.submit(t, payer, payee, eur, 900).Mode,
		"the allowance did not reset with the cycle")
}
