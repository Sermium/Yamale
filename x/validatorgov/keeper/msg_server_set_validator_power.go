package keeper

import (
	"bytes"
	"context"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"yamale/blockchain/x/validatorgov/types"
)

// SetValidatorPower sets how many seats an admitted validator holds.
//
// Admission is already a governance decision on this chain, so weight is one
// too: equal seats is the default a genesis is built with, and this message is
// how governance departs from it where there is a reason to.
//
// It does not check the concentration ceilings, and that omission is the point.
// A power set above a cap is accepted here and trimmed by the epoch check like
// any other breach, for the same reason the ceilings are enforced every epoch
// rather than at admission: a ceiling only tested where power is granted is not
// a ceiling, because most of the ways power concentrates involve nobody
// granting anything. Refusing here as well would have made the ceiling look
// enforced while leaving growth, merger and nationalisation unguarded — and
// would have made the one test that proves the real mechanism impossible to
// write.
//
// Seats move between this module's reserve and the validator by delegating and
// undelegating. That is the only path that changes consensus power without
// minting anything, without a second module reporting validator updates, and
// without writing into x/staking's delegation accounting behind its back.
func (k msgServer) SetValidatorPower(ctx context.Context, msg *types.MsgSetValidatorPower) (*types.MsgSetValidatorPowerResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authority) {
		expected, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expected, msg.Authority)
	}

	// Zero is refused rather than treated as removal. A validator with no seats
	// is one that has been taken out of the set, and taking one out should say
	// so through the path that records why — not arrive as a power update that
	// happens to be empty.
	if msg.Seats == 0 {
		return nil, types.ErrInvalidSeats
	}

	operatorBz, err := k.addressCodec.StringToBytes(msg.Validator)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid validator address")
	}
	if approved, err := k.ApprovedValidator.Has(ctx, msg.Validator); err != nil {
		return nil, err
	} else if !approved {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedValidator, "%s", msg.Validator)
	}

	valAddr := sdk.ValAddress(operatorBz)
	validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrNoValidator, "%s", msg.Validator)
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	seatBond := params.SeatBond()

	target := seatBond.Mul(math.NewIntFromUint64(msg.Seats))

	// What the RESERVE has staked on this validator, not what the validator
	// holds. Seats are stake the module put there and can take back; a third
	// party's delegation is neither.
	//
	// Measuring against validator.Tokens had two failures, and the second is
	// worse than the first. Anyone could delegate any amount and make current
	// exceed what the reserve holds, at which point releaseSeats failed at
	// ValidateUnbondAmount and governance could not lower that validator's
	// power until the stranger chose to unbond — a permanent block on a
	// governance decision, bought for the price of one delegation. And after
	// governance raised a validator to N seats against an inflated current, the
	// stranger undelegating dropped it silently below the power it was granted.
	current, err := k.reserveStake(ctx, valAddr, validator)
	if err != nil {
		return nil, err
	}

	switch {
	case target.GT(current):
		if err := k.fundSeats(ctx, validator, target.Sub(current)); err != nil {
			return nil, err
		}
	case target.LT(current):
		if err := k.releaseSeats(ctx, valAddr, current.Sub(target)); err != nil {
			return nil, err
		}
	}

	return &types.MsgSetValidatorPowerResponse{Seats: msg.Seats}, nil
}

// reserveStake is the tokens the module's own reserve has delegated to a
// validator, which is the only stake a seat decision can move.
//
// Tokens rather than shares, because the seat target is denominated in tokens
// and a validator that has been slashed has fewer tokens per share. No
// delegation is zero, not an error: a validator this module has never funded is
// an ordinary starting point.
func (k Keeper) reserveStake(ctx context.Context, valAddr sdk.ValAddress, validator stakingtypes.Validator) (math.Int, error) {
	reserve := k.authKeeper.GetModuleAddress(types.ModuleName)
	if reserve == nil {
		return math.ZeroInt(), errorsmod.Wrap(types.ErrSeatReserveEmpty, "this chain has no seat reserve account")
	}
	del, err := k.stakingKeeper.GetDelegation(ctx, reserve, valAddr)
	if err != nil {
		return math.ZeroInt(), nil
	}
	return validator.TokensFromShares(del.Shares).TruncateInt(), nil
}

// fundSeats delegates seats out of the module's reserve.
//
// The reserve is an ordinary module account holding the bond denomination, and
// it is funded rather than minted: this module has no Minter permission and
// should never have one, or the body that decides who validates would also be
// able to create the token that decides how much they weigh.
func (k Keeper) fundSeats(ctx context.Context, validator stakingtypes.Validator, amount math.Int) error {
	reserve := k.authKeeper.GetModuleAddress(types.ModuleName)
	if reserve == nil {
		return errorsmod.Wrap(types.ErrSeatReserveEmpty, "this chain has no seat reserve account")
	}

	denom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}

	// Checked before delegating rather than after it fails, so a proposal to
	// raise a validator's power is refused with the reason rather than with an
	// insufficient-funds error from three modules away.
	available := k.bankKeeper.GetBalance(ctx, reserve, denom)
	if available.Amount.LT(amount) {
		return errorsmod.Wrapf(types.ErrSeatReserveEmpty,
			"the reserve holds %s and this needs %s%s", available, amount, denom)
	}

	_, err = k.stakingKeeper.Delegate(ctx, reserve, amount, stakingtypes.Unbonded, validator, true)
	return err
}

// releaseSeats undelegates seats back into the reserve.
//
// The validator's power falls in this block; the tokens come back when the
// unbonding period ends, which is of no consequence because the reserve holds
// them for nothing but seats. Undelegation is used only here, on a governance
// path — the epoch check jails instead, because undelegation is bounded by
// x/staking's limit on concurrent unbonding entries and an end blocker that can
// hit a limit is an end blocker that can halt a chain.
func (k Keeper) releaseSeats(ctx context.Context, valAddr sdk.ValAddress, amount math.Int) error {
	reserve := k.authKeeper.GetModuleAddress(types.ModuleName)
	if reserve == nil {
		return errorsmod.Wrap(types.ErrSeatReserveEmpty, "this chain has no seat reserve account")
	}

	shares, err := k.stakingKeeper.ValidateUnbondAmount(ctx, reserve, valAddr, amount)
	if err != nil {
		return errorsmod.Wrap(err, "the reserve does not hold enough of this validator's stake to lower its power")
	}

	_, _, err = k.stakingKeeper.Undelegate(ctx, reserve, valAddr, shares)
	return err
}
