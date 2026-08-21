package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

// Every obligation query is scoped to a participant, and there is no endpoint
// that enumerates the bilateral matrix. That is not confidentiality — a gRPC
// query carries no signature, so the chain does not know who is asking, and
// anybody running a node reads the store directly. It is the difference between
// a graph that is technically public and one that every explorer publishes,
// which is worth having and is worth describing accurately.
func TestParticipantObligationsReturnsOnlyThatParticipantsRecords(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	bankC := f.newParticipant(t, coins(eur, 1_000_000))
	for _, bank := range []string{bankA, bankB, bankC} {
		f.postReserve(t, bank, coins(eur, 50_000))
	}

	f.submit(t, bankA, bankB, eur, 100) // A is a party
	f.submit(t, bankC, bankA, eur, 200) // A is a party, on the other side
	f.submit(t, bankB, bankC, eur, 300) // A is not

	cycleID := f.currentCycle(t)
	res, err := f.q.ParticipantObligations(f.ctx, &types.QueryParticipantObligationsRequest{
		Participant: bankA, CycleId: cycleID,
	})
	require.NoError(t, err)
	require.Len(t, res.Obligations, 2)
	for _, obligation := range res.Obligations {
		require.True(t, obligation.FromParticipant == bankA || obligation.ToParticipant == bankA,
			"an obligation neither side of which is the caller must not be returned")
	}

	// A participant that is party to nothing gets nothing, not everything.
	_, outsider := f.env.Addr(t)
	empty, err := f.q.ParticipantObligations(f.ctx, &types.QueryParticipantObligationsRequest{
		Participant: outsider, CycleId: cycleID,
	})
	require.NoError(t, err)
	require.Empty(t, empty.Obligations)

	// And an unscoped call is refused rather than treated as "all".
	_, err = f.q.ParticipantObligations(f.ctx, &types.QueryParticipantObligationsRequest{CycleId: cycleID})
	require.Error(t, err)
}

func TestPositionQueryReportsReserveCommitmentAndExposure(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	f.postReserve(t, bankA, coins(eur, 1_000))
	f.submit(t, bankA, bankB, eur, 600)

	res, err := f.q.Position(f.ctx, &types.QueryPositionRequest{Participant: bankA})
	require.NoError(t, err)
	require.Len(t, res.Entries, 1)
	entry := res.Entries[0]
	require.Equal(t, eur, entry.Denom)
	require.Equal(t, math.NewInt(1_000).String(), entry.Reserve.String())
	require.Equal(t, math.NewInt(600).String(), entry.Locked.String())
	require.Equal(t, math.NewInt(400).String(), entry.Available.String())
	require.Equal(t, math.NewInt(-600).String(), entry.NetPosition.String())

	_, err = f.q.Position(f.ctx, &types.QueryPositionRequest{})
	require.Error(t, err, "the participant is required; there is no chain-wide walk")
}

// The compression figure has to come from the chain so every participant quotes
// the same one, and it has to survive a window that carried nothing.
func TestCycleQueryReportsCompression(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	bankC := f.newParticipant(t, coins(eur, 1_000_000))
	for _, bank := range []string{bankA, bankB, bankC} {
		f.postReserve(t, bank, coins(eur, 50_000))
	}

	f.submit(t, bankA, bankB, eur, 1_000)
	f.submit(t, bankB, bankC, eur, 1_000)
	f.submit(t, bankC, bankA, eur, 1_000)
	cycleID := f.currentCycle(t)
	f.endBlockAt(t, 10)

	res, err := f.q.Cycle(f.ctx, &types.QueryCycleRequest{Id: cycleID})
	require.NoError(t, err)
	require.Len(t, res.Compression, 1)
	require.Equal(t, uint64(10_000), res.Compression[0].CompressionBps)

	// The window that opened after it has carried nothing, and a ratio whose
	// denominator is zero must not take the query process down.
	next, err := f.q.Cycle(f.ctx, &types.QueryCycleRequest{Id: f.currentCycle(t)})
	require.NoError(t, err)
	require.Empty(t, next.Compression)

	_, err = f.q.Cycle(f.ctx, &types.QueryCycleRequest{Id: 9_999})
	require.Error(t, err)
}

func TestCurrentCycleReportsWhenItCloses(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	res, err := f.q.CurrentCycle(f.ctx, &types.QueryCurrentCycleRequest{})
	require.NoError(t, err)
	require.Equal(t, types.FirstCycleID, res.Cycle.Id)
	require.Equal(t, int64(10), res.ClosesAtHeight)

	// With netting off, no block ever closes it, and the answer must say so
	// rather than dividing by zero to find out.
	f.setParams(t, 0)
	res, err = f.q.CurrentCycle(f.ctx, &types.QueryCurrentCycleRequest{})
	require.NoError(t, err)
	require.Zero(t, res.ClosesAtHeight)
}
