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

	// Cost stands still when either side is an escrow. See Keeper.escrowAccounts
	// for why, and for what writes the buyer's cost instead.
	if k.IsEscrow(from) || k.IsEscrow(to) {
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

	// The RECIPIENT is resolved before the sender is debited, and the order is
	// the whole of the correctness here.
	//
	// Written the other way round — debit, then look for somewhere to credit —
	// a recipient with no position makes this return having taken cost off the
	// sender and given it to nobody. The sender's reported return then rises,
	// permanently, because they are shown as having paid less for what they
	// still hold. Inside the send restriction the recipient is always settled
	// first so the position exists, which is exactly why the defect survived:
	// it is unreachable by the one path anybody tests, and waiting for the
	// second caller.
	toKey := collections.Join(assetID, to.String())
	toPos, err := k.Positions.Get(ctx, toKey)
	if err != nil {
		return nil // nowhere to carry it to, so nothing moves
	}

	fromPos.CostBasis = basis.Sub(carried)
	fromPos.CostQty = qty.Sub(covered)
	if err := k.Positions.Set(ctx, fromKey, fromPos); err != nil {
		return err
	}

	toBasis, toQty := basisOf(toPos)
	toPos.CostBasis = toBasis.Add(carried)
	toPos.CostQty = toQty.Add(covered)
	return k.Positions.Set(ctx, toKey, toPos)
}

/* ========================================================================== */
/*  What a trade says a holding cost                                          */
/* ========================================================================== */

// RecordPurchase writes what a buyer paid for shares a marketplace has just
// delivered them.
//
// # Why a fill is allowed to write cost when a transfer is not
//
// Because nobody has to be believed. A subscription records a figure the party
// that took the money asserts; a plain transfer names no figure at all, which is
// why CarryBasis can only move the sender's own cost along. A fill is the one
// event where the price IS the transaction: the money moved, the chain saw it
// move, and the number written here is the number the buyer actually parted
// with. It is the second of the two events Position.cost_basis exists for, and
// the only one that needs no trusted party.
//
// # Additive, like a subscription
//
// A buyer who bought twice at two prices carries the weighted average of the
// two, which is what an average cost is. Nothing here overwrites: a fill adds
// to what a holding already cost, so buying more of something never erases what
// the earlier shares cost.
//
// # Why the escrow must be basis-neutral for this to be right
//
// This ADDS, so it is only correct if the transfer that delivered the shares
// carried no cost in with them. It did not, because a marketplace escrow is
// registered with RegisterEscrow and CarryBasis stands still across one. Were
// that registration ever dropped, a buyer would be credited the escrow's pooled
// cost AND the price they paid, and their reported return would fall by roughly
// the amount of somebody else's.
func (k Keeper) RecordPurchase(
	ctx context.Context, assetID uint64, buyer sdk.AccAddress, quantity, paid math.Int,
) error {
	if quantity.IsNil() || !quantity.IsPositive() {
		return types.ErrInvalidSubscription.Wrap(
			"a purchase must cover a positive number of shares")
	}
	if paid.IsNil() || paid.IsNegative() {
		return types.ErrInvalidSubscription.Wrap("the amount paid cannot be negative")
	}

	key := collections.Join(assetID, buyer.String())
	pos, err := k.Positions.Get(ctx, key)
	if err != nil {
		// The delivery settled the buyer through the send restriction, so this
		// should not happen. If it does, the position is created against the
		// balance they now hold rather than the transaction being refused: the
		// shares have already moved and the money has already been paid, and a
		// failure here would unwind a trade that was in every other respect
		// complete, to protect a figure that is only ever reporting.
		asset, err := k.Assets.Get(ctx, assetID)
		if err != nil {
			return err
		}
		balance := k.bankKeeper.GetBalance(
			sdk.UnwrapSDKContext(ctx), buyer, asset.FractionDenom).Amount
		if err := k.Settle(ctx, assetID, buyer, balance); err != nil {
			return err
		}
		pos, err = k.Positions.Get(ctx, key)
		if err != nil {
			return err
		}
	}

	basis, qty := basisOf(pos)
	pos.CostBasis = basis.Add(paid)
	pos.CostQty = qty.Add(quantity)
	return k.Positions.Set(ctx, key, pos)
}

// ReleaseBasis consumes a seller's recorded cost for shares they have sold.
//
// The counterpart to RecordPurchase, and it is not optional. A seller whose
// cost stayed behind after the shares left would be carrying the cost of ten
// thousand shares against the two thousand they still hold — which reports a
// catastrophic loss on a position that has not moved, and reports it to the
// person least able to tell it is wrong.
//
// Proportional and truncated, in the same direction as CarryBasis: the fraction
// stays with the seller. Rounding the other way would let a sequence of small
// sales release more cost than was ever recorded, and cost that can be
// manufactured is a reported return that can be manufactured with it.
func (k Keeper) ReleaseBasis(
	ctx context.Context, assetID uint64, seller sdk.AccAddress, quantity math.Int,
) error {
	if quantity.IsNil() || !quantity.IsPositive() {
		return nil
	}

	key := collections.Join(assetID, seller.String())
	pos, err := k.Positions.Get(ctx, key)
	if err != nil {
		return nil // no position, so no cost was ever recorded to release
	}
	basis, qty := basisOf(pos)
	if !qty.IsPositive() || !basis.IsPositive() {
		return nil
	}

	// A seller may sell more shares than their basis covers — part subscribed at
	// a recorded price, part received as a plain transfer from somebody who had
	// none. Only the covered part is released, and the uncovered part is exactly
	// what the interface reports as not covered rather than as free.
	covered := quantity
	if covered.GT(qty) {
		covered = qty
	}
	released := basis.Mul(covered).Quo(qty)

	pos.CostBasis = basis.Sub(released)
	pos.CostQty = qty.Sub(covered)
	return k.Positions.Set(ctx, key, pos)
}
