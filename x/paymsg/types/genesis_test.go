package types_test

import (
	"testing"

	"yamale/blockchain/x/paymsg/types"

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
			genState: &types.GenesisState{ParticipantApplicationMap: []types.ParticipantApplication{{Creator: "0"}, {Creator: "1"}}, ApprovedParticipantMap: []types.ApprovedParticipant{{Participant: "0"}, {Participant: "1"}}, PaymentRecordMap: []types.PaymentRecord{{EndToEndId: "0"}, {EndToEndId: "1"}}},
			valid:    true,
		}, {
			desc: "duplicated participantApplication",
			genState: &types.GenesisState{
				ParticipantApplicationMap: []types.ParticipantApplication{
					{
						Creator: "0",
					},
					{
						Creator: "0",
					},
				},
				ApprovedParticipantMap: []types.ApprovedParticipant{{Participant: "0"}, {Participant: "1"}}, PaymentRecordMap: []types.PaymentRecord{{EndToEndId: "0"}, {EndToEndId: "1"}}},
			valid: false,
		}, {
			desc: "duplicated approvedParticipant",
			genState: &types.GenesisState{
				ApprovedParticipantMap: []types.ApprovedParticipant{
					{
						Participant: "0",
					},
					{
						Participant: "0",
					},
				},
				PaymentRecordMap: []types.PaymentRecord{{EndToEndId: "0"}, {EndToEndId: "1"}}},
			valid: false,
		}, {
			desc: "duplicated paymentRecord",
			genState: &types.GenesisState{
				PaymentRecordMap: []types.PaymentRecord{
					{
						EndToEndId: "0",
					},
					{
						EndToEndId: "0",
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
