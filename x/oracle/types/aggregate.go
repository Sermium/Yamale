package types

import (
	"sort"

	"cosmossdk.io/math"
)

// Turning many reported prices into one agreed price, and deciding when a price
// has aged out of usefulness.
//
// Both are pure functions of their inputs so they can be tested exhaustively
// without a chain, and so that the rule a client applies to decide whether to
// show a price is provably the same rule the chain applies to decide whether to
// lend against it.

// WeightedVote is one validator's report and the stake behind it.
type WeightedVote struct {
	Validator string
	Rate      math.LegacyDec
	// Power is the validator's stake. Weighting by it means a rate reflects the
	// economic majority rather than a headcount, so spinning up many tiny
	// validators buys no influence over the price.
	Power int64
}

// AggregateResult is the outcome of one voting round for one denom.
type AggregateResult struct {
	Rate math.LegacyDec
	// PowerBps is the share of all stake that reported, in basis points.
	PowerBps uint64
	// Accepted is false when too little stake reported for the result to stand.
	Accepted bool
}

// AggregateRate reduces a round's votes to a single rate.
//
// The median is taken rather than the mean, weighted by stake. A mean lets one
// participant move the result arbitrarily far by reporting an absurd number —
// a single vote of a trillion drags the average with it — whereas moving a
// weighted median requires actually controlling half the stake. That is the
// difference between a price a minority can distort and one they cannot.
//
// The threshold is applied to *reporting* stake, not to agreement: it asks
// whether enough of the network showed up, not whether they agreed. A rate
// derived from a small minority is that minority's rate, however unanimous
// they were among themselves.
func AggregateRate(votes []WeightedVote, totalPower int64, thresholdBps uint64) AggregateResult {
	if len(votes) == 0 || totalPower <= 0 {
		return AggregateResult{Rate: math.LegacyZeroDec()}
	}

	var reporting int64
	usable := make([]WeightedVote, 0, len(votes))
	for _, v := range votes {
		// A non-positive rate is not a price. Counting it would let a validator
		// drag the median toward zero with a value that cannot be meant
		// seriously.
		if v.Power <= 0 || v.Rate.IsNil() || !v.Rate.IsPositive() {
			continue
		}
		usable = append(usable, v)
		reporting += v.Power
	}
	if len(usable) == 0 {
		return AggregateResult{Rate: math.LegacyZeroDec()}
	}

	powerBps := uint64((reporting * 10000) / totalPower)
	if powerBps < thresholdBps {
		return AggregateResult{Rate: math.LegacyZeroDec(), PowerBps: powerBps}
	}

	return AggregateResult{
		Rate:     weightedMedian(usable, reporting),
		PowerBps: powerBps,
		Accepted: true,
	}
}

// weightedMedian returns the rate at which cumulative stake first reaches half
// the reporting total.
//
// Ties are broken by validator address so the result cannot depend on the order
// votes happened to be stored in — every validator must compute the same number
// from the same state, and map or iteration order is not something to rely on
// for that.
func weightedMedian(votes []WeightedVote, totalPower int64) math.LegacyDec {
	sort.Slice(votes, func(i, j int) bool {
		if !votes[i].Rate.Equal(votes[j].Rate) {
			return votes[i].Rate.LT(votes[j].Rate)
		}
		return votes[i].Validator < votes[j].Validator
	})

	half := totalPower / 2
	var cumulative int64
	for _, v := range votes {
		cumulative += v.Power
		if cumulative > half {
			return v.Rate
		}
	}
	return votes[len(votes)-1].Rate
}

// IsStale reports whether a value observed at observedAt is too old to act on
// at now, given a maximum age in seconds.
//
// A value from the future is treated as stale rather than fresh. Clock skew or
// a mistyped date should never make something look more current than it is,
// and refusing is the safe direction: the cost of pausing is inconvenience,
// the cost of lending against a price nobody stands behind is somebody's money.
func IsStale(observedAt, now int64, maxAgeSeconds uint64) bool {
	if maxAgeSeconds == 0 {
		return false
	}
	if observedAt <= 0 || observedAt > now {
		return true
	}
	return uint64(now-observedAt) > maxAgeSeconds
}

// AgeSeconds returns how old a value is, floored at zero.
func AgeSeconds(observedAt, now int64) int64 {
	if observedAt <= 0 || now <= observedAt {
		return 0
	}
	return now - observedAt
}

// ValueOf converts an amount of a denom into the quote currency at a given
// rate, where the rate prices one *display* unit and the amount is in base
// units.
//
// The exponent is why this belongs here rather than at each call site: a rate
// of "0.42 USD per YML" applied naively to 1,000,000 uyml overstates the answer
// by a factor of a million. Doing the conversion in one place means that
// mistake can only be made once.
//
// Rounds down, so a position is never valued above what it is worth. Where this
// feeds a collateral check, over-valuing is what lets somebody borrow more than
// their asset supports.
func ValueOf(baseAmount math.Int, exponent uint32, rate math.LegacyDec) math.Int {
	if baseAmount.IsNil() || !baseAmount.IsPositive() || rate.IsNil() || !rate.IsPositive() {
		return math.ZeroInt()
	}

	scale := math.LegacyNewDecFromInt(math.NewInt(10).ToLegacyDec().Power(uint64(exponent)).TruncateInt())
	if !scale.IsPositive() {
		return math.ZeroInt()
	}

	display := math.LegacyNewDecFromInt(baseAmount).Quo(scale)
	return display.Mul(rate).TruncateInt()
}
