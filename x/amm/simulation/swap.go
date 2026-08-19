package simulation

import (
	"math/rand"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/amm/keeper"
	"yamale/blockchain/x/amm/types"
)

// SimulateMsgSwap trades against a random pool in a random direction, with a
// minimum-output bound loose enough that the trade is expected to clear. The
// point is to exercise the pricing math and the reserve bookkeeping, not the
// slippage rejection path, which the keeper tests cover directly.
func SimulateMsgSwap(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgSwap{Sender: simAccount.Address.String()}

		pool, found, err := randomPool(ctx, k, r)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read pools"), nil, err
		}
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "no pools exist yet"), nil, nil
		}

		tokenInDenom, tokenOutDenom := pool.DenomA, pool.DenomB
		reserveOutStr := pool.ReserveB
		if r.Intn(2) == 0 {
			tokenInDenom, tokenOutDenom = pool.DenomB, pool.DenomA
			reserveOutStr = pool.ReserveA
		}

		reserveOut, ok := math.NewIntFromString(reserveOutStr)
		if !ok || !reserveOut.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "pool has no output reserve"), nil, nil
		}

		held := bk.SpendableCoins(ctx, simAccount.Address).AmountOf(tokenInDenom)
		if !held.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "sender holds none of the input denom"), nil, nil
		}

		// Trade at most half the balance, leaving room for fees.
		amountIn, err := simtypes.RandPositiveInt(r, held.Quo(math.NewInt(2)))
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to generate a swap amount"), nil, nil
		}

		msg.PoolId = pool.Id
		msg.TokenInDenom = tokenInDenom
		msg.TokenInAmount = amountIn.String()
		msg.TokenOutDenom = tokenOutDenom
		// A zero floor: the handler still rejects a trade that would drain the
		// output reserve, so the pool stays protected either way.
		msg.MinAmountOut = "0"

		txCtx := simulation.OperationInput{
			R:               r,
			App:             app,
			TxGen:           txGen,
			Cdc:             nil,
			Msg:             msg,
			CoinsSpentInMsg: sdk.NewCoins(sdk.NewCoin(tokenInDenom, amountIn)),
			Context:         ctx,
			SimAccount:      simAccount,
			AccountKeeper:   ak,
			Bankkeeper:      bk,
			ModuleName:      types.ModuleName,
		}

		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}
