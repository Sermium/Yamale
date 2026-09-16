package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/collections"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/market/keeper"
	marketmodule "yamale/blockchain/x/market/module"
	"yamale/blockchain/x/market/types"
	tokkeeper "yamale/blockchain/x/tokenisation/keeper"
	tokmodule "yamale/blockchain/x/tokenisation/module"
	toktypes "yamale/blockchain/x/tokenisation/types"
)

// The secondary market for tokenised shares.
//
// Two things are being proved across these files, and they are different in
// kind.
//
// The first is that nobody can lose property to the mechanism. Every offer
// escrows, every withdrawal returns exactly what was escrowed, and no path
// leaves the module account holding something no record claims. A market that
// occasionally keeps somebody's shares is not a market with a bug, it is a
// hazard.
//
// The second is that a fill writes down what it cost. That is the entire reason
// this module exists on top of a register that already transfers shares
// perfectly well: a bank send names no price, so a register built only on sends
// knows what everybody holds and nothing about what any of it cost. The tests
// below buy and sell between recorded costs specifically to try to break that.

const income = "uyml"

type fixture struct {
	env *integration.Env
	k   keeper.Keeper
	srv types.MsgServer
	q   types.QueryServer

	tok    tokkeeper.Keeper
	tokSrv toktypes.MsgServer
}

func setup(t *testing.T) *fixture {
	t.Helper()

	// Both stores, and a REAL tokenisation keeper rather than a stub. The rules
	// being enforced across the boundary are the register's own — which
	// denomination belongs to which vehicle, whether it still trades, what a
	// transfer does to a cost basis — and a hand-written stub would only prove
	// this module can read a struct the test wrote itself.
	env := integration.NewWith(t,
		[]string{types.ModuleName, toktypes.ModuleName},
		marketmodule.AppModule{}, tokmodule.AppModule{})

	// A real clock. Every deadline in this module is a period a person
	// experiences, and a zero time.Time is 1754: the arithmetic works and every
	// failure message reads as nonsense.
	env.Ctx = env.Ctx.WithBlockTime(time.Unix(1_788_000_000, 0)).WithBlockHeight(1)

	authority, err := env.AddressCodec.StringToBytes(env.AuthorityString(t))
	require.NoError(t, err)

	tok := tokkeeper.NewKeeper(
		env.Codec, env.Store(toktypes.ModuleName), env.AddressCodec,
		authority, env.BankKeeper, nil, log.NewNopLogger(),
	)
	require.NoError(t, tok.InitGenesis(env.Ctx, *toktypes.DefaultGenesis()))

	// The send restriction is registered here for the same reason app.go
	// registers it: without it a share transfer settles nothing and carries no
	// cost, and every assertion in these files about what a holding is worth
	// would pass while being enforced by nothing at all. That is not
	// hypothetical — x/tokenisation's restriction was written, commented and
	// referenced by nothing in the repository for months.
	env.BankKeeper.AppendSendRestriction(tok.SendRestrictionFn)

	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec,
		authority, env.BankKeeper, tok)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))

	// Mirrors app.registerMarketEscrow. A fixture that omitted it would still
	// pass most of these tests while the cost basis of every seller who
	// cancelled an offer was quietly destroyed.
	tok.RegisterEscrow(k.ModuleAddress())

	return &fixture{
		env: env, k: k,
		srv:    keeper.NewMsgServerImpl(k),
		q:      keeper.NewQueryServerImpl(k),
		tok:    tok,
		tokSrv: tokkeeper.NewMsgServerImpl(tok),
	}
}

// vehicle fractionalises a building and hands the whole shareholding to its
// owner, who is the first seller in most of these tests.
func (f *fixture) vehicle(t *testing.T, symbol string, supply int64) (uint64, sdk.AccAddress, string) {
	t.Helper()
	_, minter := f.env.Addr(t)

	_, err := f.tokSrv.CreateCollection(f.env.Ctx, &toktypes.MsgCreateCollection{
		Authority: f.env.AuthorityString(t),
		Collection: toktypes.Collection{
			Id: "deeds-" + symbol, Authority: minter,
			Verification:           toktypes.VERIFY_GOVERNANCE,
			ChallengeWindowSeconds: 86_400,
			DisputeBondBps:         100,
		},
	})
	require.NoError(t, err)

	minted, err := f.tokSrv.MintAsset(f.env.Ctx, &toktypes.MsgMintAsset{
		Minter: minter, CollectionId: "deeds-" + symbol, Owner: minter, Uri: "ipfs://deed",
	})
	require.NoError(t, err)

	frac, err := f.tokSrv.Fractionalise(f.env.Ctx, &toktypes.MsgFractionalise{
		Owner: minter, AssetId: minted.AssetId, Symbol: symbol,
		Supply: math.NewInt(supply), HolderShareBps: 10_000, IncomeDenom: income,
	})
	require.NoError(t, err)

	return minted.AssetId, f.acc(t, minter), frac.FractionDenom
}

