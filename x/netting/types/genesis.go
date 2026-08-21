package types

import (
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// FirstCycleID is the id of the window a fresh chain opens with.
//
// One, not zero. In proto3 a zero id is indistinguishable from an unset field,
// so "settled in cycle 0" reaches every client as "settled in no cycle at all",
// and this repository has had that same mistake three times in three modules.
const FirstCycleID uint64 = 1

// DefaultGenesis returns the default genesis state.
//
// The open window is written explicitly rather than created on demand, so that
// the end blocker and every message handler can read the current cycle without
// a branch for "there is not one yet". A branch that only executes on the very
// first obligation a chain ever sees is a branch that is never exercised again
// and never noticed when it breaks.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params: DefaultParams(),
		Cycles: []Cycle{{
			Id:             FirstCycleID,
			OpenedAtHeight: 0,
			Status:         CYCLE_STATUS_OPEN,
		}},
		CurrentCycle:    FirstCycleID,
		CycleCount:      FirstCycleID + 1,
		ObligationCount: 1,
	}
}

// Validate performs basic genesis state validation.
//
// The checks that matter are the last two. Positions in a currency must sum to
// zero within every cycle — that is the statement that netting neither created
// nor destroyed value, and a genesis that breaks it would start a chain whose
// participants collectively believe they are owed more than anyone owes. And
// the reserves must cover the debits: an imported state where a participant's
// unsettled net debit exceeds what it prefunded is a state whose next
// settlement cannot succeed, and it is far better to refuse to start than to
// discover that in an end blocker.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	if gs.CycleCount == 0 {
		return fmt.Errorf("cycle_count is zero; cycles are numbered from %d", FirstCycleID)
	}
	if gs.ObligationCount == 0 {
		return fmt.Errorf("obligation_count is zero; obligations are numbered from 1")
	}

	cycles := make(map[uint64]Cycle, len(gs.Cycles))
	openCycles := 0
	for _, cycle := range gs.Cycles {
		if cycle.Id == 0 {
			return fmt.Errorf("cycle with id 0; cycles are numbered from %d", FirstCycleID)
		}
		if _, seen := cycles[cycle.Id]; seen {
			return fmt.Errorf("duplicate cycle %d", cycle.Id)
		}
		if cycle.Id >= gs.CycleCount {
			return fmt.Errorf("cycle %d is at or beyond cycle_count %d, so the next window opened would collide with it", cycle.Id, gs.CycleCount)
		}
		if cycle.Status == CYCLE_STATUS_UNSPECIFIED {
			return fmt.Errorf("cycle %d has an unspecified status", cycle.Id)
		}
		if cycle.Status == CYCLE_STATUS_OPEN {
			openCycles++
			if cycle.Id != gs.CurrentCycle {
				return fmt.Errorf("cycle %d is open but current_cycle is %d; exactly one window is open at a time", cycle.Id, gs.CurrentCycle)
			}
		}

		seenDenom := make(map[string]bool, len(cycle.Outcomes))
		for _, outcome := range cycle.Outcomes {
			if err := sdk.ValidateDenom(outcome.Denom); err != nil {
				return fmt.Errorf("cycle %d outcome: %w", cycle.Id, err)
			}
			if seenDenom[outcome.Denom] {
				return fmt.Errorf("cycle %d has two outcomes for %s", cycle.Id, outcome.Denom)
			}
			seenDenom[outcome.Denom] = true

			if outcome.Status == DENOM_STATUS_UNSPECIFIED {
				return fmt.Errorf("cycle %d outcome for %s has an unspecified status", cycle.Id, outcome.Denom)
			}
			if outcome.GrossAmount.IsNil() || outcome.GrossAmount.IsNegative() {
				return fmt.Errorf("cycle %d outcome for %s has an invalid gross_amount", cycle.Id, outcome.Denom)
			}
			if outcome.NetAmount.IsNil() || outcome.NetAmount.IsNegative() {
				return fmt.Errorf("cycle %d outcome for %s has an invalid net_amount", cycle.Id, outcome.Denom)
			}
			// Netting can only ever reduce. A slice claiming to have funded
			// more than was submitted is arithmetic nobody should start a
			// chain on.
			if outcome.NetAmount.GT(outcome.GrossAmount) {
				return fmt.Errorf("cycle %d outcome for %s settled %s against a gross of %s; netting cannot increase what has to be funded",
					cycle.Id, outcome.Denom, outcome.NetAmount, outcome.GrossAmount)
			}
			if outcome.Status != DENOM_STATUS_HELD && outcome.HoldReason != "" {
				return fmt.Errorf("cycle %d outcome for %s is not held but carries a hold reason", cycle.Id, outcome.Denom)
			}
		}

		cycles[cycle.Id] = cycle
	}

	if openCycles != 1 {
		return fmt.Errorf("%d open cycles; exactly one window is open at a time", openCycles)
	}
	if _, ok := cycles[gs.CurrentCycle]; !ok {
		return fmt.Errorf("current_cycle is %d, which is not among the cycles", gs.CurrentCycle)
	}

	seenObligation := make(map[uint64]bool, len(gs.Obligations))
	for _, obligation := range gs.Obligations {
		if obligation.Id == 0 {
			return fmt.Errorf("obligation with id 0; obligations are numbered from 1")
		}
		if seenObligation[obligation.Id] {
			return fmt.Errorf("duplicate obligation %d", obligation.Id)
		}
		if obligation.Id >= gs.ObligationCount {
			return fmt.Errorf("obligation %d is at or beyond obligation_count %d", obligation.Id, gs.ObligationCount)
		}
		seenObligation[obligation.Id] = true

		if _, ok := cycles[obligation.CycleId]; !ok {
			return fmt.Errorf("obligation %d belongs to cycle %d, which does not exist", obligation.Id, obligation.CycleId)
		}
		if obligation.FromParticipant == "" || obligation.ToParticipant == "" {
			return fmt.Errorf("obligation %d is missing a participant", obligation.Id)
		}
		if obligation.FromParticipant == obligation.ToParticipant {
			return fmt.Errorf("obligation %d has the same participant on both sides", obligation.Id)
		}
		if err := sdk.ValidateDenom(obligation.Denom); err != nil {
			return fmt.Errorf("obligation %d: %w", obligation.Id, err)
		}
		if obligation.Amount.IsNil() || !obligation.Amount.IsPositive() {
			return fmt.Errorf("obligation %d has a non-positive amount", obligation.Id)
		}
		if obligation.Mode == SETTLEMENT_MODE_UNSPECIFIED {
			return fmt.Errorf("obligation %d has an unspecified settlement mode", obligation.Id)
		}
		if len(obligation.BatchHash) != BatchHashLength {
			return fmt.Errorf("obligation %d has a %d-byte batch_hash; %d are required", obligation.Id, len(obligation.BatchHash), BatchHashLength)
		}
	}

	// Positions are summed per (cycle, denom) and must come to zero. Kept in a
	// map keyed by a plain string rather than by anything structured, because
	// a key built from a collections.Pair holds pointers and silently never
	// matches itself — grouping by one is a bug this repository has already
	// paid for once.
	sums := make(map[string]math.Int, len(gs.Positions))
	seenPosition := make(map[string]bool, len(gs.Positions))
	debits := make(map[string]math.Int, len(gs.Positions))
	for _, position := range gs.Positions {
		cycle, ok := cycles[position.CycleId]
		if !ok {
			return fmt.Errorf("position for %s in cycle %d, which does not exist", position.Participant, position.CycleId)
		}
		if err := sdk.ValidateDenom(position.Denom); err != nil {
			return fmt.Errorf("position for %s: %w", position.Participant, err)
		}
		if position.Participant == "" {
			return fmt.Errorf("position in cycle %d has no participant", position.CycleId)
		}
		if position.Amount.IsNil() {
			return fmt.Errorf("position for %s in cycle %d has no amount", position.Participant, position.CycleId)
		}
		// A stored zero is the exact shape of the import/export bug this
		// repository has hit before: state derived by replaying obligations
		// removes a position that reaches zero, so a genesis that carries one
		// produces a state that exports differently from the one it came from.
		if position.Amount.IsZero() {
			return fmt.Errorf("position for %s in cycle %d is zero; zero positions are not stored", position.Participant, position.CycleId)
		}

		identity := fmt.Sprintf("%d/%s/%s", position.CycleId, position.Denom, position.Participant)
		if seenPosition[identity] {
			return fmt.Errorf("duplicate position for %s in %s in cycle %d", position.Participant, position.Denom, position.CycleId)
		}
		seenPosition[identity] = true

		group := fmt.Sprintf("%d/%s", position.CycleId, position.Denom)
		sum, ok := sums[group]
		if !ok {
			sum = math.ZeroInt()
		}
		sums[group] = sum.Add(position.Amount)

		if position.Amount.IsNegative() && SliceUnsettled(cycle, position.Denom) {
			key := position.Participant + "/" + position.Denom
			owed, ok := debits[key]
			if !ok {
				owed = math.ZeroInt()
			}
			debits[key] = owed.Add(position.Amount.Neg())
		}
	}

	for group, sum := range sums {
		if !sum.IsZero() {
			return fmt.Errorf("positions in %s sum to %s rather than zero; netting cannot create or destroy value", group, sum)
		}
	}

	reserves := make(map[string]math.Int, len(gs.Reserves))
	for _, reserve := range gs.Reserves {
		if reserve.Participant == "" {
			return fmt.Errorf("reserve with no participant")
		}
		if err := sdk.ValidateDenom(reserve.Denom); err != nil {
			return fmt.Errorf("reserve for %s: %w", reserve.Participant, err)
		}
		if reserve.Amount.IsNil() || reserve.Amount.IsNegative() {
			return fmt.Errorf("reserve for %s in %s is not a valid amount", reserve.Participant, reserve.Denom)
		}
		key := reserve.Participant + "/" + reserve.Denom
		if _, seen := reserves[key]; seen {
			return fmt.Errorf("duplicate reserve for %s in %s", reserve.Participant, reserve.Denom)
		}
		reserves[key] = reserve.Amount
	}

	for key, owed := range debits {
		held, ok := reserves[key]
		if !ok {
			held = math.ZeroInt()
		}
		if held.LT(owed) {
			return fmt.Errorf("%s owes %s across unsettled windows but has prefunded only %s; this state cannot settle", key, owed, held)
		}
	}

	return nil
}

// SliceUnsettled reports whether a cycle's slice of one currency still has to
// settle, and therefore whether its debits still have to be collateralised.
//
// A cycle with no recorded outcome for a currency counts as unsettled, which
// is the conservative reading: it means an imported state with missing
// bookkeeping is refused for under-collateralisation rather than started with
// obligations nobody is holding reserve against.
func SliceUnsettled(cycle Cycle, denom string) bool {
	if cycle.Status == CYCLE_STATUS_OPEN {
		return true
	}
	for _, outcome := range cycle.Outcomes {
		if outcome.Denom == denom {
			return outcome.Status != DENOM_STATUS_SETTLED
		}
	}
	return true
}
