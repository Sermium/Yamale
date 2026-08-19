package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	"yamale/blockchain/x/treasury/types"
)

// This file holds every mutation of the treasury ledger. Keeping them together
// in one small set of functions is deliberate: the module's central promise is
// that `locked` never exceeds `total` and that committed funds cannot be spent,
// and that promise is only as good as the number of places able to break it.

// getBalance reads a treasury's position in one denom, returning a zeroed entry
// when it has never held that denom.
func (k Keeper) getBalance(ctx context.Context, treasuryID uint64, denom string) (types.TreasuryBalance, error) {
	bal, err := k.Balance.Get(ctx, collections.Join(treasuryID, denom))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.TreasuryBalance{
				TreasuryId: treasuryID,
				Denom:      denom,
				Total:      math.ZeroInt().String(),
				Locked:     math.ZeroInt().String(),
			}, nil
		}
		return types.TreasuryBalance{}, err
	}
	return bal, nil
}

// balanceAmounts returns a balance's total and locked as integers.
func balanceAmounts(bal types.TreasuryBalance) (total, locked math.Int) {
	total, ok := math.NewIntFromString(bal.Total)
	if !ok {
		total = math.ZeroInt()
	}
	locked, ok = math.NewIntFromString(bal.Locked)
	if !ok {
		locked = math.ZeroInt()
	}
	return total, locked
}

// AvailableBalance returns what a treasury may actually spend in a denom:
// everything it holds, less everything already committed to locks.
//
// Every outbound path checks this rather than the raw total. That is the whole
// mechanism behind "a lock cannot be spent by anyone" — including by the admin,
// and including by a proposal that clears its signing threshold.
func (k Keeper) AvailableBalance(ctx context.Context, treasuryID uint64, denom string) (math.Int, error) {
	bal, err := k.getBalance(ctx, treasuryID, denom)
	if err != nil {
		return math.ZeroInt(), err
	}
	total, locked := balanceAmounts(bal)

	available := total.Sub(locked)
	if !available.IsPositive() {
		return math.ZeroInt(), nil
	}
	return available, nil
}

// LockedBalance returns how much of a denom is committed to active locks.
func (k Keeper) LockedBalance(ctx context.Context, treasuryID uint64, denom string) (math.Int, error) {
	bal, err := k.getBalance(ctx, treasuryID, denom)
	if err != nil {
		return math.ZeroInt(), err
	}
	_, locked := balanceAmounts(bal)
	return locked, nil
}

// setBalance writes a position back, deleting it once it is empty so a treasury
// that has fully paid out does not leave dust entries behind.
func (k Keeper) setBalance(ctx context.Context, treasuryID uint64, denom string, total, locked math.Int) error {
	key := collections.Join(treasuryID, denom)

	if total.IsZero() && locked.IsZero() {
		err := k.Balance.Remove(ctx, key)
		if err != nil && !errors.Is(err, collections.ErrNotFound) {
			return err
		}
		return nil
	}

	return k.Balance.Set(ctx, key, types.TreasuryBalance{
		TreasuryId: treasuryID,
		Denom:      denom,
		Total:      total.String(),
		Locked:     locked.String(),
	})
}

// creditBalance adds funds to a treasury, as a deposit does.
func (k Keeper) creditBalance(ctx context.Context, treasuryID uint64, denom string, amount math.Int) error {
	bal, err := k.getBalance(ctx, treasuryID, denom)
	if err != nil {
		return err
	}
	total, locked := balanceAmounts(bal)
	return k.setBalance(ctx, treasuryID, denom, total.Add(amount), locked)
}

// debitAvailable removes funds that are not committed, as a spend does. It
// fails rather than dipping into locked funds.
func (k Keeper) debitAvailable(ctx context.Context, treasuryID uint64, denom string, amount math.Int) error {
	bal, err := k.getBalance(ctx, treasuryID, denom)
	if err != nil {
		return err
	}
	total, locked := balanceAmounts(bal)

	if total.Sub(locked).LT(amount) {
		return types.ErrInsufficientFunds.Wrapf(
			"treasury %d has %s%s available (%s held, %s locked), needs %s",
			treasuryID, total.Sub(locked), denom, total, locked, amount)
	}

	return k.setBalance(ctx, treasuryID, denom, total.Sub(amount), locked)
}

// commit moves funds from available into locked when a lock is created. The
// treasury still holds them; it simply may no longer spend them.
func (k Keeper) commit(ctx context.Context, treasuryID uint64, denom string, amount math.Int) error {
	bal, err := k.getBalance(ctx, treasuryID, denom)
	if err != nil {
		return err
	}
	total, locked := balanceAmounts(bal)

	if total.Sub(locked).LT(amount) {
		return types.ErrInsufficientFunds.Wrapf(
			"treasury %d has %s%s available, cannot commit %s",
			treasuryID, total.Sub(locked), denom, amount)
	}

	return k.setBalance(ctx, treasuryID, denom, total, locked.Add(amount))
}

// releaseToBeneficiary pays committed funds out: they leave both the locked
// figure and the total, because they are leaving the treasury entirely.
func (k Keeper) releaseToBeneficiary(ctx context.Context, treasuryID uint64, denom string, amount math.Int) error {
	bal, err := k.getBalance(ctx, treasuryID, denom)
	if err != nil {
		return err
	}
	total, locked := balanceAmounts(bal)

	if locked.LT(amount) || total.LT(amount) {
		// Unreachable unless the ledger has been corrupted; refusing here keeps
		// a bookkeeping bug from becoming a withdrawal of somebody else's funds.
		return types.ErrInsufficientFunds.Wrapf(
			"treasury %d cannot release %s%s: %s held, %s locked",
			treasuryID, amount, denom, total, locked)
	}

	return k.setBalance(ctx, treasuryID, denom, total.Sub(amount), locked.Sub(amount))
}

// uncommit returns committed funds to available without paying them out, as a
// revocation does for the unvested portion.
func (k Keeper) uncommit(ctx context.Context, treasuryID uint64, denom string, amount math.Int) error {
	bal, err := k.getBalance(ctx, treasuryID, denom)
	if err != nil {
		return err
	}
	total, locked := balanceAmounts(bal)

	if locked.LT(amount) {
		return types.ErrInsufficientFunds.Wrapf(
			"treasury %d cannot uncommit %s%s: only %s locked", treasuryID, amount, denom, locked)
	}

	return k.setBalance(ctx, treasuryID, denom, total, locked.Sub(amount))
}
