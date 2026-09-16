package keeper

import (
	"context"
	stdmath "math"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/market/types"
)

// The order book: "what is it worth right now?"
//
// The only one of the three mechanisms that needs two sides to be present at
// once, and therefore the only one that will be empty for most vehicles on this
// chain. That is not a defect to be engineered around — a book with two orders
// in it is an accurate picture of the interest in a share over one building —
// but it does mean the code has to behave well at both extremes: a book with
// nothing in it, and a book somebody has deliberately made enormous.

// bookPriceKey maps a price to the key the book indexes it under.
//
// # Why a bid's price is stored inverted
//
// Both sides are walked ASCENDING, and for asks that is already what "best
// first" means: the cheapest ask is the one a buyer wants. For bids it is the
// opposite — the highest bid is the best — which would mean walking that index
// backwards.
//
// Walking backwards gets the prices right and the TIME wrong. Within one price
// the key is the order id, so a descending scan yields the newest order first,
// and the trader who rested at that price first would be filled last. Price-time
// priority is not decoration: it is the only reason anybody rests an order on a
// thin book rather than waiting to take, and a book that quietly rewards the
// latest arrival at each price teaches everybody to wait.
//
// Inverting the price instead gives an ascending scan that is highest-price
// first AND, within a price, oldest first. One scan direction, both priorities
// correct, and the arithmetic is its own inverse so the stored key decodes back
// to the real price with the same expression.
func bookPriceKey(side types.Side, unitPrice uint64) uint64 {
	if side == types.SIDE_BID {
		return stdmath.MaxUint64 - unitPrice
	}
	return unitPrice
}

// book is the index one side rests in.
func (k Keeper) book(side types.Side) collections.KeySet[collections.Triple[string, uint64, uint64]] {
	if side == types.SIDE_BID {
		return k.Bids
	}
	return k.Asks
}

func opposite(side types.Side) types.Side {
	if side == types.SIDE_BID {
		return types.SIDE_ASK
	}
	return types.SIDE_BID
}

// PlaceOrder crosses what it can against the book and rests the remainder.
func (m msgServer) PlaceOrder(
	ctx context.Context, msg *types.MsgPlaceOrder,
) (*types.MsgPlaceOrderResponse, error) {
	if msg.Side != types.SIDE_BID && msg.Side != types.SIDE_ASK {
		return nil, types.ErrInvalidSide
	}
	assetID, settlementDenom, err := m.tradableShare(ctx, msg.Denom)
	if err != nil {
		return nil, err
	}
	if msg.Quantity.IsNil() || !msg.Quantity.IsPositive() {
		return nil, types.ErrInvalidQuantity
	}
	if msg.UnitPrice == 0 {
		// A resting order at zero would give its shares away, or bid nothing for
		// them. There is no market order in this module, deliberately: on a book
		// this thin, an unpriced order is an instruction to accept whatever
		// single resting order happens to be there.
		return nil, types.ErrInvalidPrice
	}

	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	trader, err := m.addressCodec.StringToBytes(msg.Trader)
	if err != nil {
		return nil, err
	}

	id, err := nextID(ctx, m.NextOrderID)
	if err != nil {
		return nil, err
	}

	order := types.Order{
		Id:              id,
		AssetId:         assetID,
		Denom:           msg.Denom,
		Trader:          msg.Trader,
		Side:            msg.Side,
		Quantity:        msg.Quantity,
		Remaining:       msg.Quantity,
		UnitPrice:       msg.UnitPrice,
		SettlementDenom: settlementDenom,
		State:           types.ORDER_STATE_OPEN,
		PlacedAt:        blockTime(ctx),
	}

	// Escrow the whole of the trader's side BEFORE attempting a single match.
	//
	// The alternative — fill from the trader's own balance and escrow only what
	// ends up resting — is one bank send shorter and wrong under exactly the
	// conditions a market produces. It leaves a window inside the cross where
	// the trader's property is still theirs, so a fill part-way down the book
	// can fail for insufficient funds after earlier fills have settled, and the
	// handler is holding half a trade. Escrowing first makes every subsequent
	// leg a transfer of property the module already owns.
	escrow := types.EscrowedBy(order)
	if err := m.bankKeeper.SendCoinsFromAccountToModule(
		ctx, trader, types.ModuleName, sdk.NewCoins(escrow),
	); err != nil {
		return nil, err
	}

	filled, spent, err := m.cross(ctx, &order, trader, params.MaxFillsPerOrder)
	if err != nil {
		return nil, err
	}

	if order.Remaining.IsPositive() {
		// Whatever did not cross rests, at the trader's own price, with its
		// escrow already in place. The remainder is never dropped: a trader who
		// asked to buy a million, got two hundred thousand and was told the
		// order succeeded would have been lied to about the other eight hundred.
		if err := m.book(order.Side).Set(ctx, collections.Join3(
			order.Denom, bookPriceKey(order.Side, order.UnitPrice), order.Id,
		)); err != nil {
			return nil, err
		}
	} else {
		order.State = types.ORDER_STATE_FILLED
	}
	if err := m.Order.Set(ctx, order.Id, order); err != nil {
		return nil, err
	}

	// A bid escrowed at its own limit but filled at the makers' prices, which
	// are never worse and are usually better. The difference is the taker's, not
	// the sellers' — they asked for their price and got it, and a mechanism that
	// quietly paid them more would be charging the taker for the privilege of
	// having been patient.
	//
	// Refunded once, at the end, rather than per fill: the same money, one send,
	// and no dust left behind by a division that happened several times.
	if order.Side == types.SIDE_BID {
		surplus := escrow.Amount.
			Sub(spent).
			Sub(order.Remaining.Mul(math.NewIntFromUint64(order.UnitPrice)))
		if surplus.IsPositive() {
			if err := m.bankKeeper.SendCoinsFromModuleToAccount(
				ctx, types.ModuleName, trader,
				sdk.NewCoins(sdk.NewCoin(order.SettlementDenom, surplus)),
			); err != nil {
				return nil, err
			}
		}
	}

	return &types.MsgPlaceOrderResponse{
		Id:      order.Id,
		Filled:  filled,
		Settled: sdk.NewCoin(settlementDenom, spent),
		Resting: order.Remaining,
	}, nil
}

