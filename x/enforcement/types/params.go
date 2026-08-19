package types

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Defaults sized for a permissioned chain with a small, identified validator
// set and five-second blocks.
//
// Two of these decide whether the module is a safeguard or a weapon, and they
// pull in opposite directions. The freeze has to land in seconds, because that
// is how long it takes to drain a wallet; the seizure has to take hours, because
// that is how long it takes for a mistake to be noticed and argued with.
const (
	// DefaultVotingPeriodBlocks is twelve hours. Long enough for a validator
	// set spread across timezones to actually see a case; short enough that a
	// frozen account is not left waiting a week for an answer.
	DefaultVotingPeriodBlocks = 8_640

	// DefaultProvisionalFreezeBlocks is twenty-four hours — twice the voting
	// period, so a freeze can never lapse while the vote that would confirm it
	// is still running. This is the leash on the only power in the module a
	// single validator holds alone.
	DefaultProvisionalFreezeBlocks = 17_280

	// DefaultThresholdBps is two thirds of the bonded power, matching the
	// supermajority that already decides what this chain considers true. Taking
	// somebody's assets should be no easier than changing consensus.
	DefaultThresholdBps = 6_667

	// DefaultMaxReasonLength allows a paragraph. The grounds are meant to be
	// read by the accused, not to hold the case file.
	DefaultMaxReasonLength = 512

	DefaultMaxEvidenceURILength = 256
)

// NewParams creates a new Params instance.
func NewParams(
	votingPeriodBlocks, provisionalFreezeBlocks, thresholdBps uint64,
	recoveryDestination string,
	maxReasonLength, maxEvidenceURILength uint64,
	seizeRequiresEvidence bool,
	emergencyAuthority string,
) Params {
	return Params{
		VotingPeriodBlocks:      votingPeriodBlocks,
		ProvisionalFreezeBlocks: provisionalFreezeBlocks,
		ThresholdBps:            thresholdBps,
		RecoveryDestination:     recoveryDestination,
		MaxReasonLength:         maxReasonLength,
		MaxEvidenceUriLength:    maxEvidenceURILength,
		SeizeRequiresEvidence:   seizeRequiresEvidence,
		EmergencyAuthority:      emergencyAuthority,
	}
}

// DefaultParams returns a default set of parameters.
//
// The recovery destination is left empty here and refused by Validate, which
// is not a contradiction: it is how this file says the value has no default.
// It names a real institution — the foundation — and no address compiled into
// the binary could be that institution on somebody else's network. A default
// that was merely *valid* would be worse than none, because it would satisfy
// the check while pointing seizures at an account whose key is held by whoever
// happened to generate it.
//
// So DefaultGenesis is a template that has to be completed, and a chain whose
// genesis was not completed does not start. Every setup script in scripts/
// fills this in from the `foundation` key.
//
// The emergency authority is genuinely optional and stays empty: unset means
// there is no emergency path at all, which is a safe state rather than a
// broken one.
func DefaultParams() Params {
	return NewParams(
		DefaultVotingPeriodBlocks,
		DefaultProvisionalFreezeBlocks,
		DefaultThresholdBps,
		"",
		DefaultMaxReasonLength,
		DefaultMaxEvidenceURILength,
		true,
		"",
	)
}

// Validate validates the set of params.
func (p Params) Validate() error {
	if p.VotingPeriodBlocks == 0 {
		return fmt.Errorf("voting_period_blocks must be positive, or a case could never resolve")
	}
	if p.ProvisionalFreezeBlocks == 0 {
		return fmt.Errorf("provisional_freeze_blocks must be positive, or opening a case would freeze nothing")
	}
	// A freeze that lapsed before the vote ended would hand the account back
	// mid-case, which is both useless against a scammer and confusing to
	// everyone watching.
	if p.ProvisionalFreezeBlocks < p.VotingPeriodBlocks {
		return fmt.Errorf(
			"provisional_freeze_blocks (%d) must be at least voting_period_blocks (%d), or freezes lapse mid-vote",
			p.ProvisionalFreezeBlocks, p.VotingPeriodBlocks,
		)
	}
	if p.ThresholdBps > 10_000 {
		return fmt.Errorf("threshold_bps cannot exceed 10000, got %d", p.ThresholdBps)
	}
	// Below a majority, a minority of the validator set could take assets from
	// anyone. There is no configuration of this module worth having that allows
	// that, so it is refused rather than left to a governance proposal that
	// happens to look reasonable.
	if p.ThresholdBps <= 5_000 {
		return fmt.Errorf("threshold_bps must exceed 5000 so a minority cannot seize assets, got %d", p.ThresholdBps)
	}
	// The devnet ran for weeks with this empty. Two thirds of the validator set
	// could have passed a seizure that then had nowhere to send what it took,
	// and nobody noticed until a console printed the parameter. An empty string
	// is not a configuration, it is a missing one, so it is refused here rather
	// than caught later by the one message handler that happens to check.
	//
	// Validated as an address, not merely as a non-empty string: a typo lands
	// in the one place on this chain that seized funds can legally go, and
	// there is no second chance to notice it once the seizure executes.
	if strings.TrimSpace(p.RecoveryDestination) == "" {
		return fmt.Errorf("recovery_destination must name the foundation account; a chain with none can pass a seizure it cannot carry out")
	}
	if _, err := sdk.AccAddressFromBech32(p.RecoveryDestination); err != nil {
		return fmt.Errorf("recovery_destination is not a valid address: %w", err)
	}
	if p.MaxReasonLength == 0 {
		return fmt.Errorf("max_reason_length must be positive, or no case could state its grounds")
	}
	if p.MaxEvidenceUriLength == 0 {
		return fmt.Errorf("max_evidence_uri_length must be positive")
	}
	if p.EmergencyAuthority != "" {
		if _, err := sdk.AccAddressFromBech32(p.EmergencyAuthority); err != nil {
			return fmt.Errorf("emergency_authority is not a valid address: %w", err)
		}
	}

	return nil
}

// RequiredPower is the yes power a case needs to pass, given the bonded power
// it was measured against.
//
// Rounded up. At two thirds of a three-validator set, rounding down would let
// two validators out of three pass a case that the threshold says needs more
// than two — the difference between "a supermajority agreed" and "the
// arithmetic was generous".
func (p Params) RequiredPower(totalPower int64) int64 {
	if totalPower <= 0 {
		return 0
	}
	numerator := totalPower * int64(p.ThresholdBps)
	required := numerator / 10_000
	if required*10_000 != numerator {
		required++
	}
	return required
}

// ValidateReason checks the stated grounds against the configured bounds.
func (p Params) ValidateReason(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("a case must state its grounds")
	}
	if uint64(len(reason)) > p.MaxReasonLength {
		return fmt.Errorf("reason is %d characters, the maximum is %d", len(reason), p.MaxReasonLength)
	}
	return nil
}
