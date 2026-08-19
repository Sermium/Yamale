package keeper_test

import (
	"testing"

	"yamale/blockchain/x/validatorgov/types"

	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	genesisState := types.GenesisState{
		Params:                  types.DefaultParams(),
		RotationCount:           1,
		ValidatorApplicationMap: []types.ValidatorApplication{{Candidate: "0"}, {Candidate: "1"}},
		ApprovedValidatorMap: []types.ApprovedValidator{
			{Candidate: "0", Declaration: testDeclaration("ENTITY-0", "OWNER-0", "CH")},
			{Candidate: "1", Declaration: testDeclaration("ENTITY-1", "OWNER-1", "ZA")},
		},
	}

	f := initFixture(t)
	err := f.keeper.InitGenesis(f.ctx, genesisState)
	require.NoError(t, err)
	got, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.EqualExportedValues(t, genesisState.Params, got.Params)
	require.EqualExportedValues(t, genesisState.ValidatorApplicationMap, got.ValidatorApplicationMap)
	require.EqualExportedValues(t, genesisState.ApprovedValidatorMap, got.ApprovedValidatorMap)

}
