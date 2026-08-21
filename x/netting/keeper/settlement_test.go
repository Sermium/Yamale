package keeper_test

import (
	"sort"
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

// The case the whole module exists for: a ring of obligations that bilateral
// netting cannot touch and multilateral netting clears to nothing.
//
// A owes B 100, B owes C 100, C owes A 100. No pair of them owes each other
// anything, so bilateral netting compresses zero. Multilaterally every position
// is zero and nothing has to be funded at all.
func TestMultilateralCycleClearsToNothing(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	bankC := f.newParticipant(t, coins(eur, 1_000_000))
	for _, bank := range []string{bankA, bankB, bankC} {
		f.postReserve(t, bank, coins(eur, 100))
	}

	f.submit(t, bankA, bankB, eur, 100)
	f.submit(t, bankB, bankC, eur, 100)
	f.submit(t, bankC, bankA, eur, 100)

	cycleID := f.currentCycle(t)
	for _, bank := range []string{bankA, bankB, bankC} {
		require.True(t, f.position(t, cycleID, eur, bank).IsZero(),
			"every position in a closed ring is zero before the window even shuts")
		require.True(t, f.locked(t, bank, eur).IsZero(),
			"a ring commits no collateral from anybody")
	}

	f.endBlockAt(t, 10)

	outcome := f.outcome(t, cycleID, eur)
	require.Equal(t, types.DENOM_STATUS_SETTLED, outcome.Status)
	require.Equal(t, math.NewInt(300).String(), outcome.GrossAmount.String())
	require.Equal(t, math.NewInt(0).String(), outcome.NetAmount.String())
	require.Equal(t, uint64(10_000), types.CompressionBps(outcome.GrossAmount, outcome.NetAmount),
		"a ring compresses completely")

	for _, bank := range []string{bankA, bankB, bankC} {
		require.Equal(t, math.NewInt(100).String(), f.reserve(t, bank, eur).String(),
			"nobody's reserve moved, because nobody owed anything net")
	}
	f.requireCustodyBalances(t, eur)
}

// Settlement discharges positions against reserves the module already holds.
// Nothing leaves the module account, which is what makes the step unrefusable —
// there is no transfer for a freeze, a blocked address or a lapsed approval to
// stop.
func TestSettlementRearrangesReservesWithoutMovingCoins(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	f.postReserve(t, bankA, coins(eur, 500))
	f.postReserve(t, bankB, coins(eur, 500))

	custodyBefore := f.moduleHoldings(eur)
	supplyBefore := f.env.Supply(eur)

	f.submit(t, bankA, bankB, eur, 300)
	cycleID := f.currentCycle(t)
	f.endBlockAt(t, 10)

	require.Equal(t, math.NewInt(200).String(), f.reserve(t, bankA, eur).String())
	require.Equal(t, math.NewInt(800).String(), f.reserve(t, bankB, eur).String())
	require.True(t, f.locked(t, bankA, eur).IsZero(), "settling releases the collateral it consumed")

	require.Equal(t, custodyBefore.String(), f.moduleHoldings(eur).String(),
		"settlement moves no coins in or out of custody")
	require.Equal(t, supplyBefore.String(), f.env.Supply(eur).String(),
		"and creates none")
	f.requireCustodyBalances(t, eur)

	require.Equal(t, types.DENOM_STATUS_SETTLED, f.outcome(t, cycleID, eur).Status)
	require.Equal(t, math.NewInt(300).String(), f.outcome(t, cycleID, eur).NetAmount.String())
}

// A window opens the block after the one that closed the last, and obligations
// submitted in the closing block belong to the closing window. Settling at the
// start of a block instead would settle a window that was still taking traffic.
func TestWindowBoundaries(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	f.postReserve(t, bankA, coins(eur, 500))

	first := f.currentCycle(t)
	require.Equal(t, types.FirstCycleID, first)

	// Blocks that are not a boundary close nothing.
	f.endBlockAt(t, 9)
	require.Equal(t, first, f.currentCycle(t))

	f.ctx = f.ctx.WithBlockHeight(10)
	f.submit(t, bankA, bankB, eur, 100)
	require.Equal(t, math.NewInt(-100).String(), f.position(t, first, eur, bankA).String(),
		"an obligation in the closing block is in the closing window")

	f.endBlockAt(t, 10)
	second := f.currentCycle(t)
	require.Equal(t, first+1, second)
	require.Equal(t, int64(11), f.cycle(t, second).OpenedAtHeight)
	require.Equal(t, int64(10), f.cycle(t, first).ClosedAtHeight)
	require.Equal(t, types.CYCLE_STATUS_SETTLED, f.cycle(t, first).Status)
	require.Equal(t, types.CYCLE_STATUS_OPEN, f.cycle(t, second).Status)
}

// Currencies settle independently. There is no cross-currency netting, so a
// euro slice and a naira slice share no arithmetic, and holding one because the
// other failed would turn a single fault into a system-wide stop.
func TestCurrenciesSettleIndependently(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000), policy(ngn, 1_000_000))

	bankA := f.newParticipant(t, sdkCoins(eur, 1_000_000, ngn, 1_000_000))
	bankB := f.newParticipant(t, sdkCoins(eur, 1_000_000, ngn, 1_000_000))
	f.postReserve(t, bankA, sdkCoins(eur, 500, ngn, 500))
	f.postReserve(t, bankB, sdkCoins(eur, 500, ngn, 500))

	f.submit(t, bankA, bankB, eur, 300)
	f.submit(t, bankB, bankA, ngn, 200)

	cycleID := f.currentCycle(t)
	f.endBlockAt(t, 10)

	require.Equal(t, math.NewInt(200).String(), f.reserve(t, bankA, eur).String())
	require.Equal(t, math.NewInt(700).String(), f.reserve(t, bankA, ngn).String())
	require.Equal(t, math.NewInt(800).String(), f.reserve(t, bankB, eur).String())
	require.Equal(t, math.NewInt(300).String(), f.reserve(t, bankB, ngn).String())
	require.Len(t, f.cycle(t, cycleID).Outcomes, 2)
	f.requireCustodyBalances(t, eur, ngn)
}

