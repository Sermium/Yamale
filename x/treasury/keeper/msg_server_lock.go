package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/treasury/types"
)

// CreateLock commits treasury funds to a beneficiary on a schedule.
//
// Nothing is transferred here. The amount moves from the treasury's available
// balance into its locked balance, where no spend can reach it. The beneficiary
// pulls what has vested by claiming, so the chain never has to run a scheduler
// and a beneficiary who never claims costs nothing.
func (k msgServer) CreateLock(ctx context.Context, msg *types.MsgCreateLock) (*types.MsgCreateLockResponse, error) {
	beneficiaryBz, err := k.addressCodec.StringToBytes(msg.Beneficiary)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid beneficiary address")
	}

	// A beneficiary that cannot receive funds is checked here rather than at
	// claim time, because by then it is too late: both ClaimLock and RevokeLock
	// pay out through the bank, so a lock committed to a blocked address can
	// never be claimed *or* cancelled. The funds would sit in the locked
	// balance permanently, unreachable by the beneficiary, the admin and
	// governance alike.
	if k.bankKeeper.BlockedAddr(sdk.AccAddress(beneficiaryBz)) {
		return nil, types.ErrDestinationDenied.Wrapf(
			"%s cannot receive funds, so a lock to it could never be claimed or revoked", msg.Beneficiary)
	}

	if err := sdk.ValidateDenom(msg.Denom); err != nil {
		return nil, errorsmod.Wrap(err, "invalid denom")
	}

	treasury, err := k.getTreasury(ctx, msg.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := requireNotPaused(treasury); err != nil {
		return nil, err
	}
	if err := k.requireAdmin(ctx, treasury, msg.Admin); err != nil {
		return nil, err
	}

	amount, ok := math.NewIntFromString(msg.Amount)
	if !ok || !amount.IsPositive() {
		return nil, types.ErrInvalidAmount.Wrapf("lock amount %q", msg.Amount)
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateSchedule(msg.LockType, msg.StartTime, msg.CliffTime, msg.EndTime, msg.ReleaseIntervals, params.MinLockSeconds); err != nil {
		return nil, err
	}

	activeLocks, err := k.activeLockCount(ctx, treasury.Id)
	if err != nil {
		return nil, err
	}
	if activeLocks >= params.MaxLocksPerTreasury {
		return nil, types.ErrLimitReached.Wrapf(
			"treasury %d already holds the maximum of %d active locks", treasury.Id, params.MaxLocksPerTreasury)
	}

	// Commit before writing the lock: if the treasury cannot cover it, no lock
	// should exist at all.
	if err := k.commit(ctx, treasury.Id, msg.Denom, amount); err != nil {
		return nil, err
	}

	id, err := k.LockSeq.Next(ctx)
	if err != nil {
		return nil, err
	}

	lock := types.Lock{
		Id:               id,
		TreasuryId:       treasury.Id,
		Beneficiary:      msg.Beneficiary,
		Denom:            msg.Denom,
		TotalAmount:      amount.String(),
		ReleasedAmount:   math.ZeroInt().String(),
		StartTime:        msg.StartTime,
		CliffTime:        msg.CliffTime,
		EndTime:          msg.EndTime,
		ReleaseIntervals: msg.ReleaseIntervals,
		LockType:         msg.LockType,
		Revocable:        msg.Revocable,
		Active:           true,
		CreatedAtHeight:  uint64(sdk.UnwrapSDKContext(ctx).BlockHeight()),
	}

	if err := k.Lock.Set(ctx, id, lock); err != nil {
		return nil, err
	}
	if err := k.LockByTreasury.Set(ctx, collections.Join(treasury.Id, id)); err != nil {
		return nil, err
	}
	if err := k.LockByBeneficiary.Set(ctx, collections.Join(msg.Beneficiary, id)); err != nil {
		return nil, err
	}
	if err := k.ActiveLockCount.Set(ctx, treasury.Id, activeLocks+1); err != nil {
		return nil, err
	}

	return &types.MsgCreateLockResponse{Id: id}, nil
}

// ClaimLock pays the beneficiary whatever has vested but not yet been taken.
func (k msgServer) ClaimLock(ctx context.Context, msg *types.MsgClaimLock) (*types.MsgClaimLockResponse, error) {
	beneficiaryBz, err := k.addressCodec.StringToBytes(msg.Beneficiary)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid beneficiary address")
	}

	lock, err := k.getLock(ctx, msg.LockId)
	if err != nil {
		return nil, err
	}
	// Only the named beneficiary may claim. Anyone else asking is either a
	// mistake or an attempt, and neither should move funds.
	if lock.Beneficiary != msg.Beneficiary {
		return nil, types.ErrUnauthorized.Wrapf("lock %d is payable to %s", lock.Id, lock.Beneficiary)
	}
	if !lock.Active {
		return nil, types.ErrLockInactive.Wrapf("lock %d", lock.Id)
	}

	treasury, err := k.getTreasury(ctx, lock.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := requireNotPaused(treasury); err != nil {
		return nil, err
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	claimable := types.ClaimableAmount(lock, now)
	if !claimable.IsPositive() {
		return nil, types.ErrNothingToClaim.Wrapf("lock %d has nothing available at this time", lock.Id)
	}

	if err := k.releaseToBeneficiary(ctx, lock.TreasuryId, lock.Denom, claimable); err != nil {
		return nil, err
	}

	payout := sdk.NewCoins(sdk.NewCoin(lock.Denom, claimable))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(beneficiaryBz), payout); err != nil {
		return nil, err
	}

	released, _ := math.NewIntFromString(lock.ReleasedAmount)
	lock.ReleasedAmount = released.Add(claimable).String()

	// A fully-paid lock is retired but kept, so the disbursement history stays
	// auditable rather than vanishing on the final claim.
	if types.RemainingAmount(lock).IsZero() {
		lock.Active = false
		if err := k.releaseActiveLock(ctx, lock.TreasuryId); err != nil {
			return nil, err
		}
	}
	if err := k.Lock.Set(ctx, lock.Id, lock); err != nil {
		return nil, err
	}

	return &types.MsgClaimLockResponse{Released: claimable.String()}, nil
}

// RevokeLock cancels a revocable lock.
//
// Whatever has already vested is paid to the beneficiary; only the unvested
// remainder returns to the treasury. Vested funds are never clawed back — a
// commitment that can be withdrawn retroactively is not a commitment, and a
// beneficiary who has not got round to claiming should not be punished for it.
func (k msgServer) RevokeLock(ctx context.Context, msg *types.MsgRevokeLock) (*types.MsgRevokeLockResponse, error) {
	lock, err := k.getLock(ctx, msg.LockId)
	if err != nil {
		return nil, err
	}
	if !lock.Active {
		return nil, types.ErrLockInactive.Wrapf("lock %d", lock.Id)
	}
	if !lock.Revocable {
		return nil, types.ErrNotRevocable.Wrapf("lock %d was created as irrevocable", lock.Id)
	}

	treasury, err := k.getTreasury(ctx, lock.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := requireNotPaused(treasury); err != nil {
		return nil, err
	}
	if err := k.requireAdmin(ctx, treasury, msg.Admin); err != nil {
		return nil, err
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	vestedUnclaimed := types.ClaimableAmount(lock, now)
	remaining := types.RemainingAmount(lock)
	unvested := remaining.Sub(vestedUnclaimed)

	if vestedUnclaimed.IsPositive() {
		if err := k.releaseToBeneficiary(ctx, lock.TreasuryId, lock.Denom, vestedUnclaimed); err != nil {
			return nil, err
		}
		beneficiaryBz, err := k.addressCodec.StringToBytes(lock.Beneficiary)
		if err != nil {
			return nil, errorsmod.Wrap(err, "invalid beneficiary address")
		}
		payout := sdk.NewCoins(sdk.NewCoin(lock.Denom, vestedUnclaimed))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(beneficiaryBz), payout); err != nil {
			return nil, err
		}
	}

	if unvested.IsPositive() {
		if err := k.uncommit(ctx, lock.TreasuryId, lock.Denom, unvested); err != nil {
			return nil, err
		}
	}

	released, _ := math.NewIntFromString(lock.ReleasedAmount)
	lock.ReleasedAmount = released.Add(vestedUnclaimed).String()
	lock.Active = false
	if err := k.releaseActiveLock(ctx, lock.TreasuryId); err != nil {
		return nil, err
	}
	if err := k.Lock.Set(ctx, lock.Id, lock); err != nil {
		return nil, err
	}

	return &types.MsgRevokeLockResponse{
		Returned:            unvested.String(),
		VestedToBeneficiary: vestedUnclaimed.String(),
	}, nil
}

// activeLockCount reads how many of a treasury's locks are still live.
func (k Keeper) activeLockCount(ctx context.Context, treasuryID uint64) (uint64, error) {
	count, err := k.ActiveLockCount.Get(ctx, treasuryID)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

// releaseActiveLock records that one of a treasury's locks has retired, freeing
// a slot under the cap. It never underflows: a count that has somehow reached
// zero stays there rather than wrapping to the maximum, which would silently
// disable the cap entirely.
//
// Reaching zero removes the entry rather than storing a zero. A missing key and
// a stored zero mean the same thing to activeLockCount, but they are different
// bytes on disk — and InitGenesis, which rebuilds this from the locks, writes
// nothing for a treasury with none. Storing the zero here made a chain's state
// differ from the same chain exported and re-imported, which the import/export
// simulation catches as 22 mismatched key/value pairs.
func (k Keeper) releaseActiveLock(ctx context.Context, treasuryID uint64) error {
	count, err := k.activeLockCount(ctx, treasuryID)
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	if count == 1 {
		return k.ActiveLockCount.Remove(ctx, treasuryID)
	}
	return k.ActiveLockCount.Set(ctx, treasuryID, count-1)
}
