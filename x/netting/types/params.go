package types

import (
	"fmt"
	"math"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MaxCycleBlocks bounds how long a netting window may stay open.
//
// The ceiling is not about taste. cycle_blocks is compared against the block
// height, which is an int64, so a uint64 value above the int64 range converts
// to a negative number and the cycle boundary is never reached — netting would
// silently stop and every participant's reserve would stay locked against
// positions that never settle. A day of blocks at a second each is under
// ninety thousand; a million is already an absurd window and is nowhere near
// the range where the conversion misbehaves.
const MaxCycleBlocks = 1_000_000

// DefaultParams returns default module parameters.
//
// Netting is off by default, and no currency has a policy. A chain starts
// doing exactly what it does today — every obligation settling gross, in the
// block it was submitted in — and a deployment turns netting on per currency
// once it has decided what a participant may owe and posted the reserves to
// back it. The opposite default would switch a live payment system from
// immediate settlement to deferred settlement at an upgrade, which is a change
// to the credit risk every participant is running and is not something a
// binary should decide for them.
func DefaultParams() Params {
	return Params{
		CycleBlocks:   0,
		DenomPolicies: nil,
	}
}

// Validate does the sanity check on the provided params.
//
// This is a gate, not the gate. Every value it checks is checked again at the
// point it is used, because Validate() runs on a governance proposal and on
// `genesis validate` — two things an operator can route around — while the end
// blocker runs on every node on every block. A divisor that reaches a Begin or
// End blocker as zero does not fail a transaction, it stops the chain.
func (p Params) Validate() error {
	if p.CycleBlocks > MaxCycleBlocks {
		return fmt.Errorf("cycle_blocks %d exceeds the maximum of %d", p.CycleBlocks, MaxCycleBlocks)
	}

	seen := make(map[string]bool, len(p.DenomPolicies))
	for _, policy := range p.DenomPolicies {
		if err := sdk.ValidateDenom(policy.Denom); err != nil {
			return fmt.Errorf("denom policy for %q: %w", policy.Denom, err)
		}
		if seen[policy.Denom] {
			// Two policies for one currency is not a merge problem, it is an
			// ambiguity: whichever the lookup finds first decides what nets,
			// and the answer would depend on the order somebody typed them in.
			return fmt.Errorf("two denom policies for %s", policy.Denom)
		}
		seen[policy.Denom] = true

		if policy.GrossThreshold.IsNil() {
			return fmt.Errorf("denom policy for %s has no gross_threshold", policy.Denom)
		}
		if policy.GrossThreshold.IsNegative() {
			return fmt.Errorf("denom policy for %s has a negative gross_threshold %s", policy.Denom, policy.GrossThreshold)
		}
	}

	return nil
}

// GrossThresholdFor reports the amount at or above which a single obligation in
// this currency bypasses netting, and whether the currency nets at all.
//
// A currency with no policy does not net. That is returned as an explicit
// false rather than as a zero threshold so the caller cannot confuse
// "governance set the threshold to zero" with "governance has never considered
// this currency" — both settle gross today, but only one of them is a decision.
func (p Params) GrossThresholdFor(denom string) (sdkmath.Int, bool) {
	for _, policy := range p.DenomPolicies {
		if policy.Denom == denom {
			return policy.GrossThreshold, true
		}
	}
	return sdkmath.ZeroInt(), false
}

// NettingEnabled reports whether the end blocker will ever close a window.
//
// Zero disables netting rather than meaning "every block", and the check is
// here as well as in Validate because it is the guard in front of the modulo
// in the end blocker. The upper bound is part of the same question: a window
// length that cannot be compared against a block height is a window that never
// closes, which is indistinguishable from netting being off except that every
// participant's reserve stays locked.
func (p Params) NettingEnabled() bool {
	return p.CycleBlocks > 0 && p.CycleBlocks <= MaxCycleBlocks && p.CycleBlocks <= math.MaxInt64
}
