package types

import "fmt"

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
)

// NewParams creates a new Params instance.
func NewParams(plannedRotationDelayBlocks, recoveryChallengeWindowBlocks uint64) Params {
	return Params{
		PlannedRotationDelayBlocks:    plannedRotationDelayBlocks,
		RecoveryChallengeWindowBlocks: recoveryChallengeWindowBlocks,
	}
}

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	return NewParams(
		DefaultPlannedRotationDelayBlocks,
		DefaultRecoveryChallengeWindowBlocks,
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

	return nil
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
