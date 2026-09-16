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
	"yamale/blockchain/x/tokenisation/keeper"
	module "yamale/blockchain/x/tokenisation/module"
	"yamale/blockchain/x/tokenisation/types"
)

// The dividend token and the exchange behind it.
//
// A claim pays div/<asset>/<SYMBOL>; the stable it stands for is moved out of
// the vehicle's spendable ledger and reserved under that token's name. Every
// test here is about the gap between those two facts, because that gap is where
// an unbacked promise would live.

// funded builds a vehicle, funds it, and returns everything the tests need.
func funded(t *testing.T, supply, amount int64) (
	*integration.Env, keeper.Keeper, types.MsgServer, uint64, string, sdk.AccAddress,
) {
	t.Helper()
	env, k, ms := setup(t)
	id, owner, _ := vehicle(t, env, ms, supply)
	ownerAddr := mustAddr(t, env, owner)

	env.Fund(t, ownerAddr, sdk.NewCoins(sdk.NewCoin(income, math.NewInt(amount))))
	// A block passes before the distribution, so the period has length and
	// pro-rata temporis has something to apportion. Without it every test here
	// would run down ClosePeriod's zero-length branch and prove nothing about
	// the ordinary path.
	env.Ctx = env.Ctx.WithBlockTime(env.Ctx.BlockTime().Add(time.Hour))
	_, err := ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: owner, AssetId: id, Amount: sdk.NewCoin(income, math.NewInt(amount)),
	})
	require.NoError(t, err)
	return env, k, ms, id, owner, ownerAddr
}

// A claim must pay the dividend token and nothing else.
//
// The mistake this prevents is the obvious partial implementation: mint the
// dividend token AND keep sending the stable, so a holder is paid twice and the
// vault is drained at double speed by anybody who reads the response.
func TestClaimPaysTheDividendTokenAndNotTheStable(t *testing.T) {
	env, k, ms, id, owner, ownerAddr := funded(t, 1_000, 100_000)

	res, err := ms.Claim(env.Ctx, &types.MsgClaim{Holder: owner, AssetId: id})
	require.NoError(t, err)

	asset, err := k.Assets.Get(env.Ctx, id)
	require.NoError(t, err)
	div := types.DividendDenom(asset.FractionDenom)
	require.Equal(t, "div/"+asset.FractionDenom[len("tok/"):], div)

	require.Equal(t, math.NewInt(100_000), sdk.Coins(res.Paid).AmountOf(div))
	require.Equal(t, math.NewInt(100_000), env.Balance(ownerAddr, div))
	require.Equal(t, sdk.NewCoin(income, math.NewInt(100_000)), res.Backing)

	// The stable has not moved to the holder. It is still in the module
	// account, reserved.
	require.True(t, env.Balance(ownerAddr, income).IsZero(),
		"a claim must not pay the stable as well as the token")
	require.Equal(t, math.NewInt(100_000), env.ModuleBalance(types.ModuleName, income))
}

// The backing invariant, stated on its own: for every vehicle, the supply of
// its dividend token equals the stable it has reserved.
//
// The mistake this prevents is minting the token without moving the stable off
// the spendable ledger — which is not visibly broken until a SECOND
// distribution is credited against money already owed to the first one's
// tokens, and then the last holder to reach the exchange is refused.
func TestDividendSupplyAlwaysEqualsTheReserve(t *testing.T) {
	env, k, ms, id, owner, ownerAddr := funded(t, 1_000, 100_000)

	asset, err := k.Assets.Get(env.Ctx, id)
	require.NoError(t, err)
	div := types.DividendDenom(asset.FractionDenom)

	check := func(stage string) {
		t.Helper()
		vault, err := k.Vaults.Get(env.Ctx, id)
		require.NoError(t, err)
		require.Equal(t,
			env.BankKeeper.GetSupply(env.Ctx, div).Amount,
			sdk.Coins(vault.DividendReserve).AmountOf(income),
			"supply and reserve disagree after %s", stage)
	}

	check("issuance")
	_, err = ms.Claim(env.Ctx, &types.MsgClaim{Holder: owner, AssetId: id})
	require.NoError(t, err)
	check("the first claim")

	// The reserve is NOT part of Held, so a second distribution cannot be
	// credited against it.
	vault, err := k.Vaults.Get(env.Ctx, id)
	require.NoError(t, err)
	require.True(t, sdk.Coins(vault.Held).AmountOf(income).IsZero(),
		"a claimed dividend must leave the spendable ledger, or the next distribution is credited against it twice")

	env.Fund(t, ownerAddr, sdk.NewCoins(sdk.NewCoin(income, math.NewInt(40_000))))
	env.Ctx = env.Ctx.WithBlockTime(env.Ctx.BlockTime().Add(time.Hour))
	_, err = ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: owner, AssetId: id, Amount: sdk.NewCoin(income, math.NewInt(40_000)),
	})
	require.NoError(t, err)
	_, err = ms.Claim(env.Ctx, &types.MsgClaim{Holder: owner, AssetId: id})
	require.NoError(t, err)
	check("the second claim")

	_, err = ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: owner, AssetId: id, Amount: math.NewInt(70_000),
	})
	require.NoError(t, err)
	check("a partial exchange")

	_, err = ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: owner, AssetId: id, Amount: math.NewInt(70_000),
	})
	require.NoError(t, err)
	check("the last exchange")

	require.True(t, env.BankKeeper.GetSupply(env.Ctx, div).Amount.IsZero())
	require.Equal(t, math.NewInt(140_000), env.Balance(ownerAddr, income))
	require.True(t, env.ModuleBalance(types.ModuleName, income).IsZero(),
		"once every token is cashed the module must be holding nothing")
}

