package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/market/types"
)

// The auction: "what is this worth?"
//
// What a seller reaches for when the share is illiquid enough that there is no
// price to point at — which, for a closed-end vehicle over a single building or
// a single plantation, is the ordinary case rather than the exotic one. The
// seller commits the parcel and a deadline; the market answers with a number.
//
// The whole mechanism holds exactly ONE bidder's money at a time. See
// Auction.best_bidder in the proto for why, and for what that costs: there is no
// second place, and being outbid means being out rather than queued. The gain is
// that nothing in this file loops over anything.

// OpenAuction offers a parcel to the best bid received by a deadline.
func (m msgServer) OpenAuction(
	ctx context.Context, msg *types.MsgOpenAuction,
) (*types.MsgOpenAuctionResponse, error) {
	assetID, settlementDenom, err := m.tradableShare(ctx, msg.Denom)
	if err != nil {
		return nil, err
	}
	if msg.Quantity.IsNil() || !msg.Quantity.IsPositive() {
		return nil, types.ErrInvalidQuantity
	}
	// A reserve may be zero — a seller saying they will take whatever it fetches
	// — but it may not be unset. An unset Coin decodes with a nil amount, and a
	// nil amount is not zero: it panics on the first comparison, which here would
	// be the first bid.
	if msg.Reserve.Amount.IsNil() || msg.Reserve.Amount.IsNegative() {
		return nil, types.ErrInvalidPrice.Wrap(
			"a reserve must be an explicit amount; use zero for no reserve")
	}
	if msg.Reserve.Denom != settlementDenom {
		return nil, types.ErrWrongDenom.Wrapf(
			"this vehicle settles in %s", settlementDenom)
	}

	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	period := msg.ClosesAt - blockTime(ctx)
	if period < params.MinAuctionPeriod {
		return nil, types.ErrAuctionTooShort.Wrapf(
			"this auction would run for %d seconds; the minimum is %d",
			period, params.MinAuctionPeriod)
	}
	if period > params.MaxAuctionPeriod {
		return nil, types.ErrAuctionTooLong.Wrapf(
			"this auction would escrow the shares for %d seconds; the maximum is %d",
			period, params.MaxAuctionPeriod)
	}

	seller, err := m.addressCodec.StringToBytes(msg.Seller)
	if err != nil {
		return nil, err
	}

	id, err := nextID(ctx, m.NextAuctionID)
	if err != nil {
		return nil, err
	}

	if err := m.bankKeeper.SendCoinsFromAccountToModule(
		ctx, seller, types.ModuleName,
		sdk.NewCoins(sdk.NewCoin(msg.Denom, msg.Quantity)),
	); err != nil {
		return nil, err
	}

	auction := types.Auction{
		Id:       id,
		AssetId:  assetID,
		Denom:    msg.Denom,
		Seller:   msg.Seller,
		Quantity: msg.Quantity,
		Reserve:  msg.Reserve,
		ClosesAt: msg.ClosesAt,
		// Explicitly zero, never left unset, for the same reason as the reserve:
		// this is compared against the first bid that arrives, and a nil amount
		// would halt the chain in that block rather than compare as none.
		BestBid:  sdk.NewCoin(settlementDenom, math.ZeroInt()),
		State:    types.AUCTION_STATE_OPEN,
		OpenedAt: blockTime(ctx),
	}
	if err := m.Auction.Set(ctx, id, auction); err != nil {
		return nil, err
	}
	if err := m.AuctionsByDenom.Set(ctx, collections.Join(msg.Denom, id)); err != nil {
		return nil, err
	}
	return &types.MsgOpenAuctionResponse{Id: id}, nil
}

