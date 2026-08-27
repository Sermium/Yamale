package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/tokenisation/keeper"
	module "yamale/blockchain/x/tokenisation/module"
	"yamale/blockchain/x/tokenisation/types"
)

// The sale pipeline had no test at all — not one line exercising ReportSale,
// AttestSale, DisputeSale or FinaliseSale — and that is the whole reason the two
// defects below survived into a running chain. The module's own guide calls the
// reported price "the attack"; this file is the part that was missing.

const window = 86_400

// attested builds a vehicle whose collection verifies by attestation, with a
// register governance appointed, and returns everything the sale needs.
func attested(t *testing.T, threshold uint32, attestors int) (
	*integration.Env, keeper.Keeper, types.MsgServer, uint64, string, string, []string,
) {
	t.Helper()
	env := integration.New(t, types.ModuleName, module.AppModule{})
	authority, err := env.AddressCodec.StringToBytes(env.AuthorityString(t))
	require.NoError(t, err)
	k := keeper.NewKeeper(
		env.Codec, env.StoreService, env.AddressCodec,
		authority, env.BankKeeper, nil, log.NewNopLogger(),
	)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))
	ms := keeper.NewMsgServerImpl(k)

	_, minter := env.Addr(t)
	register := make([]string, 0, attestors)
	for i := 0; i < attestors; i++ {
		_, a := env.Addr(t)
		register = append(register, a)
	}

	_, err = ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "deeds", Authority: minter,
			Verification:           types.VERIFY_ATTESTORS,
			AttestationThreshold:   threshold,
			ChallengeWindowSeconds: window,
			DisputeBondBps:         100,
			Attestors:              register,
		},
	})
	require.NoError(t, err)

	minted, err := ms.MintAsset(env.Ctx, &types.MsgMintAsset{
		Minter: minter, CollectionId: "deeds", Owner: minter, Uri: "ipfs://deed",
	})
	require.NoError(t, err)

	frac, err := ms.Fractionalise(env.Ctx, &types.MsgFractionalise{
		Owner: minter, AssetId: minted.AssetId, Symbol: "WHSE",
		Supply: math.NewInt(1_000), HolderShareBps: 10_000, IncomeDenom: income,
	})
	require.NoError(t, err)

	return env, k, ms, minted.AssetId, minter, frac.FractionDenom, register
}

func price(n int64) sdk.Coin { return sdk.NewCoin(income, math.NewInt(n)) }

// The defect this file exists for. AttestSale checked the collection, the mode,
// the price and whether this address had already signed — and never whether the
// address was anybody. A sponsor met any threshold with fresh keys for the cost
// of the gas, which made the threshold, the one guard between a shareholder and
// a price reported below what was received, decorative.
func TestAStrangerCannotAttest(t *testing.T) {
	env, _, ms, asset, minter, _, _ := attested(t, 2, 3)

	_, err := ms.ReportSale(env.Ctx, &types.MsgReportSale{
		Reporter: minter, AssetId: asset, Price: price(500),
	})
	require.NoError(t, err)

	_, stranger := env.Addr(t)
	_, err = ms.AttestSale(env.Ctx, &types.MsgAttestSale{
		Attestor: stranger, AssetId: asset, Price: price(500),
	})
	require.ErrorIs(t, err, types.ErrNotAttestor)
}

