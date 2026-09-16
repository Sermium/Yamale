package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/market/types"
)

// The order book.
//
// Three properties are under test here and they are the three ways a book goes
// wrong: it fills at the wrong price, it fills in the wrong order, or it does an
// unbounded amount of work because somebody asked it to.

// give moves shares by an ordinary bank send, which is how a share reaches
// anybody who did not subscribe — and which carries no price, hence this whole
// module.
func (f *fixture) give(t *testing.T, from, to sdk.AccAddress, denom string, amount int64) {
	t.Helper()
	require.NoError(t, f.env.BankKeeper.SendCoins(f.env.Ctx, from, to,
		sdk.NewCoins(sdk.NewCoin(denom, math.NewInt(amount)))))
}

func (f *fixture) ask(t *testing.T, who sdk.AccAddress, denom string, qty, price int64) uint64 {
	t.Helper()
	res, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: f.str(t, who), Denom: denom, Side: types.SIDE_ASK,
		Quantity: math.NewInt(qty), UnitPrice: uint64(price),
	})
	require.NoError(t, err)
	return res.Id
}

func (f *fixture) setMaxFills(t *testing.T, n uint32) {
	t.Helper()
	params, err := f.k.Params.Get(f.env.Ctx)
	require.NoError(t, err)
	params.MaxFillsPerOrder = n
	require.NoError(t, f.k.Params.Set(f.env.Ctx, params))
}

// A cross fills at the RESTING order's price, and the difference goes back to
// the taker.
//
// The resting order named a price and waited; the incoming order accepted it. A
// mechanism that filled at the taker's limit instead would pay the seller more
// than they asked for, charging the taker for the privilege of having been
// patient — and it would do it invisibly, because the trade still looks like a
// trade.
func TestACrossFillsAtTheRestingPriceAndRefundsTheDifference(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "BOOK", 1_000_000)
	buyer, buyerStr := f.buyer(t, 500_000)

	f.ask(t, seller, denom, 1_000, 90)

	// A bid willing to pay 105 crosses an ask at 90. It must pay 90.
	res, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(1_000), UnitPrice: 105,
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(1_000), res.Filled)
	require.Equal(t, math.NewInt(90_000), res.Settled.Amount)
	require.True(t, res.Resting.IsZero())

	require.Equal(t, math.NewInt(1_000), f.balance(buyer, denom))
	// 500,000 - 90,000, not 500,000 - 105,000: the 15,000 of price improvement
	// is the taker's, and it is returned rather than quietly paid to the seller.
	require.Equal(t, math.NewInt(410_000), f.balance(buyer, income))
	require.True(t, f.env.ModuleBalance(types.ModuleName, income).IsZero(),
		"no price improvement may be left behind in the escrow")

	cost, qty := f.basis(t, assetID, buyer)
	require.Equal(t, math.NewInt(90_000), cost, "the basis is what was paid, not what was offered")
	require.Equal(t, math.NewInt(1_000), qty)
}

// The book fills the best price first, which is the whole reason the price sits
// in the index key ahead of the order id.
//
// The mistake this prevents is an index keyed by id alone, walked in order. It
// works — every test with one resting order passes — and it fills a buyer
// against the OLDEST ask rather than the cheapest, so a trader is worse off for
// having used the book than for having taken a listing.
func TestTheBookFillsTheBestPriceFirst(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "BOOK", 1_000_000)
	_, buyerStr := f.buyer(t, 500_000)

	// Deliberately NOT in price order: the cheapest is placed last.
	f.ask(t, seller, denom, 1_000, 110)
	f.ask(t, seller, denom, 1_000, 100)
	f.ask(t, seller, denom, 1_000, 90)

	res, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(1_500), UnitPrice: 105,
	})
	require.NoError(t, err)

	// 1,000 at 90 then 500 at 100 — the two cheapest, in that order. The ask at
	// 110 is above the limit and is not touched.
	require.Equal(t, math.NewInt(1_500), res.Filled)
	require.Equal(t, math.NewInt(140_000), res.Settled.Amount)
}

