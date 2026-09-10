package types

import "cosmossdk.io/collections"

const (
	// ModuleName is the module name and the escrow account's name.
	//
	// The module account is where committed money sits between commitment and
	// close. It must be a blocked address in the bank keeper's list, like every
	// other custody account on this chain: a direct send to it is money the
	// module has no record of and therefore no way to return, and a refund path
	// that cannot see the funds it is meant to refund is worse than none.
	ModuleName = "syndicate"

	// StoreKey is the module's store key.
	StoreKey = ModuleName

	// GovModuleName is the authority that admits platform operators.
	GovModuleName = "gov"
)

// DenomPrefix is what a deal token's denomination begins with.
//
// A prefix rather than a bare identifier so that the send restriction can
// decide a transfer is none of its business with one string comparison rather
// than a store read. The restriction runs on every bank send on the chain, and
// a module that made a lookup out of every unrelated payment would be a tax on
// the whole network.
const DenomPrefix = "syn/"

var (
	// ParamsKey is the module parameters.
	ParamsKey = collections.NewPrefix("params")

	OperatorKey = collections.NewPrefix("operator")
	InvestorKey = collections.NewPrefix("investor")
	DealKey     = collections.NewPrefix("deal")
	ListingKey  = collections.NewPrefix("listing")

	// (deal, investor) -> commitment, and (deal, holder) -> position, so one
	// deal's commitments or holders are a prefix walk rather than a scan.
	CommitmentKey = collections.NewPrefix("commitment")
	PositionKey   = collections.NewPrefix("position")

	// The indexes. Each carries an "idx_" prefix, and that is not decoration:
	// a collections prefix must not be a byte-prefix of another, and "deal"
	// sits inside "deals_by_ref". The schema builder refuses it outright, which
	// is the right place to find out and, in x/herd, where it was found.
	ByDealRefKey = collections.NewPrefix("idx_by_deal_ref")
	// denom -> deal id, for the send restriction.
	ByDenomKey = collections.NewPrefix("idx_by_denom")
	// (deal, listing) so one deal's market is a prefix walk.
	ListingsByDealKey = collections.NewPrefix("idx_listings_by_deal")

	NextDealIDKey    = collections.NewPrefix("next_deal_id")
	NextListingIDKey = collections.NewPrefix("next_listing_id")
)

// IndexScale is the fixed-point scale the distribution index is carried at.
//
// Distributions are divided by the token supply, and integer division throws
// the remainder away. Without a scale, a distribution smaller than the supply
// rounds to zero per token and the entire payment vanishes into the module
// account — which is not a rounding error, it is a silent confiscation that
// gets larger the more finely a deal is subscribed.
//
// 10^18 because a deal token is one unit per smallest unit of settlement, so a
// million-dollar raise in six-decimal USDC has a supply of 10^12 and a scale of
// 10^18 leaves six digits of precision below the last cent. The dust that
// remains is left in the pot and paid out with the next distribution rather
// than swept, because sweeping it needs somebody to sweep it to.
const IndexScale = 1_000_000_000_000_000_000

// MaxBps is one hundred percent.
const MaxBps = 10_000
