package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/syndicate/keeper"
	"yamale/blockchain/x/syndicate/types"
)

// The secondary market, and the genesis round trip.

/* ========================================================================== */
/*  The market                                                                */
/* ========================================================================== */

// Both legs in one message. Neither party ever holds the other's asset while
// waiting for their own — the recourse for a defaulting counterparty on an
// angel-sized private position is a lawsuit worth more than the position.
func TestASaleSettlesBothLegsTogether(t *testing.T) {
	f := setup(t)
	id, deal := f.fundedDeal(t, "SYN-M-001")

	beforeSeller := f.balance(t, f.alice, usd)
	beforeBuyer := f.balance(t, f.carol, usd)

	listing, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(usd, 15_000),
	})
	require.NoError(t, err)

	// The tokens leave the seller at listing. Otherwise the same position could
	// be listed three times, or listed and then transferred away, and the first
	// buyer to pay would find nothing to deliver.
	require.Equal(t, math.NewInt(50_000), f.balance(t, f.alice, deal.Denom))

	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: f.carol, ListingId: listing.Id})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(10_000), f.balance(t, f.carol, deal.Denom))
	require.Equal(t, beforeSeller.Add(math.NewInt(15_000)), f.balance(t, f.alice, usd))
	require.Equal(t, beforeBuyer.Sub(math.NewInt(15_000)), f.balance(t, f.carol, usd))
}

// A private sale is private. This is the shape that actually happens — an angel
// sells to somebody they know — and the administrative cost of it is what a
// paper syndicate cannot absorb.
func TestAPrivateListingIsFillableOnlyByItsNamedBuyer(t *testing.T) {
	f := setup(t)
	id, _ := f.fundedDeal(t, "SYN-M-002")

	listing, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id, Buyer: f.carol,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(usd, 15_000),
	})
	require.NoError(t, err)

	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: f.bob, ListingId: listing.Id})
	require.ErrorIs(t, err, types.ErrNotNamedBuyer)

	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: f.carol, ListingId: listing.Id})
	require.NoError(t, err)
}

// The onboarding gate reaches the secondary market, which is the place it would
// be easiest to leave open — a buyer who was never onboarded is somebody the
// platform has no file on at all.
func TestAnUnonboardedBuyerCannotFillAListing(t *testing.T) {
	f := setup(t)
	id, _ := f.fundedDeal(t, "SYN-M-003")

	listing, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(usd, 15_000),
	})
	require.NoError(t, err)

	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: f.stranger, ListingId: listing.Id})
	require.ErrorIs(t, err, types.ErrNotOnboarded)
}

// A named buyer whose onboarding is withdrawn between listing and fill must be
// refused. The moment that matters is the moment the tokens move, not the
// moment somebody wrote the offer.
func TestEligibilityIsRecheckedAtFillNotOnlyAtListing(t *testing.T) {
	f := setup(t)
	id, _ := f.fundedDeal(t, "SYN-M-004")

	listing, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id, Buyer: f.carol,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(usd, 15_000),
	})
	require.NoError(t, err)

	inv, err := f.k.Investor.Get(f.env.Ctx, f.carol)
	require.NoError(t, err)
	inv.Active = false
	require.NoError(t, f.k.Investor.Set(f.env.Ctx, f.carol, inv))

	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: f.carol, ListingId: listing.Id})
	require.ErrorIs(t, err, types.ErrInvestorInactive)
}

// A listing to somebody who cannot legally receive it is a deal the seller
// believes is done. They should find out now, not when the buyer tries to pay.
func TestAPrivateListingToAnUnonboardedBuyerIsRefusedAtListing(t *testing.T) {
	f := setup(t)
	id, _ := f.fundedDeal(t, "SYN-M-005")

	_, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id, Buyer: f.stranger,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(usd, 15_000),
	})
	require.ErrorIs(t, err, types.ErrNotOnboarded)
}

func TestOnlyTheSellerMayCancelAListing(t *testing.T) {
	f := setup(t)
	id, deal := f.fundedDeal(t, "SYN-M-006")

	listing, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(usd, 15_000),
	})
	require.NoError(t, err)

	_, err = f.srv.CancelListing(f.env.Ctx, &types.MsgCancelListing{
		Seller: f.bob, ListingId: listing.Id,
	})
	require.ErrorIs(t, err, types.ErrNotSeller)

	_, err = f.srv.CancelListing(f.env.Ctx, &types.MsgCancelListing{
		Seller: f.alice, ListingId: listing.Id,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(60_000), f.balance(t, f.alice, deal.Denom))
}