// The failure path, forced. Positions that do not sum to zero cannot be reached
// through the message handlers, which is why the guard exists: it is the
// response to a state this module did not build itself — an imported genesis, a
// migration, a bug in the cap. The whole currency slice must be refused, with
// every obligation in it left exactly as it was.
func TestUnbalancedSliceIsHeldEntirely(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	f.postReserve(t, bankA, coins(eur, 500))
	f.postReserve(t, bankB, coins(eur, 500))

	f.submit(t, bankA, bankB, eur, 300)
	cycleID := f.currentCycle(t)

	// Corrupt the credit side directly, behind the handlers' backs. This is the
	// only way to reach the invariant, and reaching it is the point.
	require.NoError(t, f.keeper.Position.Set(f.ctx,
		collections.Join3(cycleID, eur, bankB), math.NewInt(299)))

	reserveABefore := f.reserve(t, bankA, eur)
	reserveBBefore := f.reserve(t, bankB, eur)

	f.endBlockAt(t, 10)

	outcome := f.outcome(t, cycleID, eur)
	require.Equal(t, types.DENOM_STATUS_HELD, outcome.Status)
	require.Contains(t, outcome.HoldReason, "do not sum to zero")
	require.Equal(t, types.CYCLE_STATUS_HELD, f.cycle(t, cycleID).Status)

	require.Equal(t, reserveABefore.String(), f.reserve(t, bankA, eur).String(),
		"a held slice moves nothing at all")
	require.Equal(t, reserveBBefore.String(), f.reserve(t, bankB, eur).String())
	require.Equal(t, math.NewInt(300).String(), f.locked(t, bankA, eur).String(),
		"and the collateral behind it stays committed")

	held, err := f.q.HeldSlices(f.ctx, &types.QueryHeldSlicesRequest{})
	require.NoError(t, err)
	require.Len(t, held.Held, 1)
	require.Equal(t, cycleID, held.Held[0].CycleId)
	require.Equal(t, eur, held.Held[0].Denom)
	require.Equal(t, int64(10), held.Held[0].HeldSinceHeight)
}