// The exchange is 1:1 and pays in the vault's income denomination.
func TestExchangePaysParInTheIncomeDenom(t *testing.T) {
	env, _, ms, id, owner, ownerAddr := funded(t, 1_000, 100_000)

	_, err := ms.Claim(env.Ctx, &types.MsgClaim{Holder: owner, AssetId: id})
	require.NoError(t, err)

	res, err := ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: owner, AssetId: id, Amount: math.NewInt(30_000),
	})
	require.NoError(t, err)
	require.Equal(t, sdk.NewCoin(income, math.NewInt(30_000)), res.Paid)
	require.Equal(t, res.Paid.Amount, res.Burned.Amount, "the rate is one for one")
	require.Equal(t, math.NewInt(30_000), env.Balance(ownerAddr, income))
	require.Equal(t, math.NewInt(70_000), env.Balance(ownerAddr, res.Burned.Denom))
}

// One vehicle must never be cashed out of another's reserve.
//
// The mistake this prevents is the same one the vault ledger exists for, one
// module account down: an exchange that checked the MODULE BALANCE rather than
// the vehicle's own reserve would happily pay vehicle A's dividend token out of
// vehicle B's money, and the bank keeper could not tell the difference.
func TestOneVehicleCannotBeCashedOutOfAnothersReserve(t *testing.T) {
	env, k, ms, idA, attacker, idB, honest := twoVehicles(t)

	// B is honest, funded, and its holder claims — so B's reserve is real money
	// sitting in the shared module account.
	env.Fund(t, mustAddr(t, env, honest), sdk.NewCoins(price(1_000_000)))
	env.Ctx = env.Ctx.WithBlockTime(env.Ctx.BlockTime().Add(time.Hour))
	_, err := ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: honest, AssetId: idB, Amount: price(1_000_000),
	})
	require.NoError(t, err)
	_, err = ms.Claim(env.Ctx, &types.MsgClaim{Holder: honest, AssetId: idB})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000_000), env.ModuleBalance(types.ModuleName, income))

	// A has funded nothing, so its holder has nothing to claim.
	_, err = ms.Claim(env.Ctx, &types.MsgClaim{Holder: attacker, AssetId: idA})
	require.ErrorIs(t, err, types.ErrNothingOwed)

	// Forge A's dividend token directly, reaching past the handler that would
	// have reserved for it. This is the state an unbacked mint would leave, and
	// the exchange is the last thing standing between it and B's money.
	assetA, err := k.Assets.Get(env.Ctx, idA)
	require.NoError(t, err)
	forged := sdk.NewCoins(sdk.NewCoin(types.DividendDenom(assetA.FractionDenom), math.NewInt(900_000)))
	require.NoError(t, env.BankKeeper.MintCoins(env.Ctx, types.ModuleName, forged))
	require.NoError(t, env.BankKeeper.SendCoinsFromModuleToAccount(
		env.Ctx, types.ModuleName, mustAddr(t, env, attacker), forged))

	_, err = ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: attacker, AssetId: idA, Amount: math.NewInt(900_000),
	})
	require.ErrorIs(t, err, types.ErrReserveUnfunded)

	// B's reserve is intact and B's holder can still be paid in full.
	vaultB, err := k.Vaults.Get(env.Ctx, idB)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000_000), sdk.Coins(vaultB.DividendReserve).AmountOf(income))

	_, err = ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: honest, AssetId: idB, Amount: math.NewInt(1_000_000),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000_000), env.Balance(mustAddr(t, env, honest), income))
}

