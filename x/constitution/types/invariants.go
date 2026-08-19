package types

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Defaults sized for a permissioned chain with five-second blocks and equal
// seats.
//
// These are a template, not a configuration. Every one of them has to survive a
// genesis ceremony that thought about it, which is why DefaultInvariants leaves
// the recovery destination empty and Validate refuses it: a ceiling that
// defaulted itself into existence is a ceiling nobody chose.
const (
	// DefaultMaxEntityPowerBps is a fifth of the seats for any one admitted
	// entity, and DefaultMaxBeneficialOwnerPowerBps a quarter for the owner
	// behind however many entities it holds. The owner ceiling is the looser of
	// the two on purpose: an owner that has been admitted twice is a fact the
	// set voted for, whereas one entity holding a fifth of a network is
	// concentration in a single balance sheet.
	DefaultMaxEntityPowerBps          = 2_000
	DefaultMaxBeneficialOwnerPowerBps = 2_500

	// DefaultMaxJurisdictionPowerBps is a third minus a hair. Deliberately below
	// one third rather than at it: a third of the power is a blocking minority
	// on anything this chain decides by two thirds, including a seizure, so a
	// single national authority sitting exactly on the line could veto every
	// enforcement decision taken against anyone it protects.
	DefaultMaxJurisdictionPowerBps = 3_333

	// DefaultConcentrationEpochBlocks is twenty-four hours. Concentration is a
	// state rather than an event, so the interval is a trade between how long a
	// breach may stand and how often the check pays for itself; a day is short
	// enough that an acquisition is corrected before the next reporting cycle
	// and long enough that an operator has time to react to being demoted.
	DefaultConcentrationEpochBlocks = 17_280

	// DefaultMinActiveValidators is the size below which the check reports
	// rather than acts. Five, because at five equal seats one seat is exactly
	// the default entity ceiling: below that, a single admitted validator is
	// over the cap by arithmetic alone and there is nothing enforcement could
	// do about it except take the chain apart.
	DefaultMinActiveValidators = 5

	// DefaultEnforcementThresholdBps, DefaultEnforcementVotingPeriodBlocks and
	// DefaultEnforcementProvisionalFreezeBlocks mirror x/enforcement's own
	// defaults. They are restated here rather than imported because importing
	// them would make the module that is supposed to be constrained the source
	// of the constraint.
	DefaultEnforcementThresholdBps            = 6_667
	DefaultEnforcementVotingPeriodBlocks      = 8_640
	DefaultEnforcementProvisionalFreezeBlocks = 17_280

	// DefaultAmendmentDelayBlocks is twenty-one days. The delay is the whole
	// protection an amendment carries: it is the window in which anybody who
	// would be harmed by the change finds out it is coming while they can still
	// act on it — move assets, object publicly, or leave. Hours would satisfy
	// the letter of "delayed" and none of that.
	DefaultAmendmentDelayBlocks = 362_880

	// DefaultAmendmentThresholdBps is four fifths, above the seizure threshold
	// on purpose. Changing the rule must never be easier than using it, or the
	// cheapest way past a two-thirds seizure threshold is a two-thirds vote to
	// lower it.
	DefaultAmendmentThresholdBps = 8_000

	// MinAmendmentDelayBlocks is seven days, and it is a floor on what an
	// amendment may set the delay to — including its own successor's.
	//
	// Without it the first amendment anybody proposes is the one that shortens
	// the delay to a block, after which every subsequent change is instant and
	// the constitution is an ordinary parameter set again. It is a constant in
	// the binary rather than an invariant in the store because a floor that can
	// itself be amended is not a floor.
	MinAmendmentDelayBlocks = 120_960

	// BasisPoints is the denominator every share on this chain is measured
	// against.
	BasisPoints = 10_000
)

