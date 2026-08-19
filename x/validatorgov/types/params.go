package types

import (
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Defaults sized for this chain's five-second blocks.
//
// The two delays are the whole difference between the paths. One is short
// because possession of the key already proved everything the delay could; the
// other is long because nothing has been proved at all.
const (
	// DefaultPlannedRotationDelayBlocks is 48 hours, one governance voting
	// period on this chain. The rotation was signed by the operator being
	// replaced, so the delay is not evidence-gathering: it is the window in
	// which delegators and the other validators get to see the change coming,
	// and matching it to the voting period means it is the same window they
	// already have for every other decision that affects them.
	DefaultPlannedRotationDelayBlocks = 34_560

	// DefaultRecoveryChallengeWindowBlocks is 7 days. Measured in days because
	// it is the time the real operator has to notice a false claim that their
	// key is gone, and somebody whose phone drowned on a Friday should not lose
	// their validator by Monday.
	DefaultRecoveryChallengeWindowBlocks = 120_960

	// DefaultAttestationIntervalBlocks is one year. It matches the cycle the
	// rest of the financial system refreshes beneficial ownership on, which is
	// the point: the declaration is only useful if somebody can compare it
	// against a filing made to the same schedule elsewhere.
	DefaultAttestationIntervalBlocks = 6_307_200
)

// DefaultSeatBondAmount is one unit of consensus power.
//
// A seat is implemented as a fixed quantity of the bond denomination because
// Cosmos derives consensus power from bonded tokens and permits exactly one
// module to report validator updates. Setting a seat equal to the SDK's power
// reduction makes one seat exactly one unit of power, which is what turns every
// ceiling on this chain into a count a supervisor can check off a list — and
// what lets x/enforcement's two thirds, x/oracle's rate agreement and x/gov's
// tally become seat counts without a line of code changing in any of them.
func DefaultSeatBondAmount() math.Int { return sdk.DefaultPowerReduction }

// NewParams creates a new Params instance.
func NewParams(
	plannedRotationDelayBlocks, recoveryChallengeWindowBlocks, attestationIntervalBlocks uint64,
	seatBondAmount math.Int,
) Params {
	return Params{
		PlannedRotationDelayBlocks:    plannedRotationDelayBlocks,
		RecoveryChallengeWindowBlocks: recoveryChallengeWindowBlocks,
		AttestationIntervalBlocks:     attestationIntervalBlocks,
		SeatBondAmount:                seatBondAmount,
	}
}

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	return NewParams(
		DefaultPlannedRotationDelayBlocks,
		DefaultRecoveryChallengeWindowBlocks,
		DefaultAttestationIntervalBlocks,
		DefaultSeatBondAmount(),
	)
}

// Validate validates the set of params.
func (p Params) Validate() error {
	if p.PlannedRotationDelayBlocks == 0 {
		return fmt.Errorf("planned_rotation_delay_blocks must be positive, or a rotation would take effect in the block it was submitted in")
	}
	if p.RecoveryChallengeWindowBlocks == 0 {
		return fmt.Errorf("recovery_challenge_window_blocks must be positive, or an approved recovery would complete before the operator it names could object to it")
	}
	// A recovery that resolved faster than a planned rotation would make the
	// slow path the quick one, and every operator's best move would be to claim
	// their own key was lost.
	if p.RecoveryChallengeWindowBlocks < p.PlannedRotationDelayBlocks {
		return fmt.Errorf(
			"recovery_challenge_window_blocks (%d) must be at least planned_rotation_delay_blocks (%d), or claiming a lost key would be faster than rotating deliberately",
			p.RecoveryChallengeWindowBlocks, p.PlannedRotationDelayBlocks,
		)
	}

	if p.AttestationIntervalBlocks == 0 {
		return fmt.Errorf("attestation_interval_blocks must be positive, or every declaration on the chain would be stale from the block it was made in")
	}
	// Checked for nil as well as for sign: the field is a non-nullable
	// customtype, so a Params decoded from a store written before it existed
	// carries an Int with no big.Int behind it, and every arithmetic method on
	// that panics rather than returning false.
	if p.SeatBondAmount.IsNil() || !p.SeatBondAmount.IsPositive() {
		return fmt.Errorf("seat_bond_amount must be positive, or setting a validator's power would bond nothing and every seat would be worth zero")
	}

	return nil
}

// SeatBond is the tokens one seat carries.
//
// It substitutes the default for a nil or non-positive value for the same
// reason PlannedDelay does, and the consequence here is arithmetic rather than
// scheduling: this value multiplies a seat count, so a zero would set every
// validator's power to nothing and a nil would panic inside an end blocker.
func (p Params) SeatBond() math.Int {
	if p.SeatBondAmount.IsNil() || !p.SeatBondAmount.IsPositive() {
		return DefaultSeatBondAmount()
	}
	return p.SeatBondAmount
}

// AttestationInterval is how long a declaration stays fresh. Zero is
// substituted for the same reason: it is read at every epoch, and a zero there
// would report every validator on the chain as stale in the same block.
func (p Params) AttestationInterval() uint64 {
	if p.AttestationIntervalBlocks == 0 {
		return DefaultAttestationIntervalBlocks
	}
	return p.AttestationIntervalBlocks
}

// PlannedDelay is the delay to apply to a planned rotation.
//
// It substitutes the default for a zero rather than using one, because a zero
// here does not mean "immediately", it means the parameter was never set — by a
// hand-written genesis, or by an upgrade that added the field to a stored
// Params that predates it. Validate() rejects a zero, but Validate() only runs
// on the genesis and messages that pass through it, and this value is read in
// the end blocker. A zero reaching that path would complete every rotation in
// the block it was opened in, which is the failure this substitution exists to
// prevent.
func (p Params) PlannedDelay() uint64 {
	if p.PlannedRotationDelayBlocks == 0 {
		return DefaultPlannedRotationDelayBlocks
	}
	return p.PlannedRotationDelayBlocks
}

// ChallengeWindow is the window to apply to an approved recovery. Zero is
// substituted for the same reason as in PlannedDelay, and the consequence here
// is worse: a zero window would complete a recovery in the block it was
// approved in, leaving the operator whose key is claimed to be lost no block in
// which to prove otherwise.
func (p Params) ChallengeWindow() uint64 {
	if p.RecoveryChallengeWindowBlocks == 0 {
		return DefaultRecoveryChallengeWindowBlocks
	}
	return p.RecoveryChallengeWindowBlocks
}
