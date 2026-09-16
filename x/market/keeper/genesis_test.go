package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/market/types"
)

// Genesis.
//
// An export that does not reproduce the chain is not a description of it, and
// the failure is worse here than in most modules: every live record in this file
// corresponds to real property sitting in the module account. A record lost on
// import is shares or money the bank still holds and no message can ever reach,
// which reads to the next person who looks as a surplus.

// The round trip must be exact, counters included.
func TestGenesisRoundTripsExactly(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "GEN", 1_000_000)
	sellerStr := f.str(t, seller)
	buyer, buyerStr := f.buyer(t, 500_000)

	// One of everything, in more than one state, so the export carries both live
	// records and history.
	filled, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(1_000), Price: sdk.NewInt64Coin(income, 5_000),
	})
	require.NoError(t, err)
	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: buyerStr, ListingId: filled.Id})
	require.NoError(t, err)

	_, err = f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(2_000), Price: sdk.NewInt64Coin(income, 9_000),
	})
	require.NoError(t, err)

	_, err = f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(3_000), Reserve: sdk.NewInt64Coin(income, 1_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)

	f.ask(t, seller, denom, 4_000, 90)
	_, err = f.srv.PlaceOrder(f.env.Ctx, &types.MsgPlaceOrder{
		Trader: buyerStr, Denom: denom, Side: types.SIDE_BID,
		Quantity: math.NewInt(500), UnitPrice: 50,
	})
	require.NoError(t, err)
	_ = buyer

	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())

	// Into a clean chain, and out again.
	g := setup(t)
	require.NoError(t, g.k.InitGenesis(g.env.Ctx, *exported))
	again, err := g.k.ExportGenesis(g.env.Ctx)
	require.NoError(t, err)

	require.Equal(t, exported, again)

	// The counters explicitly, because equality of the whole struct would still
	// pass if both exports derived them the same wrong way.
	require.Equal(t, uint64(3), exported.NextListingId)
	require.Equal(t, uint64(2), exported.NextAuctionId)
	require.Equal(t, uint64(3), exported.NextOrderId)
}

// An imported order must still be crossable, which means the book was rebuilt.
//
// The book is derived rather than exported — it is a pure function of the orders
// — and the mistake that hides is forgetting to rebuild it. Every record round
// trips, every balance is right, the export compares equal, and the market
// silently stops matching: orders rest, nothing crosses, and the book looks
// empty while the orders are all still there.
func TestAnImportedOrderCanStillBeCrossed(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "GEN", 1_000_000)
	f.ask(t, seller, denom, 1_000, 90)

	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)

	g := setup(t)
	require.NoError(t, g.k.InitGenesis(g.env.Ctx, *exported))

	// The escrow moved with the chain in reality; here it is staged directly,
	// because the bank's genesis is not this module's to carry.
	g.env.FundModule(t, types.ModuleName,
		sdk.NewCoins(sdk.NewCoin(denom, math.NewInt(1_000))))

	book, err := g.q.Book(g.env.Ctx, &types.QueryBookRequest{Denom: denom})
	require.NoError(t, err)
	require.Len(t, book.Asks, 1, "the rebuilt book must contain the imported ask")
	require.Equal(t, uint64(90), book.Asks[0].UnitPrice)
}

// A counter at or below an id already present hands that id out again, and the
// second record overwrites the first — silently, because a write to an existing
// key is not an error. Here that is one order's escrow paying out against
// another order's terms.
func TestGenesisRefusesACounterThatWouldReissueAnId(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Orders = []types.Order{validOrder(3)}
	gs.NextOrderId = 3 // the next order would be order 3 as well

	require.ErrorContains(t, gs.Validate(), "next_order_id is 3")
}

// Zero is never issued, because in proto3 a zero id is indistinguishable from
// an unset field.
func TestGenesisRefusesIdZero(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Orders = []types.Order{validOrder(0)}
	gs.NextOrderId = 5

	require.ErrorContains(t, gs.Validate(), "order 0 is never issued")
}

