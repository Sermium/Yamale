package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

// parcelHeldBy checks that a parcel exists and that the account this module is
// about to treat as entitled to it is the account the registry records as its
// holder.
//
// Re-asked at every issuance rather than trusted from the mint: title moves in
// x/land without this module hearing about it, so a vehicle minted lawfully
// last year can be sitting over ground its sponsor has since sold.
func (k Keeper) parcelHeldBy(
	ctx context.Context, parcelID uint64, owner string,
) (types.LandParcel, error) {
	if k.landKeeper == nil {
		return types.LandParcel{}, types.ErrNoLandRegistry
	}
	parcel, err := k.landKeeper.LandParcel(ctx, parcelID)
	if err != nil {
		return types.LandParcel{}, err
	}
	if !parcel.Exists {
		return types.LandParcel{}, types.ErrNoParcel
	}
	if parcel.Holder != owner {
		return types.LandParcel{}, types.ErrNotParcelHolder
	}
	return parcel, nil
}

// assertLandPermitsIssue is the supervised bridge from x/land, enforced.
//
// Without it the registry's ceiling, expiry, withdrawal and restrictions are
// decorative: x/land records a decision and nothing consults it, so land
// carrying `no_fractionalisation` can be sold in shares by anybody who mints an
// asset over it. Each refusal below is one of those decisions.
//
// issuedShareBps is the share the tokens will carry — what is being sold, not
// what the sponsor retains. The registry's ceiling caps the same quantity, so
// the comparison is direct. Reading either as the retained share would invert
// the rule and permit precisely the issuance the office set out to forbid.
func (k Keeper) assertLandPermitsIssue(
	ctx context.Context, parcelID uint64, owner string, issuedShareBps uint32,
) error {
	parcel, err := k.parcelHeldBy(ctx, parcelID, owner)
	if err != nil {
		return err
	}
	// Checked before the authorisation, because a restriction imposed after the
	// permission was granted has to be able to stop issuance. If the office's
	// own grant outranked it, a standing authorisation would be a way to sell
	// around a limit imposed yesterday.
	if parcel.ForbidsFractionalisation {
		return types.ErrLandFractionalisationForbidden
	}

	auth, err := k.landKeeper.LandAuthorisation(ctx, parcelID)
	if err != nil {
		return err
	}
	if !auth.Granted {
		return types.ErrNoLandAuthorisation
	}
	if auth.Withdrawn {
		return types.ErrAuthorisationWithdrawn
	}
	// Block time, never the proposer's clock. The expiry is the registry's
	// promise that a permission granted for one purpose does not sit open for
	// years, and a deadline read from anywhere the issuer controls is no
	// deadline at all.
	if auth.ExpiresAt <= sdk.UnwrapSDKContext(ctx).BlockTime().Unix() {
		return types.ErrAuthorisationExpired
	}
	// The total over the parcel, not this vehicle's share alone. A ceiling
	// checked one vehicle at a time is not a ceiling: an owner permitted to
	// sell 40% mints a second asset over the same parcel and sells 40% again.
	issued, err := k.ParcelIssuedBps.Get(ctx, parcelID)
	if err != nil {
		issued = 0
	}
	// uint32 arithmetic in a domain bounded at 10000 per issuance cannot reach
	// 2^32 without more issuances than a block can hold, but the sum is taken
	// in uint64 anyway: an overflow here would wrap to a small total and reopen
	// the ceiling entirely.
	if uint64(issued)+uint64(issuedShareBps) > uint64(auth.MaxShareBps) {
		return types.ErrShareCeilingExceeded
	}
	return nil
}

// recordIssuedShare adds a vehicle's share to the parcel's running total.
//
// Only ever added to. A vehicle that has been realised and closed still had its
// share sold over the land, and freeing the room back up would let an owner
// cycle vehicles to issue an unbounded total against one authorisation.
func (k Keeper) recordIssuedShare(
	ctx context.Context, parcelID uint64, issuedShareBps uint32,
) error {
	issued, err := k.ParcelIssuedBps.Get(ctx, parcelID)
	if err != nil {
		issued = 0
	}
	return k.ParcelIssuedBps.Set(ctx, parcelID, issued+issuedShareBps)
}
