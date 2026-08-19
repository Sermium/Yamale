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
