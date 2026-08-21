package types_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

const (
	eur   = "ueur"
	bankA = "yml1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	bankB = "yml1bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func hash32() []byte { return make([]byte, types.BatchHashLength) }

func TestDefaultGenesisIsValidAndStartsAtOne(t *testing.T) {
	genesis := types.DefaultGenesis()
	require.NoError(t, genesis.Validate())
	require.Equal(t, uint64(1), genesis.CurrentCycle,
		"an id of zero is indistinguishable from an unset field in proto3")
	require.Equal(t, uint64(1), genesis.ObligationCount)
	require.Len(t, genesis.Cycles, 1,
		"the open window is written explicitly so nothing has to cope with it not existing")
	require.Equal(t, types.CYCLE_STATUS_OPEN, genesis.Cycles[0].Status)
}

// The invariant that says netting neither created nor destroyed value. A
// genesis that breaks it would start a chain whose participants collectively
// believe they are owed more than anyone owes.
func TestGenesisRefusesPositionsThatDoNotSumToZero(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Positions = []types.Position{
		{CycleId: 1, Denom: eur, Participant: bankA, Amount: math.NewInt(-100)},
		{CycleId: 1, Denom: eur, Participant: bankB, Amount: math.NewInt(99)},
	}
	genesis.Reserves = []types.Reserve{{Participant: bankA, Denom: eur, Amount: math.NewInt(100)}}

	require.ErrorContains(t, genesis.Validate(), "sum to")
}

// A state whose debits exceed the reserves behind them is a state whose next
// settlement cannot succeed. Refusing to start is a far better way to find that
// out than an end blocker holding every window.
func TestGenesisRefusesUndercollateralisedDebits(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Positions = []types.Position{
		{CycleId: 1, Denom: eur, Participant: bankA, Amount: math.NewInt(-100)},
		{CycleId: 1, Denom: eur, Participant: bankB, Amount: math.NewInt(100)},
	}
	genesis.Reserves = []types.Reserve{{Participant: bankA, Denom: eur, Amount: math.NewInt(99)}}

	require.ErrorContains(t, genesis.Validate(), "cannot settle")

	genesis.Reserves[0].Amount = math.NewInt(100)
	require.NoError(t, genesis.Validate())
}

// Debits in a slice that already settled are discharged, so they need no
// collateral. Requiring it would refuse every export from a chain that has ever
// closed a window.
func TestGenesisDoesNotRequireCollateralForSettledSlices(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Cycles = []types.Cycle{
		{
			Id:             1,
			Status:         types.CYCLE_STATUS_SETTLED,
			ClosedAtHeight: 10,
			Outcomes: []types.DenomOutcome{{
				Denom: eur, Status: types.DENOM_STATUS_SETTLED,
				GrossAmount: math.NewInt(100), NetAmount: math.NewInt(100),
			}},
		},
		{Id: 2, Status: types.CYCLE_STATUS_OPEN, OpenedAtHeight: 11},
	}
	genesis.CurrentCycle = 2
	genesis.CycleCount = 3
	genesis.Positions = []types.Position{
		{CycleId: 1, Denom: eur, Participant: bankA, Amount: math.NewInt(-100)},
		{CycleId: 1, Denom: eur, Participant: bankB, Amount: math.NewInt(100)},
	}

	require.NoError(t, genesis.Validate())

	// The same positions in a held slice are still owed, and still need cover.
	genesis.Cycles[0].Status = types.CYCLE_STATUS_HELD
	genesis.Cycles[0].Outcomes[0].Status = types.DENOM_STATUS_HELD
	genesis.Cycles[0].Outcomes[0].NetAmount = math.ZeroInt()
	genesis.Cycles[0].Outcomes[0].HoldReason = "positions did not balance"
	require.ErrorContains(t, genesis.Validate(), "cannot settle")
}

// A stored zero is the exact shape of the import/export bug this repository has
// hit before: a running chain that removes zero positions and an import that
// writes them would disagree the first time a pair netted out exactly.
func TestGenesisRefusesAStoredZeroPosition(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Positions = []types.Position{
		{CycleId: 1, Denom: eur, Participant: bankA, Amount: math.ZeroInt()},
	}
	require.ErrorContains(t, genesis.Validate(), "zero positions are not stored")
}

func TestGenesisRefusesIdsAtOrBeyondTheirCounter(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Cycles[0].Id = 2
	genesis.CurrentCycle = 2
	require.ErrorContains(t, genesis.Validate(), "cycle_count",
		"a cycle at the counter would collide with the next window opened")
}

func TestGenesisRefusesMoreThanOneOpenWindow(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Cycles = append(genesis.Cycles, types.Cycle{Id: 2, Status: types.CYCLE_STATUS_OPEN})
	genesis.CycleCount = 3
	require.ErrorContains(t, genesis.Validate(), "one window is open at a time")
}

func TestGenesisRefusesAnObligationWithoutABatchHash(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Obligations = []types.Obligation{{
		CycleId: 1, Id: 1, FromParticipant: bankA, ToParticipant: bankB,
		Denom: eur, Amount: math.NewInt(10), Mode: types.SETTLEMENT_MODE_NET,
	}}
	genesis.ObligationCount = 2
	require.ErrorContains(t, genesis.Validate(), "batch_hash")

	genesis.Obligations[0].BatchHash = hash32()
	require.NoError(t, genesis.Validate())
}

// Netting can only ever reduce what has to be funded. A slice claiming
// otherwise is arithmetic nobody should start a chain on.
func TestGenesisRefusesNetAboveGross(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Cycles[0].Outcomes = []types.DenomOutcome{{
		Denom: eur, Status: types.DENOM_STATUS_SETTLED,
		GrossAmount: math.NewInt(100), NetAmount: math.NewInt(101),
	}}
	require.ErrorContains(t, genesis.Validate(), "cannot increase")
}

// A counter of zero is the one genesis mistake this module cannot recover from
// on its own. The sequences are the *next* id to issue, so importing zero makes
// the first obligation a chain ever accepts id 0 — and in proto3 an id of 0 is
// indistinguishable from an unset field, so every client that receives it reads
// "belongs to no cycle" and "has no id". This repository has been bitten by
// that in four modules; nothing downstream of the import can tell the
// difference afterwards, which is why it is refused at the door.
func TestGenesisRefusesZeroCounters(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.ObligationCount = 0
	require.ErrorContains(t, genesis.Validate(), "obligation_count is zero",
		"an obligation_count of zero issues the first obligation id 0")

	genesis = types.DefaultGenesis()
	genesis.CycleCount = 0
	require.ErrorContains(t, genesis.Validate(), "zero")
}
