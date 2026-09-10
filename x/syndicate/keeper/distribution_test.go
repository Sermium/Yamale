package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/syndicate/types"
)

// collectionsJoin is a small local alias, so the tests read as tests rather
// than as key construction.
func collectionsJoin(deal uint64, who string) collections.Pair[uint64, string] {
	return collections.Join(deal, who)
}

// fundedDeal opens a deal, fills it 60/40 between alice and bob, and closes it.
func (f *fixture) fundedDeal(t *testing.T, ref string) (uint64, types.Deal) {
	t.Helper()
	id := f.openDeal(t, ref)
	require.NoError(t, f.commit(t, id, f.alice, 60_000))
	require.NoError(t, f.commit(t, id, f.bob, 40_000))
	_, err := f.srv.CloseDeal(f.env.Ctx, &types.MsgCloseDeal{Operator: f.operator, DealId: id})
	require.NoError(t, err)
	return id, f.deal(t, id)
}

func (f *fixture) distribute(t *testing.T, id uint64, amount int64, final bool) {
	t.Helper()
	_, err := f.srv.Distribute(f.env.Ctx, &types.MsgDistribute{
		Payer: f.partnerFunded(t), DealId: id,
		Amount: sdk.NewInt64Coin(usd, amount), Final: final,
	})
	require.NoError(t, err)
}

// partnerFunded makes sure the partner has money to distribute with.
func (f *fixture) partnerFunded(t *testing.T) string {
	t.Helper()
	addr, err := f.env.AddressCodec.StringToBytes(f.partner)
	require.NoError(t, err)
	if f.env.Balance(addr, usd).LT(math.NewInt(100_000)) {
		f.env.Fund(t, addr, sdk.NewCoins(sdk.NewInt64Coin(usd, 1_000_000)))
	}
	return f.partner
}

func (f *fixture) claim(t *testing.T, id uint64, who string) math.Int {
	t.Helper()
	res, err := f.srv.Claim(f.env.Ctx, &types.MsgClaim{Holder: who, DealId: id})
	require.NoError(t, err)
	return res.Paid.Amount
}

/* ========================================================================== */
/*  Distributions                                                             */
/* ========================================================================== */

// Pro-rata, and the management fee comes off the top.
func TestADistributionIsProRataAndTheFeeComesOffTheTop(t *testing.T) {
	f := setup(t)
	id, _ := f.fundedDeal(t, "SYN-D-001")

	before := f.balance(t, f.operator, usd)
	f.distribute(t, id, 10_000, false)

	// 20% management fee on the distribution.
	require.Equal(t, math.NewInt(2_000), f.balance(t, f.operator, usd).Sub(before))

	// 8,000 net, split 60/40.
	require.Equal(t, math.NewInt(4_800), f.claim(t, id, f.alice))
	require.Equal(t, math.NewInt(3_200), f.claim(t, id, f.bob))
}

// The claim of the whole product, tested by trying to break it.
//
// A distribution reaches whoever holds the token AT THAT MOMENT — including
// somebody who bought it in a private sale between two distributions, with
// nobody reconciling anything. This is what removes the administrative burden
// of tracking a chain of historical owners, which is the cost that makes paper
// syndicates illiquid.
func TestADistributionFollowsTheTokenThroughAPrivateSale(t *testing.T) {
	f := setup(t)
	id, deal := f.fundedDeal(t, "SYN-D-002")

	// First distribution: alice holds 60%.
	f.distribute(t, id, 10_000, false)

	// Alice sells half her position to carol, privately, at a price they agreed
	// between themselves. The platform is not consulted about the price and does
	// not need to be.
	listing, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id, Buyer: f.carol,
		Quantity: math.NewInt(30_000), Price: sdk.NewInt64Coin(usd, 45_000),
	})
	require.NoError(t, err)
	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: f.carol, ListingId: listing.Id})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(30_000), f.balance(t, f.alice, deal.Denom))
	require.Equal(t, math.NewInt(30_000), f.balance(t, f.carol, deal.Denom))

	// Second distribution, same size. Alice now holds 30%, carol 30%, bob 40%.
	f.distribute(t, id, 10_000, false)

	// Alice: 60% of the first 8,000 = 4,800, plus 30% of the second = 2,400.
	require.Equal(t, math.NewInt(7_200), f.claim(t, id, f.alice))
	// Carol held nothing for the first and 30% of the second.
	require.Equal(t, math.NewInt(2_400), f.claim(t, id, f.carol),
		"a buyer must not inherit the seller's holding period")
	// Bob held 40% throughout.
	require.Equal(t, math.NewInt(6_400), f.claim(t, id, f.bob))
}

