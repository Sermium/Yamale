package keeper

import (
	"context"

	"yamale/blockchain/x/amm/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// JoinPool deposits liquidity proportional to the pool's current reserves.
// amountA drives the deposit; the required amountB is derived from the pool
// ratio, and msg.AmountB is the depositor's declared maximum (a simple
// slippage bound) - only the required amount is ever taken.
func (k msgServer) JoinPool(ctx context.Context, msg *types.MsgJoinPool) (*types.MsgJoinPoolResponse, error) {
	senderBz, err := k.addressCodec.StringToBytes(msg.Sender)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid sender address")
	}

	pool, err := k.Pool.Get(ctx, msg.PoolId)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrPoolNotFound, "pool %d not found", msg.PoolId)
	}

	amountA, ok := math.NewIntFromString(msg.AmountA)
	if !ok || !amountA.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid amountA %s", msg.AmountA)
	}
	maxAmountB, ok := math.NewIntFromString(msg.AmountB)
	if !ok || !maxAmountB.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid amountB %s", msg.AmountB)
	}

	// See the note in Swap: a discarded parse error here is a nil math.Int and
	// a panic at first use, and a zero reserve is a division by zero.
	reserveA, okA := math.NewIntFromString(pool.ReserveA)
	reserveB, okB := math.NewIntFromString(pool.ReserveB)
	totalShares, okS := math.NewIntFromString(pool.TotalShares)
	if !okA || !okB || !okS {
		return nil, errorsmod.Wrapf(types.ErrCorruptPool,
			"pool %d holds reserves %q and %q against %q shares",
			msg.PoolId, pool.ReserveA, pool.ReserveB, pool.TotalShares)
	}
	if !reserveA.IsPositive() || !reserveB.IsPositive() || !totalShares.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrCorruptPool,
			"pool %d has nothing in it, so there is no price to join at", msg.PoolId)
	}

	// requiredB = ceil(reserveB * amountA / reserveA), and
	// sharesMinted = floor(totalShares * amountA / reserveA).
	//
	// The two round in opposite directions on purpose, and both toward the
	// pool. Rounding the required deposit down let a depositor pay a fraction
	// less than their proportional share while still receiving shares, which
	// dilutes the B-per-A ratio the existing providers own — a real transfer of
	// value out of them, repeatable, and free apart from the fee.
	requiredB := ceilDiv(reserveB.Mul(amountA), reserveA)
	if requiredB.GT(maxAmountB) {
		return nil, errorsmod.Wrapf(types.ErrInsufficientDeposit, "depositing %s%s requires %s%s, only %s%s offered",
			amountA, pool.DenomA, requiredB, pool.DenomB, maxAmountB, pool.DenomB)
	}
	sharesMinted := totalShares.Mul(amountA).Quo(reserveA)
	if !sharesMinted.IsPositive() {
		return nil, errorsmod.Wrap(types.ErrInvalidAmount, "deposit too small to mint any LP shares")
	}

	deposit := sdk.NewCoins(sdk.NewCoin(pool.DenomA, amountA), sdk.NewCoin(pool.DenomB, requiredB))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sdk.AccAddress(senderBz), types.ModuleName, deposit); err != nil {
		return nil, err
	}

	pool.ReserveA = reserveA.Add(amountA).String()
	pool.ReserveB = reserveB.Add(requiredB).String()
	pool.TotalShares = totalShares.Add(sharesMinted).String()
	if err := k.Pool.Set(ctx, msg.PoolId, pool); err != nil {
		return nil, err
	}

	lpCoins := sdk.NewCoins(sdk.NewCoin(types.LPDenom(msg.PoolId), sharesMinted))
	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, lpCoins); err != nil {
		return nil, err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(senderBz), lpCoins); err != nil {
		return nil, err
	}

	return &types.MsgJoinPoolResponse{}, nil
}

// ceilDiv divides rounding away from zero, for the operands this module uses
// (both non-negative).
func ceilDiv(numerator, denominator math.Int) math.Int {
	if !denominator.IsPositive() {
		return numerator
	}
	quotient := numerator.Quo(denominator)
	if quotient.Mul(denominator).Equal(numerator) {
		return quotient
	}
	return quotient.AddRaw(1)
}
