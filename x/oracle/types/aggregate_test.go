package types_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/oracle/types"
)

func dec(s string) math.LegacyDec { return math.LegacyMustNewDecFromStr(s) }

func vote(validator, rate string, power int64) types.WeightedVote {
	return types.WeightedVote{Validator: validator, Rate: dec(rate), Power: power}
}

// ---------------------------------------------------------------- median

func TestAggregateTakesTheWeightedMedian(t *testing.T) {
	result := types.AggregateRate([]types.WeightedVote{
		vote("a", "1.00", 10),
		vote("b", "1.10", 10),
		vote("c", "1.20", 10),
	}, 30, 5000)

	require.True(t, result.Accepted)
	require.Equal(t, dec("1.10"), result.Rate)
	require.Equal(t, uint64(10000), result.PowerBps)
}

// The reason for a median rather than a mean.
//
// A minority liar can nudge the result to an adjacent honest vote — adding a
// vote shifts where the midpoint falls — but that is the whole of their
// influence. The guarantee is that the agreed rate is always one somebody
// actually reported and stays inside the honest range, however extreme the lie.
// A mean offers no such bound: the same vote would drag the average to roughly
// 250 million.
func TestAMinorityLiarCannotDragTheRate(t *testing.T) {
	honest := []types.WeightedVote{
		vote("a", "1.00", 10),
		vote("b", "1.01", 10),
		vote("c", "0.99", 10),
	}

	clean := types.AggregateRate(honest, 40, 5000)
	require.Equal(t, dec("1.00"), clean.Rate)

	attacked := types.AggregateRate(append(honest, vote("d", "1000000000", 10)), 40, 5000)

	require.True(t, attacked.Rate.GTE(dec("0.99")) && attacked.Rate.LTE(dec("1.01")),
		"the rate must stay within the honest range, got %s", attacked.Rate)
	require.True(t, attacked.Rate.LT(dec("2")), "the absurd value must have no pull at all")
}

// Half the stake is what it actually takes to control the rate — which is the
// same bound the chain's consensus already rests on, so an oracle capture is
// not cheaper than a consensus capture.
func TestControllingTheRateRequiresHalfTheStake(t *testing.T) {
	result := types.AggregateRate([]types.WeightedVote{
		vote("honest-a", "1.00", 20),
		vote("honest-b", "1.00", 29),
		vote("attacker", "500.00", 51),
	}, 100, 5000)

	require.Equal(t, dec("500.00"), result.Rate, "a majority of stake does control the median")
}

// Stake, not headcount: many small validators must not outweigh the economic
// majority.
func TestManySmallVotersDoNotOutweighStake(t *testing.T) {
	votes := []types.WeightedVote{vote("whale", "1.00", 100)}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		votes = append(votes, vote(id, "5.00", 1))
	}

	result := types.AggregateRate(votes, 105, 5000)
	require.Equal(t, dec("1.00"), result.Rate)
}

func TestMedianIsIndependentOfVoteOrder(t *testing.T) {
	forward := types.AggregateRate([]types.WeightedVote{
		vote("a", "1.00", 10), vote("b", "2.00", 10), vote("c", "3.00", 10),
	}, 30, 0)
	reversed := types.AggregateRate([]types.WeightedVote{
		vote("c", "3.00", 10), vote("b", "2.00", 10), vote("a", "1.00", 10),
	}, 30, 0)

	require.Equal(t, forward.Rate, reversed.Rate, "every validator must compute the same rate")
}

// Equal rates from different validators must break ties deterministically, or
// two nodes could disagree and the chain would halt.
func TestTiedRatesResolveDeterministically(t *testing.T) {
	a := types.AggregateRate([]types.WeightedVote{
		vote("zeta", "1.00", 10), vote("alpha", "1.00", 10),
	}, 20, 0)
	b := types.AggregateRate([]types.WeightedVote{
		vote("alpha", "1.00", 10), vote("zeta", "1.00", 10),
	}, 20, 0)
	require.Equal(t, a.Rate, b.Rate)
}

// ------------------------------------------------------------- threshold

