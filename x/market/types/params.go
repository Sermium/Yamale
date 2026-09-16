package types

import "fmt"

// DefaultParams are a starting position, not a finding.
func DefaultParams() Params {
	return Params{
		// Twenty resting orders per incoming order. Chosen from what a book on
		// this chain will actually look like rather than from what an exchange
		// needs: a closed-end vehicle over one building has tens of holders,
		// not thousands, so an order crossing twenty price levels has already
		// swept a book deeper than most of these shares will ever have. The
		// remainder rests and the next block continues it, so the bound costs a
		// large trader a block rather than a fill.
		MaxFillsPerOrder: 20,
		// An hour, so an auction cannot be opened and closed inside a block
		// with one favoured bidder in it. Price discovery nobody had time to
		// join is not price discovery, it is a sale that was written down as an
		// auction.
		MinAuctionPeriod: 60 * 60,
		// Thirty days. The seller's shares are escrowed for the whole of it and
		// they cannot withdraw once anybody has bid, so this is a ceiling on
		// how long a seller can lock themselves in by one signature.
		MaxAuctionPeriod: 30 * 24 * 60 * 60,
	}
}

// Validate refuses parameters that would quietly disable the protections.
func (p Params) Validate() error {
	// Zero is the dangerous value, not a large one. At zero no incoming order
	// ever crosses: every order rests, the book fills with orders that overlap
	// in price and never trade, and the market appears to work — orders are
	// accepted, the book is deep — while having become a machine for escrowing
	// property and returning it. A market that refuses orders is visibly
	// broken; a market that accepts every order and matches none is not.
	if p.MaxFillsPerOrder == 0 {
		return fmt.Errorf(
			"max_fills_per_order is zero, so no order could ever cross another and " +
				"the book would rest every order it accepted")
	}
	if p.MinAuctionPeriod <= 0 {
		return fmt.Errorf(
			"min_auction_period must be positive; at zero an auction can be opened and "+
				"settled in one block with one bidder in it, got %d", p.MinAuctionPeriod)
	}
	if p.MaxAuctionPeriod <= p.MinAuctionPeriod {
		return fmt.Errorf(
			"max_auction_period (%d) must be longer than min_auction_period (%d), or no "+
				"auction has a deadline it can legally be opened with",
			p.MaxAuctionPeriod, p.MinAuctionPeriod)
	}
	return nil
}
