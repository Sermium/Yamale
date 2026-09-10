package types

import "fmt"

// DefaultParams are a starting position, not a finding. Every one of them is a
// ceiling on what an operator may do to an investor, and they should be argued
// about by whoever adopts this.
func DefaultParams() Params {
	return Params{
		// 500 basis points — five percent — for sourcing and syndication
		// together. Angel syndicates commonly charge in this region at entry,
		// and the ceiling exists so that a deal charging twenty is refused by
		// the chain rather than noticed by an investor afterwards.
		MaxEntryFeeBps: 500,
		// 2000 basis points of each distribution. Deliberately generous as a
		// CEILING, because it is where carried interest would sit if a deal
		// wanted to express one, and deliberately not the default any deal
		// should use.
		MaxManagementFeeBps: 2000,
		// Ninety days. Money committed is escrowed, out of the investor's hands
		// and earning nothing, against a deal that may never happen. A quarter
		// is long enough to fill a round and short enough that the investor is
		// not financing an indefinite search for the last cheque.
		MaxOpenPeriod: 90 * 24 * 60 * 60,
		// Twenty-four hours, so a deal cannot be opened and closed inside a
		// block with one favoured investor in it. The whole promise is access;
		// a raise nobody had time to see is the opposite of it.
		MinOpenPeriod: 24 * 60 * 60,
	}
}

// Validate refuses parameters that would quietly disable the protections.
func (p Params) Validate() error {
	if p.MaxEntryFeeBps > MaxBps {
		return fmt.Errorf(
			"max_entry_fee_bps is %d, which is more than the whole raise", p.MaxEntryFeeBps)
	}
	if p.MaxManagementFeeBps > MaxBps {
		return fmt.Errorf(
			"max_management_fee_bps is %d, which would take every distribution",
			p.MaxManagementFeeBps)
	}
	// Zero would mean a deal must close in the same instant it opens, which no
	// deal can do — so the module would be unusable rather than strict.
	if p.MinOpenPeriod <= 0 {
		return fmt.Errorf(
			"min_open_period must be positive; at zero a deal can be opened and closed "+
				"in one block with one favoured investor in it, got %d", p.MinOpenPeriod)
	}
	if p.MaxOpenPeriod <= p.MinOpenPeriod {
		return fmt.Errorf(
			"max_open_period (%d) must be longer than min_open_period (%d), or no "+
				"deal has a window it can legally close in",
			p.MaxOpenPeriod, p.MinOpenPeriod)
	}
	return nil
}