// cross matches an incoming order against the opposite side of the book.
//
// # The bound, and why it is a count rather than a gas charge
//
// At most maxFills resting orders are consumed, and then the loop stops whatever
// is left. This is the one place in the module where the amount of work is
// chosen by the person paying for it: an order priced through the whole book
// matches every level, and on a book with ten thousand resting asks that is ten
// thousand bank sends, ten thousand runs of x/tokenisation's send restriction,
// and a block that does not finish.
//
// Gas does not fix that, because gas is a price rather than a limit — a
// participant willing to pay it can still stop the chain, and the ones who would
// want to are the ones who can. A count is a ceiling on the work per transaction
// that does not depend on anybody's willingness to pay for the damage.
//
// # Why progress is guaranteed
//
// Every iteration either fills a resting order completely and removes it from
// the book, or exhausts the incoming order and returns. It can never fill zero
// and go round again, because an order with nothing left to fill is never in the
// book — that invariant is enforced when an order rests, and checked again on
// genesis import, where it would otherwise arrive as a key that matches every
// incoming order for nothing and consumes the whole fill budget doing it.
func (m msgServer) cross(
	ctx context.Context, taker *types.Order, takerAddr sdk.AccAddress, maxFills uint32,
) (filled math.Int, spent math.Int, err error) {
	filled, spent = math.ZeroInt(), math.ZeroInt()
	makerBook := m.book(opposite(taker.Side))

	for i := uint32(0); i < maxFills; i++ {
		if !taker.Remaining.IsPositive() {
			break
		}

		maker, found, err := m.bestResting(ctx, makerBook, taker.Denom)
		if err != nil {
			return filled, spent, err
		}
		if !found || !crosses(*taker, maker) {
			break
		}

		// A trader crossing their own resting order is refused outright rather
		// than skipped. Skipping looks kinder and is worse: the two orders stay
		// in the book overlapping in price for ever, and every later order has
		// to step over them, spending fill budget on a pair that will never
		// trade. Refusing tells the one person who can fix it — cancel the other
		// order — that there is something to fix.
		if maker.Trader == taker.Trader {
			return filled, spent, types.ErrSelfDeal.Wrapf(
				"this order would cross your own order %d", maker.Id)
		}

		qty := taker.Remaining
		if qty.GT(maker.Remaining) {
			qty = maker.Remaining
		}
		// The MAKER's price. The resting order named a price and waited; the
		// incoming order accepted it.
		price := math.NewIntFromUint64(maker.UnitPrice)
		cost := qty.Mul(price)

		makerAddr, err := m.addressCodec.StringToBytes(maker.Trader)
		if err != nil {
			return filled, spent, err
		}

		// The maker is taken out of the book before anything moves, for the same
		// reason a listing is marked filled before it is delivered: delivery runs
		// the send restriction, which is code this module does not own.
		maker.Remaining = maker.Remaining.Sub(qty)
		if !maker.Remaining.IsPositive() {
			maker.State = types.ORDER_STATE_FILLED
			if err := makerBook.Remove(ctx, collections.Join3(
				maker.Denom, bookPriceKey(maker.Side, maker.UnitPrice), maker.Id,
			)); err != nil {
				return filled, spent, err
			}
		}
		if err := m.Order.Set(ctx, maker.Id, maker); err != nil {
			return filled, spent, err
		}

		paid := sdk.NewCoin(taker.SettlementDenom, cost)

		// Both sides' property is already in the module account — the taker's
		// was escrowed at the top of PlaceOrder, the maker's when their order
		// rested — so a fill is two transfers out of escrow and cannot half
		// happen for want of funds.
		if taker.Side == types.SIDE_BID {
			// The taker buys: the maker's money out of the taker's escrow, the
			// maker's shares out of the maker's.
			if err := m.bankKeeper.SendCoinsFromModuleToAccount(
				ctx, types.ModuleName, makerAddr, sdk.NewCoins(paid),
			); err != nil {
				return filled, spent, err
			}
			if err := m.deliver(
				ctx, taker.AssetId, taker.Denom, makerAddr, takerAddr, qty, paid,
			); err != nil {
				return filled, spent, err
			}
		} else {
			// The taker sells: their proceeds out of the maker's escrowed money,
			// the shares out of their own.
			if err := m.bankKeeper.SendCoinsFromModuleToAccount(
				ctx, types.ModuleName, takerAddr, sdk.NewCoins(paid),
			); err != nil {
				return filled, spent, err
			}
			if err := m.deliver(
				ctx, taker.AssetId, taker.Denom, takerAddr, makerAddr, qty, paid,
			); err != nil {
				return filled, spent, err
			}
		}

		taker.Remaining = taker.Remaining.Sub(qty)
		filled = filled.Add(qty)
		spent = spent.Add(cost)
	}

	return filled, spent, nil
}

