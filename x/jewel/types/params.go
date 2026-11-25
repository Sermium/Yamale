package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MaxSaleFeeBps caps the marketplace fee at 10% of the price.
const MaxSaleFeeBps = 1000

// MinReassignmentWindow is the shortest veto window governance may set, in
// seconds: a week, so an owner who is away still hears of a reassignment in
// time to refuse it.
const MinReassignmentWindow = 7 * 24 * 3600

// DefaultParams are the spec's numbers. The office stays empty: appointing it
// is governance's act, not a default.
func DefaultParams() Params {
	return Params{
		OfferTtl:           14 * 24 * 3600,
		VoucherMaxTtl:      5 * 60,
		ReassignmentWindow: 30 * 24 * 3600,
		SaleFeeBps:         100,
		SaleDenoms:         []string{"uusd", "uusdc", "ueurc"},
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
	if p.ReassignmentWindow < MinReassignmentWindow {
		return fmt.Errorf("reassignment_window must be at least %d seconds (7 days); a shorter window could move a jewel before its owner heard of it, got %d",
			MinReassignmentWindow, p.ReassignmentWindow)
	}
	if p.SaleFeeBps > MaxSaleFeeBps {
		return fmt.Errorf("sale_fee_bps is at most %d (10%%), got %d", MaxSaleFeeBps, p.SaleFeeBps)
	}
	seen := map[string]bool{}
	for _, d := range p.SaleDenoms {
		if err := sdk.ValidateDenom(d); err != nil {
			return fmt.Errorf("sale_denoms: %w", err)
		}
		if seen[d] {
			return fmt.Errorf("sale_denoms names %s twice", d)
		}
		seen[d] = true
	}
	return nil
}

// ForSale says whether a jewel may be listed in this currency.
func (p Params) ForSale(denom string) bool {
	for _, d := range p.SaleDenoms {
		if d == denom {
			return true
		}
	}
	return false
}
