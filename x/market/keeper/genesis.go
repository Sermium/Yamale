package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/market/types"
)

// InitGenesis writes a market in, rebuilding the indexes from the records.
//
// The by-denom indexes and the book are derived rather than imported: they are
// pure functions of the listings, auctions and orders, so importing them would
// import the same fact twice and invite the two copies to disagree. A book that
// disagreed with its orders would quote a price against an order that is no
// longer open, and the cross that followed would find nothing to fill.
//
// The counters are the opposite case. They are NOT recomputed from the highest
// id present, because a counter derived that way moves backwards on any file
// whose newest record has been removed — and an identifier handed out twice is
// two trades sharing a name, one of which has somebody's escrow behind it.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}

	for _, l := range gs.Listings {
		if err := k.Listing.Set(ctx, l.Id, l); err != nil {
			return err
		}
		if err := k.ListingsByDenom.Set(ctx, collections.Join(l.Denom, l.Id)); err != nil {
			return err
		}
	}
	for _, a := range gs.Auctions {
		if err := k.Auction.Set(ctx, a.Id, a); err != nil {
			return err
		}
		if err := k.AuctionsByDenom.Set(ctx, collections.Join(a.Denom, a.Id)); err != nil {
			return err
		}
	}
	for _, o := range gs.Orders {
		if err := k.Order.Set(ctx, o.Id, o); err != nil {
			return err
		}
		// Only OPEN orders go into the book. A filled or cancelled order is
		// history: it escrows nothing, so a key pointing at it would offer a
		// counterparty property the module no longer holds.
		if o.State == types.ORDER_STATE_OPEN {
			if err := k.book(o.Side).Set(ctx, collections.Join3(
				o.Denom, bookPriceKey(o.Side, o.UnitPrice), o.Id,
			)); err != nil {
				return err
			}
		}
	}

	if err := k.NextListingID.Set(ctx, gs.NextListingId); err != nil {
		return err
	}
	if err := k.NextAuctionID.Set(ctx, gs.NextAuctionId); err != nil {
		return err
	}
	return k.NextOrderID.Set(ctx, gs.NextOrderId)
}

// ExportGenesis reads the market out.
//
// Every record, not only the live ones. A filled listing is the record of a
// price somebody actually paid for a share in a vehicle that may have four
// holders and no other trade in its history — which makes it the scarcest
// information this chain holds about what such a thing is worth, and the
// cheapest thing to lose by exporting only what is still open.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	gs := types.GenesisState{Params: params}

	if err := k.Listing.Walk(ctx, nil, func(_ uint64, l types.Listing) (bool, error) {
		gs.Listings = append(gs.Listings, l)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Auction.Walk(ctx, nil, func(_ uint64, a types.Auction) (bool, error) {
		gs.Auctions = append(gs.Auctions, a)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Order.Walk(ctx, nil, func(_ uint64, o types.Order) (bool, error) {
		gs.Orders = append(gs.Orders, o)
		return false, nil
	}); err != nil {
		return nil, err
	}

	// Peek rather than Next: reading a counter must not advance it. Exporting
	// twice has to produce the same file both times, or the export is not a
	// description of the chain but an event in its history.
	if gs.NextListingId, err = k.NextListingID.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.NextAuctionId, err = k.NextAuctionID.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.NextOrderId, err = k.NextOrderID.Peek(ctx); err != nil {
		return nil, err
	}
	return &gs, nil
}
