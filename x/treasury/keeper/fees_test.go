package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/treasury/types"
)

// routeFeesTo enables fee routing into treasuryID.
func (f *fixture) routeFeesTo(t *testing.T, treasuryID uint64) {
	t.Helper()
	params := types.DefaultParams()
	params.RouteFeesToOperatingTreasury = true
	params.FeeOperatingTreasuryId = treasuryID
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
}

// The sweep has to move the coins and credit the ledger, and the two figures
// have to agree afterwards. A sweep that moved the money without crediting
// would strand it: the module account would hold coins no treasury records, and
// nothing in the module pays out what no treasury holds.
func TestSweepCreditsTheOperatingTreasury(t *testing.T) {
	f := initFixture(t)
	_, creator := f.env.Addr(t)

	created, err := f.ms.CreateTreasury(f.ctx, &types.MsgCreateTreasury{Creator: creator, Name: "operating"})
	require.NoError(t, err)
	f.routeFeesTo(t, created.Id)

	fees := sdk.NewCoins(sdk.NewCoin("uzar", math.NewInt(7_500)), sdk.NewCoin("ungn", math.NewInt(2_500)))
	f.env.FundModule(t, authtypes.FeeCollectorName, fees)

	swept, err := f.keeper.SweepIntoOperatingTreasury(f.ctx, authtypes.FeeCollectorName)
	require.NoError(t, err)
	require.Equal(t, fees, swept)

	feeCollector := authtypes.NewModuleAddress(authtypes.FeeCollectorName)
	require.True(t, f.env.BankKeeper.GetAllBalances(f.ctx, feeCollector).IsZero(),
		"anything left in the fee collector is allocated to validators by x/distribution next block")

	treasuryAcc := authtypes.NewModuleAddress(types.ModuleName)
	require.Equal(t, fees, f.env.BankKeeper.GetAllBalances(f.ctx, treasuryAcc))

	for _, coin := range fees {
		available, err := f.keeper.AvailableBalance(f.ctx, created.Id, coin.Denom)
		require.NoError(t, err)
		require.Equal(t, coin.Amount, available,
			"the ledger has to agree with the module account, or the difference is unspendable")
	}
}

// Fees accumulate between sweeps, so the second sweep must add to the first
// rather than replace it.
func TestSweepAccumulates(t *testing.T) {
	f := initFixture(t)
	_, creator := f.env.Addr(t)

	created, err := f.ms.CreateTreasury(f.ctx, &types.MsgCreateTreasury{Creator: creator})
	require.NoError(t, err)
	f.routeFeesTo(t, created.Id)

	for range 3 {
		f.env.FundModule(t, authtypes.FeeCollectorName, sdk.NewCoins(sdk.NewCoin("uzar", math.NewInt(1_000))))
		_, err := f.keeper.SweepIntoOperatingTreasury(f.ctx, authtypes.FeeCollectorName)
		require.NoError(t, err)
	}

	available, err := f.keeper.AvailableBalance(f.ctx, created.Id, "uzar")
	require.NoError(t, err)
	require.Equal(t, math.NewInt(3_000), available)
}

// With routing off the fee collector is left exactly as it was. This is the
// default, and it is what every profile with a native token relies on: touching
// the fee collector there would silently take validators' rewards away.
func TestSweepIsANoOpWhenRoutingIsOff(t *testing.T) {
	f := initFixture(t)

	fees := sdk.NewCoins(sdk.NewCoin("uzar", math.NewInt(500)))
	f.env.FundModule(t, authtypes.FeeCollectorName, fees)

	swept, err := f.keeper.SweepIntoOperatingTreasury(f.ctx, authtypes.FeeCollectorName)
	require.NoError(t, err)
	require.Nil(t, swept)

	feeCollector := authtypes.NewModuleAddress(authtypes.FeeCollectorName)
	require.Equal(t, fees, f.env.BankKeeper.GetAllBalances(f.ctx, feeCollector))
}

// A treasury id that names nothing is refused rather than improvised around.
// Moving the coins anyway would put them in the module account with no ledger
// entry naming them, which is a permanent loss; leaving them in the fee
// collector is merely the old behaviour, and the caller logs it every block.
func TestSweepRefusesAnOperatingTreasuryThatDoesNotExist(t *testing.T) {
	f := initFixture(t)
	f.routeFeesTo(t, 42)

	fees := sdk.NewCoins(sdk.NewCoin("uzar", math.NewInt(500)))
	f.env.FundModule(t, authtypes.FeeCollectorName, fees)

	_, err := f.keeper.SweepIntoOperatingTreasury(f.ctx, authtypes.FeeCollectorName)
	require.ErrorIs(t, err, types.ErrTreasuryNotFound)

	feeCollector := authtypes.NewModuleAddress(authtypes.FeeCollectorName)
	require.Equal(t, fees, f.env.BankKeeper.GetAllBalances(f.ctx, feeCollector),
		"a refused sweep must not have moved anything")
}

// The flag decides whether routing happens, not the id. A treasury id that
// names a real treasury is not on its own a request to route anything there —
// which is the whole reason the parameter is two fields and not one.
func TestAValidTreasuryIdWithoutTheFlagRoutesNothing(t *testing.T) {
	f := initFixture(t)
	_, creator := f.env.Addr(t)

	created, err := f.ms.CreateTreasury(f.ctx, &types.MsgCreateTreasury{Creator: creator})
	require.NoError(t, err)

	params := types.DefaultParams()
	params.FeeOperatingTreasuryId = created.Id
	params.RouteFeesToOperatingTreasury = false
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	fees := sdk.NewCoins(sdk.NewCoin("uzar", math.NewInt(100)))
	f.env.FundModule(t, authtypes.FeeCollectorName, fees)

	swept, err := f.keeper.SweepIntoOperatingTreasury(f.ctx, authtypes.FeeCollectorName)
	require.NoError(t, err)
	require.Nil(t, swept)
	require.Equal(t, fees, f.env.BankKeeper.GetAllBalances(f.ctx, authtypes.NewModuleAddress(authtypes.FeeCollectorName)))
}
