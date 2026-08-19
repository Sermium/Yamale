package types_test

import (
	"testing"

	"yamale/blockchain/x/validatorgov/types"

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
			desc:     "valid genesis state",
			genState: &types.GenesisState{Params: types.DefaultParams(), RotationCount: 1, ValidatorApplicationMap: []types.ValidatorApplication{{Candidate: "0"}, {Candidate: "1"}}, ApprovedValidatorMap: []types.ApprovedValidator{{Candidate: "0"}, {Candidate: "1"}}},
			valid:    true,
		}, {
			desc: "duplicated validatorApplication",
			genState: &types.GenesisState{
				ValidatorApplicationMap: []types.ValidatorApplication{
					{
						Candidate: "0",
					},
					{
						Candidate: "0",
					},
				},
				ApprovedValidatorMap: []types.ApprovedValidator{{Candidate: "0"}, {Candidate: "1"}}},
			valid: false,
		}, {
			desc: "duplicated approvedValidator",
			genState: &types.GenesisState{
				ApprovedValidatorMap: []types.ApprovedValidator{
					{
						Candidate: "0",
					},
					{
						Candidate: "0",
					},
				},
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