// DefaultInvariants returns the template a genesis ceremony completes.
//
// The recovery destination is left empty and refused by Validate, which is not
// a contradiction: it is how this file says the value has no default. It names
// a real institution, and no address compiled into a binary is that institution
// on somebody else's network. A default that was merely *valid* would be worse
// than none, because it would satisfy the check while pointing every future
// seizure at an account whose key is held by whoever happened to generate it.
func DefaultInvariants() Invariants {
	return Invariants{
		MaxEntityPowerBps:                  DefaultMaxEntityPowerBps,
		MaxBeneficialOwnerPowerBps:         DefaultMaxBeneficialOwnerPowerBps,
		MaxJurisdictionPowerBps:            DefaultMaxJurisdictionPowerBps,
		ConcentrationEpochBlocks:           DefaultConcentrationEpochBlocks,
		MinActiveValidators:                DefaultMinActiveValidators,
		EnforcementThresholdBps:            DefaultEnforcementThresholdBps,
		EnforcementRecoveryDestination:     "",
		EnforcementVotingPeriodBlocks:      DefaultEnforcementVotingPeriodBlocks,
		EnforcementProvisionalFreezeBlocks: DefaultEnforcementProvisionalFreezeBlocks,
		AmendmentDelayBlocks:               DefaultAmendmentDelayBlocks,
		AmendmentThresholdBps:              DefaultAmendmentThresholdBps,
	}
}

// Validate refuses a settlement the chain cannot honestly enforce.
//
// Every field is checked for being unset, because zero is what an incomplete
// genesis and a field added by an upgrade both look like, and because three of
// these values reach a Begin/EndBlocker where a zero is a divisor or a schedule
// that never fires. Params.Validate() has not been sufficient protection on
// this chain before; the point of checking here is that InitGenesis refuses to
// start rather than discovering it at the first epoch.
func (inv Invariants) Validate() error {
	caps := []struct {
		name string
		bps  uint64
	}{
		{"max_entity_power_bps", inv.MaxEntityPowerBps},
		{"max_beneficial_owner_power_bps", inv.MaxBeneficialOwnerPowerBps},
		{"max_jurisdiction_power_bps", inv.MaxJurisdictionPowerBps},
	}
	for _, c := range caps {
		if c.bps == 0 {
			return fmt.Errorf("%s must be set; a ceiling of zero would demote every validator on the chain", c.name)
		}
		if c.bps > BasisPoints {
			return fmt.Errorf("%s cannot exceed %d, got %d", c.name, BasisPoints, c.bps)
		}
	}

	if inv.ConcentrationEpochBlocks == 0 {
		return fmt.Errorf("concentration_epoch_blocks must be positive; it is the modulus the epoch check divides by, and a zero there halts the chain in the first block")
	}
	if inv.MinActiveValidators == 0 {
		return fmt.Errorf("min_active_validators must be positive, or the epoch check could demote the last validator and stop block production")
	}

	// A ceiling below one seat's worth of power is unsatisfiable: at the floor
	// every admitted validator is over it by arithmetic, and a check that
	// enforced it would try to demote the entire set. Refused here because it
	// is a contradiction in the settlement itself, not a state the chain could
	// grow out of.
	minShare := uint64(BasisPoints) / uint64(inv.MinActiveValidators)
	if uint64(BasisPoints)%uint64(inv.MinActiveValidators) != 0 {
		minShare++
	}
	for _, c := range caps {
		if c.bps < minShare {
			return fmt.Errorf(
				"%s is %d, but one validator out of min_active_validators (%d) already holds %d basis points, so no set this small could ever satisfy it",
				c.name, c.bps, inv.MinActiveValidators, minShare,
			)
		}
	}

	if inv.EnforcementVotingPeriodBlocks == 0 {
		return fmt.Errorf("enforcement_voting_period_blocks must be positive, or a case could never resolve")
	}
	if inv.EnforcementProvisionalFreezeBlocks == 0 {
		return fmt.Errorf("enforcement_provisional_freeze_blocks must be positive, or opening a case would freeze nothing")
	}
	if inv.EnforcementProvisionalFreezeBlocks < inv.EnforcementVotingPeriodBlocks {
		return fmt.Errorf(
			"enforcement_provisional_freeze_blocks (%d) must be at least enforcement_voting_period_blocks (%d), or freezes lapse mid-vote",
			inv.EnforcementProvisionalFreezeBlocks, inv.EnforcementVotingPeriodBlocks,
		)
	}
	if inv.EnforcementThresholdBps > BasisPoints {
		return fmt.Errorf("enforcement_threshold_bps cannot exceed %d, got %d", BasisPoints, inv.EnforcementThresholdBps)
	}
	// Below a majority, a minority of the validator set could take assets from
	// anyone. There is no settlement worth having that allows that, so it is
	// refused rather than left to an amendment that happens to look reasonable.
	if inv.EnforcementThresholdBps <= 5_000 {
		return fmt.Errorf("enforcement_threshold_bps must exceed 5000 so a minority cannot seize assets, got %d", inv.EnforcementThresholdBps)
	}

	// The devnet ran for weeks with this empty. Two thirds of the validator set
	// could have passed a seizure that then had nowhere to send what it took,
	// and nobody noticed until a console printed the parameter.
	if strings.TrimSpace(inv.EnforcementRecoveryDestination) == "" {
		return fmt.Errorf("enforcement_recovery_destination must name the foundation account; a chain with none can pass a seizure it cannot carry out")
	}
	if _, err := sdk.AccAddressFromBech32(inv.EnforcementRecoveryDestination); err != nil {
		return fmt.Errorf("enforcement_recovery_destination is not a valid address: %w", err)
	}

	if inv.AmendmentDelayBlocks < MinAmendmentDelayBlocks {
		return fmt.Errorf(
			"amendment_delay_blocks is %d, below the floor of %d; a delay short enough to pass unnoticed is the same as no delay",
			inv.AmendmentDelayBlocks, MinAmendmentDelayBlocks,
		)
	}
	if inv.AmendmentThresholdBps > BasisPoints {
		return fmt.Errorf("amendment_threshold_bps cannot exceed %d, got %d", BasisPoints, inv.AmendmentThresholdBps)
	}
	// Changing the rule must never be easier than using it. An amendment
	// threshold at or below the seizure threshold means the cheapest route past
	// a two-thirds seizure vote is a two-thirds vote to lower it.
	if inv.AmendmentThresholdBps <= inv.EnforcementThresholdBps {
		return fmt.Errorf(
			"amendment_threshold_bps (%d) must exceed enforcement_threshold_bps (%d), or amending the seizure threshold is no harder than reaching it",
			inv.AmendmentThresholdBps, inv.EnforcementThresholdBps,
		)
	}

	return nil
}

