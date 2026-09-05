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

// One module account serves every vehicle on the chain, so its balance is not
// an ownership record. These tests are all about the difference.

// twoVehicles builds a chain carrying two unrelated closed-end vehicles under
// a governance-verified collection, so neither needs an attestor to report a
// sale. It returns the environment and both vehicles' ids and owners.
func twoVehicles(t *testing.T) (*integration.Env, keeper.Keeper, types.MsgServer, uint64, string, uint64, string) {
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

	_, honest := env.Addr(t)
	_, attacker := env.Addr(t)

	_, err = ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "warehouses", Authority: honest,
			Verification:           types.VERIFY_GOVERNANCE,
			ChallengeWindowSeconds: window,
			DisputeBondBps:         100,
		},
	})
	require.NoError(t, err)

	mint := func(owner string) uint64 {
		minted, err := ms.MintAsset(env.Ctx, &types.MsgMintAsset{
			Minter: owner, CollectionId: "warehouses", Owner: owner, Uri: "ipfs://deed",
		})
		require.NoError(t, err)
		_, err = ms.Fractionalise(env.Ctx, &types.MsgFractionalise{
			Owner: owner, AssetId: minted.AssetId, Symbol: "WHSE",
			Supply: math.NewInt(1_000), HolderShareBps: 10_000, IncomeDenom: income,
		})
		require.NoError(t, err)
		return minted.AssetId
	}

	// The collection's authority is the only account that may mint into it, so
	// both vehicles are minted by the honest party and one is then handed over
	// — which is exactly how an attacker acquires a vehicle in the collection
	// they want to be paid out of.
	idA := mint(honest)
	idB := mint(honest)
	_, err = ms.TransferAsset(env.Ctx, &types.MsgTransferAsset{
		Owner: honest, AssetId: idA, Recipient: attacker,
	})
	require.NoError(t, err)
	require.NoError(t, env.BankKeeper.SendCoins(env.Ctx,
		mustAddr(t, env, honest), mustAddr(t, env, attacker),
		sdk.NewCoins(sdk.NewCoin(fractionDenom(t, k, env, idA), math.NewInt(1_000)))))

	return env, k, ms, idA, attacker, idB, honest
}

func mustAddr(t *testing.T, env *integration.Env, s string) sdk.AccAddress {
	t.Helper()
	a, err := env.AddressCodec.StringToBytes(s)
	require.NoError(t, err)
	return a
}

func fractionDenom(t *testing.T, k keeper.Keeper, env *integration.Env, id uint64) string {
	t.Helper()
	a, err := k.Assets.Get(env.Ctx, id)
	require.NoError(t, err)
	return a.FractionDenom
}

// The audit's proof of concept, kept as a regression test.
//
// Vehicle A reports a sale it never received a penny for and finalises. Before
// the vault ledger existed, FinaliseSale credited A's index with the whole
// reported price while moving no coins, and A's owner then redeemed — paid out
// of the module account, which was holding vehicle B's rent. B's holders lost
// money to a vehicle they had never heard of, and nothing in the bank keeper
// could tell the difference.
func TestASaleNobodyPaidForCannotDrainAnotherVehicle(t *testing.T) {
	env, k, ms, idA, attacker, idB, honest := twoVehicles(t)

	// Vehicle B is honest and funded. Its vault holds real money.
	env.Fund(t, mustAddr(t, env, honest), sdk.NewCoins(price(1_000_000)))
	_, err := ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: honest, AssetId: idB, Amount: price(1_000_000),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000_000), env.ModuleBalance(types.ModuleName, income))

	// Vehicle A reports a sale for most of what B is holding, and pays nothing.
	_, err = ms.ReportSale(env.Ctx, &types.MsgReportSale{
		Reporter: attacker, AssetId: idA, Price: price(900_000),
	})
	require.NoError(t, err)
	env.Ctx = env.Ctx.WithBlockTime(env.Ctx.BlockTime().Add((window + 1) * time.Second))

	// Verified by its collection, past its window, undisputed — and refused,
	// because none of that is the same thing as the money being here.
	_, err = ms.FinaliseSale(env.Ctx, &types.MsgFinaliseSale{Caller: attacker, AssetId: idA})
	require.ErrorIs(t, err, types.ErrProceedsUnpaid)

	// So redemption never opens, and B's vault is untouched.
	_, err = ms.Redeem(env.Ctx, &types.MsgRedeem{
		Holder: attacker, AssetId: idA, Amount: math.NewInt(1_000),
	})
	require.ErrorIs(t, err, types.ErrWrongStatus)
	require.Equal(t, math.NewInt(1_000_000), env.ModuleBalance(types.ModuleName, income))

	vaultB, err := k.Vaults.Get(env.Ctx, idB)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000_000), sdk.Coins(vaultB.Held).AmountOf(income))

	vaultA, err := k.Vaults.Get(env.Ctx, idA)
	require.NoError(t, err)
	require.True(t, sdk.Coins(vaultA.Held).IsZero())
}

