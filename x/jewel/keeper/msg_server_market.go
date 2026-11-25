package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/jewel/types"
)

// The marketplace. An owner names a price; anyone pays it and receives the
// jewel in the same transaction, so nobody ever holds the money and nobody has
// to be trusted to hand the jewel over. The fee is kept in x/jewel's own
// account, which no key opens; only governance pays out of it.

// ListJewel puts a jewel up for sale, or changes its price.
func (k msgServer) ListJewel(ctx context.Context, req *types.MsgListJewel) (*types.MsgListJewelResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	owner, err := k.actsForOwner(ctx, req.Owner, j)
	if err != nil {
		return nil, err
	}
	if err := k.requireSaleable(ctx, owner, j); err != nil {
		return nil, err
	}
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := req.Price.Validate(); err != nil || !req.Price.IsPositive() {
		return nil, types.ErrNoPrice
	}
	if !p.ForSale(req.Price.Denom) {
		return nil, types.ErrDenomNotForSale.Wrapf("%s", req.Price.Denom)
	}
	l := types.Listing{Seller: owner, Price: req.Price, ListedAt: k.now(ctx)}
	if err := k.Listing.Set(ctx, j.Id, l); err != nil {
		return nil, err
	}
	return &types.MsgListJewelResponse{},
		k.emit(ctx, &types.EventJewelListed{JewelId: j.Id, Seller: owner, Price: req.Price})
}

// UnlistJewel takes a jewel off the marketplace.
func (k msgServer) UnlistJewel(ctx context.Context, req *types.MsgUnlistJewel) (*types.MsgUnlistJewelResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.actsForOwner(ctx, req.Owner, j); err != nil {
		return nil, err
	}
	if has, err := k.Listing.Has(ctx, j.Id); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNotListed.Wrapf("jewel %d", j.Id)
	}
	if err := k.Listing.Remove(ctx, j.Id); err != nil {
		return nil, err
	}
	return &types.MsgUnlistJewelResponse{}, k.emit(ctx, &types.EventJewelUnlisted{JewelId: j.Id})
}

// BuyJewel pays for a listed jewel and takes it.
func (k msgServer) BuyJewel(ctx context.Context, req *types.MsgBuyJewel) (*types.MsgBuyJewelResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	l, err := k.Listing.Get(ctx, j.Id)
	if isNotFound(err) {
		return nil, types.ErrNotListed.Wrapf("jewel %d", j.Id)
	}
	if err != nil {
		return nil, err
	}
	buyer, err := k.canonical(req.Buyer)
	if err != nil {
		return nil, err
	}
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	// Every change of owner removes the listing, so a listing by someone
	// else than the owner cannot exist; refuse rather than pay a stranger.
	if l.Seller != owner {
		return nil, types.ErrNotListed.Wrapf("jewel %d", j.Id)
	}
	if buyer == owner {
		return nil, types.ErrBuyOwn
	}
	if req.Price.Denom != l.Price.Denom || req.Price.Amount.IsNil() || !req.Price.Amount.Equal(l.Price.Amount) {
		return nil, types.ErrPriceMoved.Wrapf("listed at %s", l.Price)
	}
	// Checked again at the sale: the jewel may have been reported stolen, a
	// reassignment proposed or its shop suspended since it was listed.
	if err := k.requireSaleable(ctx, owner, j); err != nil {
		return nil, err
	}
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	fee := sdk.NewCoin(l.Price.Denom, l.Price.Amount.MulRaw(int64(p.SaleFeeBps)).Quo(math.NewInt(10_000)))
	toSeller := l.Price.Sub(fee)
	buyerAddr, err := k.addressCodec.StringToBytes(buyer)
	if err != nil {
		return nil, types.ErrInvalidAddress.Wrapf("%s", buyer)
	}
	sellerAddr, err := k.addressCodec.StringToBytes(owner)
	if err != nil {
		return nil, err
	}
	if toSeller.IsPositive() {
		if err := k.bank.SendCoins(ctx, buyerAddr, sellerAddr, sdk.NewCoins(toSeller)); err != nil {
			return nil, err
		}
	}
	if fee.IsPositive() {
		if err := k.bank.SendCoinsFromAccountToModule(ctx, buyerAddr, types.ModuleName, sdk.NewCoins(fee)); err != nil {
			return nil, err
		}
	}
	if err := k.transfer(ctx, &j, buyer, types.TransferSale); err != nil {
		return nil, err
	}
	return &types.MsgBuyJewelResponse{},
		k.emit(ctx, &types.EventJewelSold{JewelId: j.Id, Seller: owner, Buyer: buyer, Price: l.Price, Fee: fee})
}

// WithdrawSaleFees pays marketplace fees out of x/jewel's account.
func (k msgServer) WithdrawSaleFees(ctx context.Context, req *types.MsgWithdrawSaleFees) (*types.MsgWithdrawSaleFeesResponse, error) {
	authority, err := k.addressCodec.BytesToString(k.authority)
	if err != nil {
		return nil, err
	}
	signer, err := k.canonical(req.Authority)
	if err != nil {
		return nil, err
	}
	if signer != authority {
		return nil, types.ErrInvalidSigner.Wrapf("%s", req.Authority)
	}
	to, err := k.addressCodec.StringToBytes(req.To)
	if err != nil {
		return nil, types.ErrInvalidAddress.Wrapf("%s", req.To)
	}
	if !req.Amount.IsValid() || req.Amount.IsZero() {
		return nil, types.ErrNoPrice
	}
	if err := k.bank.SendCoinsFromModuleToAccount(ctx, types.ModuleName, to, req.Amount); err != nil {
		return nil, err
	}
	return &types.MsgWithdrawSaleFeesResponse{}, nil
}

// requireSaleable refuses a sale of a stolen jewel, of one with a reassignment
// pending, and of a suspended jeweler's stock.
func (k Keeper) requireSaleable(ctx context.Context, owner string, j types.Jewel) error {
	if j.Stolen {
		return types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	if has, err := k.Reassignment.Has(ctx, j.Id); err != nil {
		return err
	} else if has {
		return types.ErrReassignmentOpen.Wrapf("jewel %d", j.Id)
	}
	return k.requireSellable(ctx, owner, j)
}
