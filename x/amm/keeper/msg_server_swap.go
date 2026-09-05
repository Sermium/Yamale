package keeper

import (
	"context"

	"yamale/blockchain/x/amm/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const bpsDenominator = 10000

// Swap executes a constant-product (x*y=k) trade against a pool. The swap
// fee is retained in the pool (added to the input-side reserve), benefiting
// LPs, following the standard Uniswap-v2 model.
func (k msgServer) Swap(ctx context.Context, msg *types.MsgSwap) (*types.MsgSwapResponse, error) {
	senderBz, err := k.addressCodec.StringToBytes(msg.Sender)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid sender address")
	}

	pool, err := k.Pool.Get(ctx, msg.PoolId)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrPoolNotFound, "pool %d not found", msg.PoolId)
	}

	tokenInAmount, ok := math.NewIntFromString(msg.TokenInAmount)
	if !ok || !tokenInAmount.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid tokenInAmount %s", msg.TokenInAmount)
	}
	minAmountOut, ok := math.NewIntFromString(msg.MinAmountOut)
	if !ok || minAmountOut.IsNegative() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid minAmountOut %s", msg.MinAmountOut)
	}

	// Parsed with the error, not past it. math.NewIntFromString yields a
	// math.Int with a nil inner value on failure, and that panics on first use
	// rather than erroring. The module writes these strings itself, so this is
	// unreachable from a message today — which is exactly why it is worth
	// spending three lines on: it turns any future migration bug from a panic
	// into a refused transaction naming the pool.
	reserveA, okA := math.NewIntFromString(pool.ReserveA)
	reserveB, okB := math.NewIntFromString(pool.ReserveB)
	if !okA || !okB {
		return nil, errorsmod.Wrapf(types.ErrCorruptPool,
			"pool %d holds reserves %q and %q", msg.PoolId, pool.ReserveA, pool.ReserveB)
	}

	var reserveIn, reserveOut math.Int
	var inIsA bool
	switch {
	case msg.TokenInDenom == pool.DenomA && msg.TokenOutDenom == pool.DenomB:
		reserveIn, reserveOut, inIsA = reserveA, reserveB, true
	case msg.TokenInDenom == pool.DenomB && msg.TokenOutDenom == pool.DenomA:
		reserveIn, reserveOut, inIsA = reserveB, reserveA, false
	default:
		return nil, errorsmod.Wrapf(types.ErrDenomNotInPool, "pool %d does not trade %s for %s", msg.PoolId, msg.TokenInDenom, msg.TokenOutDenom)
	}

	feeBps := math.NewInt(bpsDenominator - int64(pool.SwapFeeBps))
	amountInAfterFee := tokenInAmount.Mul(feeBps).Quo(math.NewInt(bpsDenominator))

	// amountOut = reserveOut * amountInAfterFee / (reserveIn + amountInAfterFee)
	//
	// The rounding direction of this single division is what protects the
	// pool. Truncating here rounds the trader's output *down*, so any
	// fractional remainder is left behind as reserve. Computing the
	// algebraically equivalent
	//
	//	reserveOut - (reserveIn * reserveOut) / (reserveIn + amountInAfterFee)
	//
	// instead would truncate the subtrahend and therefore round the output
	// *up*, letting every swap take up to one unit more than the curve
	// allows — enough to push x*y below its previous value and bleed the
	// pool one unit at a time.
	amountOut := reserveOut.Mul(amountInAfterFee).Quo(reserveIn.Add(amountInAfterFee))
	// Truncation is right and it is what protects the pool, but truncating to
	// zero is a different thing from rounding a payout down. sdk.NewCoins drops
	// a zero coin, so the transfer out became a no-op while the input was still
	// taken and the reserve still updated — payment for nothing, and settled
	// rather than refused, on any swap where the caller passed a
	// min_amount_out of zero.
	if !amountOut.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrZeroOutput,
			"%s%s is too small to buy any %s at this pool's price", tokenInAmount, msg.TokenInDenom, msg.TokenOutDenom)
	}
	if amountOut.LT(minAmountOut) {
		return nil, errorsmod.Wrapf(types.ErrSlippage, "swap would return %s%s, less than the minimum %s%s requested", amountOut, msg.TokenOutDenom, minAmountOut, msg.TokenOutDenom)
	}
	// Rounding down already keeps amountOut strictly below reserveOut, so this
	// is a belt-and-braces check rather than a reachable branch.
	if amountOut.GTE(reserveOut) {
		return nil, errorsmod.Wrap(types.ErrInsufficientDeposit, "swap would drain the pool's output reserve")
	}

	tokenIn := sdk.NewCoins(sdk.NewCoin(msg.TokenInDenom, tokenInAmount))
	tokenOut := sdk.NewCoins(sdk.NewCoin(msg.TokenOutDenom, amountOut))

	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sdk.AccAddress(senderBz), types.ModuleName, tokenIn); err != nil {
		return nil, err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(senderBz), tokenOut); err != nil {
		return nil, err
	}

	newReserveIn := reserveIn.Add(tokenInAmount)
	newReserveOut := reserveOut.Sub(amountOut)
	if inIsA {
		pool.ReserveA, pool.ReserveB = newReserveIn.String(), newReserveOut.String()
	} else {
		pool.ReserveB, pool.ReserveA = newReserveIn.String(), newReserveOut.String()
	}
	if err := k.Pool.Set(ctx, msg.PoolId, pool); err != nil {
		return nil, err
	}

	return &types.MsgSwapResponse{}, nil
}
