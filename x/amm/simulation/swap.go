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

		// A trade too small to buy a single unit of the output is refused by the
		// handler rather than settled — taking payment and returning nothing was
		// a real defect, and refusing it is the fix. The simulator treats a
		// refused delivery as fatal, so the operation has to decline here rather
		// than submit a transaction that is correctly rejected.
		//
		// The arithmetic mirrors the handler's exactly, including the fee and
		// the truncation direction. Recomputing it approximately would either
		// let a doomed swap through or skip a valid one, and both are ways for
		// this operation to stop covering the path it exists to cover.
		reserveInStr := pool.ReserveA
		if tokenInDenom == pool.DenomB {
			reserveInStr = pool.ReserveB
		}
		reserveIn, ok := math.NewIntFromString(reserveInStr)
		if !ok || !reserveIn.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "pool has no input reserve"), nil, nil
		}
		feeBps := math.NewInt(10_000 - int64(pool.SwapFeeBps))
		amountInAfterFee := amountIn.Mul(feeBps).Quo(math.NewInt(10_000))
		if !amountInAfterFee.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "swap amount is consumed entirely by the fee"), nil, nil
		}
		amountOut := reserveOut.Mul(amountInAfterFee).Quo(reserveIn.Add(amountInAfterFee))
		if !amountOut.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "swap is too small to buy a whole unit at this price"), nil, nil
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
