package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

// Imported into a *second* environment, never the one that exported. Three
// round-trip tests in this repository were recently found to be vacuous for
// exactly that reason: importing into the environment that already held the
// state proves that InitGenesis did not delete anything, which is not what the
// test claims to prove.
func TestGenesisRoundTripsWithACycleInFlight(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000), policy(ngn, 500_000))

	bankA := f.newParticipant(t, coins(eur, 9_000_000))
	bankB := f.newParticipant(t, coins(eur, 9_000_000))
	bankC := f.newParticipant(t, coins(eur, 9_000_000))
	for _, bank := range []string{bankA, bankB, bankC} {
		f.postReserve(t, bank, coins(eur, 50_000))
	}

	// One window settled, one still open and holding positions — the state an
	// upgrade at an arbitrary height would actually find.
	f.submit(t, bankA, bankB, eur, 900)
	f.submit(t, bankB, bankC, eur, 400)
	f.submit(t, bankC, bankA, eur, 250)
	f.endBlockAt(t, 10)

	f.ctx = f.ctx.WithBlockHeight(11)
	f.submit(t, bankA, bankB, eur, 700)
	f.submit(t, bankB, bankA, eur, 200)
	f.submit(t, bankA, bankC, eur, 1_000_000) // gross: at the threshold

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate(),
		"an export the chain would refuse to import is not an export")
	require.NotEmpty(t, exported.Positions, "the fixture produced no in-flight positions to round trip")

	// A fresh chain, started from what the old one wrote.
	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))

	reexported, err := g.keeper.ExportGenesis(g.ctx)
	require.NoError(t, err)
	require.Equal(t, exported.String(), reexported.String())

	// And the derived state the file does not carry has to come back too.
	// Locked is the collateral check; a chain that imported it as zero would
	// let every participant withdraw money it has already committed.
	for _, bank := range []string{bankA, bankB, bankC} {
		require.Equal(t, f.locked(t, bank, eur).String(), g.locked(t, bank, eur).String(),
			"locked collateral for %s was not rebuilt from the positions", bank)
	}

	// The obligation index is derived too, and a participant that could not
	// page through its own obligations after an upgrade has lost its
	// reconciliation record.
	original, err := f.q.ParticipantObligations(f.ctx, &types.QueryParticipantObligationsRequest{
		Participant: bankA, CycleId: f.currentCycle(t),
	})
	require.NoError(t, err)
	require.NotEmpty(t, original.Obligations)
	restored, err := g.q.ParticipantObligations(g.ctx, &types.QueryParticipantObligationsRequest{
		Participant: bankA, CycleId: f.currentCycle(t),
	})
	require.NoError(t, err)
	require.Equal(t, original.Obligations, restored.Obligations)
}

// A held slice has to survive an upgrade as a held slice. Importing it as
// settled would silently discharge obligations nobody paid; importing it as
// nothing would drop them. Both are worse than the hold.
func TestGenesisRoundTripsAHeldSlice(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	f.postReserve(t, bankA, coins(eur, 5_000))
	f.postReserve(t, bankB, coins(eur, 5_000))

	f.submit(t, bankA, bankB, eur, 300)
	cycleID := f.currentCycle(t)
	require.NoError(t, f.keeper.Position.Set(f.ctx,
		collections.Join3(cycleID, eur, bankB), math.NewInt(299)))
	f.endBlockAt(t, 10)

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	// Deliberately not Validate()d: the corruption that caused the hold is a
	// state the chain refuses to start from, which is itself the right
	// behaviour and is asserted separately in the types package.
	g := initFixtureSkippingValidation(t, exported)

	held, err := g.q.HeldSlices(g.ctx, &types.QueryHeldSlicesRequest{})
	require.NoError(t, err)
	require.Len(t, held.Held, 1, "the retry queue is derived from the cycles' outcomes and must be rebuilt")
	require.Equal(t, cycleID, held.Held[0].CycleId)

	// The collateral behind a held debit stays committed across the upgrade.
	require.Equal(t, math.NewInt(300).String(), g.locked(t, bankA, eur).String())
}

// A window whose positions all cancelled out exports no positions at all,
// because a zero position is removed rather than stored. That is the exact
// shape of the import/export bug this repository has hit before: a running
// chain that stored zeros and an import that wrote none would disagree the
// first time a bilateral pair netted out exactly.
func TestZeroPositionsAreNeverStored(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	f.postReserve(t, bankA, coins(eur, 5_000))
	f.postReserve(t, bankB, coins(eur, 5_000))

	f.submit(t, bankA, bankB, eur, 300)
	f.submit(t, bankB, bankA, eur, 300)

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.Empty(t, exported.Positions, "a position that reached zero must be removed, not stored")
	require.NoError(t, exported.Validate())

	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))
	reexported, err := g.keeper.ExportGenesis(g.ctx)
	require.NoError(t, err)
	require.Equal(t, exported.String(), reexported.String())
}

