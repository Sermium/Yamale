package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

func usd(n int64) sdk.Coin { return sdk.NewInt64Coin("uusd", n*1_000_000) }

// owned registers a jewel and sells it to a fresh account through the card,
// so the marketplace tests start from a piece a customer owns.
func (f *fixture) owned(t *testing.T, staff, inscription string) (uint64, sdk.AccAddress, string) {
	t.Helper()
	id := f.register(t, staff, inscription)
	card := f.issue(t, staff, id)
	addr, owner := f.env.Addr(t)
	f.claimFiled(t, card, id, owner)
	_, err := f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: id})
	require.NoError(t, err)
	return id, addr, owner
}

func (f *fixture) list(t *testing.T, owner string, id uint64, price sdk.Coin) {
	t.Helper()
	_, err := f.srv.ListJewel(f.env.Ctx, &types.MsgListJewel{Owner: owner, JewelId: id, Price: price})
	require.NoError(t, err)
}

func TestABuyerPaysAndTakesTheJewelInOneStep(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id, sellerAddr, seller := f.owned(t, staff, "MD-0001")
	f.list(t, seller, id, usd(1200))

	res, err := f.qs.Jewel(f.env.Ctx, &types.QueryJewelRequest{Id: id})
	require.NoError(t, err)
	require.NotNil(t, res.Listing)
	require.Equal(t, seller, res.Listing.Seller)
	all, err := f.qs.Listings(f.env.Ctx, &types.QueryListingsRequest{})
	require.NoError(t, err)
	require.Len(t, all.Listings, 1)
	require.Equal(t, id, all.Listings[0].Jewel.Id)

	buyerAddr, buyer := f.env.NewFundedAddr(t, sdk.NewCoins(usd(2000)))
	_, err = f.srv.BuyJewel(f.env.Ctx, &types.MsgBuyJewel{Buyer: buyer, JewelId: id, Price: usd(1200)})
	require.NoError(t, err)

	require.Equal(t, buyer, f.owner(t, id))
	require.Equal(t, math.NewInt(800_000_000), f.env.Balance(buyerAddr, "uusd"))
	require.Equal(t, math.NewInt(1_188_000_000), f.env.Balance(sellerAddr, "uusd"), "99% to the seller")
	require.Equal(t, math.NewInt(12_000_000), f.env.ModuleBalance(types.ModuleName, "uusd"), "1% kept as the fee")
	require.Equal(t, uint32(2), f.jewel(t, id).OwnerCount)
	has, err := f.k.Listing.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.False(t, has, "a sale ends the listing")
	tr := f.transfers(t)
	require.Equal(t, types.EventJewelTransferred{JewelId: id, From: seller, To: buyer, Kind: types.TransferSale}, tr[len(tr)-1])
	require.True(t, f.eventTypes()["blockchain.jewel.v1.EventJewelSold"])
}

func TestAShopSellsItsStockOnTheMarketplace(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.issue(t, staff, id)
	f.list(t, staff, id, usd(500))
	_, buyer := f.env.NewFundedAddr(t, sdk.NewCoins(usd(500)))
	_, err := f.srv.BuyJewel(f.env.Ctx, &types.MsgBuyJewel{Buyer: buyer, JewelId: id, Price: usd(500)})
	require.NoError(t, err)
	require.Equal(t, buyer, f.owner(t, id))
	businessAddr, err := f.env.AddressCodec.StringToBytes(business)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(495_000_000), f.env.Balance(businessAddr, "uusd"))
	f.requireSpent(t, jid, id)
}