// A dividend claimed before the sale survives the sale, the trading halt and
// the closure of the vehicle.
//
// The mistake this prevents is gating the exchange on status the way every
// other payout path in this module is gated. A dividend token is a debt that
// crystallised before the sale; refusing it because the warehouse has since
// been sold would be the chain defaulting on money it had already set aside.
// It also prevents the subtler version: paying redemption out of a pool that
// includes the reserve, which does not refuse anything — it just means the last
// dividend holder finds the money gone.
func TestDividendSurvivesRealisationAndClosure(t *testing.T) {
	env, k, ms, id, owner, ownerAddr := funded(t, 1_000, 100_000)

	_, err := ms.Claim(env.Ctx, &types.MsgClaim{Holder: owner, AssetId: id})
	require.NoError(t, err)
	asset, err := k.Assets.Get(env.Ctx, id)
	require.NoError(t, err)
	div := types.DividendDenom(asset.FractionDenom)

	// Sell the vehicle for a real price, actually paid, and finalise it. The
	// proceeds land in Held, which the reserve is not part of.
	env.Fund(t, ownerAddr, sdk.NewCoins(price(500_000)))
	_, err = ms.ReportSale(env.Ctx, &types.MsgReportSale{
		Reporter: owner, AssetId: id, Price: price(500_000),
	})
	require.NoError(t, err)
	_, err = ms.PaySaleProceeds(env.Ctx, &types.MsgPaySaleProceeds{
		Payer: owner, AssetId: id, Amount: price(500_000),
	})
	require.NoError(t, err)
	env.Ctx = env.Ctx.WithBlockTime(env.Ctx.BlockTime().Add((window + 1) * time.Second))
	_, err = ms.FinaliseSale(env.Ctx, &types.MsgFinaliseSale{Caller: owner, AssetId: id})
	require.NoError(t, err)

	asset, err = k.Assets.Get(env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.STATUS_REALISED, asset.Status)

	// The shares cannot move at all now. The dividend token still can.
	//
	// Asked of the restriction directly rather than through a bank send,
	// because the integration environment wires no send restrictions -- the app
	// registers this one. Going through SendCoins here would assert on the test
	// harness rather than on the rule.
	_, stranger := env.Addr(t)
	strangerAddr := mustAddr(t, env, stranger)
	_, err = k.SendRestrictionFn(env.Ctx, ownerAddr, strangerAddr,
		sdk.NewCoins(sdk.NewCoin(asset.FractionDenom, math.NewInt(1))))
	require.ErrorIs(t, err, types.ErrTradingHalted,
		"trading in a realised vehicle's shares must be halted")

	_, err = k.SendRestrictionFn(env.Ctx, ownerAddr, strangerAddr,
		sdk.NewCoins(sdk.NewCoin(div, math.NewInt(100_000))))
	require.NoError(t, err,
		"a dividend token is a settled debt and must stay transferable after the halt")
	require.NoError(t, env.BankKeeper.SendCoins(env.Ctx, ownerAddr, strangerAddr,
		sdk.NewCoins(sdk.NewCoin(div, math.NewInt(100_000)))))

	// Redeem everything, which takes the vehicle to CLOSED and pays out of
	// Held. The reserve must be untouched by it.
	_, err = ms.Redeem(env.Ctx, &types.MsgRedeem{
		Holder: owner, AssetId: id, Amount: math.NewInt(1_000),
	})
	require.NoError(t, err)
	asset, err = k.Assets.Get(env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.STATUS_CLOSED, asset.Status)

	vault, err := k.Vaults.Get(env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(100_000), sdk.Coins(vault.DividendReserve).AmountOf(income),
		"redemption must pay out of the unreserved balance, never out of the dividend reserve")

	// And the stranger who was handed the token is paid in full, in a vehicle
	// that no longer exists as a going concern.
	res, err := ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: stranger, AssetId: id, Amount: math.NewInt(100_000),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(100_000), res.Paid.Amount)
	require.Equal(t, math.NewInt(100_000), env.Balance(mustAddr(t, env, stranger), income))
}