func (f *fixture) acc(t *testing.T, bech string) sdk.AccAddress {
	t.Helper()
	b, err := f.env.AddressCodec.StringToBytes(bech)
	require.NoError(t, err)
	return b
}

func (f *fixture) str(t *testing.T, addr sdk.AccAddress) string {
	t.Helper()
	s, err := f.env.AddressCodec.BytesToString(addr)
	require.NoError(t, err)
	return s
}

// buyer returns a funded account with no shares.
func (f *fixture) buyer(t *testing.T, funds int64) (sdk.AccAddress, string) {
	t.Helper()
	return f.env.NewFundedAddr(t, sdk.NewCoins(sdk.NewInt64Coin(income, funds)))
}

func (f *fixture) balance(addr sdk.AccAddress, denom string) math.Int {
	return f.env.Balance(addr, denom)
}

// basis reads what the register believes a holding cost.
func (f *fixture) basis(t *testing.T, assetID uint64, holder sdk.AccAddress) (math.Int, math.Int) {
	t.Helper()
	pos, err := f.tok.Positions.Get(f.env.Ctx, collections.Join(assetID, holder.String()))
	if err != nil {
		return math.ZeroInt(), math.ZeroInt()
	}
	cost, qty := pos.CostBasis, pos.CostQty
	if cost.IsNil() {
		cost = math.ZeroInt()
	}
	if qty.IsNil() {
		qty = math.ZeroInt()
	}
	return cost, qty
}

// realise puts a vehicle beyond trading, the way a sale does.
func (f *fixture) realise(t *testing.T, assetID uint64) {
	t.Helper()
	asset, err := f.tok.Assets.Get(f.env.Ctx, assetID)
	require.NoError(t, err)
	asset.Status = toktypes.STATUS_REALISED
	require.NoError(t, f.tok.Assets.Set(f.env.Ctx, assetID, asset))
}

/* ========================================================================== */
/*  Fixed price                                                               */
/* ========================================================================== */

// Both legs in one message. Neither party ever holds the other's property while
// waiting for their own — the recourse against a defaulting counterparty on a
// private share is a lawsuit worth more than the position.
func TestASaleSettlesBothLegsInOneMessage(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "WHSE", 1_000_000)
	buyer, buyerStr := f.buyer(t, 500_000)

	listed, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(income, 15_000),
	})
	require.NoError(t, err)

	// The shares leave the seller AT LISTING. Otherwise the same parcel could be
	// listed three times, or listed and then transferred away, and the first
	// buyer to pay would find nothing to deliver.
	require.Equal(t, math.NewInt(990_000), f.balance(seller, denom))
	require.Equal(t, math.NewInt(10_000), f.env.ModuleBalance(types.ModuleName, denom))

	beforeSeller := f.balance(seller, income)

	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: buyerStr, ListingId: listed.Id})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(10_000), f.balance(buyer, denom))
	require.Equal(t, beforeSeller.AddRaw(15_000), f.balance(seller, income))
	require.Equal(t, math.NewInt(485_000), f.balance(buyer, income))
	// Nothing of either side is left behind in the escrow.
	require.True(t, f.env.ModuleBalance(types.ModuleName, denom).IsZero())
	require.True(t, f.env.ModuleBalance(types.ModuleName, income).IsZero())

	_ = assetID
}

