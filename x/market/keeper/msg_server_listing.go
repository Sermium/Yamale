package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/market/types"
)

// The fixed-price listing: "I will take this much for this parcel."
//
// The simplest of the three and the one most trades will use, because most
// secondary sales of an illiquid private share are agreed between two people
// before either touches a chain. There is no price discovery to do; what the
// chain adds is that neither party has to trust the other to go through with it.

// List offers a parcel of shares for a fixed total.
func (m msgServer) List(ctx context.Context, msg *types.MsgList) (*types.MsgListResponse, error) {
	assetID, settlementDenom, err := m.tradableShare(ctx, msg.Denom)
	if err != nil {
		return nil, err
	}
	if msg.Quantity.IsNil() || !msg.Quantity.IsPositive() {
		return nil, types.ErrInvalidQuantity
	}
	if msg.Price.Amount.IsNil() || !msg.Price.Amount.IsPositive() {
		return nil, types.ErrInvalidPrice
	}
	// The price must be in the vehicle's own income denomination. A listing
	// priced in anything else would write the buyer's cost basis in a currency
	// the vehicle never pays in, and the return computed from it afterwards
	// would subtract one currency from another and print the answer as a
	// percentage.
	if msg.Price.Denom != settlementDenom {
		return nil, types.ErrWrongDenom.Wrapf(
			"this vehicle settles in %s", settlementDenom)
	}

	seller, err := m.addressCodec.StringToBytes(msg.Seller)
	if err != nil {
		return nil, err
	}

	id, err := nextID(ctx, m.NextListingID)
	if err != nil {
		return nil, err
	}

	// The shares leave now. An offer backed by shares the seller still holds can
	// be listed again, or transferred away, and the first buyer to pay finds
	// there is nothing to deliver — at which point the chain has taken money for
	// an asset that is not there and the only remedy left is off-chain, which is
	// exactly the position this market exists to get holders out of.
	if err := m.bankKeeper.SendCoinsFromAccountToModule(
		ctx, seller, types.ModuleName,
		sdk.NewCoins(sdk.NewCoin(msg.Denom, msg.Quantity)),
	); err != nil {
		return nil, err
	}

	listing := types.Listing{
		Id:       id,
		AssetId:  assetID,
		Denom:    msg.Denom,
		Seller:   msg.Seller,
		Quantity: msg.Quantity,
		Price:    msg.Price,
		State:    types.LISTING_STATE_LIVE,
		ListedAt: blockTime(ctx),
	}
	if err := m.Listing.Set(ctx, id, listing); err != nil {
		return nil, err
	}
	if err := m.ListingsByDenom.Set(ctx, collections.Join(msg.Denom, id)); err != nil {
		return nil, err
	}
	return &types.MsgListResponse{Id: id}, nil
}

// Buy takes a listing whole: money one way, shares the other, in one message.
//
// Both legs together, which is the point. A two-message version — pay, then ask
// the seller to deliver — reintroduces exactly the counterparty risk that a
// secondary market for an illiquid private share cannot absorb, because the
// recourse against a defaulting counterparty is a lawsuit worth more than the
// position.
func (m msgServer) Buy(ctx context.Context, msg *types.MsgBuy) (*types.MsgBuyResponse, error) {
	listing, err := m.liveListing(ctx, msg.ListingId)
	if err != nil {
		return nil, err
	}
	// Re-checked at fill, not only at listing. A vehicle can be realised between
	// the two, and the moment that matters is the moment the shares move.
	if _, _, err := m.tradableShare(ctx, listing.Denom); err != nil {
		return nil, err
	}
	if listing.Seller == msg.Buyer {
		return nil, types.ErrSelfDeal
	}

	buyer, err := m.addressCodec.StringToBytes(msg.Buyer)
	if err != nil {
		return nil, err
	}
	seller, err := m.addressCodec.StringToBytes(listing.Seller)
	if err != nil {
		return nil, err
	}

	// The listing is closed BEFORE anything moves.
	//
	// Not for tidiness: deliver sends shares to the buyer, and a share transfer
	// runs x/tokenisation's send restriction, which is arbitrary code this
	// module does not own. Marking the listing filled first means that even if
	// something downstream ever re-entered this handler, it would find the
	// listing no longer live rather than a second chance to spend the same
	// escrow.
	listing.State = types.LISTING_STATE_FILLED
	listing.FilledBy = msg.Buyer
	listing.FilledAt = blockTime(ctx)
	if err := m.Listing.Set(ctx, listing.Id, listing); err != nil {
		return nil, err
	}

	// Settlement first. If the buyer cannot pay, nothing else has happened.
	if err := m.bankKeeper.SendCoins(
		ctx, buyer, seller, sdk.NewCoins(listing.Price),
	); err != nil {
		return nil, err
	}
	if err := m.deliver(
		ctx, listing.AssetId, listing.Denom, seller, buyer, listing.Quantity, listing.Price,
	); err != nil {
		return nil, err
	}

	return &types.MsgBuyResponse{}, nil
}

// CancelListing withdraws an offer and returns the escrowed shares.
func (m msgServer) CancelListing(
	ctx context.Context, msg *types.MsgCancelListing,
) (*types.MsgCancelListingResponse, error) {
	listing, err := m.liveListing(ctx, msg.ListingId)
	if err != nil {
		return nil, err
	}
	if listing.Seller != msg.Seller {
		return nil, types.ErrNotSeller.Wrapf(
			"listing %d belongs to %s", listing.Id, listing.Seller)
	}

	seller, err := m.addressCodec.StringToBytes(listing.Seller)
	if err != nil {
		return nil, err
	}

	// Returned even if the vehicle has since been realised, which is why this
	// path does NOT go through tradableShare. The shares are the seller's; a
	// halt that stopped somebody recovering their own escrowed property would be
	// a confiscation dressed as a safety rule.
	//
	// The send restriction exempts the module account from the halt for exactly
	// this reason, so the transfer succeeds where an ordinary one would not.
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, seller,
		sdk.NewCoins(sdk.NewCoin(listing.Denom, listing.Quantity)),
	); err != nil {
		return nil, err
	}

	listing.State = types.LISTING_STATE_CANCELLED
	return &types.MsgCancelListingResponse{}, m.Listing.Set(ctx, listing.Id, listing)
}

// liveListing resolves a listing that can still be acted on.
func (m msgServer) liveListing(ctx context.Context, id uint64) (types.Listing, error) {
	listing, err := m.Listing.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return types.Listing{}, types.ErrNoListing.Wrapf("listing %d", id)
		}
		return types.Listing{}, err
	}
	if listing.State != types.LISTING_STATE_LIVE {
		return types.Listing{}, types.ErrListingClosed.Wrapf(
			"listing %d is %s", id, listing.State)
	}
	return listing, nil
}