// The same, through a plain bank send that this module never saw.
//
// This is the one the send restriction exists for. If settlement happened in
// the module's own handlers, a MsgSend between two holders would move the
// tokens and leave both positions unsettled — and the buyer would collect the
// seller's entire holding period on the next distribution.
func TestSettlementSurvivesATransferTheModuleNeverSaw(t *testing.T) {
	f := setup(t)
	id, deal := f.fundedDeal(t, "SYN-D-003")

	f.distribute(t, id, 10_000, false)

	alice, err := f.env.AddressCodec.StringToBytes(f.alice)
	require.NoError(t, err)
	carol, err := f.env.AddressCodec.StringToBytes(f.carol)
	require.NoError(t, err)

	require.NoError(t, f.env.BankKeeper.SendCoins(f.env.Ctx, alice, carol,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, math.NewInt(60_000)))))

	f.distribute(t, id, 10_000, false)

	// Alice earned the whole of the first distribution and none of the second.
	require.Equal(t, math.NewInt(4_800), f.claim(t, id, f.alice))
	// Carol earned the whole of the second and none of the first.
	require.Equal(t, math.NewInt(4_800), f.claim(t, id, f.carol))
}

// A holder who has never transferred has no stored position at all. A response
// computed only from stored positions would report zero for them — which is
// most holders, most of the time, and precisely the people who would conclude
// the platform had lost their money.
func TestEntitlementIsVisibleBeforeAnythingHasBeenSettled(t *testing.T) {
	f := setup(t)
	id, _ := f.fundedDeal(t, "SYN-D-004")
	f.distribute(t, id, 10_000, false)

	res, err := f.q.Entitlement(f.env.Ctx, &types.QueryEntitlementRequest{
		DealId: id, Holder: f.alice,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(60_000), res.Balance)
	require.Equal(t, math.NewInt(4_800), res.Claimable.Amount)
	require.Equal(t, uint32(6_000), res.ShareBps, "60% of the deal, in basis points")
}

// A new holder starts at the CURRENT index, not at zero. Starting at zero would
// pay them every distribution the deal has ever made, out of a pot sized for
// one — and the shortfall would land on whoever claimed last.
func TestANewHolderDoesNotInheritTheDealsWholeHistory(t *testing.T) {
	f := setup(t)
	id, deal := f.fundedDeal(t, "SYN-D-005")

	f.distribute(t, id, 10_000, false)
	f.distribute(t, id, 10_000, false)

	alice, err := f.env.AddressCodec.StringToBytes(f.alice)
	require.NoError(t, err)
	carol, err := f.env.AddressCodec.StringToBytes(f.carol)
	require.NoError(t, err)
	require.NoError(t, f.env.BankKeeper.SendCoins(f.env.Ctx, alice, carol,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, math.NewInt(60_000)))))

	res, err := f.q.Entitlement(f.env.Ctx, &types.QueryEntitlementRequest{
		DealId: id, Holder: f.carol,
	})
	require.NoError(t, err)
	require.Zero(t, res.Claimable.Amount.Int64(),
		"a holder who arrived after both distributions is owed nothing from either")
}