// At one price the oldest order fills first.
//
// This is the property that makes resting on a thin book worth doing, and it is
// the one the obvious implementation of a bid index gets wrong. The best bid is
// the HIGHEST, so a naive index is walked backwards to find it — which gets the
// prices right and reverses the ids, filling the newest order at each price
// first. The book here stores a bid's price inverted so that a single ascending
// scan is highest-price-first AND oldest-first within a price.
func TestAtOnePriceTheOldestOrderFillsFirst(t *testing.T) {
	f := setup(t)
	_, owner, denom := f.vehicle(t, "BOOK", 1_000_000)
	early, earlyStr := f.buyer(t, 500_000)
	late, lateStr := f.buyer(t, 500_000)

	// Two bids at the same price, one after the other.
	for _, who := range []string{earlyStr, lateStr} {
		_, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
			Trader: who, Denom: denom, Side: types.SIDE_BID,
			Quantity: math.NewInt(1_000), UnitPrice: 100,
		})
		require.NoError(t, err)
	}

	// A seller hits the book for exactly one of them.
	res, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: f.str(t, owner), Denom: denom, Side: types.SIDE_ASK,
		Quantity: math.NewInt(1_000), UnitPrice: 100,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000), res.Filled)

	require.Equal(t, math.NewInt(1_000), f.balance(early, denom),
		"the trader who rested first must be the one filled")
	require.True(t, f.balance(late, denom).IsZero())
}

// The cross is bounded, and the remainder rests rather than being dropped.
//
// This is the one place in the module where the amount of work is chosen by the
// person paying for it: an order priced through the whole book matches every
// level, and on a deep book that is thousands of bank sends, thousands of runs
// of the register's send restriction, and a block that does not finish. Gas does
// not fix it, because gas is a price rather than a limit.
//
// The remainder resting — rather than the order being refused, or silently
// truncated — is the other half. Truncating is the worst of the three: a trader
// who asked to buy five hundred, got two hundred and was told the order
// succeeded has been lied to about the other three hundred.
func TestTheCrossIsBoundedAndTheRemainderRests(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "BOOK", 1_000_000)
	buyer, buyerStr := f.buyer(t, 500_000)

	f.setMaxFills(t, 2)

	// Five resting asks, all of them within the incoming bid's limit.
	for _, price := range []int64{90, 91, 92, 93, 94} {
		f.ask(t, seller, denom, 100, price)
	}

	res, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(500), UnitPrice: 100,
	})
	require.NoError(t, err)

	// Exactly two levels consumed: 100 at 90 and 100 at 91.
	require.Equal(t, math.NewInt(200), res.Filled)
	require.Equal(t, math.NewInt(18_100), res.Settled.Amount)
	require.Equal(t, math.NewInt(300), res.Resting, "the rest is in the book, not lost")

	order, err := f.k.Order.Get(f.env.Ctx, res.Id)
	require.NoError(t, err)
	require.Equal(t, types.ORDER_STATE_OPEN, order.State)
	require.Equal(t, math.NewInt(300), order.Remaining)

	// And the escrow is exactly what the resting remainder needs: 300 at the
	// taker's own limit of 100. Anything more would be money the module is
	// holding for nothing; anything less and the order could not be filled.
	require.Equal(t, math.NewInt(30_000), f.env.ModuleBalance(types.ModuleName, income))
	require.Equal(t, math.NewInt(200), f.balance(buyer, denom))

	// The next transaction continues where this one stopped, which is what makes
	// the bound a delay rather than a refusal.
	res2, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(200), UnitPrice: 100,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(200), res2.Filled)
}

