package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	"yamale/blockchain/x/treasury/types"
)

// Capacity describes how much a treasury may pay out right now, and why.
//
// Three separate things can bind a spend — the available balance, the
// per-transaction cap, and what is left of the period allowance — and a caller
// that only knew one of them would be wrong whenever another is tighter. So
// this reports all of them plus the answer that actually matters,
// MaxSingleSpend.
type Capacity struct {
	// Available is the unlocked balance.
	Available math.Int
	// RemainingThisPeriod is what the period limit still allows, or Available
	// when no period limit is configured.
	RemainingThisPeriod math.Int
	// MaxSingleSpend is the largest amount a single Spend would accept: the
	// smallest of the three constraints.
	MaxSingleSpend math.Int
	// PerTransactionLimit is the configured per-transaction cap, empty when
	// unset.
	PerTransactionLimit string
	// PeriodResetsAt is when the current window rolls over, zero when no period
	// limit is configured.
	PeriodResetsAt int64
	// HasPolicy reports whether any policy applies to this denom.
	HasPolicy bool
}

// SpendCapacityAt computes what a treasury may spend of a denom at a given
// time.
//
// This is the single place that reasons about the spend window. Both the
// SpendCapacity query and the enforcement path in Spend derive from the same
// rules here, so a client can never be shown an allowance the chain would then
// refuse.
func (k Keeper) SpendCapacityAt(ctx context.Context, treasuryID uint64, denom string, now int64) (Capacity, error) {
	available, err := k.AvailableBalance(ctx, treasuryID, denom)
	if err != nil {
		return Capacity{}, err
	}

	capacity := Capacity{
		Available:           available,
		RemainingThisPeriod: available,
		MaxSingleSpend:      available,
	}

	key := collections.Join(treasuryID, denom)
	policy, err := k.SpendPolicy.Get(ctx, key)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return capacity, nil
		}
		return Capacity{}, err
	}
	capacity.HasPolicy = true
	capacity.PerTransactionLimit = policy.PerTransactionLimit

	if perTx, ok := parseLimit(policy.PerTransactionLimit); ok && perTx.LT(capacity.MaxSingleSpend) {
		capacity.MaxSingleSpend = perTx
	}

	periodLimit, hasPeriodLimit := parseLimit(policy.PeriodLimit)
	if !hasPeriodLimit {
		return capacity, nil
	}

	spent, windowStart, err := k.spendWindowAt(ctx, key, policy, now)
	if err != nil {
		return Capacity{}, err
	}

	remaining := periodLimit.Sub(spent)
	if remaining.IsNegative() {
		remaining = math.ZeroInt()
	}
	if remaining.GT(available) {
		remaining = available
	}
	capacity.RemainingThisPeriod = remaining
	if remaining.LT(capacity.MaxSingleSpend) {
		capacity.MaxSingleSpend = remaining
	}
	capacity.PeriodResetsAt = windowStart + int64(policy.PeriodSeconds)

	return capacity, nil
}

// spendWindowAt returns how much has been spent in the window covering now, and
// when that window opened. A window whose period has elapsed reports zero spent
// and reopens at now, which is what makes the limit a rate rather than a
// lifetime cap.
func (k Keeper) spendWindowAt(
	ctx context.Context,
	key collections.Pair[uint64, string],
	policy types.SpendPolicy,
	now int64,
) (spent math.Int, windowStart int64, err error) {
	window, err := k.SpendWindow.Get(ctx, key)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return math.ZeroInt(), now, nil
		}
		return math.ZeroInt(), 0, err
	}

	if policy.PeriodSeconds > 0 && now-window.WindowStart >= int64(policy.PeriodSeconds) {
		return math.ZeroInt(), now, nil
	}

	spent, ok := math.NewIntFromString(window.Spent)
	if !ok {
		spent = math.ZeroInt()
	}
	return spent, window.WindowStart, nil
}