// initFixtureSkippingValidation imports a state the chain would normally refuse
// to start from, so the tests about held slices can exercise the import path
// without the corruption that caused the hold stopping them at the door.
func initFixtureSkippingValidation(t *testing.T, genesis *types.GenesisState) *fixture {
	t.Helper()
	g := initFixture(t)

	require.NoError(t, g.keeper.Params.Set(g.ctx, genesis.Params))
	for _, cycle := range genesis.Cycles {
		require.NoError(t, g.keeper.Cycle.Set(g.ctx, cycle.Id, cycle))
		for _, outcome := range cycle.Outcomes {
			if outcome.Status == types.DENOM_STATUS_HELD {
				require.NoError(t, g.keeper.HeldSlice.Set(g.ctx, collections.Join(cycle.Id, outcome.Denom)))
			}
		}
	}
	for _, obligation := range genesis.Obligations {
		require.NoError(t, g.keeper.Obligation.Set(g.ctx,
			collections.Join(obligation.CycleId, obligation.Id), obligation))
	}
	for _, reserve := range genesis.Reserves {
		require.NoError(t, g.keeper.Reserve.Set(g.ctx,
			collections.Join(reserve.Participant, reserve.Denom), reserve.Amount))
	}
	cycles := map[uint64]types.Cycle{}
	for _, cycle := range genesis.Cycles {
		cycles[cycle.Id] = cycle
	}
	locked := map[string]math.Int{}
	for _, position := range genesis.Positions {
		require.NoError(t, g.keeper.Position.Set(g.ctx,
			collections.Join3(position.CycleId, position.Denom, position.Participant), position.Amount))
		if !position.Amount.IsNegative() || !types.SliceUnsettled(cycles[position.CycleId], position.Denom) {
			continue
		}
		key := position.Participant + "\x00" + position.Denom
		running, ok := locked[key]
		if !ok {
			running = math.ZeroInt()
		}
		locked[key] = running.Add(position.Amount.Neg())
	}
	for key, amount := range locked {
		participant, denom, found := splitTestKey(key)
		require.True(t, found)
		require.NoError(t, g.keeper.Locked.Set(g.ctx, collections.Join(participant, denom), amount))
	}
	require.NoError(t, g.keeper.CurrentCycle.Set(g.ctx, genesis.CurrentCycle))
	require.NoError(t, g.keeper.CycleSeq.Set(g.ctx, genesis.CycleCount))
	require.NoError(t, g.keeper.ObligationSeq.Set(g.ctx, genesis.ObligationCount))
	return g
}

func splitTestKey(key string) (string, string, bool) {
	for i := range len(key) {
		if key[i] == 0 {
			return key[:i], key[i+1:], true
		}
	}
	return "", "", false
}

// A participant string in genesis becomes a store key directly, and nothing on
// the settlement path decodes it again. So a mistyped address does not fail
// anywhere — it produces a reserve credited to an identifier no key can sign
// for, and a cycle that settles real value into it. There is no message in this
// module that moves it back out, which makes refusing to start the only
// remedy that exists.
func TestInitGenesisRefusesAParticipantThatIsNotAnAddress(t *testing.T) {
	f := initFixture(t)
	real := f.newParticipant(t, coins(eur, 1_000))

	for name, mutate := range map[string]func(*types.GenesisState){
		"reserve": func(gs *types.GenesisState) {
			gs.Reserves = []types.Reserve{{Participant: "not-an-address", Denom: eur, Amount: math.NewInt(100)}}
		},
		"position": func(gs *types.GenesisState) {
			gs.Positions = []types.Position{
				{CycleId: 1, Denom: eur, Participant: "not-an-address", Amount: math.NewInt(-100)},
				{CycleId: 1, Denom: eur, Participant: real, Amount: math.NewInt(100)},
			}
			gs.Reserves = []types.Reserve{{Participant: "not-an-address", Denom: eur, Amount: math.NewInt(100)}}
		},
		"obligation": func(gs *types.GenesisState) {
			gs.Obligations = []types.Obligation{{
				CycleId: 1, Id: 1, FromParticipant: "not-an-address", ToParticipant: real,
				Denom: eur, Amount: math.NewInt(10), Mode: types.SETTLEMENT_MODE_NET,
				BatchHash: make([]byte, types.BatchHashLength),
			}}
			gs.ObligationCount = 2
		},
	} {
		genesis := types.DefaultGenesis()
		mutate(genesis)
		require.NoError(t, genesis.Validate(),
			"%s: the field checks pass, which is exactly why the address check has to exist", name)

		g := initFixture(t)
		require.ErrorContains(t, g.keeper.InitGenesis(g.ctx, *genesis), "not an address",
			"a %s naming a non-address must stop the chain starting", name)
	}
}