func TestTooLittleStakeReportingIsRejected(t *testing.T) {
	// Two validators out of a much larger set agree perfectly — and that is
	// exactly the case the threshold exists to refuse.
	result := types.AggregateRate([]types.WeightedVote{
		vote("a", "1.00", 5),
		vote("b", "1.00", 5),
	}, 100, 5000)

	require.False(t, result.Accepted)
	require.Equal(t, uint64(1000), result.PowerBps)
	require.True(t, result.Rate.IsZero(), "a rejected round must not produce a rate")
}

func TestExactlyAtThresholdIsAccepted(t *testing.T) {
	result := types.AggregateRate([]types.WeightedVote{vote("a", "1.00", 50)}, 100, 5000)
	require.True(t, result.Accepted)
	require.Equal(t, uint64(5000), result.PowerBps)
}

func TestNonPositiveRatesAreIgnored(t *testing.T) {
	result := types.AggregateRate([]types.WeightedVote{
		vote("a", "1.00", 10),
		vote("b", "0", 10),
		{Validator: "c", Rate: dec("-5.00"), Power: 10},
	}, 30, 0)

	require.True(t, result.Accepted)
	require.Equal(t, dec("1.00"), result.Rate)
	require.Equal(t, uint64(3333), result.PowerBps, "only the usable vote counts toward the threshold")
}

func TestEmptyAndDegenerateRoundsAreSafe(t *testing.T) {
	require.False(t, types.AggregateRate(nil, 100, 5000).Accepted)
	require.False(t, types.AggregateRate([]types.WeightedVote{vote("a", "1", 10)}, 0, 5000).Accepted)
	require.False(t, types.AggregateRate([]types.WeightedVote{vote("a", "1", 0)}, 100, 0).Accepted)
}

// --------------------------------------------------------------- staleness

func TestStaleness(t *testing.T) {
	const now = 1_000_000
	const maxAge = 300

	require.False(t, types.IsStale(now, now, maxAge), "a value from this instant is fresh")
	require.False(t, types.IsStale(now-300, now, maxAge), "exactly at the bound is still usable")
	require.True(t, types.IsStale(now-301, now, maxAge), "one second past the bound is not")
	require.True(t, types.IsStale(0, now, maxAge), "a value that was never set is stale")
}

// Clock skew must never make something look fresher than it is.
func TestAFutureTimestampIsTreatedAsStale(t *testing.T) {
	require.True(t, types.IsStale(2_000_000, 1_000_000, 300))
}

func TestZeroMaxAgeDisablesTheCheck(t *testing.T) {
	require.False(t, types.IsStale(1, 1_000_000, 0))
}

func TestAgeSecondsNeverGoesNegative(t *testing.T) {
	require.Equal(t, int64(0), types.AgeSeconds(2_000_000, 1_000_000))
	require.Equal(t, int64(500), types.AgeSeconds(999_500, 1_000_000))
}

// --------------------------------------------------------------- valuation

// A rate prices one display unit, so applying it to base units without scaling
// overstates the answer by the denom's exponent — a million, here.
func TestValueOfScalesByExponent(t *testing.T) {
	// 1,000,000 uyml is 1 YML, worth 0.42 quote units.
	require.Equal(t, "0", types.ValueOf(math.NewInt(1_000_000), 6, dec("0.42")).String(),
		"0.42 truncates to zero whole quote units")

	// 1,000 YML at 0.42 is 420.
	require.Equal(t, "420", types.ValueOf(math.NewInt(1_000_000_000), 6, dec("0.42")).String())
}

// Rounding down matters wherever this feeds a collateral check: over-valuing
// collateral is what lets somebody borrow more than their asset supports.
func TestValueOfRoundsDown(t *testing.T) {
	// 1 YML at 0.999999 is 0.999999 quote units, which is not one.
	require.Equal(t, "0", types.ValueOf(math.NewInt(1_000_000), 6, dec("0.999999")).String())
	// 10 YML at 0.999999 is 9.99999, which is nine.
	require.Equal(t, "9", types.ValueOf(math.NewInt(10_000_000), 6, dec("0.999999")).String())
}

func TestValueOfRejectsNonsense(t *testing.T) {
	require.Equal(t, "0", types.ValueOf(math.NewInt(0), 6, dec("1")).String())
	require.Equal(t, "0", types.ValueOf(math.NewInt(-5), 6, dec("1")).String())
	require.Equal(t, "0", types.ValueOf(math.NewInt(1_000_000), 6, dec("0")).String())
}
