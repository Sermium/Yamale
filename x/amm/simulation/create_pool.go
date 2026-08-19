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

// minPoolReserve is the smallest deposit worth simulating: below this the
// initial sqrt(a*b) share calculation truncates to zero and the message is
// rejected, which would make the operation a no-op every time.
const minPoolReserve = 1_000_000

// SimulateMsgCreatePool bootstraps a constant-product pool from two denoms the
// creator actually holds. Until stablecoin issuers start minting, accounts
// only hold the bond denom and this operation has nothing to pair.
func SimulateMsgCreatePool(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgCreatePool{Creator: simAccount.Address.String()}

		// Pool denoms must be ones the creator can actually deposit.
		spendable := bk.SpendableCoins(ctx, simAccount.Address)
		fundedDenoms := denomsAtLeast(spendable, math.NewInt(minPoolReserve))
		if len(fundedDenoms) < 2 {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "creator does not hold two fundable denoms"), nil, nil
		}

		i := r.Intn(len(fundedDenoms))
		j := r.Intn(len(fundedDenoms) - 1)
		if j >= i {
			j++
		}
		denomA, denomB := fundedDenoms[i], fundedDenoms[j]

		// Deposit at most half of each side, leaving room for fees.
		amountA, err := randDepositAmount(r, spendable.AmountOf(denomA))
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to generate amountA"), nil, nil
		}
		amountB, err := randDepositAmount(r, spendable.AmountOf(denomB))
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to generate amountB"), nil, nil
		}

		msg.DenomA = denomA
		msg.AmountA = amountA.String()
		msg.DenomB = denomB
		msg.AmountB = amountB.String()
		// Fees between 0 and 1%, the range real AMMs use.
		msg.SwapFeeBps = uint64(r.Intn(101))

		deposit := sdk.NewCoins(sdk.NewCoin(denomA, amountA), sdk.NewCoin(denomB, amountB))

		txCtx := simulation.OperationInput{
			R:               r,
			App:             app,
			TxGen:           txGen,
			Cdc:             nil,
			Msg:             msg,
			CoinsSpentInMsg: deposit,
			Context:         ctx,
			SimAccount:      simAccount,
			AccountKeeper:   ak,
			Bankkeeper:      bk,
			ModuleName:      types.ModuleName,
		}

		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}

// denomsAtLeast returns the denoms in coins whose amount reaches minimum,
// excluding LP share denoms, which are not meaningful pool reserves.
func denomsAtLeast(coins sdk.Coins, minimum math.Int) []string {
	denoms := make([]string, 0, len(coins))
	for _, c := range coins {
		if c.Amount.LT(minimum) || types.IsLPDenom(c.Denom) {
			continue
		}
		denoms = append(denoms, c.Denom)
	}
	return denoms
}

// randDepositAmount returns a random amount up to half of held, so the account
// keeps a balance for transaction fees.
func randDepositAmount(r *rand.Rand, held math.Int) (math.Int, error) {
	return simtypes.RandPositiveInt(r, held.Quo(math.NewInt(2)))
}