// Bid improves on the standing bid, refunding it in the same message.
//
// The refund happens HERE, paid for by the bidder who caused it, rather than at
// settlement. That is what keeps the whole mechanism free of loops: at any
// moment the module holds one bidder's money per auction, so settlement moves
// exactly two things however many people bid.
func (m msgServer) Bid(ctx context.Context, msg *types.MsgBid) (*types.MsgBidResponse, error) {
	auction, err := m.openAuction(ctx, msg.AuctionId)
	if err != nil {
		return nil, err
	}
	// The deadline is enforced on the way in. An auction that kept taking bids
	// after it closed would let somebody who watched the result bid one unit
	// more than the winner.
	if blockTime(ctx) >= auction.ClosesAt {
		return nil, types.ErrAuctionClosed.Wrapf(
			"auction %d closed at %d", auction.Id, auction.ClosesAt)
	}
	if auction.Seller == msg.Bidder {
		// A seller bidding on their own auction is a reserve they did not have
		// to declare, paid for with money that comes straight back to them.
		return nil, types.ErrSelfDeal
	}
	if _, _, err := m.tradableShare(ctx, auction.Denom); err != nil {
		return nil, err
	}

	if msg.Amount.Amount.IsNil() || !msg.Amount.Amount.IsPositive() {
		return nil, types.ErrInvalidPrice
	}
	if msg.Amount.Denom != auction.BestBid.Denom {
		return nil, types.ErrWrongDenom.Wrapf(
			"this auction settles in %s", auction.BestBid.Denom)
	}
	if msg.Amount.Amount.LT(auction.Reserve.Amount) {
		return nil, types.ErrBidTooLow.Wrapf(
			"the reserve is %s", auction.Reserve)
	}
	// Strictly greater. Equal would replace one bidder with another for the same
	// money — the seller gains nothing and the earlier bidder, who committed
	// first, loses their place to somebody who matched them.
	if !msg.Amount.Amount.GT(auction.BestBid.Amount) {
		return nil, types.ErrBidTooLow.Wrapf(
			"the standing bid is %s", auction.BestBid)
	}

	bidder, err := m.addressCodec.StringToBytes(msg.Bidder)
	if err != nil {
		return nil, err
	}

	// The new money in first, then the old money out. In that order, so that a
	// bidder who cannot actually pay is refused before the standing bid has been
	// refunded — the other order would let anybody unseat a real bid with an
	// offer they had no funds for, and the auction would lose its best price to
	// a message that then failed.
	if err := m.bankKeeper.SendCoinsFromAccountToModule(
		ctx, bidder, types.ModuleName, sdk.NewCoins(msg.Amount),
	); err != nil {
		return nil, err
	}
	if auction.BestBidder != "" {
		previous, err := m.addressCodec.StringToBytes(auction.BestBidder)
		if err != nil {
			return nil, err
		}
		if err := m.bankKeeper.SendCoinsFromModuleToAccount(
			ctx, types.ModuleName, previous, sdk.NewCoins(auction.BestBid),
		); err != nil {
			return nil, err
		}
	}

	auction.BestBidder = msg.Bidder
	auction.BestBid = msg.Amount
	return &types.MsgBidResponse{}, m.Auction.Set(ctx, auction.Id, auction)
}

