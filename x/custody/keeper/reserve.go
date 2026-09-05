package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/custody/types"
)

// The reserve figure, and why it is derived rather than written.
//
// ReportReserve used to overwrite Reserves[denom].Held with whatever a single
// attestor said. That is the figure the Solvency query publishes
// unauthenticated, on the deliberate argument that "a solvency figure only
// anybody with a login can check is not accountability" — and it was one
// signature. Meanwhile AttestDeposit required two attestors to agree before
// anything was minted. The two sides of the same ledger carried different
// standards of proof.
//
// What is required here is not exact agreement. A reserve moves: two honest
// attestors reporting hours apart will disagree by construction, and a rule
// demanding equality would deadlock the module rather than secure it. So the
// rule is enough recent reports, and the lowest of them counts — the direction
// that refuses an honest mint rather than permitting an unbacked one.

// recomputeReserve republishes the reserve figure for one asset from the
// reports standing behind it.
//
// Called after every report. Below the threshold the published reserve is
// REMOVED rather than left at its last value: an asset that no longer has
// enough live attestors has no attested reserve, and solvencyOf already treats
// an absent reserve as not solvent. Leaving a stale figure in place would let a
// reserve keep vouching for itself after everybody stopped looking.
func (k Keeper) recomputeReserve(ctx context.Context, denom string) error {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	oldest := height - int64(params.ReserveReportMaxAgeBlocks)

	var (
		count    uint32
		lowest   math.Int
		lowestBy string
		asOf     int64
	)
	rng := collections.NewPrefixedPairRange[string, string](denom)
	err = k.ReserveReports.Walk(ctx, rng, func(_ collections.Pair[string, string], r types.ReserveReport) (bool, error) {
		if r.AsOfHeight < oldest {
			return false, nil
		}
		// An attestor struck off the register stops counting immediately. The
		// report stays on the record — who said what is the point of keeping
		// them individually — but it no longer stands behind a figure.
		if ok, err := k.Attestors.Has(ctx, r.Attestor); err != nil {
			return false, err
		} else if !ok {
			return false, nil
		}
		count++
		if lowest.IsNil() || r.Held.LT(lowest) {
			lowest, lowestBy = r.Held, r.Attestor
		}
		// The figure's age is the age of its weakest input, not its freshest.
		if asOf == 0 || r.AsOfHeight < asOf {
			asOf = r.AsOfHeight
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	if count < params.AttestationThreshold {
		if err := k.Reserves.Remove(ctx, denom); err != nil {
			return err
		}
		return nil
	}
	return k.Reserves.Set(ctx, denom, types.Reserve{
		Denom:      denom,
		Held:       lowest,
		AsOfHeight: asOf,
		Attestor:   lowestBy,
		Attestors:  count,
	})
}

// attestedReserve is the figure the chain will act on, and whether there is one.
func (k Keeper) attestedReserve(ctx context.Context, denom string) (math.Int, bool) {
	r, err := k.Reserves.Get(ctx, denom)
	if err != nil {
		return math.ZeroInt(), false
	}
	return r.Held, true
}