// Moving a dividend token must not be mistaken for moving a share.
//
// The mistake this prevents: registering the dividend denom in ByDenom, or
// matching on the symbol rather than the prefix, so the send restriction
// settles the sender's income position against their DIVIDEND balance. Nothing
// errors. The positions are simply wrong afterwards, and the error shows up as
// a holder being owed a number nobody can explain.
func TestTransferringADividendTokenDoesNotSettleAPosition(t *testing.T) {
	env, k, ms, id, owner, ownerAddr := funded(t, 1_000, 100_000)

	_, err := ms.Claim(env.Ctx, &types.MsgClaim{Holder: owner, AssetId: id})
	require.NoError(t, err)
	asset, err := k.Assets.Get(env.Ctx, id)
	require.NoError(t, err)
	div := types.DividendDenom(asset.FractionDenom)

	restricted, err := k.IsRestrictedDenom(env.Ctx, div)
	require.NoError(t, err)
	require.False(t, restricted,
		"a dividend token is a 1:1 claim with no status that changes it, so there is no stale price to protect an AMM from")

	before, err := k.Positions.Get(env.Ctx, collections.Join(id, owner))
	require.NoError(t, err)

	_, stranger := env.Addr(t)
	require.NoError(t, env.BankKeeper.SendCoins(env.Ctx, ownerAddr, mustAddr(t, env, stranger),
		sdk.NewCoins(sdk.NewCoin(div, math.NewInt(25_000)))))

	after, err := k.Positions.Get(env.Ctx, collections.Join(id, owner))
	require.NoError(t, err)
	require.Equal(t, before, after,
		"a dividend transfer must not touch the shareholder's position")

	// The recipient, who holds no shares at all, has not acquired one either.
	_, err = k.Positions.Get(env.Ctx, collections.Join(id, stranger))
	require.Error(t, err, "receiving a dividend token must not make somebody a shareholder")

	// And they can still cash it.
	_, err = ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: stranger, AssetId: id, Amount: math.NewInt(25_000),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(25_000), env.Balance(mustAddr(t, env, stranger), income))
}

// Exchanging more than you hold is refused by the burn, and refused before the
// reserve is debited.
//
// The mistake this prevents is debiting the ledger first and letting the bank
// keeper's failure revert it: correct only by accident of ordering, and the
// accident stops holding the moment somebody wraps the payout in anything that
// swallows an error.
func TestExchangeRefusesMoreThanTheHolderHolds(t *testing.T) {
	env, k, ms, id, owner, _ := funded(t, 1_000, 100_000)

	_, err := ms.Claim(env.Ctx, &types.MsgClaim{Holder: owner, AssetId: id})
	require.NoError(t, err)

	_, err = ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: owner, AssetId: id, Amount: math.NewInt(100_001),
	})
	require.Error(t, err)

	vault, err := k.Vaults.Get(env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(100_000), sdk.Coins(vault.DividendReserve).AmountOf(income),
		"a refused exchange must leave the reserve exactly where it was")

	_, err = ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: owner, AssetId: id, Amount: math.ZeroInt(),
	})
	require.ErrorIs(t, err, types.ErrInvalidAmount)
}

// A vehicle with no shareholding has no dividend denomination, and must refuse
// rather than default to one.
//
// The mistake this prevents: deriving the denom from an empty fraction denom
// and getting "div//", which would be ONE denomination shared by every
// unfractionalised vehicle on the chain. Its holders would then be exchanging
// against each other's reserves, which is the exact failure the asset id in the
// denom exists to prevent.
func TestAnUnfractionalisedAssetHasNoDividendDenom(t *testing.T) {
	require.Equal(t, "", types.DividendDenom(""),
		"an empty fraction denom must not produce a dividend denom")
	require.Equal(t, "", types.DividendDenom("tok/"),
		"a prefix with no vehicle behind it must not produce a dividend denom")
	require.Equal(t, "", types.DividendDenom("uusd"))
	require.Equal(t, "div/7/WHSE", types.DividendDenom("tok/7/WHSE"))

	env, _, ms := setup(t)
	_, minter := env.Addr(t)
	_, err := ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "deeds", Authority: minter,
			Verification: types.VERIFY_GOVERNANCE, ChallengeWindowSeconds: 86_400,
		},
	})
	require.NoError(t, err)
	minted, err := ms.MintAsset(env.Ctx, &types.MsgMintAsset{
		Minter: minter, CollectionId: "deeds", Owner: minter, Uri: "ipfs://deed",
	})
	require.NoError(t, err)

	_, err = ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: minter, AssetId: minted.AssetId, Amount: math.NewInt(1),
	})
	require.ErrorIs(t, err, types.ErrNoDividendDenom)
}

