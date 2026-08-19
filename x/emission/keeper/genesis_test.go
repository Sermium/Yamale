package keeper_test

import (
	"testing"

	"yamale/blockchain/x/emission/types"

	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	genesisState := types.GenesisState{
		Params: types.DefaultParams(),
		EmissionState: &types.EmissionState{CurrentProvisionsPerBlock: "33",
			LastReductionPeriod: 43,
		}}

	f := initFixture(t)
	err := f.keeper.InitGenesis(f.ctx, genesisState)
	require.NoError(t, err)
	got, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.EqualExportedValues(t, genesisState.Params, got.Params)
	require.EqualExportedValues(t, genesisState.EmissionState, got.EmissionState)

}

// A genesis file that omits emission_state must still produce a chain that can
// build its first block: BeginBlocker reads the state every block, so leaving
// it unset would halt the chain at height 1.
func TestInitGenesisWithoutEmissionStateSeedsFromParams(t *testing.T) {
	f := initFixture(t)

	genesisState := types.GenesisState{Params: types.DefaultParams()}
	require.NoError(t, f.keeper.InitGenesis(f.ctx, genesisState))

	state, err := f.keeper.EmissionState.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, types.DefaultParams().GenesisProvisionsPerBlock, state.CurrentProvisionsPerBlock)
	require.Equal(t, uint64(0), state.LastReductionPeriod)

	// The first block mints rather than failing.
	require.NoError(t, f.keeper.BeginBlocker(f.env.Ctx.WithBlockHeight(1)))
}