// RequiredPower is the ratified power an amendment needs, given the power
// recorded when it opened.
//
// Rounded up, for the same reason x/enforcement rounds its threshold up: at
// four fifths of a five-validator set, rounding down would let four validators
// pass a change the threshold says needs more than four — the difference
// between "the set agreed" and "the arithmetic was generous".
func (inv Invariants) RequiredPower(totalPower int64) int64 {
	if totalPower <= 0 {
		return 0
	}
	numerator := totalPower * int64(inv.AmendmentThresholdBps)
	required := numerator / BasisPoints
	if required*BasisPoints != numerator {
		required++
	}
	return required
}

// AllowedPower is the most a group may hold out of totalPower under capBps.
//
// Rounded down, which is the opposite of RequiredPower and deliberately so: a
// ceiling rounded up would let a group sit a hair above it forever, and the
// whole argument for arithmetic caps is that nobody has to argue about the
// hair.
func AllowedPower(totalPower int64, capBps uint64) int64 {
	if totalPower <= 0 || capBps == 0 {
		return 0
	}
	return totalPower * int64(capBps) / BasisPoints
}

// PowerBps is a group's share of the total, in basis points.
//
// The divisor is guarded at the point of use rather than trusted to have been
// validated upstream. This is called from an end blocker, and a zero total —
// every validator jailed, a set not yet bonded, a chain one block after an
// import — would halt the chain rather than report a share of nothing.
func PowerBps(power, totalPower int64) uint64 {
	if totalPower <= 0 || power <= 0 {
		return 0
	}
	return uint64(power * BasisPoints / totalPower)
}
