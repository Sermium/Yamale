package keeper_test

import (
	"testing"

	"yamale/blockchain/x/builderfee/types"

	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	genesisState := types.GenesisState{
		Params:                types.DefaultParams(),
		BuilderApplicationMap: []types.BuilderApplication{{MsgTypeUrl: "0"}, {MsgTypeUrl: "1"}}, ApprovedBuilderMap: []types.ApprovedBuilder{{MsgTypeUrl: "0"}, {MsgTypeUrl: "1"}}}

	f := initFixture(t)
	err := f.keeper.InitGenesis(f.ctx, genesisState)
	require.NoError(t, err)
	got, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.EqualExportedValues(t, genesisState.Params, got.Params)
	require.EqualExportedValues(t, genesisState.BuilderApplicationMap, got.BuilderApplicationMap)
	require.EqualExportedValues(t, genesisState.ApprovedBuilderMap, got.ApprovedBuilderMap)

}
