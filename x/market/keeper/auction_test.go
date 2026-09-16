package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/market/types"
)

// The auction.
//
// The property under test throughout is that the mechanism holds exactly ONE
// bidder's money at a time. That is what removes the refund loop from
// settlement, and a refund loop in a handler is not a performance problem on a
// chain — it is an unbounded amount of work whose length is chosen by whoever
// wanted it to be long.

const hour = 60 * 60

// day is the deadline helper: every auction in this file runs for one.
func (f *fixture) day() int64 {
	return f.env.Ctx.BlockTime().Unix() + 24*hour
}

// advanceTo moves the chain's clock, which is the only way a deadline in this
// module can pass — every time comparison reads the block, never the host.
func (f *fixture) advanceTo(unix int64) {
	f.env.Ctx = f.env.Ctx.WithBlockTime(time.Unix(unix, 0))
}

// A deadline is a deadline. Settling early would let anybody end the bidding at
// whatever the standing bid happened to be, which is the whole of the seller's
// downside in an auction.
func TestAnAuctionCannotBeSettledBeforeItsDeadline(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	_, bidderStr := f.buyer(t, 500_000)

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 10_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)

	_, err = f.srv.SettleAuction(f.env.Ctx, &types.MsgSettleAuction{
		Settler: bidderStr, AuctionId: opened.Id,
	})
	require.ErrorIs(t, err, types.ErrAuctionRunning)
}

// The heart of the design: a losing bid is refunded in the transaction of the
// bid that beat it, so the module never holds more than one bidder's money.
//
// The mistake this prevents is the obvious implementation — escrow every bid,
// refund the losers at settlement — which makes settlement O(bids). Fifty
// thousand one-unit bids is not a hypothetical attack, it is the cheapest one
// available against an auction, and it would land on the validator that
// happened to include the settlement.
func TestOnlyTheBestBidIsHeldAndTheOneItBeatIsRefundedAtOnce(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	first, firstStr := f.buyer(t, 500_000)
	second, secondStr := f.buyer(t, 500_000)

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 10_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)

	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: firstStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 20_000),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(480_000), f.balance(first, income))
	require.Equal(t, math.NewInt(20_000), f.env.ModuleBalance(types.ModuleName, income))

	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: secondStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 25_000),
	})
	require.NoError(t, err)

	// The first bidder is whole again, immediately, without having to do
	// anything and without waiting for the auction to end.
	require.Equal(t, math.NewInt(500_000), f.balance(first, income))
	require.Equal(t, math.NewInt(475_000), f.balance(second, income))
	// And the module holds exactly one bid, not two.
	require.Equal(t, math.NewInt(25_000), f.env.ModuleBalance(types.ModuleName, income))
}

// A bid must beat the standing bid strictly, and meet the reserve.
//
// Equal is refused deliberately: it would replace one bidder with another for
// the same money, so the seller gains nothing and the bidder who committed
// first loses their place to somebody who merely matched them.
func TestABidMustBeatTheStandingBidAndMeetTheReserve(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	_, oneStr := f.buyer(t, 500_000)
	_, twoStr := f.buyer(t, 500_000)

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 20_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)

	// Below the reserve: refused outright rather than recorded and ignored, so
	// nobody is left believing they are in an auction they cannot win.
	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: oneStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 19_999),
	})
	require.ErrorIs(t, err, types.ErrBidTooLow)

	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: oneStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 20_000),
	})
	require.NoError(t, err)

	// Matching is not beating.
	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: twoStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 20_000),
	})
	require.ErrorIs(t, err, types.ErrBidTooLow)
}

