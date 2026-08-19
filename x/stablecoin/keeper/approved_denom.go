package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
)

// IsApprovedDenom reports whether a denom is one this chain's governance has
// approved an issuer for.
//
// The ante chain needs this to decide whether a fee is payable (see
// x/stablecoin/ante), and it must not reach into the ApprovedIssuer map
// directly: a missing key there is an error value, not a false, and treating
// the two the same in a fee check would turn a storage fault into "this denom
// is fine" for every transaction on the chain.
func (k Keeper) IsApprovedDenom(ctx context.Context, denom string) (bool, error) {
	has, err := k.ApprovedIssuer.Has(ctx, denom)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return has, nil
}
