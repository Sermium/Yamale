package keeper_test

import (
	"testing"

	"yamale/blockchain/x/paymsg/types"

	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	genesisState := types.GenesisState{
		Params:                    types.DefaultParams(),
		ParticipantApplicationMap: []types.ParticipantApplication{{Creator: "0"}, {Creator: "1"}}, ApprovedParticipantMap: []types.ApprovedParticipant{{Participant: "0"}, {Participant: "1"}}, PaymentRecordMap: []types.PaymentRecord{{EndToEndId: "0"}, {EndToEndId: "1"}}}

	f := initFixture(t)
	err := f.keeper.InitGenesis(f.ctx, genesisState)
	require.NoError(t, err)
	got, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.EqualExportedValues(t, genesisState.Params, got.Params)
	require.EqualExportedValues(t, genesisState.ParticipantApplicationMap, got.ParticipantApplicationMap)
	require.EqualExportedValues(t, genesisState.ApprovedParticipantMap, got.ApprovedParticipantMap)
	require.EqualExportedValues(t, genesisState.PaymentRecordMap, got.PaymentRecordMap)

}