// An auction that names a bidder but escrows nothing for them, or holds money
// for nobody, is the shape that loses somebody's capital.
//
// The bidder and the amount are one fact written twice. A bidder with no amount
// is refunded nothing when they are outbid; an amount with no bidder is money in
// the module account that settlement pays to the seller on behalf of nobody, and
// which no refund path can ever reach.
func TestGenesisRefusesAnAuctionWhoseBidderAndMoneyDisagree(t *testing.T) {
	base := validAuction(1)

	orphaned := base
	orphaned.BestBidder = ""
	orphaned.BestBid = sdk.NewInt64Coin(income, 5_000)
	gs := types.DefaultGenesis()
	gs.Auctions = []types.Auction{orphaned}
	gs.NextAuctionId = 2
	require.ErrorContains(t, gs.Validate(), "no bidder")

	unfunded := base
	unfunded.BestBidder = "yam1bidder"
	unfunded.BestBid = sdk.NewInt64Coin(income, 0)
	gs2 := types.DefaultGenesis()
	gs2.Auctions = []types.Auction{unfunded}
	gs2.NextAuctionId = 2
	require.ErrorContains(t, gs2.Validate(), "escrows nothing")
}

// A nil amount is not a zero amount.
//
// `math.Int`'s zero value is nil and PANICS on arithmetic rather than erroring,
// so an auction imported with an unset reserve would take the chain down in the
// block somebody first bid on it — after the import succeeded, which means the
// file that caused it is already the chain's own state.
func TestGenesisRefusesAnUnsetAmountRatherThanReadingItAsZero(t *testing.T) {
	a := validAuction(1)
	a.Reserve = sdk.Coin{Denom: income} // amount left nil, which is not zero

	gs := types.DefaultGenesis()
	gs.Auctions = []types.Auction{a}
	gs.NextAuctionId = 2
	require.ErrorContains(t, gs.Validate(), "explicit zero")
}

// An open order with nothing left to fill would sit in the book matching every
// incoming order for zero — the one way this module's bounded cross could still
// fail to make progress, because an iteration that fills nothing and removes
// nothing goes round again.
func TestGenesisRefusesAnOpenOrderWithNothingLeftToFill(t *testing.T) {
	o := validOrder(1)
	o.Remaining = math.ZeroInt()

	gs := types.DefaultGenesis()
	gs.Orders = []types.Order{o}
	gs.NextOrderId = 2
	require.ErrorContains(t, gs.Validate(), "matching")
}

// An order with more left than it was ever for would deliver more than it
// escrowed.
func TestGenesisRefusesAnOrderThatWouldDeliverMoreThanItEscrowed(t *testing.T) {
	o := validOrder(1)
	o.Remaining = o.Quantity.AddRaw(1)

	gs := types.DefaultGenesis()
	gs.Orders = []types.Order{o}
	gs.NextOrderId = 2
	require.ErrorContains(t, gs.Validate(), "more than it escrowed")
}

// Parameters that would quietly disable a protection are refused.
//
// Zero fills is the dangerous value, not a large one. At zero no incoming order
// ever crosses: every order rests, the book fills with orders that overlap in
// price and never trade, and the market APPEARS to work — orders are accepted,
// the book is deep — while having become a machine for escrowing property and
// returning it. A market that refuses orders is visibly broken; one that accepts
// every order and matches none is not.
func TestParamsThatWouldStopTheBookMatchingAtAllAreRefused(t *testing.T) {
	p := types.DefaultParams()
	p.MaxFillsPerOrder = 0
	require.ErrorContains(t, p.Validate(), "no order could ever cross")

	p = types.DefaultParams()
	p.MinAuctionPeriod = 0
	require.ErrorContains(t, p.Validate(), "min_auction_period must be positive")

	p = types.DefaultParams()
	p.MaxAuctionPeriod = p.MinAuctionPeriod
	require.ErrorContains(t, p.Validate(), "must be longer than")
}

// The default genesis is one the module would accept, which is the cheapest
// possible guard against a default that cannot start a chain.
func TestTheDefaultGenesisIsValid(t *testing.T) {
	require.NoError(t, types.DefaultGenesis().Validate())
}

func validOrder(id uint64) types.Order {
	return types.Order{
		Id: id, AssetId: 1, Denom: "tok/1/GEN", Trader: "yam1trader",
		Side:      types.SIDE_ASK,
		Quantity:  math.NewInt(1_000),
		Remaining: math.NewInt(1_000),
		UnitPrice: 90, SettlementDenom: income,
		State: types.ORDER_STATE_OPEN,
	}
}

func validAuction(id uint64) types.Auction {
	return types.Auction{
		Id: id, AssetId: 1, Denom: "tok/1/GEN", Seller: "yam1seller",
		Quantity: math.NewInt(1_000),
		Reserve:  sdk.NewInt64Coin(income, 0),
		ClosesAt: 1_788_086_400,
		BestBid:  sdk.NewInt64Coin(income, 0),
		State:    types.AUCTION_STATE_OPEN,
	}
}