// A bidder who cannot actually pay must not be able to unseat a real bid.
//
// This is entirely about the ORDER of the two transfers inside Bid. Refunding
// the standing bid before taking the new money would let anybody knock out the
// best price with an offer they had no funds for: the refund lands, the escrow
// fails, and — in a handler that did not revert cleanly — the auction has lost
// its best bid to a message that failed.
func TestABidderWhoCannotPayDoesNotUnseatTheStandingBid(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	honest, honestStr := f.buyer(t, 500_000)
	_, brokeStr := f.buyer(t, 100) // nowhere near enough

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 10_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)

	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: honestStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 20_000),
	})
	require.NoError(t, err)

	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: brokeStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 30_000),
	})
	require.Error(t, err)

	auction, err := f.k.Auction.Get(f.env.Ctx, opened.Id)
	require.NoError(t, err)
	require.Equal(t, honestStr, auction.BestBidder, "the standing bid must survive a failed bid")
	require.Equal(t, math.NewInt(20_000), auction.BestBid.Amount)
	require.Equal(t, math.NewInt(480_000), f.balance(honest, income))
}

// An auction nobody bid on returns the shares and does not panic.
//
// The panic is the point of this test. `math.Int`'s zero value is nil and nil
// PANICS on arithmetic rather than erroring, so an auction whose best_bid was
// left unset — which is exactly what an auction nobody bid on looks like — would
// take the chain down in the block somebody settled it. The handler writes an
// explicit zero at open for precisely this path.
func TestAnAuctionNobodyBidOnReturnsTheSharesWithoutPanicking(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	_, anybodyStr := f.buyer(t, 10)

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 10_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(900_000), f.balance(seller, denom))

	f.advanceTo(f.day() + 1)

	_, err = f.srv.SettleAuction(f.env.Ctx, &types.MsgSettleAuction{
		Settler: anybodyStr, AuctionId: opened.Id,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000_000), f.balance(seller, denom))
}

// A reserve of zero is a seller saying they will take whatever it fetches, and
// it must be storable without becoming a nil amount.
func TestAnAuctionWithNoReserveIsOpenedAndSettledWithoutPanicking(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	winner, winnerStr := f.buyer(t, 500_000)

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 0),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)

	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: winnerStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 1),
	})
	require.NoError(t, err)

	f.advanceTo(f.day() + 1)
	_, err = f.srv.SettleAuction(f.env.Ctx, &types.MsgSettleAuction{
		Settler: winnerStr, AuctionId: opened.Id,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(100_000), f.balance(winner, denom))
}

// Settlement delivers to the winner, pays the seller, and writes the price the
// winner actually paid as their cost.
//
// Anybody may settle, and this test uses a third party to prove it: if
// settlement needed the seller's signature, a seller who disliked the result
// could strand the winner's capital indefinitely by doing nothing.
func TestAnAuctionSettlesToTheWinnerAndRecordsWhatTheyPaid(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	loser, loserStr := f.buyer(t, 500_000)
	winner, winnerStr := f.buyer(t, 500_000)
	_, strangerStr := f.buyer(t, 10)

	beforeSeller := f.balance(seller, income)

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 10_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)

	for _, b := range []struct {
		who    string
		amount int64
	}{{loserStr, 20_000}, {winnerStr, 33_000}} {
		_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
			Bidder: b.who, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, b.amount),
		})
		require.NoError(t, err)
	}

	f.advanceTo(f.day() + 1)
	_, err = f.srv.SettleAuction(f.env.Ctx, &types.MsgSettleAuction{
		Settler: strangerStr, AuctionId: opened.Id,
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(100_000), f.balance(winner, denom))
	require.Equal(t, beforeSeller.AddRaw(33_000), f.balance(seller, income))
	require.Equal(t, math.NewInt(500_000), f.balance(loser, income), "the loser is whole")
	require.True(t, f.env.ModuleBalance(types.ModuleName, income).IsZero())
	require.True(t, f.env.ModuleBalance(types.ModuleName, denom).IsZero())

	// An auction is the mechanism for a share with no price to point at, so the
	// number it discovers is the most valuable cost basis this module can write.
	cost, qty := f.basis(t, assetID, winner)
	require.Equal(t, math.NewInt(33_000), cost)
	require.Equal(t, math.NewInt(100_000), qty)
}