// SettleAuction closes an auction out after its deadline.
//
// Anybody may send it. The winner's money and the seller's shares are both in
// the module account, and if settlement needed the seller's signature a seller
// who disliked the result could strand the winner's capital indefinitely by
// doing nothing.
//
// There is no sweeper that would do this automatically. See the module's
// AppModule for why.
func (m msgServer) SettleAuction(
	ctx context.Context, msg *types.MsgSettleAuction,
) (*types.MsgSettleAuctionResponse, error) {
	auction, err := m.openAuction(ctx, msg.AuctionId)
	if err != nil {
		return nil, err
	}
	if blockTime(ctx) < auction.ClosesAt {
		return nil, types.ErrAuctionRunning.Wrapf(
			"auction %d closes at %d", auction.Id, auction.ClosesAt)
	}

	seller, err := m.addressCodec.StringToBytes(auction.Seller)
	if err != nil {
		return nil, err
	}

	// Closed before anything moves, for the same reason Buy marks the listing
	// filled first: delivery runs the send restriction, which is code this
	// module does not own, and a settled auction must not be settleable twice.
	auction.State = types.AUCTION_STATE_SETTLED
	auction.SettledAt = blockTime(ctx)
	if err := m.Auction.Set(ctx, auction.Id, auction); err != nil {
		return nil, err
	}

	// A vehicle realised while the auction was running cannot have its shares
	// delivered to a new holder, and should not: they have become a claim on a
	// known fixed sum, which is not the thing anybody bid on.
	//
	// So the auction UNWINDS rather than failing. Failing is the worse half of
	// both outcomes — the shares stay in escrow and the winner's money stays
	// with them, permanently, because a realised vehicle never becomes tradable
	// again. Any error from the register unwinds, not only a halt: if it cannot
	// say the vehicle still trades, this is not the moment to assume it.
	_, _, tradeErr := m.tradableShare(ctx, auction.Denom)

	// Nobody bid, nobody beat the reserve — which is the same thing, because a
	// bid below the reserve was never accepted — or there is nothing left to
	// sell them. Everybody gets their own property back.
	//
	// Returning the shares is deliberately NOT gated on the vehicle still
	// trading. The seller is recovering what was already theirs, and a halt that
	// prevented it would strand them for the life of the vehicle. The register
	// exempts registered escrows from the halt for exactly this case.
	if auction.BestBidder == "" || tradeErr != nil {
		if err := m.bankKeeper.SendCoinsFromModuleToAccount(
			ctx, types.ModuleName, seller,
			sdk.NewCoins(sdk.NewCoin(auction.Denom, auction.Quantity)),
		); err != nil {
			return nil, err
		}
		if auction.BestBidder != "" {
			bidder, err := m.addressCodec.StringToBytes(auction.BestBidder)
			if err != nil {
				return nil, err
			}
			if err := m.bankKeeper.SendCoinsFromModuleToAccount(
				ctx, types.ModuleName, bidder, sdk.NewCoins(auction.BestBid),
			); err != nil {
				return nil, err
			}
		}
		return &types.MsgSettleAuctionResponse{}, nil
	}

	winner, err := m.addressCodec.StringToBytes(auction.BestBidder)
	if err != nil {
		return nil, err
	}

	// The money to the seller, the shares to the winner. Both out of escrow:
	// neither party has held the other's property at any point since the bid.
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, seller, sdk.NewCoins(auction.BestBid),
	); err != nil {
		return nil, err
	}
	if err := m.deliver(
		ctx, auction.AssetId, auction.Denom, seller, winner, auction.Quantity, auction.BestBid,
	); err != nil {
		return nil, err
	}

	return &types.MsgSettleAuctionResponse{}, nil
}

// CancelAuction withdraws an auction nobody has bid on.
func (m msgServer) CancelAuction(
	ctx context.Context, msg *types.MsgCancelAuction,
) (*types.MsgCancelAuctionResponse, error) {
	auction, err := m.openAuction(ctx, msg.AuctionId)
	if err != nil {
		return nil, err
	}
	if auction.Seller != msg.Seller {
		return nil, types.ErrNotSeller.Wrapf(
			"auction %d belongs to %s", auction.Id, auction.Seller)
	}
	// Once somebody has committed money the seller is in it. A seller who could
	// withdraw after seeing the bids would be running a free valuation exercise
	// paid for by the bidders, whose capital was escrowed for the duration and
	// who would get nothing for it.
	if auction.BestBidder != "" {
		return nil, types.ErrAuctionHasBid.Wrapf(
			"auction %d has a standing bid of %s; it can only be settled",
			auction.Id, auction.BestBid)
	}

	seller, err := m.addressCodec.StringToBytes(auction.Seller)
	if err != nil {
		return nil, err
	}
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, seller,
		sdk.NewCoins(sdk.NewCoin(auction.Denom, auction.Quantity)),
	); err != nil {
		return nil, err
	}

	auction.State = types.AUCTION_STATE_CANCELLED
	return &types.MsgCancelAuctionResponse{}, m.Auction.Set(ctx, auction.Id, auction)
}

// openAuction resolves an auction that has not been closed out.
func (m msgServer) openAuction(ctx context.Context, id uint64) (types.Auction, error) {
	auction, err := m.Auction.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return types.Auction{}, types.ErrNoAuction.Wrapf("auction %d", id)
		}
		return types.Auction{}, err
	}
	if auction.State != types.AUCTION_STATE_OPEN {
		return types.Auction{}, types.ErrAuctionClosed.Wrapf(
			"auction %d is %s", id, auction.State)
	}
	return auction, nil
}
