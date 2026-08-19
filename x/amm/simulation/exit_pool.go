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

// SimulateMsgExitPool redeems part of a liquidity provider's shares. It runs
// against a pool the sender actually holds shares in, rather than a random
// one, so the operation does real work instead of bouncing off the balance
// check.
func SimulateMsgExitPool(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgExitPool{Sender: simAccount.Address.String()}

		pool, shares, found, err := randomHeldPool(ctx, k, bk, r, simAccount.Address)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read pools"), nil, err
		}
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "sender holds no LP shares"), nil, nil
		}

		redeemed, err := simtypes.RandPositiveInt(r, shares)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to generate a share amount"), nil, nil
		}

		msg.PoolId = pool.Id
		msg.Shares = redeemed.String()

		txCtx := simulation.OperationInput{
			R:               r,
			App:             app,
			TxGen:           txGen,
			Cdc:             nil,
			Msg:             msg,
			CoinsSpentInMsg: sdk.NewCoins(sdk.NewCoin(types.LPDenom(pool.Id), redeemed)),
			Context:         ctx,
			SimAccount:      simAccount,
			AccountKeeper:   ak,
			Bankkeeper:      bk,
			ModuleName:      types.ModuleName,
		}

		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}

// randomHeldPool picks a pool that addr holds LP shares in, returning the pool
// and the share balance.
func randomHeldPool(
	ctx sdk.Context,
	k keeper.Keeper,
	bk types.BankKeeper,
	r *rand.Rand,
	addr sdk.AccAddress,
) (types.Pool, math.Int, bool, error) {
	var pool types.Pool

	spendable := bk.SpendableCoins(ctx, addr)

	held := make([]types.Pool, 0)
	balances := make([]math.Int, 0)
	for _, c := range spendable {
		if !types.IsLPDenom(c.Denom) || !c.Amount.IsPositive() {
			continue
		}
		id, ok := types.PoolIDFromLPDenom(c.Denom)
		if !ok {
			continue
		}
		p, err := k.Pool.Get(ctx, id)
		if err != nil {
			continue // the pool went away; the shares are unredeemable
		}
		held = append(held, p)
		balances = append(balances, c.Amount)
	}

	if len(held) == 0 {
		return pool, math.ZeroInt(), false, nil
	}

	i := r.Intn(len(held))
	return held[i], balances[i], true, nil
}
