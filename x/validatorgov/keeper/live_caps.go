package keeper

import (
	"context"
	"errors"
	"fmt"

	constitutiontypes "yamale/blockchain/x/constitution/types"
	"yamale/blockchain/x/validatorgov/types"
)

// OperatorWithinCaps answers, right now, whether a validator's groups are
// inside their concentration ceilings.
//
// # Why a live answer is needed at all
//
// The ceilings are enforced by a sweep at every ConcentrationEpochBlocks
// boundary, and between boundaries a group may exceed its ceiling with no
// consequence. That is fine for the thing the sweep does — a demotion is a
// correction and corrections can be periodic.
//
// It is not fine for enforcement. A single bonded validator can freeze an
// account in one block, and two thirds of bonded power can carry a seizure, and
// neither of those waits for an epoch. So there was a window, as long as one
// epoch, in which a group that had crossed a constitutional ceiling held
// exactly the powers the constitution was written to deny it — and a freeze
// imposed inside that window is not undone by the demotion that follows.
//
// Putting ConcentrationEpochBlocks under constitutional protection stops
// governance lengthening that window deliberately. It does not remove the
// window that periodic sampling inherently produces. This function is how a
// power stops waiting for the sweep: the caller asks at the moment the power is
// exercised, which makes being within caps a PRECONDITION of the power rather
// than a correction after its use.
//
// Raised as finding 3.3 by an independent review on 2026-08-31.
//
// # What it reports
//
// Within, and if not, which ceiling. An operator that holds no seat, or holds
// one with no power, is within: there is no group for it to concentrate and
// refusing it would deny a power to somebody who cannot threaten anything with
// it.
//
// A chain with no constitution reports within, for the same reason
// ConcentrationEndBlocker enforces nothing there: absent ceilings are not
// ceilings of zero, and reading them that way would refuse every enforcement
// action on a chain that has simply not written its constitution yet.
func (k Keeper) OperatorWithinCaps(ctx context.Context, operator string) (bool, string, error) {
	inv, err := k.constitutionKeeper.GetInvariants(ctx)
	if err != nil {
		if isNoInvariants(err) {
			return true, "", nil
		}
		return false, "", err
	}
	caps := types.CapsFrom(inv)

	holders, err := k.activeSeatHolders(ctx)
	if err != nil {
		return false, "", err
	}

	var candidate types.SeatHolder
	var found bool
	others := make([]types.SeatHolder, 0, len(holders))
	for _, holder := range holders {
		if holder.Operator == operator {
			candidate, found = holder, true
			continue
		}
		others = append(others, holder)
	}
	if !found {
		// Not a seat holder: an enforcement authority that is not a validator,
		// or a validator that is jailed or unbonded. It belongs to no
		// concentration group, so there is nothing here to refuse it for.
		return true, "", nil
	}

	// WithinCaps asks the hypothetical — is every group this candidate belongs
	// to inside its ceiling with the candidate counted in — which is exactly
	// the question here, and is why it is reused rather than reimplemented. A
	// second copy of this arithmetic would be a second thing to keep in step
	// with the constitution.
	if types.WithinCaps(candidate, others, caps) {
		return true, "", nil
	}
	return false, breachedCap(candidate, others, caps), nil
}

// breachedCap names the ceiling that refused, so the error can say which.
//
// Recomputed one ceiling at a time rather than returned by WithinCaps, because
// WithinCaps is used on the restoration path where the answer is a yes or a no
// and widening its signature for one caller's error message would make every
// other call site carry a value it discards.
func breachedCap(candidate types.SeatHolder, others []types.SeatHolder, caps types.CapSet) string {
	// One ceiling at a time, by blanking the other two. A ceiling of zero would
	// refuse everything, so the ones being ignored are set to the maximum
	// rather than left at zero — the point is to isolate which ceiling bites,
	// not to invent a stricter test.
	const noCeiling = 10_000
	for _, each := range []struct {
		set   types.CapSet
		label string
	}{
		{types.CapSet{Entity: caps.Entity, BeneficialOwner: noCeiling, Jurisdiction: noCeiling}, "legal entity"},
		{types.CapSet{Entity: noCeiling, BeneficialOwner: caps.BeneficialOwner, Jurisdiction: noCeiling}, "beneficial owner"},
		{types.CapSet{Entity: noCeiling, BeneficialOwner: noCeiling, Jurisdiction: caps.Jurisdiction}, "jurisdiction"},
	} {
		if !types.WithinCaps(candidate, others, each.set) {
			return each.label
		}
	}
	// Unreachable while WithinCaps is the conjunction of the three above, and
	// reported rather than left blank if that ever stops being true.
	return "a concentration ceiling"
}

func isNoInvariants(err error) bool {
	return errors.Is(err, constitutiontypes.ErrNoInvariants)
}

// AssertOperatorWithinCaps is the form a message handler wants: nil, or an
// error saying which ceiling refused.
func (k Keeper) AssertOperatorWithinCaps(ctx context.Context, operator string) error {
	within, breached, err := k.OperatorWithinCaps(ctx, operator)
	if err != nil {
		return err
	}
	if within {
		return nil
	}
	return fmt.Errorf(
		"%s is above the %s concentration ceiling, so it may not exercise an enforcement power "+
			"until the breach clears; the ceiling is a precondition of the power, not a correction "+
			"applied after it", operator, breached)
}