// The query is the backing check as an outsider sees it, and the two figures it
// compares must come from different places.
func TestDividendReserveQueryReportsBothSides(t *testing.T) {
	env, k, ms, id, owner, _ := funded(t, 1_000, 100_000)
	qs := keeper.NewQueryServerImpl(k)

	res, err := qs.DividendReserve(env.Ctx, &types.QueryDividendReserveRequest{AssetId: id})
	require.NoError(t, err)
	require.True(t, res.Outstanding.Amount.IsZero())
	require.True(t, res.Reserve.Amount.IsZero())
	require.Equal(t, math.NewInt(100_000), res.Unreserved.Amount,
		"before any claim the whole vault is unreserved")

	_, err = ms.Claim(env.Ctx, &types.MsgClaim{Holder: owner, AssetId: id})
	require.NoError(t, err)

	res, err = qs.DividendReserve(env.Ctx, &types.QueryDividendReserveRequest{AssetId: id})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(100_000), res.Outstanding.Amount)
	require.Equal(t, math.NewInt(100_000), res.Reserve.Amount)
	require.True(t, res.Unreserved.Amount.IsZero(),
		"a claimed dividend must leave the pool a later distribution is credited against")
	require.Equal(t, income, res.Reserve.Denom)
	require.Equal(t, "div/"+res.DividendDenom[len("div/"):], res.DividendDenom)
}

// Genesis must round-trip with a reserve on it, not just with an empty vault.
//
// The existing round-trip test fractionalises and stops, so every Coin field on
// the vault is nil throughout and an export that dropped one would still match.
// This one funds, claims and partially exchanges first, so Held, Funded and
// DividendReserve are all non-empty and one of them has been subtracted down —
// which is the case that turns a slice into a zero-length one and is where a
// round-trip usually stops being exact.
func TestGenesisRoundTripsWithADividendReserve(t *testing.T) {
	env, k, ms, id, owner, _ := funded(t, 1_000, 100_000)

	_, err := ms.Claim(env.Ctx, &types.MsgClaim{Holder: owner, AssetId: id})
	require.NoError(t, err)
	_, err = ms.ExchangeDividend(env.Ctx, &types.MsgExchangeDividend{
		Holder: owner, AssetId: id, Amount: math.NewInt(40_000),
	})
	require.NoError(t, err)

	exported, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())

	var reserve math.Int
	for _, v := range exported.Vaults {
		if v.AssetId == id {
			reserve = sdk.Coins(v.DividendReserve).AmountOf(income)
		}
	}
	require.Equal(t, math.NewInt(60_000), reserve,
		"the reserve is what the exchange pays from; an export that loses it loses the backing")

	other := integration.New(t, types.ModuleName, module.AppModule{})
	authority, err := other.AddressCodec.StringToBytes(other.AuthorityString(t))
	require.NoError(t, err)
	fresh := keeper.NewKeeper(other.Codec, other.StoreService, other.AddressCodec,
		authority, other.BankKeeper, nil, log.NewNopLogger())
	require.NoError(t, fresh.InitGenesis(other.Ctx, *exported))

	again, err := fresh.ExportGenesis(other.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, again)
}

// A vault whose coin carries no amount must be refused at validation.
//
// math.Int's zero value is nil and panics on arithmetic rather than erroring,
// so a hand-edited genesis with {"denom": "uusd"} and no amount is not a wrong
// number — it is a validator that halts on the first transaction touching that
// vehicle, long after anybody is watching the import.
func TestGenesisRefusesAVaultCoinWithNoAmount(t *testing.T) {
	for _, tc := range []struct {
		name  string
		vault types.Vault
	}{
		{"held", types.Vault{AssetId: 1, Denom: income,
			CumulativePerToken: math.LegacyZeroDec(), Held: []sdk.Coin{{Denom: income}}}},
		{"funded", types.Vault{AssetId: 1, Denom: income,
			CumulativePerToken: math.LegacyZeroDec(), Funded: []sdk.Coin{{Denom: income}}}},
		{"dividend reserve", types.Vault{AssetId: 1, Denom: income,
			CumulativePerToken: math.LegacyZeroDec(), DividendReserve: []sdk.Coin{{Denom: income}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gs := types.DefaultGenesis()
			gs.Vaults = []types.Vault{tc.vault}
			require.ErrorIs(t, gs.Validate(), types.ErrInvalidAmount)
		})
	}
}
