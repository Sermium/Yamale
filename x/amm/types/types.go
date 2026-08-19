package types

import (
	"fmt"
	"strconv"
	"strings"
)

// lpDenomPrefix namespaces the bank denoms minted for pool shares.
const lpDenomPrefix = "amm/pool/"

// LPDenom returns the bank denom used for a pool's liquidity-provider shares.
func LPDenom(poolID uint64) string {
	return fmt.Sprintf("%s%d", lpDenomPrefix, poolID)
}

// IsLPDenom reports whether denom is a pool-share denom rather than an
// ordinary tradeable asset.
func IsLPDenom(denom string) bool {
	return strings.HasPrefix(denom, lpDenomPrefix)
}

// PoolIDFromLPDenom recovers the pool id encoded in an LP share denom,
// reporting false if denom is not one.
func PoolIDFromLPDenom(denom string) (uint64, bool) {
	rest, found := strings.CutPrefix(denom, lpDenomPrefix)
	if !found {
		return 0, false
	}
	id, err := strconv.ParseUint(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
