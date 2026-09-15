package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

/* ========================================================================== */
/*  What a holding cost                                                       */
/* ========================================================================== */

// zeroIfNil is the guard every one of these fields needs.
//
// `math.Int`'s zero value is nil and PANICS on arithmetic, not returns an
// error. Positions written before cost_basis existed decode with both fields
// nil, and a chain that halted the first time somebody transferred one of those
// tokens would be a much worse outcome than the feature being absent. The
// upgrade migrates them to zero; this exists because a migration that missed a
// row must not take the chain down with it.
func zeroIfNil(v math.Int) math.Int {
	if v.IsNil() {
		return math.ZeroInt()
	}
	return v
}

// basisOf returns a position's recorded cost, with both fields safe to use.
func basisOf(pos types.Position) (basis math.Int, qty math.Int) {
	return zeroIfNil(pos.CostBasis), zeroIfNil(pos.CostQty)
}

// RecordSubscription adds to what a holder is recorded as having paid.
//
// Additive: a second subscription adds to both the money and the quantity, so
// an investor who subscribed twice at two prices ends up carrying a weighted
// average — which is what an average cost is.
//
// The quantity is checked against the holder's actual balance. A basis covering
// more tokens than the account holds would report a return on tokens nobody
// has, and it is the sort of error that is invisible until somebody sells.
func (k Keeper) RecordSubscription(
	ctx context.Context, assetID uint64, holder sdk.AccAddress, quantity, paid math.Int,
) (math.Int, math.Int, error) {
	if !quantity.IsPositive() {
		return math.ZeroInt(), math.ZeroInt(), types.ErrInvalidSubscription.Wrap(
			"a subscription must cover a positive number of tokens")
	}
	if paid.IsNegative() {
		return math.ZeroInt(), math.ZeroInt(), types.ErrInvalidSubscription.Wrap(
			"the amount paid cannot be negative")
	}

	asset, err := k.Assets.Get(ctx, assetID)
	if err != nil {
		return math.ZeroInt(), math.ZeroInt(), types.ErrAssetNotFound
	}
	if asset.FractionDenom == "" {
		return math.ZeroInt(), math.ZeroInt(), types.ErrInvalidSubscription.Wrap(
			"nothing has been issued against this asset, so nobody subscribed to anything")
	}

	balance := k.bankKeeper.GetBalance(
		sdk.UnwrapSDKContext(ctx), holder, asset.FractionDenom).Amount

	key := collections.Join(assetID, holder.String())
	pos, err := k.Positions.Get(ctx, key)
	if err != nil {
		// Settling first, so the holder's place in the distribution index is
		// established before a basis is written against it. A position that
		// existed only to carry a cost would be credited the asset's whole
		// distribution history the next time it settled.
		if err := k.Settle(ctx, assetID, holder, balance); err != nil {
			return math.ZeroInt(), math.ZeroInt(), err
		}
		pos, err = k.Positions.Get(ctx, key)
		if err != nil {
			return math.ZeroInt(), math.ZeroInt(), err
		}
	}

	basis, qty := basisOf(pos)
	newQty := qty.Add(quantity)
	if newQty.GT(balance) {
		return math.ZeroInt(), math.ZeroInt(), types.ErrInvalidSubscription.Wrapf(
			"that would record a cost for %s tokens while the account holds %s",
			newQty, balance)
	}

	pos.CostBasis = basis.Add(paid)
	pos.CostQty = newQty
	if err := k.Positions.Set(ctx, key, pos); err != nil {
		return math.ZeroInt(), math.ZeroInt(), err
	}
	return pos.CostBasis, pos.CostQty, nil
}

// CarryBasis moves cost with the tokens on a transfer.
//
// # Why a transfer carries cost at all
//
// A plain bank send names no price, so there is nothing to record as a purchase.
// What there is, is the sender's own cost for those tokens — and a transfer with
// no consideration genuinely inherits it in most treatments. Carrying it
// proportionally keeps the sender's remaining basis matched to their remaining
// tokens, which is the property that makes an average cost stay true.
//
// # What it must never do
//
// Give the recipient a basis of zero and call it a cost. Zero cost reports an
// infinite return, and it is the most flattering figure this module could
// produce; a recipient whose sender had no recorded basis gets none, and the
// interface shows COST NOT RECORDED rather than a percentage.
//
// # Proportional, and rounded down
//
// The carried amount is `basis * moved / covered`, truncated. Truncation leaves
// the fraction with the SENDER, which is the direction that cannot manufacture
// cost: the total basis across both sides after a transfer is never more than
// it was before, so no sequence of transfers can inflate a return.
func (k Keeper) CarryBasis(
	ctx context.Context, assetID uint64, from, to sdk.AccAddress, moved math.Int,
) error {
	if !moved.IsPositive() || from.Equals(to) {
		return nil
	}

	fromKey := collections.Join(assetID, from.String())
	fromPos, err := k.Positions.Get(ctx, fromKey)
	if err != nil {
		return nil // no position, so no basis to carry
	}
	basis, qty := basisOf(fromPos)
	if !qty.IsPositive() || !basis.IsPositive() {
		return nil
	}

	// A sender may move more tokens than their basis covers — part subscribed,
	// part received untracked. Only the covered part carries.
	covered := moved
	if covered.GT(qty) {
		covered = qty
	}
	carried := basis.Mul(covered).Quo(qty)

	fromPos.CostBasis = basis.Sub(carried)
	fromPos.CostQty = qty.Sub(covered)
	if err := k.Positions.Set(ctx, fromKey, fromPos); err != nil {
		return err
	}

	toKey := collections.Join(assetID, to.String())
	toPos, err := k.Positions.Get(ctx, toKey)
	if err != nil {
		return nil // the recipient is settled by the restriction; nothing to hold basis yet
	}
	toBasis, toQty := basisOf(toPos)
	toPos.CostBasis = toBasis.Add(carried)
	toPos.CostQty = toQty.Add(covered)
	return k.Positions.Set(ctx, toKey, toPos)
}
