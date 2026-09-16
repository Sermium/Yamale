package types

import "cosmossdk.io/collections"

const (
	ModuleName = "tokenisation"
	StoreKey   = ModuleName
)

// Prefixes are distinct and permanent. Old data does not disappear when a
// prefix is reused, it becomes unreadable under a decoder that expects
// something else.
var (
	ParamsKey      = collections.NewPrefix(0)
	CollectionsKey = collections.NewPrefix(1)
	AssetsKey      = collections.NewPrefix(2)
	VaultsKey      = collections.NewPrefix(3)
	PositionsKey   = collections.NewPrefix(4)
	SalesKey       = collections.NewPrefix(5)
	ByDenomKey     = collections.NewPrefix(6)
	NextAssetIDKey = collections.NewPrefix(7)
	// ParcelIssuedBpsKey totals the shares issued over one x/land parcel across
	// every vehicle opened on it.
	ParcelIssuedBpsKey = collections.NewPrefix(8)

	// Covenants are keyed by their own id, with a (asset, covenant) key set
	// beside them so one vehicle's filings are a prefix walk rather than a scan
	// of every filing on the chain.
	CovenantsKey        = collections.NewPrefix(9)
	CovenantsByAssetKey = collections.NewPrefix(10)
	NextCovenantIDKey   = collections.NewPrefix(11)

	// (asset, period) -> PeriodRate. Keyed as a pair so one vehicle's rate
	// history is a prefix walk, and so a settle reads exactly the two entries
	// it needs without scanning.
	PeriodRatesKey = collections.NewPrefix(12)
)

// Denomination prefixes. Two registers, one per vehicle: what you own, and
// what you are owed.
const (
	// FractionPrefix marks a shareholding.
	FractionPrefix = "tok/"

	// DividendPrefix marks a claim on one vehicle's dividend reserve.
	//
	// A different prefix rather than a different symbol, so the two can never
	// be mistaken for one another by a string comparison that matched the
	// symbol alone — and so ByDenom, which drives the send restriction and the
	// AMM's classifier, answers "not mine" for a dividend denom without having
	// to know what one is. Settling a dividend transfer as though it were a
	// share transfer would credit a holder's income history against a balance
	// in the wrong token.
	DividendPrefix = "div/"
)

// FractionDenom is the denom minted against an asset.
//
// The asset id is in the denom because symbols are not unique and never will
// be: two issuers will both want SOLAR, and a chain that let the second one
// have it would have let them mint claims on the first one's warehouse.
func FractionDenom(assetID uint64, symbol string) string {
	return FractionPrefix + itoa(assetID) + "/" + symbol
}

// DividendDenom is the claim token one vehicle pays its dividends in, derived
// from its fraction denom by swapping the prefix: tok/7/WHSE -> div/7/WHSE.
//
// Derived rather than rebuilt from (id, symbol) because the symbol is not
// stored anywhere after fractionalisation — only the assembled fraction denom
// is. Rebuilding it would mean carrying the symbol a second time, and two
// copies of a string that must agree is a way for them to disagree: the one
// that matters here mints a token, and a dividend token whose denom does not
// match the one the exchange burns is money issued against nothing.
//
// Returns "" for anything that is not a fraction denom, including the empty
// denom of an asset that was never fractionalised. Every caller must refuse on
// empty rather than carry on: minting "div//" would put one denom across every
// unfractionalised vehicle on the chain, and its holders would be exchanging
// against each other's reserves.
func DividendDenom(fractionDenom string) string {
	if len(fractionDenom) <= len(FractionPrefix) ||
		fractionDenom[:len(FractionPrefix)] != FractionPrefix {
		return ""
	}
	return DividendPrefix + fractionDenom[len(FractionPrefix):]
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// SHA256Len is how long a digest is. Checked rather than assumed: a hash field
// that accepts any length accepts a truncated one, and a truncated SHA-256 is
// not a weaker proof but a forgeable one.
const SHA256Len = 32

// Bounds on the free text a covenant carries. Not because storage is scarce,
// but because a title long enough to be a document is how somebody puts the
// accounts in the field built to hold their name -- which would be exactly the
// public disclosure the digest design exists to avoid.
const (
	MaxCovenantTitle = 256
	MaxCovenantURI   = 512
)
