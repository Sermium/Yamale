package keeper_test

import (
	"testing"

	"yamale/blockchain/x/stablecoin/types"

	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	genesisState := types.GenesisState{
		Params:               types.DefaultParams(),
		IssuerApplicationMap: []types.IssuerApplication{{Denom: "0"}, {Denom: "1"}}, ApprovedIssuerMap: []types.ApprovedIssuer{{Denom: "0"}, {Denom: "1"}}}

	f := initFixture(t)
	err := f.keeper.InitGenesis(f.ctx, genesisState)
	require.NoError(t, err)
	got, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.EqualExportedValues(t, genesisState.Params, got.Params)
	require.EqualExportedValues(t, genesisState.IssuerApplicationMap, got.IssuerApplicationMap)
	require.EqualExportedValues(t, genesisState.ApprovedIssuerMap, got.ApprovedIssuerMap)

}
