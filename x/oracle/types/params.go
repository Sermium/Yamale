package types

import (
	"fmt"
	"strings"
)

// Defaults for a permissioned network with a small, accountable validator set.
//
// The two ages are the parameters that decide behaviour in a crisis, and they
// differ by four orders of magnitude on purpose: a currency is repriced every
// few seconds, a building every few months. Using one number for both would
// either make FX unusable or make property valuations look current long after
// they stopped being so.
const (
	// DefaultVotePeriod agrees a rate roughly every minute at 5s blocks.
	DefaultVotePeriod = 12

	// DefaultVoteThresholdBps requires half the stake to report. Below this the
	// previous rate stands and ages rather than a minority setting the price.
	DefaultVoteThresholdBps = 5000

	DefaultQuoteSymbol = "USD"

	// DefaultMaxRateAgeSeconds is fifteen minutes: long enough to survive a
	// validator restart, short enough that a feed which has genuinely stopped
	// is refused before anyone lends against it.
	DefaultMaxRateAgeSeconds = 900

	// DefaultMaxAppraisalAgeSeconds is 100 days, covering a quarterly valuation
	// cycle with room for the report to arrive.
	DefaultMaxAppraisalAgeSeconds = 8_640_000

	DefaultMaxClassIDsPerAppraiser = 50
)

// DefaultAcceptedDenoms are the fiat-referenced currencies the chain expects
// rates for at genesis. The native token is included because collateral and
// fees are denominated in it.
func DefaultAcceptedDenoms() []string {
	return []string{"uyml", "uusd", "uchf", "ueur", "ugbp", "ujpy"}
}

// NewParams creates a new Params instance.
func NewParams(
	votePeriod, thresholdBps uint64,
	quoteSymbol string,
	acceptedDenoms []string,
	maxRateAge, maxAppraisalAge, maxClassIDs uint64,
) Params {
	return Params{
		VotePeriod:              votePeriod,
		VoteThresholdBps:        thresholdBps,
		QuoteSymbol:             quoteSymbol,
		AcceptedDenoms:          acceptedDenoms,
		MaxRateAgeSeconds:       maxRateAge,
		MaxAppraisalAgeSeconds:  maxAppraisalAge,
		MaxClassIdsPerAppraiser: maxClassIDs,
	}
}

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	return NewParams(
		DefaultVotePeriod,
		DefaultVoteThresholdBps,
		DefaultQuoteSymbol,
		DefaultAcceptedDenoms(),
		DefaultMaxRateAgeSeconds,
		DefaultMaxAppraisalAgeSeconds,
		DefaultMaxClassIDsPerAppraiser,
	)
}

// Validate validates the set of params.
func (p Params) Validate() error {
	if p.VotePeriod == 0 {
		return fmt.Errorf("vote_period must be positive")
	}
	if p.VoteThresholdBps == 0 || p.VoteThresholdBps > 10000 {
		return fmt.Errorf("vote_threshold_bps must be between 1 and 10000, got %d", p.VoteThresholdBps)
	}
	// Below a simple majority a minority of stake could set the price, which
	// defeats the point of aggregating at all.
	if p.VoteThresholdBps < 5000 {
		return fmt.Errorf("vote_threshold_bps must be at least 5000 so a minority cannot set the rate, got %d", p.VoteThresholdBps)
	}
	if strings.TrimSpace(p.QuoteSymbol) == "" {
		return fmt.Errorf("quote_symbol must be set")
	}
	if p.MaxRateAgeSeconds == 0 {
		return fmt.Errorf("max_rate_age_seconds must be positive, or a stopped feed would never be detected")
	}
	if p.MaxAppraisalAgeSeconds == 0 {
		return fmt.Errorf("max_appraisal_age_seconds must be positive, or a stale valuation would never be detected")
	}
	if p.MaxClassIdsPerAppraiser == 0 {
		return fmt.Errorf("max_class_ids_per_appraiser must be positive")
	}

	seen := make(map[string]bool, len(p.AcceptedDenoms))
	for _, denom := range p.AcceptedDenoms {
		if strings.TrimSpace(denom) == "" {
			return fmt.Errorf("accepted_denoms contains an empty denom")
		}
		if seen[denom] {
			return fmt.Errorf("accepted_denoms contains %q twice", denom)
		}
		seen[denom] = true
	}

	return nil
}

// Accepts reports whether the chain expects rates for a denom.
func (p Params) Accepts(denom string) bool {
	for _, d := range p.AcceptedDenoms {
		if d == denom {
			return true
		}
	}
	return false
}
