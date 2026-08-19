package keeper

import (
	"context"

	"yamale/blockchain/x/amm/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ExitPool burns LP shares and returns the proportional share of both
// reserves to the sender.
func (k msgServer) ExitPool(ctx context.Context, msg *types.MsgExitPool) (*types.MsgExitPoolResponse, error) {
	senderBz, err := k.addressCodec.StringToBytes(msg.Sender)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid sender address")
	}

	pool, err := k.Pool.Get(ctx, msg.PoolId)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrPoolNotFound, "pool %d not found", msg.PoolId)
	}

	shares, ok := math.NewIntFromString(msg.Shares)
	if !ok || !shares.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid shares %s", msg.Shares)
	}

	reserveA, _ := math.NewIntFromString(pool.ReserveA)
	reserveB, _ := math.NewIntFromString(pool.ReserveB)
	totalShares, _ := math.NewIntFromString(pool.TotalShares)
	if shares.GT(totalShares) {
		return nil, errorsmod.Wrapf(types.ErrInsufficientShares, "requested %s exceeds total shares %s", shares, totalShares)
	}

	amountA := reserveA.Mul(shares).Quo(totalShares)
	amountB := reserveB.Mul(shares).Quo(totalShares)

	lpCoins := sdk.NewCoins(sdk.NewCoin(types.LPDenom(msg.PoolId), shares))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sdk.AccAddress(senderBz), types.ModuleName, lpCoins); err != nil {
		return nil, errorsmod.Wrap(err, "insufficient LP shares in balance")
	}
	if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, lpCoins); err != nil {
		return nil, err
	}

	payout := sdk.NewCoins(sdk.NewCoin(pool.DenomA, amountA), sdk.NewCoin(pool.DenomB, amountB))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(senderBz), payout); err != nil {
		return nil, err
	}

	pool.ReserveA = reserveA.Sub(amountA).String()
	pool.ReserveB = reserveB.Sub(amountB).String()
	pool.TotalShares = totalShares.Sub(shares).String()
	if err := k.Pool.Set(ctx, msg.PoolId, pool); err != nil {
		return nil, err
	}

	return &types.MsgExitPoolResponse{}, nil
}