// The same defence one level down, in case a future handler credits an index
// without going through FinaliseSale: a vehicle can never pay out more than
// its own ledger says it holds, whatever the module account balance is.
func TestAVehicleCannotPayOutMoreThanItHolds(t *testing.T) {
	env, k, ms, idA, attacker, idB, honest := twoVehicles(t)

	env.Fund(t, mustAddr(t, env, honest), sdk.NewCoins(price(1_000_000)))
	_, err := ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: honest, AssetId: idB, Amount: price(1_000_000),
	})
	require.NoError(t, err)

	// The attacker is on the register at the index's starting point, so what
	// follows accrues to them rather than starting them at the new figure.
	require.NoError(t, k.Settle(env.Ctx, idA, mustAddr(t, env, attacker), math.NewInt(1_000)))

	// Reach past the handlers and raise A's index directly, as a credit that
	// nothing was collected for would have to.
	vault, err := k.Vaults.Get(env.Ctx, idA)
	require.NoError(t, err)
	vault.CumulativePerToken = math.LegacyNewDec(500)
	require.NoError(t, k.Vaults.Set(env.Ctx, idA, vault))

	asset, err := k.Assets.Get(env.Ctx, idA)
	require.NoError(t, err)
	asset.Status = types.STATUS_REALISED
	require.NoError(t, k.Assets.Set(env.Ctx, idA, asset))

	// The index says A owes 500,000. The bank keeper could settle it. The
	// ledger says A holds nothing, and the ledger is what decides.
	_, err = ms.Redeem(env.Ctx, &types.MsgRedeem{
		Holder: attacker, AssetId: idA, Amount: math.NewInt(1_000),
	})
	require.ErrorIs(t, err, types.ErrVaultUnfunded)
	require.Equal(t, math.NewInt(1_000_000), env.ModuleBalance(types.ModuleName, income))
}

// FundVault takes the holders' share and leaves the rest with the funder.
//
// It used to take the whole payment into the module account and credit only
// holder_share_bps of it to the index, so on a vehicle with a 2,500 bps share
// three quarters of every rent payment sat in an account with no key and no
// message that paid it back.
func TestFundVaultCollectsOnlyTheHoldersShare(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{})
	authority, err := env.AddressCodec.StringToBytes(env.AuthorityString(t))
	require.NoError(t, err)
	k := keeper.NewKeeper(
		env.Codec, env.StoreService, env.AddressCodec,
		authority, env.BankKeeper, nil, log.NewNopLogger(),
	)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))
	ms := keeper.NewMsgServerImpl(k)

	_, sponsor := env.Addr(t)
	_, err = ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "rentals", Authority: sponsor,
			Verification:           types.VERIFY_GOVERNANCE,
			ChallengeWindowSeconds: window,
		},
	})
	require.NoError(t, err)
	minted, err := ms.MintAsset(env.Ctx, &types.MsgMintAsset{
		Minter: sponsor, CollectionId: "rentals", Owner: sponsor, Uri: "ipfs://lease",
	})
	require.NoError(t, err)
	_, err = ms.Fractionalise(env.Ctx, &types.MsgFractionalise{
		Owner: sponsor, AssetId: minted.AssetId, Symbol: "RENT",
		Supply: math.NewInt(1_000), HolderShareBps: 2_500, IncomeDenom: income,
	})
	require.NoError(t, err)

	tenantAddr, tenant := env.Addr(t)
	env.Fund(t, tenantAddr, sdk.NewCoins(price(1_000_000)))

	res, err := ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: tenant, AssetId: minted.AssetId, Amount: price(1_000_000),
	})
	require.NoError(t, err)

	// A quarter collected, three quarters still with whoever signed for it.
	require.Equal(t, price(250_000), res.Collected)
	require.Equal(t, math.NewInt(750_000), env.Balance(tenantAddr, income))
	require.Equal(t, math.NewInt(250_000), env.ModuleBalance(types.ModuleName, income))

	// And the whole of it is claimable, which is the test that the module is
	// not sitting on anything: the sponsor's share was never taken.
	vault, err := k.Vaults.Get(env.Ctx, minted.AssetId)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(250_000), sdk.Coins(vault.Held).AmountOf(income))
	require.Equal(t, math.NewInt(1_000_000), sdk.Coins(vault.Funded).AmountOf(income))

	sponsorAddr := mustAddr(t, env, sponsor)
	paid, err := ms.Claim(env.Ctx, &types.MsgClaim{Holder: sponsor, AssetId: minted.AssetId})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(250_000), sdk.Coins(paid.Paid).AmountOf(income))
	require.Equal(t, math.NewInt(250_000), env.Balance(sponsorAddr, income))
	require.True(t, env.ModuleBalance(types.ModuleName, income).IsZero())
}
