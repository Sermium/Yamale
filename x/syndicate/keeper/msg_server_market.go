package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/syndicate/types"
)

// The secondary market.
//
// This is the differentiator the whole proposition rests on: an investor is not
// locked in for the life of the investment. Private companies have historically
// offered no liquidity at all, and the reason is administrative rather than
// legal — a paper syndicate CAN sell a position on, but the cost of
// re-papering the cap table and tracking who is owed what afterwards exceeds
// the value of most angel-sized positions.
//
// That cost is what the distribution index removes. Once entitlement is
// computed from a balance and an index, a sale is a transfer and nothing else:
// no reconciliation, no chain of historical owners, no statement to reissue.
//
// Two shapes, and both are here because the private one is what actually
// happens. An angel sells to somebody they already know; an open listing is the
// exception, not the rule.

// List offers part of a position for sale.
//
// The tokens are escrowed on listing. Otherwise a seller could list the same
// position three times, or list and then transfer away, and the first buyer to
// pay would find the seller had nothing to deliver — at which point the
// platform is arbitrating a dispute it has no facts about.
func (m msgServer) List(ctx context.Context, msg *types.MsgList) (*types.MsgListResponse, error) {
	deal, err := m.tradableDeal(ctx, msg.DealId)
	if err != nil {
		return nil, err
	}
	if !msg.Quantity.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	if msg.Price.Denom != deal.SettlementDenom {
		return nil, types.ErrWrongDenom.Wrapf(
			"this deal settles in %s", deal.SettlementDenom)
	}
	if !msg.Price.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}

	seller, err := m.addressCodec.StringToBytes(msg.Seller)
	if err != nil {
		return nil, err
	}

	// A named buyer must already be onboarded, checked HERE rather than only at
	// fill. A private listing to somebody who cannot legally receive it is a
	// deal the seller believes is done, and they find out otherwise only when
	// the buyer tries to pay.
	if msg.Buyer != "" {
		if _, err := m.addressCodec.StringToBytes(msg.Buyer); err != nil {
			return nil, types.ErrInvalidInvestor.Wrapf("%q", msg.Buyer)
		}
		if msg.Buyer == msg.Seller {
			return nil, types.ErrSelfDeal
		}
		if err := m.eligible(ctx, msg.Buyer); err != nil {
			return nil, err
		}
	}

	id, err := nextID(ctx, m.NextListingID)
	if err != nil {
		return nil, err
	}

	if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, seller, types.ModuleName,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, msg.Quantity))); err != nil {
		return nil, err
	}

	listing := types.Listing{
		Id:       id,
		DealId:   deal.Id,
		Seller:   msg.Seller,
		Buyer:    msg.Buyer,
		Quantity: msg.Quantity,
		Price:    msg.Price,
		State:    types.LISTING_STATE_LIVE,
		ListedAt: blockTime(ctx),
	}
	if err := m.Listing.Set(ctx, id, listing); err != nil {
		return nil, err
	}
	if err := m.ListingsByDeal.Set(ctx, collections.Join(deal.Id, id)); err != nil {
		return nil, err
	}
	return &types.MsgListResponse{Id: id}, nil
}

// CancelListing withdraws an offer and returns the escrowed tokens.
func (m msgServer) CancelListing(
	ctx context.Context, msg *types.MsgCancelListing,
) (*types.MsgCancelListingResponse, error) {
	listing, err := m.liveListing(ctx, msg.ListingId)
	if err != nil {
		return nil, err
	}
	if listing.Seller != msg.Seller {
		return nil, types.ErrNotSeller.Wrapf("listing %d belongs to %s", listing.Id, listing.Seller)
	}

	deal, err := m.Deal.Get(ctx, listing.DealId)
	if err != nil {
		return nil, err
	}
	seller, err := m.addressCodec.StringToBytes(listing.Seller)
	if err != nil {
		return nil, err
	}

	// Returned even on a realised deal. The tokens are the seller's; a halt
	// that stopped somebody recovering their own escrowed property would be a
	// confiscation dressed as a safety rule.
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, seller,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, listing.Quantity))); err != nil {
		return nil, err
	}

	listing.State = types.LISTING_STATE_CANCELLED
	return &types.MsgCancelListingResponse{}, m.Listing.Set(ctx, listing.Id, listing)
}

// Buy fills a listing: settlement one way, tokens the other, in one message.
//
// Both legs together, which is the whole point. Ownership transfers
// automatically on completion of sale, so neither party holds the other's asset
// while waiting for their own. A two-message version — pay, then ask the seller
// to deliver — reintroduces exactly the counterparty risk that a secondary
// market for an illiquid private position cannot absorb, because the recourse
// for a defaulting counterparty is a lawsuit worth more than the position.
func (m msgServer) Buy(ctx context.Context, msg *types.MsgBuy) (*types.MsgBuyResponse, error) {
	listing, err := m.liveListing(ctx, msg.ListingId)
	if err != nil {
		return nil, err
	}
	deal, err := m.tradableDeal(ctx, listing.DealId)
	if err != nil {
		return nil, err
	}

	if listing.Seller == msg.Buyer {
		return nil, types.ErrSelfDeal
	}
	if listing.Buyer != "" && listing.Buyer != msg.Buyer {
		return nil, types.ErrNotNamedBuyer.Wrapf("listing %d names %s", listing.Id, listing.Buyer)
	}

	// Re-checked at fill even for a private listing whose buyer was checked at
	// listing time. Onboarding can be withdrawn between the two, and the moment
	// that matters is the moment the tokens move.
	if err := m.eligible(ctx, msg.Buyer); err != nil {
		return nil, err
	}

	buyer, err := m.addressCodec.StringToBytes(msg.Buyer)
	if err != nil {
		return nil, err
	}
	seller, err := m.addressCodec.StringToBytes(listing.Seller)
	if err != nil {
		return nil, err
	}

	// Settlement first. If the buyer cannot pay, nothing else has happened.
	if err := m.bankKeeper.SendCoins(ctx, buyer, seller,
		sdk.NewCoins(listing.Price)); err != nil {
		return nil, err
	}
	// Then the tokens, out of escrow. This goes through the bank, so it goes
	// through the send restriction, which settles both sides of the
	// distribution index before the balances move.
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, buyer,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, listing.Quantity))); err != nil {
		return nil, err
	}

	listing.State = types.LISTING_STATE_FILLED
	listing.FilledBy = msg.Buyer
	listing.FilledAt = blockTime(ctx)
	return &types.MsgBuyResponse{}, m.Listing.Set(ctx, listing.Id, listing)
}

// tradableDeal resolves a deal whose tokens exist and still move.
func (m msgServer) tradableDeal(ctx context.Context, id uint64) (types.Deal, error) {
	deal, err := m.Deal.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return types.Deal{}, types.ErrNoDeal.Wrapf("deal %d", id)
		}
		return types.Deal{}, err
	}
	switch deal.State {
	case types.DEAL_STATE_CLOSED:
		return deal, nil
	case types.DEAL_STATE_REALISED:
		return types.Deal{}, types.ErrTradingHalted.Wrapf("deal %d", id)
	default:
		return types.Deal{}, types.ErrDealNotFunded.Wrapf("deal %d is %s", id, deal.State)
	}
}

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
