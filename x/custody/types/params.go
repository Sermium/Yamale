package types

import "fmt"

// DefaultParams returns the module's default parameters.
//
// Two attestors, a one-hour delay at five-second blocks, and ten basis points.
// The threshold is the number that matters: one attestor is one compromised key
// away from unlimited issuance, which is how every bridge that mattered was
// emptied.
func DefaultParams() Params {
	return Params{
		AttestationThreshold:  2,
		RedemptionDelayBlocks: 720,
		FeeBps:                10,
		// A day at five-second blocks. A reserve statement older than that is
		// not evidence about today's reserve, and a threshold made of one live
		// attestor and two who reported last year is a threshold of one.
		ReserveReportMaxAgeBlocks: 17_280,
	}
}

// NewParams constructs Params.
func NewParams(threshold uint32, delay uint64, feeBps uint32) Params {
	p := DefaultParams()
	p.AttestationThreshold = threshold
	p.RedemptionDelayBlocks = delay
	p.FeeBps = feeBps
	return p
}

// Validate refuses the configurations that make the module unsafe rather than
// merely unusual.
func (p Params) Validate() error {
	// One attestor is not a threshold, it is a single point of unlimited
	// issuance. Refused outright rather than warned about.
	if p.AttestationThreshold < 2 {
		return fmt.Errorf("attestation_threshold must be at least 2, got %d", p.AttestationThreshold)
	}
	// A fee above 10% is far more likely to be a misplaced decimal than an
	// intention, and it would be charged on somebody's principal.
	if p.FeeBps > 1000 {
		return fmt.Errorf("fee_bps must be 1000 (10%%) or less, got %d", p.FeeBps)
	}
	// Zero would mean a report never goes stale, and a threshold of attestors
	// who each reported once, years apart, is not a threshold. Refused with the
	// same directness as a threshold below two, and for the same reason.
	if p.ReserveReportMaxAgeBlocks == 0 {
		return fmt.Errorf("reserve_report_max_age_blocks must be positive; a reserve report that never expires is not evidence about today's reserve")
	}
	return nil
}
