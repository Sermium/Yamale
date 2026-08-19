package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/validatorgov/keeper"
	"yamale/blockchain/x/validatorgov/types"
)

// TestGenesisRoundTripWithPendingRotations exports a chain carrying every shape
// of rotation, imports it into a fresh keeper, and exports again.
//
// The two exports are compared as encoded bytes rather than field by field. A
// comparison that walked the fields would pass while the module quietly stored
// a zero where the first export wrote nothing, or derived a value that genesis
// had written explicitly — and both of those are how an import stops producing
// the chain that was exported.
func TestGenesisRoundTripWithPendingRotations(t *testing.T) {
	source := initFixture(t)
	ms := keeper.NewMsgServerImpl(source.keeper)

	source.at(1)

	// A planned rotation counting down its delay.
	plannedAddr, planned := source.approveOperator(t)
	source.staking.AddValidator(plannedAddr)
	_, plannedTo := source.env.Addr(t)
	_, err := ms.RotateOperator(source.env.Ctx, &types.MsgRotateOperator{
		Creator: planned, NewOperator: plannedTo,
	})
	require.NoError(t, err)

	// An approved recovery, mid challenge window, with its validator paused.
	pausedAddr, contested := source.approveOperator(t)
	source.staking.AddValidator(pausedAddr)
	_, contestedTo := source.env.Addr(t)
	approvedRecovery, err := ms.ProposeOperatorRecovery(source.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: contestedTo, CurrentOperator: contested, NewOperator: contestedTo, Reason: "drowned phone",
	})
	require.NoError(t, err)
	_, err = ms.ApproveOperatorRecovery(source.env.Ctx, &types.MsgApproveOperatorRecovery{
		Authority: source.env.AuthorityString(t), RotationId: approvedRecovery.RotationId, Approve: true,
	})
	require.NoError(t, err)

	// A proposed recovery still waiting on the quorum, which carries no
	// completion height at all. This is the one that breaks a naive import: a
	// zero written into the completion queue would take effect in the first
	// block after the import.
	_, unapproved := source.approveOperator(t)
	_, unapprovedTo := source.env.Addr(t)
	waiting, err := ms.ProposeOperatorRecovery(source.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: unapprovedTo, CurrentOperator: unapproved, NewOperator: unapprovedTo, Reason: "unreachable",
	})
	require.NoError(t, err)

	// And a resolved one, so the export carries history as well as open work.
	_, cancelled := source.approveOperator(t)
	_, cancelledTo := source.env.Addr(t)
	withdrawn, err := ms.RotateOperator(source.env.Ctx, &types.MsgRotateOperator{
		Creator: cancelled, NewOperator: cancelledTo,
	})
	require.NoError(t, err)
	_, err = ms.CancelOperatorRotation(source.env.Ctx, &types.MsgCancelOperatorRotation{
		Creator: cancelled, RotationId: withdrawn.RotationId,
	})
	require.NoError(t, err)

	exported, err := source.keeper.ExportGenesis(source.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())
	require.Len(t, exported.OperatorRotations, 4)
	require.Equal(t, uint64(5), exported.RotationCount)

	// Exporting must not move the chain it exported. A read that consumed the
	// sequence would leave a gap in the ids of a chain that was exported and
	// then kept running, which is what taking a snapshot of a live node does.
	againFromSource, err := source.keeper.ExportGenesis(source.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported.RotationCount, againFromSource.RotationCount,
		"exporting twice must produce the same next id")

	// Import into a keeper that has seen nothing.
	target := initFixture(t)
	target.at(1)
	require.NoError(t, target.keeper.InitGenesis(target.env.Ctx, *exported))

	reExported, err := target.keeper.ExportGenesis(target.env.Ctx)
	require.NoError(t, err)

	require.Equal(t,
		source.env.Codec.MustMarshalJSON(exported),
		target.env.Codec.MustMarshalJSON(reExported),
		"the re-export of an imported genesis must be byte-for-byte what was imported",
	)

	// The indexes were rebuilt, not carried, so check they came back: the
	// pending lookups the ante veto and the delegator query both depend on.
	for _, operator := range []string{planned, contested, unapproved} {
		_, open, err := target.keeper.PendingRotationFor(target.env.Ctx, operator)
		require.NoError(t, err)
		require.True(t, open, "%s should still have an open rotation after the import", operator)
	}
	_, stillOpen, err := target.keeper.PendingRotationFor(target.env.Ctx, cancelled)
	require.NoError(t, err)
	require.False(t, stillOpen, "a resolved rotation must not be re-opened by an import")

	// The unapproved recovery must not complete just because blocks passed.
	params, err := target.keeper.Params.Get(target.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, target.keeper.EndBlocker(target.at(int64(params.RecoveryChallengeWindowBlocks)*2)))

	stillWaiting, err := target.keeper.Rotation.Get(target.env.Ctx, waiting.RotationId)
	require.NoError(t, err)
	require.Equal(t, types.ROTATION_STATUS_PENDING, stillWaiting.Status)
	require.False(t, stillWaiting.Approved)
}

// TestGenesisWithoutRotationCountSeedsTheSequenceAtOne covers a genesis written
// by hand, or by a binary that predates rotations. The field is absent, which
// decodes as zero, and importing it must still leave the sequence pointing at
// one.
//
// The assertion is on the sequence rather than on the id of a rotation opened
// afterwards, so that this test covers the import guard alone. The id handed
// out is guarded a second time on the way out, and a test that only looked at
// the id would pass with either guard removed — which is how two defences in
// depth quietly become none.
func TestGenesisWithoutRotationCountSeedsTheSequenceAtOne(t *testing.T) {
	f := initFixture(t)
	require.NoError(t, f.keeper.InitGenesis(f.env.Ctx, types.GenesisState{Params: types.DefaultParams()}))

	next, err := f.keeper.RotationSeq.Peek(f.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), next)
}

// TestRotationIdsAreNeverZeroWithoutGenesis covers the other guard: a keeper
// whose sequence was never seeded at all. In proto3 an id of zero is
// indistinguishable from an unset field, so a rotation numbered zero would be a
// pending rotation no query could name and no cancel could reach.
func TestRotationIdsAreNeverZeroWithoutGenesis(t *testing.T) {
	f := newFixture(t, false)

	seeded, err := f.keeper.RotationSeq.Peek(f.env.Ctx)
	require.NoError(t, err)
	require.Zero(t, seeded, "this fixture must start from an unseeded sequence, or it tests nothing")

	f.at(1)
	_, operator := f.approveOperator(t)
	_, incoming := f.env.Addr(t)

	resp, err := keeper.NewMsgServerImpl(f.keeper).RotateOperator(f.env.Ctx, &types.MsgRotateOperator{
		Creator: operator, NewOperator: incoming,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), resp.RotationId)
}
