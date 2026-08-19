package types_test

import (
	"testing"

	"yamale/blockchain/x/builderfee/types"

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
			genState: &types.GenesisState{BuilderApplicationMap: []types.BuilderApplication{{MsgTypeUrl: "0"}, {MsgTypeUrl: "1"}}, ApprovedBuilderMap: []types.ApprovedBuilder{{MsgTypeUrl: "0"}, {MsgTypeUrl: "1"}}},
			valid:    true,
		}, {
			desc: "duplicated builderApplication",
			genState: &types.GenesisState{
				BuilderApplicationMap: []types.BuilderApplication{
					{
						MsgTypeUrl: "0",
					},
					{
						MsgTypeUrl: "0",
					},
				},
				ApprovedBuilderMap: []types.ApprovedBuilder{{MsgTypeUrl: "0"}, {MsgTypeUrl: "1"}}},
			valid: false,
		}, {
			desc: "duplicated approvedBuilder",
			genState: &types.GenesisState{
				ApprovedBuilderMap: []types.ApprovedBuilder{
					{
						MsgTypeUrl: "0",
					},
					{
						MsgTypeUrl: "0",
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