// A filled listing cannot be filled again, which is the double-spend of a
// secondary market.
func TestAFilledListingCannotBeFilledTwice(t *testing.T) {
	f := setup(t)
	id, _ := f.fundedDeal(t, "SYN-M-007")

	listing, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.alice, DealId: id,
		Quantity: math.NewInt(10_000), Price: sdk.NewInt64Coin(usd, 15_000),
	})
	require.NoError(t, err)

	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: f.carol, ListingId: listing.Id})
	require.NoError(t, err)
	_, err = f.srv.Buy(f.env.Ctx, &types.MsgBuy{Buyer: f.bob, ListingId: listing.Id})
	require.ErrorIs(t, err, types.ErrListingClosed)
}

/* ========================================================================== */
/*  Terms and ceilings                                                        */
/* ========================================================================== */

// The fee ceiling is a promise the operator cannot quietly withdraw. An
// investor agreeing to a fee schedule is agreeing under an information
// asymmetry the platform itself created.
func TestAFeeAboveTheCeilingIsRefused(t *testing.T) {
	f := setup(t)
	_, err := f.srv.OpenDeal(f.env.Ctx, &types.MsgOpenDeal{
		Operator: f.operator, Reference: "SYN-T-001", Company: "X", Partner: "Y",
		DrawdownAccount: f.drawdown, SettlementDenom: usd,
		Target: math.NewInt(100_000), Minimum: math.NewInt(50_000),
		MinimumTicket: math.NewInt(1_000),
		// 40% at entry, against a 5% ceiling.
		Fees:     types.FeeSchedule{SourcingBps: 2000, SyndicationBps: 2000},
		ClosesAt: f.env.Ctx.BlockTime().Unix() + week,
	})
	require.ErrorIs(t, err, types.ErrFeeTooHigh)
}

// A raise nobody had time to see is the opposite of the access the platform
// sells. The floor stops a deal being opened and closed inside a block with one
// favoured investor in it.
func TestADealCannotBeOpenedAndClosedInsideABlock(t *testing.T) {
	f := setup(t)
	_, err := f.srv.OpenDeal(f.env.Ctx, &types.MsgOpenDeal{
		Operator: f.operator, Reference: "SYN-T-002", Company: "X", Partner: "Y",
		DrawdownAccount: f.drawdown, SettlementDenom: usd,
		Target: math.NewInt(100_000), Minimum: math.NewInt(50_000),
		MinimumTicket: math.NewInt(1_000),
		ClosesAt:      f.env.Ctx.BlockTime().Unix() + 60,
	})
	require.ErrorIs(t, err, types.ErrWindowTooShort)
}

// Committed money is escrowed, out of the investor's hands and earning nothing.
// Without a ceiling the platform could hold a raise open indefinitely while it
// looked for the last cheque, and the investors would be financing that search.
func TestADealCannotHoldMoneyIndefinitely(t *testing.T) {
	f := setup(t)
	_, err := f.srv.OpenDeal(f.env.Ctx, &types.MsgOpenDeal{
		Operator: f.operator, Reference: "SYN-T-003", Company: "X", Partner: "Y",
		DrawdownAccount: f.drawdown, SettlementDenom: usd,
		Target: math.NewInt(100_000), Minimum: math.NewInt(50_000),
		MinimumTicket: math.NewInt(1_000),
		ClosesAt:      f.env.Ctx.BlockTime().Unix() + 400*day,
	})
	require.ErrorIs(t, err, types.ErrWindowTooLong)
}

// A minimum above the target is a deal that can never close, and every
// commitment to it is money escrowed against an outcome that cannot happen.
func TestADealThatCanNeverCloseIsRefused(t *testing.T) {
	f := setup(t)
	_, err := f.srv.OpenDeal(f.env.Ctx, &types.MsgOpenDeal{
		Operator: f.operator, Reference: "SYN-T-004", Company: "X", Partner: "Y",
		DrawdownAccount: f.drawdown, SettlementDenom: usd,
		Target: math.NewInt(50_000), Minimum: math.NewInt(100_000),
		MinimumTicket: math.NewInt(1_000),
		ClosesAt:      f.env.Ctx.BlockTime().Unix() + week,
	})
	require.ErrorIs(t, err, types.ErrInvalidTerms)
}

