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

// FractionDenom is the denom minted against an asset.
//
// The asset id is in the denom because symbols are not unique and never will
// be: two issuers will both want SOLAR, and a chain that let the second one
// have it would have let them mint claims on the first one's warehouse.
func FractionDenom(assetID uint64, symbol string) string {
	return "tok/" + itoa(assetID) + "/" + symbol
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