// The headline requirement, and the reason this module exists on top of a
// register that already transfers shares.
//
// A bank send names no price, so a register built only on sends can say what
// everybody holds and nothing about what any of it cost — which means it cannot
// answer the one question a holder actually has. A fill knows the price, so it
// must write it; leaving it unrecorded would make a trade less informative than
// a subscription, which is exactly backwards.
func TestAFillWritesTheBuyersCostBasisFromThePricePaid(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "WHSE", 1_000_000)
	buyer, buyerStr := f.buyer(t, 500_000)

	listed, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(income, 15_000),
	})
	require.NoError(t, err)
	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: buyerStr, ListingId: listed.Id})
	require.NoError(t, err)

	cost, qty := f.basis(t, assetID, buyer)
	require.Equal(t, math.NewInt(15_000), cost, "the buyer paid 15,000 and the register must say so")
	require.Equal(t, math.NewInt(10_000), qty, "and it covers exactly the shares they bought")
}

// A second purchase adds rather than replaces, so a holder who bought twice at
// two prices carries the weighted average — which is what an average cost is.
//
// The mistake this prevents is an implementation that SETS the basis on each
// fill. It looks right on a single trade and silently erases the first purchase
// on the second, reporting a return computed against only the most recent
// price.
func TestBuyingTwiceAtTwoPricesCarriesTheWeightedAverage(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "WHSE", 1_000_000)
	buyer, buyerStr := f.buyer(t, 500_000)
	sellerStr := f.str(t, seller)

	for _, deal := range []struct{ qty, price int64 }{{10_000, 15_000}, {5_000, 12_000}} {
		listed, err := f.srv.List(f.env.Ctx, &types.MsgList{
			Seller: sellerStr, Denom: denom,
			Quantity: math.NewInt(deal.qty), Price: sdk.NewInt64Coin(income, deal.price),
		})
		require.NoError(t, err)
		_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: buyerStr, ListingId: listed.Id})
		require.NoError(t, err)
	}

	cost, qty := f.basis(t, assetID, buyer)
	require.Equal(t, math.NewInt(27_000), cost)
	require.Equal(t, math.NewInt(15_000), qty)
}

// The seller's cost for what they sold is consumed, so their remaining cost
// stays matched to their remaining shares.
//
// Without this the seller carries the cost of a million shares against the
// nine hundred thousand they still hold, and the register reports a
// catastrophic loss on a position that has not moved — to the person least
// able to tell it is wrong.
func TestAFillReleasesTheSellersCostForWhatTheySold(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "WHSE", 1_000_000)
	_, buyerStr := f.buyer(t, 500_000)
	sellerStr := f.str(t, seller)

	// Give the seller a recorded cost: 1,000,000 shares that cost 200,000.
	_, _, err := f.tok.RecordSubscription(
		f.env.Ctx, assetID, seller, math.NewInt(1_000_000), math.NewInt(200_000))
	require.NoError(t, err)

	listed, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(100_000), Price: sdk.NewInt64Coin(income, 30_000),
	})
	require.NoError(t, err)
	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: buyerStr, ListingId: listed.Id})
	require.NoError(t, err)

	// A tenth of the shares went, so a tenth of the cost went with them.
	cost, qty := f.basis(t, assetID, seller)
	require.Equal(t, math.NewInt(180_000), cost)
	require.Equal(t, math.NewInt(900_000), qty)
}

// Listing and then withdrawing must leave the seller exactly as they were.
//
// This is the specific defect that the escrow being basis-neutral prevents.
// x/tokenisation carries a sender's cost along with the shares on any transfer,
// so an escrow that was treated as an ordinary recipient would take a
// proportional slice of the seller's cost on the way in — and the seller would
// get their shares back with that cost spent on an offer nobody took. Their
// reported return would rise, permanently, for no reason they could ever find.
func TestAWithdrawnOfferLeavesTheSellersCostExactlyAsItWas(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "WHSE", 1_000_000)
	sellerStr := f.str(t, seller)

	_, _, err := f.tok.RecordSubscription(
		f.env.Ctx, assetID, seller, math.NewInt(1_000_000), math.NewInt(200_000))
	require.NoError(t, err)
	beforeCost, beforeQty := f.basis(t, assetID, seller)

	listed, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(400_000), Price: sdk.NewInt64Coin(income, 90_000),
	})
	require.NoError(t, err)
	_, err = f.srv.CancelListing(f.env.Ctx, &types.MsgCancelListing{
		Seller: sellerStr, ListingId: listed.Id,
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(1_000_000), f.balance(seller, denom))
	afterCost, afterQty := f.basis(t, assetID, seller)
	require.Equal(t, beforeCost, afterCost, "an offer nobody took must not cost the seller their basis")
	require.Equal(t, beforeQty, afterQty)
}