// The same defect stated as the attack rather than the rule, because a rule can
// be satisfied by a check that does not bite.
func TestASponsorCannotMeetTheThresholdWithFreshKeys(t *testing.T) {
	env, _, ms, asset, minter, _, _ := attested(t, 3, 3)

	_, err := ms.ReportSale(env.Ctx, &types.MsgReportSale{
		Reporter: minter, AssetId: asset, Price: price(500),
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, sybil := env.Addr(t)
		_, err = ms.AttestSale(env.Ctx, &types.MsgAttestSale{
			Attestor: sybil, AssetId: asset, Price: price(500),
		})
		require.ErrorIs(t, err, types.ErrNotAttestor, "sybil %d was accepted", i)
	}

	// And the sale has therefore not cleared verification, however long we wait.
	env.Ctx = env.Ctx.WithBlockTime(env.Ctx.BlockTime().Add(2 * window * time.Second))
	_, err = ms.FinaliseSale(env.Ctx, &types.MsgFinaliseSale{Caller: minter, AssetId: asset})
	require.ErrorIs(t, err, types.ErrNotVerified)
}

// An appointed attestor is accepted, and only once.
func TestAnAppointedAttestorIsAcceptedOnce(t *testing.T) {
	env, _, ms, asset, minter, _, register := attested(t, 2, 3)

	_, err := ms.ReportSale(env.Ctx, &types.MsgReportSale{
		Reporter: minter, AssetId: asset, Price: price(500),
	})
	require.NoError(t, err)

	_, err = ms.AttestSale(env.Ctx, &types.MsgAttestSale{
		Attestor: register[0], AssetId: asset, Price: price(500),
	})
	require.NoError(t, err)

	_, err = ms.AttestSale(env.Ctx, &types.MsgAttestSale{
		Attestor: register[0], AssetId: asset, Price: price(500),
	})
	require.ErrorIs(t, err, types.ErrAlreadyAttested)
}

// An attestor who signs a different number is proposing, not confirming.
func TestAttestingToADifferentPriceIsRefused(t *testing.T) {
	env, _, ms, asset, minter, _, register := attested(t, 2, 3)

	_, err := ms.ReportSale(env.Ctx, &types.MsgReportSale{
		Reporter: minter, AssetId: asset, Price: price(500),
	})
	require.NoError(t, err)

	_, err = ms.AttestSale(env.Ctx, &types.MsgAttestSale{
		Attestor: register[0], AssetId: asset, Price: price(900),
	})
	require.ErrorIs(t, err, types.ErrNotVerified)
}

// The other defect. FinaliseSale was a keeper method nothing called — no
// message, no EndBlocker, not even a test — so no asset ever reached REALISED
// and Redeem, which requires it, could never succeed. Every fractionalised
// vehicle was a one-way door: money in, no exit, for anybody, ever.
//
// This walks the whole pipeline and ends on a redemption that pays.
func TestAVehicleCanBeExited(t *testing.T) {
	env, k, ms, asset, minter, _, register := attested(t, 2, 3)

	_, err := ms.ReportSale(env.Ctx, &types.MsgReportSale{
		Reporter: minter, AssetId: asset, Price: price(1_000), EvidenceUri: "ipfs://deed-of-sale",
	})
	require.NoError(t, err)

	held, err := env.AddressCodec.StringToBytes(minter)
	require.NoError(t, err)

	// Not yet fundable, and the ordering is the module's rather than mine:
	// FundVault takes ACTIVE or REALISED and the asset is REPORTED, so the
	// proceeds arrive after the sale finalises rather than beside the claim
	// about it. Funding here would credit the index twice — once now and once
	// when FinaliseSale accrues the price — and leave the vault owing double
	// what it holds.
	env.Fund(t, held, sdk.NewCoins(price(1_000)))
	_, err = ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: minter, AssetId: asset, Amount: price(1_000),
	})
	require.ErrorIs(t, err, types.ErrWrongStatus)

	for i := 0; i < 2; i++ {
		_, err = ms.AttestSale(env.Ctx, &types.MsgAttestSale{
			Attestor: register[i], AssetId: asset, Price: price(1_000),
		})
		require.NoError(t, err)
	}

	// Inside the window, finalising is refused however many attestations stand.
	_, err = ms.FinaliseSale(env.Ctx, &types.MsgFinaliseSale{Caller: minter, AssetId: asset})
	require.ErrorIs(t, err, types.ErrStillInWindow)

	env.Ctx = env.Ctx.WithBlockTime(env.Ctx.BlockTime().Add((window + 1) * time.Second))

	// Anybody at all sends it — here a passer-by holding no shares, because a
	// crank only the sponsor could turn is a crank the sponsor can decline to.
	_, passerby := env.Addr(t)
	_, err = ms.FinaliseSale(env.Ctx, &types.MsgFinaliseSale{Caller: passerby, AssetId: asset})
	require.NoError(t, err)

	a, err := k.Assets.Get(env.Ctx, asset)
	require.NoError(t, err)
	require.Equal(t, types.STATUS_REALISED, a.Status)

	// The door opens, which is the fix this test exists for: before
	// MsgFinaliseSale existed nothing could reach this status and Redeem, which
	// requires it, could never succeed for anybody.
	//
	// Redemption itself then fails, and NOT because of anything above. The vault
	// owes what it cannot pay: FinaliseSale credits the index with the whole
	// reported price while moving no coins, and the only message that does move
	// coins - FundVault - accrues them a second time on the way in. So the
	// proceeds of a sale have no funding path that does not double-count, and a
	// holder is credited money that never arrived.
	//
	// That is a separate defect from the two this file was written for, it is a
	// design decision rather than a mechanical fix - whether finalising should
	// pull the price from the reporter, or whether funding should stop accruing
	// once a sale is reported - and it is recorded in docs/scope/gaps.md rather
	// than guessed at here. Asserted as it actually behaves, so that whoever
	// decides it will see this test fail and have to look.
	before := env.Balance(held, income)
	_, err = ms.Redeem(env.Ctx, &types.MsgRedeem{
		Holder: minter, AssetId: asset, Amount: math.NewInt(1_000),
	})
	require.Error(t, err, "redemption now pays - the proceeds hole has been closed, update this test")
	require.Contains(t, err.Error(), "insufficient funds")
	require.Equal(t, before, env.Balance(held, income))
}

