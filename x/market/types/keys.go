package types

import "cosmossdk.io/collections"

const (
	// ModuleName is the module name and the escrow account's name.
	//
	// The module account holds every resting order's property: the shares
	// behind an ask or a listing, and the money behind a bid or a standing
	// auction bid. It must be a blocked address in the bank keeper's list, like
	// every other custody account on this chain. A direct send to it is
	// property the module has no record of — it matches no order, so no cancel
	// returns it and no fill spends it — and it sits there permanently while
	// making the account's balance disagree with the sum of what the orders
	// claim.
	ModuleName = "market"

	// StoreKey is the module's store key.
	StoreKey = ModuleName

	// GovModuleName is the authority that sets the market-wide limits.
	GovModuleName = "gov"
)

var (
	// ParamsKey is the module parameters.
	ParamsKey = collections.NewPrefix("params")

	ListingKey = collections.NewPrefix("listing")
	AuctionKey = collections.NewPrefix("auction")
	OrderKey   = collections.NewPrefix("order")

	// The indexes. Each carries an "idx_" prefix, and that is not decoration: a
	// collections prefix must not be a byte-prefix of another, and "order" sits
	// inside "orders_by_denom". The schema builder refuses it outright, which is
	// the right place to find out and, in x/herd, where it was found.

	// (denom, listing) and (denom, auction), so one share's offers are a prefix
	// walk rather than a scan of every offer on the chain.
	ListingsByDenomKey = collections.NewPrefix("idx_listings_by_denom")
	AuctionsByDenomKey = collections.NewPrefix("idx_auctions_by_denom")

	// The book: (denom, unit_price, order) per side, holding ONLY orders that
	// are still open.
	//
	// The price sits in the key, ahead of the id, and that is the whole reason
	// the book works. "The best ask on this share" is then the first key of a
	// range scan over the denom prefix, and "the best bid" is the first key of
	// the same scan read backwards — both O(1) reads rather than a walk of every
	// resting order to find a minimum. A uint64 price key is what makes it
	// true: collections encodes it big-endian and fixed-width, so byte order is
	// numeric order.
	//
	// Two sets rather than one with the side in the key, because the two are
	// read in opposite directions and a reader should not have to remember
	// which.
	BidsKey = collections.NewPrefix("idx_bids")
	AsksKey = collections.NewPrefix("idx_asks")

	NextListingIDKey = collections.NewPrefix("next_listing_id")
	NextAuctionIDKey = collections.NewPrefix("next_auction_id")
	NextOrderIDKey   = collections.NewPrefix("next_order_id")
)

// MaxBookDepth is the most resting orders one book query returns per side.
//
// A bound on a READ rather than on a handler, which is the unusual one. The
// book is the only structure in this module whose size grows with how well the
// market is doing, and a query serving all of it is a public endpoint that gets
// more expensive to abuse the more successful the chain becomes. A client that
// wants the whole book can page through prices; an interface showing a ladder
// wants twenty levels and never asked for more.
const MaxBookDepth = 100

// DefaultBookDepth is what a request that names no depth gets.
const DefaultBookDepth = 20
