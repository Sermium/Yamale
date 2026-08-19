package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/oracle/types"
)

// Export then import must reproduce the state byte for byte. A module whose
// derived state does not survive the round trip fails TestAppImportExport under
// simulation, and a counter rebuilt wrongly here would either overwrite a
// superseded valuation or leave a gap in the record.
func TestGenesisRoundTrip(t *testing.T) {
	f := initFixture(t)
	f.nft.mint(classID, nftID)

	operator, feeder := f.addValidator(t, 100)
	f.vote(t, feeder, operator, denom, "1.25")
	f.tally(t)

	hot, hotStr := f.env.Addr(t)
	_ = hot
	_, err := f.ms.DelegateFeeder(f.ctx, &types.MsgDelegateFeeder{
		Operator: feeder, Validator: operator, Feeder: hotStr,
	})
	require.NoError(t, err)

	appraiser := f.approvedAppraiser(t, classID)
	now := f.env.Ctx.BlockTime().Unix()
	require.NoError(t, f.submitAppraisal(t, appraiser, "1000", now))
	f.at(now+86_400, f.env.Ctx.BlockHeight()+1)
	require.NoError(t, f.submitAppraisal(t, appraiser, "900", now+86_400))
	f.at(now+172_800, f.env.Ctx.BlockHeight()+1)
	require.NoError(t, f.submitAppraisal(t, appraiser, "800", now+172_800))

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate(), "an exported genesis must be one the chain would accept")

	require.Len(t, exported.ExchangeRates, 1)
	require.Len(t, exported.Appraisals, 1)
	require.Len(t, exported.AppraisalHistory, 2)
	require.Len(t, exported.FeederDelegations, 1)
	require.Len(t, exported.Appraisers, 1)

	// A fresh module, initialised from what the first one exported.
	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))

	reExported, err := g.keeper.ExportGenesis(g.ctx)
	require.NoError(t, err)
	require.Equal(t, exported, reExported)

	// The rebuilt sequence must point past the imported history, or the next
	// revaluation would overwrite one of these entries.
	seq, err := g.keeper.AppraisalSeq.Get(g.ctx, collections.Join(classID, nftID))
	require.NoError(t, err)
	require.Equal(t, uint64(2), seq)

	g.nft.mint(classID, nftID)
	g.at(now+259_200, 100)
	require.NoError(t, g.submitAppraisal(t, appraiser, "700", now+259_200))

	after, err := g.keeper.ExportGenesis(g.ctx)
	require.NoError(t, err)
	require.Len(t, after.AppraisalHistory, 3, "the superseded valuation must be added, not overwritten")
}

// The default genesis is what a chain starts from, so it has to be valid and to
// leave no rate agreed: a chain that started with prices nobody voted for would
// be lending against numbers with no author.
func TestDefaultGenesisIsEmptyAndValid(t *testing.T) {
	f := initFixture(t)

	genesis := types.DefaultGenesis()
	require.NoError(t, genesis.Validate())
	require.Empty(t, genesis.ExchangeRates)
	require.Empty(t, genesis.Appraisers)

	require.NoError(t, f.keeper.InitGenesis(f.ctx, *genesis))
	_, err := f.keeper.PriceOf(f.ctx, denom, math.NewInt(1_000_000))
	require.ErrorIs(t, err, types.ErrRateUnavailable)
}

func TestGenesisRejectsUnattributableValuations(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Appraisals = []types.Appraisal{{
		ClassId: classID, NftId: nftID, Value: "1000", ValueDenom: "uusd",
		Appraiser: "cosmos1nobody", ValuedAt: 1_000_000,
	}}
	require.Error(t, genesis.Validate(), "a valuation signed by nobody must not survive genesis")

	// A superseded valuation in the current set claims to be both what the
	// asset is worth today and to have been replaced.
	genesis = types.DefaultGenesis()
	genesis.Appraisers = []types.Appraiser{{
		Address: "cosmos1somebody", Status: types.AppraiserStatus_APPRAISER_STATUS_APPROVED,
	}}
	genesis.Appraisals = []types.Appraisal{{
		ClassId: classID, NftId: nftID, Value: "1000", ValueDenom: "uusd",
		Appraiser: "cosmos1somebody", ValuedAt: 1_000_000, Superseded: true,
	}}
	require.Error(t, genesis.Validate())
}