// A maker filled in part keeps resting with the remainder, and its escrow is
// reduced by exactly what left.
func TestAPartiallyFilledMakerKeepsRestingWithTheRemainder(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "BOOK", 1_000_000)
	_, buyerStr := f.buyer(t, 500_000)

	askID := f.ask(t, seller, denom, 1_000, 90)

	_, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(400), UnitPrice: 90,
	})
	require.NoError(t, err)

	order, err := f.k.Order.Get(f.env.Ctx, askID)
	require.NoError(t, err)
	require.Equal(t, types.ORDER_STATE_OPEN, order.State)
	require.Equal(t, math.NewInt(600), order.Remaining)
	require.Equal(t, math.NewInt(600), f.env.ModuleBalance(types.ModuleName, denom))
}

// Selling into the book works the same way from the other side, and records the
// cost basis against the BUYER — who here is the one who was resting.
func TestSellingIntoARestingBidPaysTheSellerAndRecordsTheBuyersCost(t *testing.T) {
	f := setup(t)
	assetID, owner, denom := f.vehicle(t, "BOOK", 1_000_000)
	buyer, buyerStr := f.buyer(t, 500_000)

	// A second holder, who did not subscribe and whose shares therefore carry no
	// recorded cost at all.
	seller, sellerStr := f.env.Addr(t)
	f.give(t, owner, seller, denom, 5_000)

	_, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(2_000), UnitPrice: 120,
	})
	require.NoError(t, err)

	res, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: sellerStr, Denom: denom, Side: types.SIDE_ASK,
		Quantity: math.NewInt(2_000), UnitPrice: 100,
	})
	require.NoError(t, err)

	// Filled at the resting bid's 120, not the incoming ask's 100: the maker
	// named a price and the taker accepted it, whichever side the maker is on.
	require.Equal(t, math.NewInt(2_000), res.Filled)
	require.Equal(t, math.NewInt(240_000), res.Settled.Amount)
	require.Equal(t, math.NewInt(240_000), f.balance(seller, income))
	require.Equal(t, math.NewInt(2_000), f.balance(buyer, denom))

	cost, qty := f.basis(t, assetID, buyer)
	require.Equal(t, math.NewInt(240_000), cost)
	require.Equal(t, math.NewInt(2_000), qty)
}

// Cancelling a resting order returns exactly what it escrowed, on either side.
//
// Computed from the order rather than remembered separately — two numbers that
// must agree are one number too many — and a bid's escrow is its REMAINING
// quantity at its own price, not its original quantity, or a partly filled
// order would refund money the fills already spent.
func TestCancellingARestingOrderReturnsExactlyWhatItEscrowed(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "BOOK", 1_000_000)
	sellerStr := f.str(t, seller)
	buyer, buyerStr := f.buyer(t, 500_000)

	// An ask, partly filled, then withdrawn.
	askID := f.ask(t, seller, denom, 1_000, 90)
	_, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(400), UnitPrice: 90,
	})
	require.NoError(t, err)

	_, err = f.srv.CancelOrder(f.env.Ctx, &types.MsgCancelOrder{
		Trader: sellerStr, OrderId: askID,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(999_600), f.balance(seller, denom))
	require.True(t, f.env.ModuleBalance(types.ModuleName, denom).IsZero())

	// A bid, never filled, withdrawn in full.
	bid, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(500), UnitPrice: 50,
	})
	require.NoError(t, err)
	before := f.balance(buyer, income)
	require.Equal(t, math.NewInt(25_000), f.env.ModuleBalance(types.ModuleName, income))

	_, err = f.srv.CancelOrder(f.env.Ctx, &types.MsgCancelOrder{
		Trader: buyerStr, OrderId: bid.Id,
	})
	require.NoError(t, err)
	require.Equal(t, before.AddRaw(25_000), f.balance(buyer, income))
	require.True(t, f.env.ModuleBalance(types.ModuleName, income).IsZero())
}