// The pot must cover what it owes. This is the arithmetic check behind the
// index: every claim together must not exceed what was paid in net of fees.
func TestThePotCoversEveryClaim(t *testing.T) {
	f := setup(t)
	id, _ := f.fundedDeal(t, "SYN-D-006")

	f.distribute(t, id, 10_000, false)
	f.distribute(t, id, 7_777, false)
	f.distribute(t, id, 3, false)

	paid := f.claim(t, id, f.alice).Add(f.claim(t, id, f.bob))

	deal := f.deal(t, id)
	require.True(t, paid.LTE(deal.Distributed),
		"claims (%s) exceeded what was distributed (%s)", paid, deal.Distributed)

	// The remainder of the division stays in the pot rather than being swept.
	// It is small, and it is paid out with the next distribution.
	dust := deal.Distributed.Sub(paid)
	require.True(t, dust.LT(math.NewInt(10)),
		"the rounding dust is %s, which is too much to be rounding", dust)
}

// A distribution smaller than the token supply must not vanish.
//
// Without the fixed-point scale on the index, integer division rounds it to
// zero per token and the whole payment disappears into the module account —
// which is not a rounding error but a silent confiscation, and it gets larger
// the more finely a deal is subscribed.
func TestATinyDistributionIsNotRoundedAway(t *testing.T) {
	f := setup(t)
	id, _ := f.fundedDeal(t, "SYN-D-007")

	// 100 against a supply of 100,000: one thousandth of a unit per token.
	f.distribute(t, id, 100, false)
	require.True(t, f.deal(t, id).DistributionIndex.IsPositive(),
		"the index did not move, so this payment is owed to nobody")

	// Repeated until the accumulated fraction crosses one unit per holder.
	for i := 0; i < 200; i++ {
		f.distribute(t, id, 100, false)
	}
	require.True(t, f.claim(t, id, f.alice).IsPositive())
}

/* ========================================================================== */
/*  Realisation                                                               */
/* ========================================================================== */

// Once exited, every token is a claim on a known fixed sum. A market still
// quoting a price for a position that no longer changes is a trap for whoever
// buys last.
func TestARealisedDealStopsTrading(t *testing.T) {
	f := setup(t)
	id, deal := f.fundedDeal(t, "SYN-R-001")

	f.distribute(t, id, 50_000, true)
	require.Equal(t, types.DEAL_STATE_REALISED, f.deal(t, id).State)

	alice, err := f.env.AddressCodec.StringToBytes(f.alice)
	require.NoError(t, err)
	carol, err := f.env.AddressCodec.StringToBytes(f.carol)
	require.NoError(t, err)

	err = f.env.BankKeeper.SendCoins(f.env.Ctx, alice, carol,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, math.NewInt(1_000))))
	require.ErrorIs(t, err, types.ErrTradingHalted)

	_, err = f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id,
		Quantity: math.NewInt(1_000), Price: sdk.NewInt64Coin(usd, 1_000),
	})
	require.ErrorIs(t, err, types.ErrTradingHalted)

	// But the proceeds are still claimable, which is the entire point of the
	// halt: the token is a claim on a fixed sum and nothing else.
	require.Equal(t, math.NewInt(24_000), f.claim(t, id, f.alice))
}

// A seller must be able to recover their own escrowed tokens even after the
// halt. A rule that stopped them would be a confiscation dressed as a safety
// measure.
func TestAListingCanBeCancelledAfterRealisation(t *testing.T) {
	f := setup(t)
	id, deal := f.fundedDeal(t, "SYN-R-002")

	listing, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(usd, 15_000),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(50_000), f.balance(t, f.alice, deal.Denom))

	f.distribute(t, id, 50_000, true)

	_, err = f.srv.CancelListing(f.env.Ctx, &types.MsgCancelListing{
		Seller: f.alice, ListingId: listing.Id,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(60_000), f.balance(t, f.alice, deal.Denom))
}
