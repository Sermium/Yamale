package types

import (
	"fmt"
	"math"
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

	// DefaultSeizureDelayBlocks is twelve hours, and it is a floor rather than
	// the whole schedule: every seizure waits at least this long between being
	// decided and being carried out, however small it is.
	//
	// The delay is not hesitation, it is the window in which the decision can
	// still be undone for free. Before it expires the ombudsman can veto and
	// governance can reverse, and nothing has to be given back because nothing
	// has moved. After it expires, undoing means asking the foundation to send
	// money back, which is a favour rather than a mechanism.
	DefaultSeizureDelayBlocks = 8_640

	// DefaultSeizureWindowBlocks is seven days. Long enough that a validator set
	// which decided to empty the chain would have to keep deciding it, week
	// after week, in public, rather than doing it in one sitting.
	DefaultSeizureWindowBlocks = 120_960

	// DefaultMaxSeizuresPerWindow is how many seizures may execute in one
	// window. Five is far above any plausible week of honest enforcement on a
	// permissioned settlement chain and far below anything that could be called
	// mass expropriation, which is the gap the number is chosen to sit in.
	DefaultMaxSeizuresPerWindow = 5

	// MaxSeizuresPerWindowLimit bounds what governance may set the count cap to.
	//
	// Not a policy limit — a liveness one. Every seizure inside the window is a
	// ledger record the end blocker has to be able to walk and prune, so the cap
	// is also the bound on that work. A cap of a billion would make the bound
	// meaningless and the block time unpredictable.
	MaxSeizuresPerWindowLimit = 10_000

	// MaxSeizureDelayBlocks is roughly a year at five-second blocks, and it is
	// the largest delay any tier may carry.
	//
	// Two failures at once. A delay measured in centuries is a seizure that
	// never executes while the account stays frozen forever, which is a taking
	// dressed as a wait. And `height + int64(delay)` on an unbounded uint64
	// overflows into a negative height, which would make the case due
	// immediately — the exact opposite of what was configured.
	MaxSeizureDelayBlocks = 6_307_200

	// MaxSeizureDelayTiers bounds the schedule so that sizing one case cannot
	// become an unbounded walk over a parameter list governance can grow.
	MaxSeizureDelayTiers = 32

	// MaxIssuingAuthorityLength and MaxInstrumentReferenceLength bound the two
	// free-text halves of a legal instrument.
	//
	// Constants rather than parameters, unlike the reason and evidence bounds,
	// because these are not policy. A court names itself in a few words and
	// numbers its own orders in a few characters; there is no deployment whose
	// courts need more, and every parameter added here is one more thing the
	// constitutional layer has to pin and one more thing governance could set
	// to zero.
	MaxIssuingAuthorityLength    = 256
	MaxInstrumentReferenceLength = 128

	// MaxDelegationsAssessed bounds how many of a target's delegations are
	// counted when sizing a case. A target with more than this is assessed at
	// less than they hold, which sizes the delay short — but it is the same
	// bound the seizure itself unbonds under, so the alternative is an
	// assessment of funds the seizure would never reach.
	MaxDelegationsAssessed = 100
)