// The escrow means the same parcel cannot be offered twice.
func TestSharesAlreadyOfferedCannotBeOfferedAgain(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "WHSE", 1_000)
	sellerStr := f.str(t, seller)

	_, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(1_000), Price: sdk.NewInt64Coin(income, 5_000),
	})
	require.NoError(t, err)

	// The whole holding is in escrow, so the bank refuses the second offer. The
	// mistake this prevents is an implementation that escrows at FILL: both
	// listings would be accepted, and whichever buyer paid second would have
	// bought shares that were no longer there.
	_, err = f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(1_000), Price: sdk.NewInt64Coin(income, 6_000),
	})
	require.Error(t, err)
}

// A listing priced in a currency the vehicle does not pay in would record a
// cost in one denomination against a return in another, and the subtraction
// that produces a return would be meaningless while still producing a number.
func TestAListingCannotBePricedInACurrencyTheVehicleDoesNotPay(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "WHSE", 1_000)

	_, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100), Price: sdk.NewInt64Coin("uusdc", 5_000),
	})
	require.ErrorIs(t, err, types.ErrWrongDenom)
}

// Once a vehicle is sold its shares are a claim on a known fixed sum and the
// register refuses to move them at all. An offer resting over one could never
// be filled, so it is refused where somebody is present to read the reason
// rather than as a bank error halfway through a settlement.
func TestARealisedVehicleCannotBeOfferedForSale(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "WHSE", 1_000)
	f.realise(t, assetID)

	_, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100), Price: sdk.NewInt64Coin(income, 5_000),
	})
	require.ErrorIs(t, err, types.ErrTradingHalted)
}

// A seller whose vehicle is realised while their offer is standing must still
// get their own property back.
//
// The mistake this prevents is checking "is this vehicle tradable" on the
// cancel path along with every other path. It reads as consistent and it is a
// confiscation: the shares are the seller's, and a halt that stopped somebody
// recovering escrowed property would strand them for the life of the vehicle.
func TestAWithdrawalSucceedsEvenAfterTheVehicleStopsTrading(t *testing.T) {
	f := setup(t)
	assetID, seller, denom := f.vehicle(t, "WHSE", 1_000)
	sellerStr := f.str(t, seller)

	listed, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: sellerStr, Denom: denom,
		Quantity: math.NewInt(400), Price: sdk.NewInt64Coin(income, 5_000),
	})
	require.NoError(t, err)

	f.realise(t, assetID)

	_, err = f.srv.CancelListing(f.env.Ctx, &types.MsgCancelListing{
		Seller: sellerStr, ListingId: listed.Id,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000), f.balance(seller, denom))
}

// A filled listing is closed, and a second buyer must not be able to pay for
// shares that have already gone.
func TestAListingCannotBeFilledTwice(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "WHSE", 1_000)
	_, firstStr := f.buyer(t, 50_000)
	_, secondStr := f.buyer(t, 50_000)

	listed, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(400), Price: sdk.NewInt64Coin(income, 5_000),
	})
	require.NoError(t, err)

	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: firstStr, ListingId: listed.Id})
	require.NoError(t, err)
	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: secondStr, ListingId: listed.Id})
	require.ErrorIs(t, err, types.ErrListingClosed)
}

// Only the seller may withdraw their own offer.
func TestOnlyTheSellerMayWithdrawAnOffer(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "WHSE", 1_000)
	_, strangerStr := f.buyer(t, 1_000)

	listed, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(400), Price: sdk.NewInt64Coin(income, 5_000),
	})
	require.NoError(t, err)

	_, err = f.srv.CancelListing(f.env.Ctx, &types.MsgCancelListing{
		Seller: strangerStr, ListingId: listed.Id,
	})
	require.ErrorIs(t, err, types.ErrNotSeller)
}

// Ids start at 1. In proto3 a zero id is indistinguishable from an unset field,
// so a client that forgot to set one would otherwise address listing zero and
// be told it succeeded.
func TestIdsStartAtOne(t *testing.T) {
	f := setup(t)
	_, seller, denom := f.vehicle(t, "WHSE", 1_000)

	listed, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.str(t, seller), Denom: denom,
		Quantity: math.NewInt(100), Price: sdk.NewInt64Coin(income, 5_000),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), listed.Id)
}