// A cancelled order leaves the book, so nothing can match against an order whose
// escrow has already gone back.
func TestACancelledOrderIsNoLongerInTheBook(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "BOOK", 1_000_000)
	_, buyerStr := f.buyer(t, 500_000)

	askID := f.ask(t, seller, denom, 1_000, 90)
	_, err := f.srv.CancelOrder(f.env.Ctx, &types.MsgCancelOrder{
		Trader: f.str(t, seller), OrderId: askID,
	})
	require.NoError(t, err)

	res, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(1_000), UnitPrice: 200,
	})
	require.NoError(t, err)
	require.True(t, res.Filled.IsZero(), "a withdrawn order must not be matchable")
	require.Equal(t, math.NewInt(1_000), res.Resting)
}

// A trader crossing their own resting order is refused rather than skipped.
//
// Skipping looks kinder and is worse: the two orders stay in the book
// overlapping in price for ever, and every later order has to step over them,
// spending its bounded fill budget on a pair that will never trade. Refusing
// tells the one person who can fix it that there is something to fix.
func TestAnOrderThatWouldCrossItsOwnTraderIsRefused(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "BOOK", 1_000_000)
	sellerStr := f.str(t, seller)

	// Funded, so the bid's escrow succeeds and the transaction actually reaches
	// the cross. Without this the test passes for the wrong reason — an empty
	// wallet — and would keep passing with the self-trade check deleted.
	f.env.Fund(t, seller, sdk.NewCoins(sdk.NewInt64Coin(income, 500_000)))

	f.ask(t, seller, denom, 1_000, 90)

	_, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: sellerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(1_000), UnitPrice: 100,
	})
	require.ErrorIs(t, err, types.ErrSelfDeal)
}

// An order at no price would give its shares away, or bid nothing for them.
// There is no market order in this module: on a book this thin an unpriced
// order is an instruction to accept whatever single resting order is there.
func TestAnOrderMustCarryAPrice(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "BOOK", 1_000_000)

	_, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: f.str(t, seller), Denom: denom, Side: types.SIDE_ASK,
		Quantity: math.NewInt(100), UnitPrice: 0,
	})
	require.ErrorIs(t, err, types.ErrInvalidPrice)
}

// An order with no side at all is refused. In proto3 an unset enum is zero, so
// this is the shape of a message a client forgot to fill in — and defaulting it
// to either side would pick one at random on the trader's behalf.
func TestAnOrderMustBeABidOrAnAsk(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "BOOK", 1_000_000)

	_, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: f.str(t, seller), Denom: denom, Side: types.SIDE_UNSPECIFIED,
		Quantity: math.NewInt(100), UnitPrice: 90,
	})
	require.ErrorIs(t, err, types.ErrInvalidSide)
}

// The book query reads both sides in priority order and bounds its own depth.
func TestTheBookQueryReadsBothSidesBestPriceFirst(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "BOOK", 1_000_000)
	_, buyerStr := f.buyer(t, 500_000)

	for _, price := range []int64{110, 90, 100} {
		f.ask(t, seller, denom, 100, price)
	}
	for _, price := range []int64{50, 70, 60} {
		_, err := f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
			Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
			Quantity: math.NewInt(100), UnitPrice: uint64(price),
		})
		require.NoError(t, err)
	}

	res, err := f.q.Book(f.env.Ctx, &types.QueryBookRequest{Denom: denom})
	require.NoError(t, err)

	require.Equal(t, income, res.SettlementDenom)
	require.Len(t, res.Asks, 3)
	require.Equal(t, []uint64{90, 100, 110},
		[]uint64{res.Asks[0].UnitPrice, res.Asks[1].UnitPrice, res.Asks[2].UnitPrice})
	require.Len(t, res.Bids, 3)
	require.Equal(t, []uint64{70, 60, 50},
		[]uint64{res.Bids[0].UnitPrice, res.Bids[1].UnitPrice, res.Bids[2].UnitPrice})
}
