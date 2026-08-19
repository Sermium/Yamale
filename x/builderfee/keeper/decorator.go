package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// FeeShareDecorator splits a governance-configurable share of each
// successful tx's gas fee (already deducted to the fee collector module
// account by the ante handler) to the registered builder for the first
// message type in the tx that has an approved builder. The remainder stays
// with the fee collector for normal validator distribution.
type FeeShareDecorator struct {
	keeper Keeper
}

func NewFeeShareDecorator(k Keeper) FeeShareDecorator {
	return FeeShareDecorator{keeper: k}
}

func (d FeeShareDecorator) PostHandle(ctx sdk.Context, tx sdk.Tx, simulate, success bool, next sdk.PostHandler) (sdk.Context, error) {
	if !success || simulate {
		return next(ctx, tx, simulate, success)
	}

	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		return next(ctx, tx, simulate, success)
	}
	fee := feeTx.GetFee()
	if fee.IsZero() {
		return next(ctx, tx, simulate, success)
	}

	params, err := d.keeper.Params.Get(ctx)
	if err != nil || params.BuilderFeeShareBps == 0 {
		return next(ctx, tx, simulate, success)
	}

	for _, msg := range tx.GetMsgs() {
		approved, err := d.keeper.ApprovedBuilder.Get(ctx, sdk.MsgTypeURL(msg))
		if err != nil {
			continue // no approved builder registered for this message type
		}

		payoutAddr, err := sdk.AccAddressFromBech32(approved.PayoutAddress)
		if err != nil {
			continue
		}

		splitCoins := sdk.NewCoins()
		for _, c := range fee {
			amt := c.Amount.MulRaw(int64(params.BuilderFeeShareBps)).QuoRaw(10000)
			if amt.IsPositive() {
				splitCoins = splitCoins.Add(sdk.NewCoin(c.Denom, amt))
			}
		}
		if !splitCoins.IsZero() {
			// A failed payout must not fail the transaction.
			//
			// This runs after the message has already executed successfully, so
			// returning an error here would undo somebody's payment or trade
			// because a *fee split* could not be paid — and the payout address
			// is set by governance, not by the sender, so the sender has no way
			// to avoid it. A builder whose address later became unpayable, for
			// instance by being a blocked module account, would have taken down
			// every transaction of that message type.
			//
			// The share simply stays with the fee collector, which is where it
			// would have gone had no builder been registered.
			if err := d.keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, authtypes.FeeCollectorName, payoutAddr, splitCoins); err != nil {
				ctx.Logger().Error(
					"builder fee split failed; the share stays with the fee collector",
					"builder", approved.PayoutAddress,
					"msg_type", sdk.MsgTypeURL(msg),
					"amount", splitCoins.String(),
					"err", err,
				)
			}
		}
		break // only the first matching registered message type is paid
	}

	return next(ctx, tx, simulate, success)
}