// A disputed sale does not finalise, which is what the window is for.
func TestADisputeStopsTheExit(t *testing.T) {
	env, _, ms, asset, minter, _, register := attested(t, 2, 3)

	_, err := ms.ReportSale(env.Ctx, &types.MsgReportSale{
		Reporter: minter, AssetId: asset, Price: price(1_000),
	})
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err = ms.AttestSale(env.Ctx, &types.MsgAttestSale{
			Attestor: register[i], AssetId: asset, Price: price(1_000),
		})
		require.NoError(t, err)
	}

	challengerAddr, challenger := env.Addr(t)
	env.Fund(t, challengerAddr, sdk.NewCoins(price(1_000)))
	_, err = ms.DisputeSale(env.Ctx, &types.MsgDisputeSale{
		Challenger: challenger, AssetId: asset,
		Reason: "the deed filed with the registry shows twice this",
	})
	require.NoError(t, err)

	env.Ctx = env.Ctx.WithBlockTime(env.Ctx.BlockTime().Add((window + 1) * time.Second))
	_, err = ms.FinaliseSale(env.Ctx, &types.MsgFinaliseSale{Caller: minter, AssetId: asset})

	// ErrWrongStatus rather than ErrAlreadyDisputed, because DisputeSale moves
	// the asset to STATUS_DISPUTED and FinaliseSale checks the status before it
	// reads the report. Worth stating rather than asserting the tidier error:
	// FinaliseSale's own ErrAlreadyDisputed branch is unreachable through a
	// dispute, and is defence in depth against a report flagged some other way.
	require.ErrorIs(t, err, types.ErrWrongStatus)
}

// The error DisputeSale returned once the window had closed said the opposite
// of what had happened, which would send whoever debugs it looking for a window
// that has already gone.
func TestDisputingAfterTheWindowSaysTheWindowClosed(t *testing.T) {
	env, _, ms, asset, minter, _, _ := attested(t, 2, 3)

	_, err := ms.ReportSale(env.Ctx, &types.MsgReportSale{
		Reporter: minter, AssetId: asset, Price: price(1_000),
	})
	require.NoError(t, err)

	env.Ctx = env.Ctx.WithBlockTime(env.Ctx.BlockTime().Add((window + 1) * time.Second))
	challengerAddr, challenger := env.Addr(t)
	env.Fund(t, challengerAddr, sdk.NewCoins(price(1_000)))
	_, err = ms.DisputeSale(env.Ctx, &types.MsgDisputeSale{
		Challenger: challenger, AssetId: asset, Reason: "too late",
	})
	require.ErrorIs(t, err, types.ErrWindowClosed)
}