// A held slice is retried unchanged, never recomputed. Once whatever blocked it
// is gone, it settles at its original amounts against its original
// counterparties — which is what "no unwinding" means in code rather than in a
// design document.
func TestHeldSliceIsRetriedUnchangedAndClears(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	bankA := f.newParticipant(t, coins(eur, 1_000_000))
	bankB := f.newParticipant(t, coins(eur, 1_000_000))
	f.postReserve(t, bankA, coins(eur, 500))
	f.postReserve(t, bankB, coins(eur, 500))

	f.submit(t, bankA, bankB, eur, 300)
	cycleID := f.currentCycle(t)
	require.NoError(t, f.keeper.Position.Set(f.ctx,
		collections.Join3(cycleID, eur, bankB), math.NewInt(299)))
	f.endBlockAt(t, 10)
	require.Equal(t, types.DENOM_STATUS_HELD, f.outcome(t, cycleID, eur).Status)

	// New business continues in the window that opened after the held one. That
	// is the reason not to unwind: a stuck slice must not stop the system.
	f.ctx = f.ctx.WithBlockHeight(11)
	f.submit(t, bankB, bankA, eur, 50)
	require.Equal(t, math.NewInt(300).String(), f.locked(t, bankA, eur).String(),
		"the held debit stays collateralised while the new window runs")

	// Repair the corruption, as a chain upgrade would.
	require.NoError(t, f.keeper.Position.Set(f.ctx,
		collections.Join3(cycleID, eur, bankB), math.NewInt(300)))

	f.endBlockAt(t, 20)

	require.Equal(t, types.DENOM_STATUS_SETTLED, f.outcome(t, cycleID, eur).Status)
	require.Empty(t, f.outcome(t, cycleID, eur).HoldReason)
	require.Equal(t, types.CYCLE_STATUS_SETTLED, f.cycle(t, cycleID).Status)
	require.Equal(t, math.NewInt(300).String(), f.outcome(t, cycleID, eur).NetAmount.String(),
		"the retried slice settled the amount originally owed, not a recomputed one")

	// 500 - 300 (held slice) + 50 (the second window) for A.
	require.Equal(t, math.NewInt(250).String(), f.reserve(t, bankA, eur).String())
	require.Equal(t, math.NewInt(750).String(), f.reserve(t, bankB, eur).String())
	require.True(t, f.locked(t, bankA, eur).IsZero())

	held, err := f.q.HeldSlices(f.ctx, &types.QueryHeldSlicesRequest{})
	require.NoError(t, err)
	require.Empty(t, held.Held)
	f.requireCustodyBalances(t, eur)
}

// A cycle_blocks of zero reaching the modulo would panic inside an end blocker,
// which is not a failed transaction but a chain that stops. It has happened on
// this chain before, which is why the guard is at the point of division rather
// than only in Params.Validate().
func TestZeroCycleBlocksDoesNotHaltTheChain(t *testing.T) {
	f := initFixture(t)
	// Written straight to the store, bypassing Validate, exactly as a
	// hand-edited genesis would.
	require.NoError(t, f.keeper.Params.Set(f.ctx, types.Params{
		CycleBlocks:   0,
		DenomPolicies: []types.DenomPolicy{policy(eur, 1_000_000)},
	}))

	require.NotPanics(t, func() {
		require.NoError(t, f.keeper.EndBlocker(f.ctx.WithBlockHeight(10)))
	})
	require.Equal(t, types.FirstCycleID, f.currentCycle(t),
		"with netting off no window is ever closed")
}

// The compression the chain reports has to be computed by the chain, so that
// every participant quotes the same number — and it has to survive a window
// that carried nothing, where the denominator is zero.
func TestCompressionIsReportedAndGuardsItsDivisor(t *testing.T) {
	require.Equal(t, uint64(0), types.CompressionBps(math.ZeroInt(), math.ZeroInt()),
		"an empty window compressed nothing because it carried nothing")
	require.Equal(t, uint64(0), types.CompressionBps(math.NewInt(100), math.NewInt(100)))
	require.Equal(t, uint64(5_000), types.CompressionBps(math.NewInt(100), math.NewInt(50)))
	require.Equal(t, uint64(10_000), types.CompressionBps(math.NewInt(100), math.ZeroInt()))
	require.Equal(t, uint64(0), types.CompressionBps(math.NewInt(100), math.NewInt(200)),
		"netting cannot increase what has to be funded, so this is a bug, not 200% compression")
}

// Atomicity, forced at the only point where it can actually be lost: a slice
// that fails *after* some of its legs have already been written.
//
// Every other hold in this suite is refused by the zero-sum check, which runs
// before anything moves — so those tests pass whether settlement is atomic or
// not. This one makes the *second* debtor in store order unfundable, so the
// first debtor's reserve has already been debited and its collateral already
// released by the time the failure is found. Without the cached branch those
// two writes survive: one institution has paid into a window that never
// settled, its counterparties were told nothing, and no message in this module
// can put it back.
func TestASliceThatFailsMidwayWritesNothing(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	debtorFirst, debtorShort, creditor := f.threeBanksInStoreOrder(t)

	f.submit(t, debtorFirst, creditor, eur, 100)
	f.submit(t, debtorShort, creditor, eur, 100)
	cycleID := f.currentCycle(t)

	// Written behind the handlers' backs. The net debit cap makes this
	// unreachable through a message, which is exactly why the settlement path
	// checks the reserve again: the state it is defending against is an
	// imported genesis or a bad migration, not a transaction.
	require.NoError(t, f.keeper.Reserve.Set(f.ctx,
		collections.Join(debtorShort, eur), math.NewInt(50)))

	f.endBlockAt(t, 10)

	outcome := f.outcome(t, cycleID, eur)
	require.Equal(t, types.DENOM_STATUS_HELD, outcome.Status)
	require.Contains(t, outcome.HoldReason, "insufficient reserve",
		"the hold must name the leg that could not be funded")
	require.True(t, outcome.NetAmount.IsZero())

	require.Equal(t, math.NewInt(100).String(), f.reserve(t, debtorFirst, eur).String(),
		"the first debtor was debited before the failure was found, and that write must not survive")
	require.Equal(t, math.NewInt(100).String(), f.locked(t, debtorFirst, eur).String(),
		"nor may its collateral be released for a slice that did not settle")
	require.Equal(t, math.NewInt(50).String(), f.reserve(t, debtorShort, eur).String())
	require.Equal(t, math.NewInt(100).String(), f.reserve(t, creditor, eur).String(),
		"and the creditor must not have been paid part of a cycle")

	// Deliberately no custody check here. The corruption above removed 50 from
	// the recorded reserve without removing it from the module account, so the
	// two no longer agree — by the test's own hand, before the end blocker ran.
	// That invariant is asserted on states this module actually built, in
	// TestRecordedReservesAlwaysEqualCustody.
}