// NewParams creates a new Params instance.
func NewParams(
	votingPeriodBlocks, provisionalFreezeBlocks, thresholdBps uint64,
	recoveryDestination string,
	maxReasonLength, maxEvidenceURILength uint64,
	seizeRequiresEvidence bool,
	seizureDelayBlocks uint64,
	seizureDelayTiers []SeizureDelayTier,
	seizureWindowBlocks uint64,
	seizureWindowCap sdk.Coins,
	maxSeizuresPerWindow uint64,
	ombudsman string,
) Params {
	return Params{
		VotingPeriodBlocks:      votingPeriodBlocks,
		ProvisionalFreezeBlocks: provisionalFreezeBlocks,
		ThresholdBps:            thresholdBps,
		RecoveryDestination:     recoveryDestination,
		MaxReasonLength:         maxReasonLength,
		MaxEvidenceUriLength:    maxEvidenceURILength,
		SeizeRequiresEvidence:   seizeRequiresEvidence,
		SeizureDelayBlocks:      seizureDelayBlocks,
		SeizureDelayTiers:       seizureDelayTiers,
		SeizureWindowBlocks:     seizureWindowBlocks,
		SeizureWindowCap:        seizureWindowCap,
		MaxSeizuresPerWindow:    maxSeizuresPerWindow,
		Ombudsman:               ombudsman,
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
// The emergency authority and the ombudsman are genuinely optional and stay
// empty: unset means there is no emergency path and no veto, which is a safe
// state rather than a broken one. An implicit office would be worse than none.
//
// The delay tiers and the value cap are empty here for the same reason the
// recovery destination is. Both are denominated in a currency, and no
// denomination compiled into this binary is anybody else's currency — a default
// tier priced in `uyml` would be a live schedule that silently matches nothing
// on a chain issuing Kenyan shillings, which is worse than an absent one
// because it satisfies the check. Both are required by Validate, so a genesis
// that did not state them does not start.
func DefaultParams() Params {
	return NewParams(
		DefaultVotingPeriodBlocks,
		DefaultProvisionalFreezeBlocks,
		DefaultThresholdBps,
		"",
		DefaultMaxReasonLength,
		DefaultMaxEvidenceURILength,
		true,
		DefaultSeizureDelayBlocks,
		nil,
		DefaultSeizureWindowBlocks,
		nil,
		DefaultMaxSeizuresPerWindow,
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

	if err := p.validateSeizureDelay(); err != nil {
		return err
	}
	if err := p.validateSeizureWindow(); err != nil {
		return err
	}
	return p.validateOmbudsman()
}

// validateSeizureDelay checks the schedule that decides how long a seizure
// waits between being decided and being carried out.
func (p Params) validateSeizureDelay() error {
	// Zero is refused rather than treated as "no delay". The delay is the only
	// window in which a decided seizure can still be stopped at no cost to
	// anybody, so a delay of zero does not disable a safeguard, it removes the
	// ombudsman's veto entirely while leaving the office on the record.
	if p.SeizureDelayBlocks == 0 {
		return fmt.Errorf(
			"seizure_delay_blocks must be positive, or a seizure executes in the block it is decided and the veto has no window to be cast in")
	}
	if p.SeizureDelayBlocks > MaxSeizureDelayBlocks {
		return fmt.Errorf("seizure_delay_blocks is %d, the maximum is %d", p.SeizureDelayBlocks, MaxSeizureDelayBlocks)
	}

	// At least one tier, because the requirement is that delays *scale* with
	// the amount. A schedule with no tiers is a constant, and a constant is
	// what the tiers exist to replace.
	if len(p.SeizureDelayTiers) == 0 {
		return fmt.Errorf(
			"seizure_delay_tiers must name at least one tier; with none, every seizure waits the same time whether it takes a day's takings or somebody's savings")
	}
	if len(p.SeizureDelayTiers) > MaxSeizureDelayTiers {
		return fmt.Errorf("seizure_delay_tiers has %d entries, the maximum is %d", len(p.SeizureDelayTiers), MaxSeizureDelayTiers)
	}

	seen := make(map[string]bool, len(p.SeizureDelayTiers))
	for i, tier := range p.SeizureDelayTiers {
		if err := tier.Threshold.Validate(); err != nil {
			return fmt.Errorf("seizure_delay_tiers[%d] threshold is invalid: %w", i, err)
		}
		if !tier.Threshold.IsPositive() {
			return fmt.Errorf(
				"seizure_delay_tiers[%d] threshold is %s; a threshold of zero matches every seizure including the ones that take nothing",
				i, tier.Threshold)
		}
		// A tier that adds no delay is not a tier, it is a line in the
		// parameters that reads as a safeguard and is not one.
		if tier.DelayBlocks == 0 {
			return fmt.Errorf("seizure_delay_tiers[%d] has a delay of zero blocks, which is not a delay", i)
		}
		if tier.DelayBlocks > MaxSeizureDelayBlocks {
			return fmt.Errorf(
				"seizure_delay_tiers[%d] delay is %d blocks, the maximum is %d; beyond that the seizure never executes while the account stays frozen, which is a taking dressed as a wait",
				i, tier.DelayBlocks, MaxSeizureDelayBlocks)
		}
		key := tier.Threshold.String()
		if seen[key] {
			return fmt.Errorf("seizure_delay_tiers has two tiers at %s; one of them is dead text", key)
		}
		seen[key] = true
	}
	return nil
}

// validateSeizureWindow checks the rolling cap.
func (p Params) validateSeizureWindow() error {
	// A window of zero would make every seizure fall outside the window the
	// moment it happened, so nothing would ever count and the cap would never
	// bind. That is a cap that reads as one and is not one, which is worse than
	// having none.
	if p.SeizureWindowBlocks == 0 {
		return fmt.Errorf("seizure_window_blocks must be positive, or every seizure leaves the window in the block it enters it and the cap never binds")
	}

	// Zero would leave every seizure the validators passed sitting unexecutable
	// forever with the account frozen — a broken chain rather than a safe one.
	// A deployment that never wants seizures should not be voting for them.
	if p.MaxSeizuresPerWindow == 0 {
		return fmt.Errorf("max_seizures_per_window must be positive, or every passed seizure waits forever with its target still frozen")
	}
	if p.MaxSeizuresPerWindow > MaxSeizuresPerWindowLimit {
		return fmt.Errorf(
			"max_seizures_per_window is %d, the maximum is %d; the cap is also the bound on the ledger the end blocker walks",
			p.MaxSeizuresPerWindow, MaxSeizuresPerWindowLimit)
	}

	if p.SeizureWindowCap.Empty() {
		return fmt.Errorf(
			"seizure_window_cap must name at least one denomination; without a value cap the chain is bounded only by how many accounts it empties per window, not by how much it takes from them")
	}
	if err := p.SeizureWindowCap.Validate(); err != nil {
		return fmt.Errorf("seizure_window_cap is invalid: %w", err)
	}

	// Individually legal and collectively wrong is the failure mode this
	// module's genesis keeps hitting. A schedule that makes large seizures in
	// a currency wait a week, on a chain whose window does not cap that
	// currency at all, is exactly that: it reads as two safeguards and is one.
	for _, tier := range p.SeizureDelayTiers {
		if !p.SeizureWindowCap.AmountOf(tier.Threshold.Denom).IsPositive() {
			return fmt.Errorf(
				"seizure_delay_tiers delays seizures of %s but seizure_window_cap does not cap %s, so that denomination is rate-limited by count alone; add a %s cap",
				tier.Threshold.Denom, tier.Threshold.Denom, tier.Threshold.Denom)
		}
	}
	return nil
}

// validateOmbudsman checks the office that can stop a case and never start one.
//
// The checks here are the ones that can be made without asking the chain
// anything: an address, and the one other role in this struct that must not be
// held by the same key.
//
// Two of the office's exclusions are NOT here, and both are enforced where the
// state that answers them is:
//
//   - whether the ombudsman is a bonded validator. That is staking's answer, and
//     it can change after the parameters were set, so it is checked in every
//     handler that could open or advance a case.
//   - whether the ombudsman holds ROLE_ENFORCEMENT_AUTHORITY, which is what the
//     retired emergency_authority field became. That used to be a comparison of
//     two strings in this struct and is now a grant in another module, so
//     UpdateParams asks the perimeter keeper before writing, and the handlers
//     refuse the ombudsman outright whichever order the two happened in. The
//     handler check is the one that actually holds the property — a grant made
//     after the parameters were set cannot be caught by the parameters.
func (p Params) validateOmbudsman() error {
	if p.Ombudsman == "" {
		return nil
	}
	if _, err := sdk.AccAddressFromBech32(p.Ombudsman); err != nil {
		return fmt.Errorf("ombudsman is not a valid address: %w", err)
	}
	// The recovery destination is where seized funds land. An ombudsman who is
	// also the beneficiary of seizures is not outside the process it checks.
	if p.Ombudsman == p.RecoveryDestination {
		return fmt.Errorf(
			"ombudsman and recovery_destination are the same address; the office that can stop a seizure must not be the one that receives what seizures take")
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

// SeizureDelayFor is how long a seizure of this value waits between being
// decided and being carried out.
//
// The longest matching tier wins, and the floor applies to everything. Longest
// rather than first-match so the answer does not depend on the order governance
// happened to write the tiers in: an ordering bug in a parameter list is
// invisible until the day it moves somebody's savings at the speed meant for
// pocket change.
//
// Guarded at the point of use rather than trusted from Validate. This is called
// from the end blocker's path, and a parameter set that reached state without
// passing Validate — an upgrade that wrote params directly, a migration that
// did not re-check — must not be able to produce a delay of zero here. Zero
// would execute a seizure in the block it was decided, with no window for the
// veto that is supposed to be able to stop it.
func (p Params) SeizureDelayFor(value sdk.Coins) uint64 {
	delay := p.SeizureDelayBlocks
	for _, tier := range p.SeizureDelayTiers {
		if tier.DelayBlocks <= delay {
			continue
		}
		if value.AmountOf(tier.Threshold.Denom).GTE(tier.Threshold.Amount) {
			delay = tier.DelayBlocks
		}
	}
	if delay == 0 {
		delay = DefaultSeizureDelayBlocks
	}
	if delay > MaxSeizureDelayBlocks {
		delay = MaxSeizureDelayBlocks
	}
	return delay
}

// WindowStartHeight is the earliest height still inside the rolling window.
//
// There is deliberately no division anywhere in this module's window
// arithmetic — no bucket index, no modulus — because a bucketed window needs a
// divisor and a divisor that arrives as a zero from genesis halts the chain in
// a Begin or EndBlocker. Subtraction cannot do that. What it can do is
// underflow, so both ends are guarded here:
//
// A window of zero blocks would put the start at the current height, so every
// seizure would leave the window in the block it entered it and the cap would
// never bind. That fails open, which is the direction that loses somebody's
// money, so a zero window is treated as the default instead — the cap keeps
// binding on parameters that should never have been accepted.
//
// A window longer than the chain is old would put the start below genesis, so
// it is clamped to zero rather than being allowed to go negative and turn the
// range scan into one that matches nothing.
func (p Params) WindowStartHeight(height int64) int64 {
	window := p.SeizureWindowBlocks
	if window == 0 {
		window = DefaultSeizureWindowBlocks
	}
	if window > uint64(math.MaxInt64) {
		return 0
	}
	start := height - int64(window)
	if start < 0 {
		return 0
	}
	return start
}

// WindowBlocks is the window length with the same zero guard applied, for the
// callers that need to schedule a retry rather than compute a start height.
//
// Guarded at both ends. A retry scheduled `0` blocks ahead is a case that is
// re-checked every block forever, which is a busy loop wearing a safeguard's
// clothes. And a window past the range of an int64 overflows the `height +
// int64(window)` that schedules the retry, landing it at a negative height —
// which makes the case due immediately, the exact opposite of a long window.
// Both are unreachable through Validate and both are reachable through a
// migration that wrote parameters straight into the store.
func (p Params) WindowBlocks() uint64 {
	if p.SeizureWindowBlocks == 0 {
		return DefaultSeizureWindowBlocks
	}
	if p.SeizureWindowBlocks > uint64(math.MaxInt32) {
		return uint64(math.MaxInt32)
	}
	return p.SeizureWindowBlocks
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