// crosses reports whether an incoming order and a resting one agree on a price.
//
// A bid crosses an ask it is willing to pay at least; an ask crosses a bid that
// offers at least what it wants. Equality crosses — two orders naming the same
// price agree, and refusing that would leave a book where the best bid and the
// best ask are identical and nothing trades.
func crosses(taker, maker types.Order) bool {
	if taker.Side == types.SIDE_BID {
		return maker.UnitPrice <= taker.UnitPrice
	}
	return maker.UnitPrice >= taker.UnitPrice
}

// bestResting reads the best order on one side of one book.
//
// A single key from a range scan rather than a walk of everything resting, which
// is the entire reason the price sits in the index key ahead of the id. The walk
// stops at the first key it sees: both indexes are ordered so that the first key
// IS the best, asks by price ascending and bids by inverted price. See
// bookPriceKey.
func (m msgServer) bestResting(
	ctx context.Context,
	side collections.KeySet[collections.Triple[string, uint64, uint64]],
	denom string,
) (types.Order, bool, error) {
	var best types.Order
	found := false

	rng := collections.NewPrefixedTripleRange[string, uint64, uint64](denom)
	err := side.Walk(ctx, rng, func(key collections.Triple[string, uint64, uint64]) (bool, error) {
		order, err := m.Order.Get(ctx, key.K3())
		if err != nil {
			if isNotFound(err) {
				// An index key with no order behind it cannot happen: both are
				// written in the same transaction and removed in the same one.
				// Skipping rather than failing anyway, because the alternative
				// is a market that cannot trade at all until somebody migrates
				// a single stale key out of the store.
				return false, nil
			}
			return true, err
		}
		best, found = order, true
		return true, nil // the first key is the best; stop
	})
	return best, found, err
}

// CancelOrder withdraws a resting order and returns what it still escrows.
func (m msgServer) CancelOrder(
	ctx context.Context, msg *types.MsgCancelOrder,
) (*types.MsgCancelOrderResponse, error) {
	order, err := m.Order.Get(ctx, msg.OrderId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoOrder.Wrapf("order %d", msg.OrderId)
		}
		return nil, err
	}
	if order.State != types.ORDER_STATE_OPEN {
		return nil, types.ErrOrderClosed.Wrapf("order %d is %s", order.Id, order.State)
	}
	if order.Trader != msg.Trader {
		return nil, types.ErrNotTrader.Wrapf("order %d belongs to %s", order.Id, order.Trader)
	}

	trader, err := m.addressCodec.StringToBytes(order.Trader)
	if err != nil {
		return nil, err
	}

	// Out of the book first, so that nothing can match against an order that is
	// about to have its escrow returned.
	if err := m.book(order.Side).Remove(ctx, collections.Join3(
		order.Denom, bookPriceKey(order.Side, order.UnitPrice), order.Id,
	)); err != nil {
		return nil, err
	}

	// Computed from the order rather than remembered separately — two numbers
	// that must agree are one number too many — and returned whatever has since
	// happened to the vehicle. This is the trader's own property.
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, trader, sdk.NewCoins(types.EscrowedBy(order)),
	); err != nil {
		return nil, err
	}

	order.State = types.ORDER_STATE_CANCELLED
	return &types.MsgCancelOrderResponse{}, m.Order.Set(ctx, order.Id, order)
}