func TestTwoDealsCannotShareAReference(t *testing.T) {
	f := setup(t)
	f.openDeal(t, "SYN-T-005")
	_, err := f.srv.OpenDeal(f.env.Ctx, &types.MsgOpenDeal{
		Operator: f.operator, Reference: "SYN-T-005", Company: "X", Partner: "Y",
		DrawdownAccount: f.drawdown, SettlementDenom: usd,
		Target: math.NewInt(100_000), Minimum: math.NewInt(50_000),
		MinimumTicket: math.NewInt(1_000),
		ClosesAt:      f.env.Ctx.BlockTime().Unix() + week,
	})
	require.ErrorIs(t, err, types.ErrRefTaken)
}

// Only the operator that OPENED a deal may close it. A second admitted platform
// able to close another's deal could move somebody else's escrowed money to a
// drawdown account of its choosing.
func TestASecondOperatorCannotCloseAnothersDeal(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-T-006")
	require.NoError(t, f.commit(t, id, f.alice, 60_000))

	_, other := f.env.Addr(t)
	require.NoError(t, f.k.Operator.Set(f.env.Ctx, other, types.Operator{
		Address: other, Name: "Another Platform", Jurisdiction: sn, Active: true,
	}))

	f.advance(week + 1)
	_, err := f.srv.CloseDeal(f.env.Ctx, &types.MsgCloseDeal{Operator: other, DealId: id})
	require.ErrorIs(t, err, types.ErrNotOperator)
}

/* ========================================================================== */
/*  Genesis                                                                   */
/* ========================================================================== */

// A platform that renumbers on import is a different platform.
//
// The position check is the one that matters here and would be the easiest to
// leave out. A position is a running total of what a holder has ALREADY been
// paid; it cannot be recomputed from balances, and one reset to zero on import
// pays that holder every distribution the deal has ever made, out of a pot
// sized for one.
func TestGenesisRoundTripsByteForByte(t *testing.T) {
	f := setup(t)

	id, _ := f.fundedDeal(t, "SYN-G-001")
	f.distribute(t, id, 10_000, false)
	f.claim(t, id, f.alice)

	// An open deal with live commitments, so escrowed money is exported too.
	open := f.openDeal(t, "SYN-G-002")
	require.NoError(t, f.commit(t, open, f.carol, 5_000))

	// And a live listing.
	_, err := f.srv.List(f.env.Ctx, &types.MsgList{
		Seller: f.bob, DealId: id,
		Quantity: math.NewInt(5_000), Price: sdk.NewInt64Coin(usd, 7_500),
	})
	require.NoError(t, err)

	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)

	fresh := integration.NewWith(t, []string{types.ModuleName})
	k2 := keeper.NewKeeper(fresh.StoreService, fresh.Codec, fresh.AddressCodec,
		fresh.Authority, fresh.BankKeeper, nil)
	require.NoError(t, k2.InitGenesis(fresh.Ctx, *exported))

	reexported, err := k2.ExportGenesis(fresh.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, reexported)
}

// The indexes are rebuilt rather than exported, so the only proof they came
// back is to use them. An index that silently returned empty would leave every
// deal present and every lookup failing — and the denom index is what the send
// restriction consults, so an empty one means every deal token becomes freely
// transferable to anybody.
func TestTheIndexesAreRebuiltOnImportAndActuallyWork(t *testing.T) {
	f := setup(t)
	id, deal := f.fundedDeal(t, "SYN-G-003")

	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)

	fresh := integration.NewWith(t, []string{types.ModuleName})
	k2 := keeper.NewKeeper(fresh.StoreService, fresh.Codec, fresh.AddressCodec,
		fresh.Authority, fresh.BankKeeper, nil)
	require.NoError(t, k2.InitGenesis(fresh.Ctx, *exported))

	q := keeper.NewQueryServerImpl(k2)
	byRef, err := q.DealByRef(fresh.Ctx, &types.QueryDealByRefRequest{Reference: "SYN-G-003"})
	require.NoError(t, err)
	require.Equal(t, id, byRef.Deal.Id)

	restricted, err := k2.IsRestrictedDenom(fresh.Ctx, deal.Denom)
	require.NoError(t, err)
	require.True(t, restricted,
		"the denom index did not come back, so this deal token is now freely transferable")
}

func TestAnEmptyPlatformRoundTrips(t *testing.T) {
	env := integration.NewWith(t, []string{types.ModuleName})
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority,
		env.BankKeeper, nil)

	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))
	exported, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.Equal(t, types.DefaultGenesis(), exported)
}