// The retry queue is derived from the cycles' own outcomes, so it has to be
// rebuilt by the keeper's InitGenesis rather than carried in the file — and the
// test has to go through that InitGenesis. A held slice imported as settled
// would silently discharge obligations nobody paid; imported as nothing it
// would drop them.
//
// The state exported here is one a chain would legitimately refuse to start
// from until the shortfall was funded, so the reserve is topped up first: what
// an operator actually does about a hold is post the money, and the export then
// validates and the slice clears on the first boundary after the upgrade.
func TestInitGenesisRebuildsTheRetryQueueAndTheHoldStillClears(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, 10, policy(eur, 1_000_000))

	debtorFirst, debtorShort, creditor := f.threeBanksInStoreOrder(t)
	f.submit(t, debtorFirst, creditor, eur, 100)
	f.submit(t, debtorShort, creditor, eur, 100)
	cycleID := f.currentCycle(t)

	require.NoError(t, f.keeper.Reserve.Set(f.ctx,
		collections.Join(debtorShort, eur), math.NewInt(50)))
	f.endBlockAt(t, 10)
	require.Equal(t, types.DENOM_STATUS_HELD, f.outcome(t, cycleID, eur).Status)

	f.postReserve(t, debtorShort, coins(eur, 50))

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate(),
		"a funded hold is a state the chain must be willing to start from")

	// A fresh chain, through the real InitGenesis, on a second store.
	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))

	held, err := g.q.HeldSlices(g.ctx, &types.QueryHeldSlicesRequest{})
	require.NoError(t, err)
	require.Len(t, held.Held, 1, "the retry queue must be rebuilt from the cycles' outcomes")
	require.Equal(t, cycleID, held.Held[0].CycleId)
	require.Equal(t, eur, held.Held[0].Denom)

	// The collateral behind a held debit stays committed across the upgrade,
	// or the debtors could withdraw the money the slice is waiting on.
	require.Equal(t, math.NewInt(100).String(), g.locked(t, debtorFirst, eur).String())
	require.Equal(t, math.NewInt(100).String(), g.locked(t, debtorShort, eur).String())

	g.endBlockAt(t, 20)

	require.Equal(t, types.DENOM_STATUS_SETTLED, g.outcome(t, cycleID, eur).Status)
	require.Equal(t, math.NewInt(200).String(), g.outcome(t, cycleID, eur).NetAmount.String(),
		"the retried slice settled what was originally owed, not a recomputed figure")
	require.True(t, g.locked(t, debtorFirst, eur).IsZero())
	require.True(t, g.locked(t, debtorShort, eur).IsZero())
	require.Equal(t, math.NewInt(300).String(), g.reserve(t, creditor, eur).String())

	remaining, err := g.q.HeldSlices(g.ctx, &types.QueryHeldSlicesRequest{})
	require.NoError(t, err)
	require.Empty(t, remaining.Held)
}

// threeBanksInStoreOrder returns three approved institutions, each holding a
// reserve of 100, in the order the settlement path will encounter them.
//
// Sorted, because the position store is walked in key order and the tests above
// need to know which debtor is reached first: a failure on the *first* leg
// would prove nothing about atomicity, because nothing has been written yet.
func (f *fixture) threeBanksInStoreOrder(t *testing.T) (first, second, third string) {
	t.Helper()
	banks := []string{
		f.newParticipant(t, coins(eur, 1_000_000)),
		f.newParticipant(t, coins(eur, 1_000_000)),
		f.newParticipant(t, coins(eur, 1_000_000)),
	}
	sort.Strings(banks)
	for _, bank := range banks {
		f.postReserve(t, bank, coins(eur, 100))
	}
	return banks[0], banks[1], banks[2]
}