func TestAPurchaseIsRefusedUnlessThePriceIsTheOneSeenAndTheBuyerCanPay(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id, _, seller := f.owned(t, staff, "MD-0001")
	_, buyer := f.env.NewFundedAddr(t, sdk.NewCoins(usd(100)))

	_, err := f.srv.BuyJewel(f.env.Ctx, &types.MsgBuyJewel{Buyer: buyer, JewelId: id, Price: usd(50)})
	require.ErrorIs(t, err, types.ErrNotListed)

	f.list(t, seller, id, usd(50))
	f.list(t, seller, id, usd(80)) // the seller raises it after the buyer looked
	_, err = f.srv.BuyJewel(f.env.Ctx, &types.MsgBuyJewel{Buyer: buyer, JewelId: id, Price: usd(50)})
	require.ErrorIs(t, err, types.ErrPriceMoved)
	_, err = f.srv.BuyJewel(f.env.Ctx, &types.MsgBuyJewel{Buyer: buyer, JewelId: id, Price: sdk.NewInt64Coin("uusdc", 80_000_000)})
	require.ErrorIs(t, err, types.ErrPriceMoved)
	_, err = f.srv.BuyJewel(f.env.Ctx, &types.MsgBuyJewel{Buyer: seller, JewelId: id, Price: usd(80)})
	require.ErrorIs(t, err, types.ErrBuyOwn)

	f.list(t, seller, id, usd(150))
	_, err = f.srv.BuyJewel(f.env.Ctx, &types.MsgBuyJewel{Buyer: buyer, JewelId: id, Price: usd(150)})
	require.Error(t, err, "a buyer holding 100 cannot pay 150")
	require.Equal(t, seller, f.owner(t, id))
}

func TestListingIsRefusedForStolenPiecesOddCurrenciesAndStrangers(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id, _, seller := f.owned(t, staff, "MD-0001")
	_, stranger := f.env.Addr(t)

	_, err := f.srv.ListJewel(f.env.Ctx, &types.MsgListJewel{Owner: stranger, JewelId: id, Price: usd(10)})
	require.ErrorIs(t, err, types.ErrNotOwner)
	_, err = f.srv.ListJewel(f.env.Ctx, &types.MsgListJewel{Owner: seller, JewelId: id, Price: sdk.NewInt64Coin("uyml", 10)})
	require.ErrorIs(t, err, types.ErrDenomNotForSale)
	_, err = f.srv.ListJewel(f.env.Ctx, &types.MsgListJewel{Owner: seller, JewelId: id, Price: usd(0)})
	require.ErrorIs(t, err, types.ErrNoPrice)

	f.list(t, seller, id, usd(10))
	_, err = f.srv.UnlistJewel(f.env.Ctx, &types.MsgUnlistJewel{Owner: stranger, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner)

	// A theft report ends the listing, and a stolen piece cannot be listed.
	_, err = f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: seller, JewelId: id})
	require.NoError(t, err)
	has, err := f.k.Listing.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.False(t, has)
	_, err = f.srv.ListJewel(f.env.Ctx, &types.MsgListJewel{Owner: seller, JewelId: id, Price: usd(10)})
	require.ErrorIs(t, err, types.ErrStolen)
}

func TestAGiftEndsTheListingAndUnlistingIsTheOwners(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id, _, seller := f.owned(t, staff, "MD-0001")
	f.list(t, seller, id, usd(10))
	_, err := f.srv.UnlistJewel(f.env.Ctx, &types.MsgUnlistJewel{Owner: seller, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.UnlistJewel(f.env.Ctx, &types.MsgUnlistJewel{Owner: seller, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotListed)

	f.list(t, seller, id, usd(10))
	_, friend := f.env.Addr(t)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: seller, JewelId: id, To: friend})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: friend, JewelId: id})
	require.NoError(t, err)
	has, err := f.k.Listing.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.False(t, has, "the listing never follows the jewel to its new owner")
}

func TestOnlyGovernancePaysOutTheFees(t *testing.T) {
	f := setup(t)
	f.env.FundModule(t, types.ModuleName, sdk.NewCoins(usd(30)))
	toAddr, to := f.env.Addr(t)
	_, stranger := f.env.Addr(t)
	_, err := f.srv.WithdrawSaleFees(f.env.Ctx, &types.MsgWithdrawSaleFees{Authority: stranger, To: to, Amount: sdk.NewCoins(usd(10))})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
	gov, err := f.env.AddressCodec.BytesToString(f.env.Authority)
	require.NoError(t, err)
	_, err = f.srv.WithdrawSaleFees(f.env.Ctx, &types.MsgWithdrawSaleFees{Authority: gov, To: to, Amount: sdk.NewCoins(usd(10))})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(10_000_000), f.env.Balance(toAddr, "uusd"))
	require.Equal(t, math.NewInt(20_000_000), f.env.ModuleBalance(types.ModuleName, "uusd"))
}