// Once somebody has committed money the seller is in it.
//
// A seller who could withdraw after seeing the bids would be running a free
// valuation exercise paid for by the bidders, whose capital was escrowed for the
// duration and who would get nothing for it.
func TestASellerCannotWithdrawAnAuctionSomebodyHasBidOn(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	sellerStr := f.str(t, seller)
	_, bidderStr := f.buyer(t, 500_000)

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 10_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)

	// Before any bid, withdrawing is fine.
	_, err = f.srv.CancelAuction(f.env.Ctx, &types.MsgCancelAuction{
		Seller: sellerStr, AuctionId: opened.Id,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000_000), f.balance(seller, denom))

	again, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 10_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)
	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: bidderStr, AuctionId: again.Id, Amount: sdk.NewInt64Coin(income, 20_000),
	})
	require.NoError(t, err)

	_, err = f.srv.CancelAuction(f.env.Ctx, &types.MsgCancelAuction{
		Seller: sellerStr, AuctionId: again.Id,
	})
	require.ErrorIs(t, err, types.ErrAuctionHasBid)
}

// An auction that closes in the same block it opened is not an auction, it is a
// sale to whoever was watching the mempool.
func TestAnAuctionTooShortOrTooLongIsRefused(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	sellerStr := f.str(t, seller)
	now := f.env.Ctx.BlockTime().Unix()

	_, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(100), Reserve: sdk.NewInt64Coin(income, 1),
		ClosesAt: now + 60,
	})
	require.ErrorIs(t, err, types.ErrAuctionTooShort)

	_, err = f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(100), Reserve: sdk.NewInt64Coin(income, 1),
		ClosesAt: now + 400*24*hour,
	})
	require.ErrorIs(t, err, types.ErrAuctionTooLong)
}

// A vehicle realised mid-auction unwinds rather than stranding both sides.
//
// The register halts every transfer of a realised vehicle's shares to a new
// holder, so the winner cannot be delivered to — and a settlement that simply
// failed would be the worst outcome available: the shares stay in escrow and the
// winner's money stays with them, permanently, because a realised vehicle never
// becomes tradable again. Everybody gets their own property back instead.
func TestAVehicleRealisedMidAuctionUnwindsInsteadOfStrandingBothSides(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	bidder, bidderStr := f.buyer(t, 500_000)

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 10_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)
	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: bidderStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 25_000),
	})
	require.NoError(t, err)

	f.realise(t, assetID)
	f.advanceTo(f.day() + 1)

	_, err = f.srv.SettleAuction(f.env.Ctx, &types.MsgSettleAuction{
		Settler: bidderStr, AuctionId: opened.Id,
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(1_000_000), f.balance(seller, denom), "the seller's shares come back")
	require.Equal(t, math.NewInt(500_000), f.balance(bidder, income), "and the bidder's money")
	require.True(t, f.balance(bidder, denom).IsZero(), "nobody acquired a halted vehicle's shares")
}

// A settled auction cannot be settled again, which would pay the seller twice
// out of whatever else the escrow account happens to be holding.
func TestAnAuctionCannotBeSettledTwice(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "PLNT", 1_000_000)
	_, winnerStr := f.buyer(t, 500_000)

	opened, err := f.srv.OpenAuction(f.env.Ctx, &types.MsgOpenAuction{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100_000), Reserve: sdk.NewInt64Coin(income, 10_000),
		ClosesAt: f.day(),
	})
	require.NoError(t, err)
	_, err = f.srv.Bid(f.env.Ctx, &types.MsgBid{
		Bidder: winnerStr, AuctionId: opened.Id, Amount: sdk.NewInt64Coin(income, 25_000),
	})
	require.NoError(t, err)

	f.advanceTo(f.day() + 1)
	_, err = f.srv.SettleAuction(f.env.Ctx, &types.MsgSettleAuction{
		Settler: winnerStr, AuctionId: opened.Id,
	})
	require.NoError(t, err)

	_, err = f.srv.SettleAuction(f.env.Ctx, &types.MsgSettleAuction{
		Settler: winnerStr, AuctionId: opened.Id,
	})
	require.ErrorIs(t, err, types.ErrAuctionClosed)
}
