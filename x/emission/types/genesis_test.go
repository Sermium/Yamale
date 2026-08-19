package types_test

import (
	"testing"

	"yamale/blockchain/x/emission/types"

	"github.com/stretchr/testify/require"
)

func TestGenesisState_Validate(t *testing.T) {
	tests := []struct {
		desc     string
		genState *types.GenesisState
		valid    bool
	}{
		{
			desc:     "default is valid",
			genState: types.DefaultGenesis(),
			valid:    true,
		},
		{
			desc: "valid genesis state",
			genState: &types.GenesisState{
				Params: types.DefaultParams(),
				EmissionState: &types.EmissionState{
					CurrentProvisionsPerBlock: "28",
					LastReductionPeriod:       81,
				},
			},
			valid: true,
		},
		{
			desc:     "zero params are rejected",
			genState: &types.GenesisState{},
			valid:    false,
		},
		{
			desc: "reduction factor above one is rejected",
			genState: &types.GenesisState{
				Params: types.NewParams(types.DefaultReductionPeriodInBlocks, "1.5", types.DefaultGenesisProvisionsPerBlock),
			},
			valid: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.genState.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
