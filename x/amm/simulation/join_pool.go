package simulation

import (
	"math/rand"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/amm/keeper"
	"yamale/blockchain/x/amm/types"
)

// SimulateMsgJoinPool adds proportional liquidity to an existing pool.
func SimulateMsgJoinPool(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgJoinPool{Sender: simAccount.Address.String()}

		pool, found, err := randomPool(ctx, k, r)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read pools"), nil, err
		}
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "no pools exist yet"), nil, nil
		}

		reserveA, ok := math.NewIntFromString(pool.ReserveA)
		if !ok || !reserveA.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "pool has no A-side reserve"), nil, nil
		}
		reserveB, ok := math.NewIntFromString(pool.ReserveB)
		if !ok || !reserveB.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "pool has no B-side reserve"), nil, nil
		}

		spendable := bk.SpendableCoins(ctx, simAccount.Address)
		heldA := spendable.AmountOf(pool.DenomA)
		heldB := spendable.AmountOf(pool.DenomB)
		if !heldA.IsPositive() || !heldB.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "sender does not hold both pool denoms"), nil, nil
		}

		// The A-side deposit drives the join; cap it so the derived B-side
		// requirement (reserveB * amountA / reserveA) still fits the balance.
		maxA := heldA.Quo(math.NewInt(2))
		if affordableA := heldB.Quo(math.NewInt(2)).Mul(reserveA).Quo(reserveB); affordableA.LT(maxA) {
			maxA = affordableA
		}
		if !maxA.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "sender cannot cover both sides of a join"), nil, nil
		}

		amountA, err := simtypes.RandPositiveInt(r, maxA)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to generate amountA"), nil, nil
		}

		// The message's B amount is a declared maximum; the handler only takes
		// what the pool ratio requires. Offer the exact requirement, rounded up
		// by one to absorb integer truncation.
		requiredB := reserveB.Mul(amountA).Quo(reserveA).AddRaw(1)
		if requiredB.GT(heldB) {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "sender cannot cover the required B-side deposit"), nil, nil
		}

		msg.PoolId = pool.Id
		msg.AmountA = amountA.String()
		msg.AmountB = requiredB.String()

		spent := sdk.NewCoins(
			sdk.NewCoin(pool.DenomA, amountA),
			sdk.NewCoin(pool.DenomB, requiredB),
		)

		txCtx := simulation.OperationInput{
			R:               r,
			App:             app,
			TxGen:           txGen,
			Cdc:             nil,
			Msg:             msg,
			CoinsSpentInMsg: spent,
			Context:         ctx,
			SimAccount:      simAccount,
			AccountKeeper:   ak,
			Bankkeeper:      bk,
			ModuleName:      types.ModuleName,
		}

		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}

// randomPool returns a randomly chosen existing pool.
func randomPool(ctx sdk.Context, k keeper.Keeper, r *rand.Rand) (types.Pool, bool, error) {
	var pool types.Pool

	iter, err := k.Pool.Iterate(ctx, new(collections.Range[uint64]))
	if err != nil {
		return pool, false, err
	}
	defer iter.Close()

	pools, err := iter.Values()
	if err != nil {
		return pool, false, err
	}
	if len(pools) == 0 {
		return pool, false, nil
	}

	return pools[r.Intn(len(pools))], true, nil
}