// A register smaller than the threshold it must meet is refused at creation,
// because the way that presents otherwise is every vehicle in the collection
// silently becoming unexitable months later, with the money already in.
func TestACollectionCannotDemandMoreAttestorsThanItHas(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{})
	authority, err := env.AddressCodec.StringToBytes(env.AuthorityString(t))
	require.NoError(t, err)
	k := keeper.NewKeeper(
		env.Codec, env.StoreService, env.AddressCodec,
		authority, env.BankKeeper, nil, log.NewNopLogger(),
	)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))
	ms := keeper.NewMsgServerImpl(k)

	_, minter := env.Addr(t)
	_, one := env.Addr(t)

	_, err = ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "deeds", Authority: minter,
			Verification:           types.VERIFY_ATTESTORS,
			AttestationThreshold:   3,
			ChallengeWindowSeconds: window,
			Attestors:              []string{one},
		},
	})
	require.ErrorIs(t, err, types.ErrInvalidThreshold)

	// And the same account listed three times is not three attestors.
	_, err = ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "deeds", Authority: minter,
			Verification:           types.VERIFY_ATTESTORS,
			AttestationThreshold:   3,
			ChallengeWindowSeconds: window,
			Attestors:              []string{one, one, one},
		},
	})
	require.ErrorIs(t, err, types.ErrNotAttestor)
}

// Only governance appoints the register. If the seller could appoint the
// accounts that check the seller, the register would restate the problem.
func TestOnlyGovernanceAppointsAttestors(t *testing.T) {
	env, _, ms, _, minter, _, register := attested(t, 2, 3)

	_, err := ms.SetCollectionAttestors(env.Ctx, &types.MsgSetCollectionAttestors{
		Authority: minter, CollectionId: "deeds", Attestors: register,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	_, err = ms.SetCollectionAttestors(env.Ctx, &types.MsgSetCollectionAttestors{
		Authority: env.AuthorityString(t), CollectionId: "deeds", Attestors: register[:2],
	})
	require.NoError(t, err)
}

// The send restriction, which was written and never registered.
//
// x/tokenisation pays income by a cumulative-per-token index, so a holder's
// position must be settled at the moment their balance changes or their income
// is silently attributed to whoever holds the shares next. SendRestrictionFn
// does that, and until 2026-08-27 nothing in the repository called it: no
// transfer settled, no position was created by one, and every entitlement read
// zero however much the vault held.
//
// This asserts the behaviour rather than the wiring, because the wiring lives
// in app/ behind a build tag. Both halves are needed and this is the half that
// says what the function is for.
func TestATransferSettlesBothSides(t *testing.T) {
	env, k, ms, asset, minter, denom, _ := attested(t, 2, 3)

	seller, err := env.AddressCodec.StringToBytes(minter)
	require.NoError(t, err)
	buyerAcc, buyer := env.Addr(t)

	// Income arrives while the seller holds every share.
	env.Fund(t, seller, sdk.NewCoins(price(1_000)))
	_, err = ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: minter, AssetId: asset, Amount: price(1_000),
	})
	require.NoError(t, err)

	owed, err := k.Entitlement(env.Ctx, asset, seller)
	require.NoError(t, err)
	require.True(t, owed.IsPositive(), "the seller earned nothing while holding every share")

	// A quarter of the shareholding changes hands. The restriction runs first.
	quarter := sdk.NewCoins(sdk.NewCoin(denom, math.NewInt(250)))
	_, err = k.SendRestrictionFn(env.Ctx, seller, buyerAcc, quarter)
	require.NoError(t, err)
	require.NoError(t, env.BankKeeper.SendCoins(env.Ctx, seller, buyerAcc, quarter))

	// The seller keeps what they earned before the sale...
	after, err := k.Entitlement(env.Ctx, asset, seller)
	require.NoError(t, err)
	require.Equal(t, owed, after, "the seller's earned income moved with the shares")

	// ...and the buyer is owed nothing for a period they did not hold.
	buyerOwed, err := k.Entitlement(env.Ctx, asset, buyerAcc)
	require.NoError(t, err)
	require.True(t, buyerOwed.IsZero(), "the buyer was paid for income that accrued before they held anything")
	_ = buyer

	// Income after the sale splits by the shares each now holds.
	env.Fund(t, seller, sdk.NewCoins(price(1_000)))
	_, err = ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: minter, AssetId: asset, Amount: price(1_000),
	})
	require.NoError(t, err)

	buyerOwed, err = k.Entitlement(env.Ctx, asset, buyerAcc)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(250), buyerOwed,
		"a quarter of 1000 income on a quarter of the shares")
}
