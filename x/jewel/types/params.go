package types

import "fmt"

// DefaultParams are the spec's numbers. The office stays empty: appointing it
// is governance's act, not a default.
func DefaultParams() Params {
	return Params{
		OfferTtl:           14 * 24 * 3600,
		VoucherMaxTtl:      5 * 60,
		ReassignmentWindow: 30 * 24 * 3600,
	}
}

// Validate refuses settings that would switch a protection off.
func (p Params) Validate() error {
	if p.OfferTtl <= 0 {
		return fmt.Errorf("offer_ttl must be positive; at zero no offer could ever be accepted, got %d", p.OfferTtl)
	}
	if p.VoucherMaxTtl <= 0 || p.VoucherMaxTtl > 3600 {
		return fmt.Errorf("voucher_max_ttl must be between 1 and 3600 seconds; a long-lived voucher is a benefit someone can spend without the owner present, got %d", p.VoucherMaxTtl)
	}
	if p.ReassignmentWindow <= 0 {
		return fmt.Errorf("reassignment_window must be positive; at zero the office could move a jewel before its owner could object, got %d", p.ReassignmentWindow)
	}
	return nil
}
