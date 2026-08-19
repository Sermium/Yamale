package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/oracle/types"
)

// The distinction a consumer must be able to make: a price that has never
// existed is a configuration gap, a price that has stopped updating is an
// emergency. Returning "not found" for both would make them look identical.
func TestNoRateAndStaleRateAreDifferentAnswers(t *testing.T) {
	f := initFixture(t)

	_, err := f.keeper.PriceOf(f.ctx, denom, math.NewInt(1_000_000))
	require.ErrorIs(t, err, types.ErrRateUnavailable)

	operator, feeder := f.addValidator(t, 100)
	f.vote(t, feeder, operator, denom, "2.00")
	f.tally(t)

	agreedAt := f.env.Ctx.BlockTime().Unix()

	price, err := f.keeper.PriceOf(f.ctx, denom, math.NewInt(1_000_000))
	require.NoError(t, err)
	require.False(t, price.Stale)
	require.Equal(t, int64(0), price.AgeSeconds)
	require.Equal(t, "USD", price.Denom)

	// The feed stops. The value is still readable, still says how old it is,
	// and is now flagged as not to be acted on.
	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	f.at(agreedAt+int64(params.MaxRateAgeSeconds)+1, f.env.Ctx.BlockHeight()+1)

	price, err = f.keeper.PriceOf(f.ctx, denom, math.NewInt(1_000_000))
	require.NoError(t, err, "a stale price must still be readable")
	require.True(t, price.Stale)
	require.Equal(t, int64(params.MaxRateAgeSeconds)+1, price.AgeSeconds)

	// The form a lending path uses refuses instead, because those callers act
	// on the answer immediately and the safe failure is to stop.
	_, err = f.keeper.RequireFreshPrice(f.ctx, denom, math.NewInt(1_000_000))
	require.ErrorIs(t, err, types.ErrStale)
}

// The rate prices a display unit, so valuing base units without scaling would
// overstate the answer by a factor of a million.
func TestPriceOfScalesByTheDenomExponent(t *testing.T) {
	f := initFixture(t)
	operator, feeder := f.addValidator(t, 100)

	f.vote(t, feeder, operator, denom, "2.50")
	f.tally(t)

	// 1 uusd is 0.000001 display units at 2.50 → 0.0000025, truncated to 0.
	price, err := f.keeper.PriceOf(f.ctx, denom, math.NewInt(1))
	require.NoError(t, err)
	require.Equal(t, "0", price.Value.String())

	// 4 display units at 2.50 is 10.
	price, err = f.keeper.PriceOf(f.ctx, denom, math.NewInt(4_000_000))
	require.NoError(t, err)
	require.Equal(t, "10", price.Value.String())
}

// Staleness of a valuation is measured from the date it reflects, not the date
// it was filed. A quarterly valuation submitted three months late describes a
// quarter-old world however recently it was typed in.
func TestAssetValueAgesFromTheValuationDate(t *testing.T) {
	f := initFixture(t)
	f.nft.mint(classID, nftID)
	appraiser := f.approvedAppraiser(t)

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)

	submittedAt := f.env.Ctx.BlockTime().Unix()
	valuedAt := submittedAt - int64(params.MaxAppraisalAgeSeconds) - 1

	require.NoError(t, f.submitAppraisal(t, appraiser, "1000", valuedAt))

	price, err := f.keeper.ValueOfAsset(f.ctx, classID, nftID)
	require.NoError(t, err)
	require.Equal(t, "1000", price.Value.String())
	require.Equal(t, appraiser, price.Source, "the number must be attributable to who signed it")
	require.True(t, price.Stale, "an old valuation filed today is still an old valuation")

	_, err = f.keeper.RequireFreshAssetValue(f.ctx, classID, nftID)
	require.ErrorIs(t, err, types.ErrStale)

	// Never valued at all is a different answer again.
	_, err = f.keeper.ValueOfAsset(f.ctx, classID, "never-minted")
	require.ErrorIs(t, err, types.ErrAppraisalMissing)
}

// Whatever else changes, a consumer written against PriceSource must be able to
// ask both questions through the one interface.
func TestKeeperSatisfiesPriceSource(t *testing.T) {
	f := initFixture(t)
	var source types.PriceSource = f.keeper

	_, err := source.PriceOf(f.ctx, denom, math.NewInt(1))
	require.ErrorIs(t, err, types.ErrRateUnavailable)

	_, err = source.ValueOfAsset(f.ctx, classID, nftID)
	require.ErrorIs(t, err, types.ErrAppraisalMissing)
}
