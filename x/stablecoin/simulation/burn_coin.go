package simulation

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/stablecoin/keeper"
	"yamale/blockchain/x/stablecoin/types"
)

// SimulateMsgBurnCoin destroys part of an approved issuer's own holdings of
// the currency they issue.
func SimulateMsgBurnCoin(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgBurnCoin{}

		denom, issuer, found, err := randomApprovedIssuer(ctx, k, r, accs)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read approved issuers"), nil, err
		}
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "no approved issuer is a simulation account"), nil, nil
		}

		// An issuer can only burn what they themselves hold.
		held := bk.SpendableCoins(ctx, issuer.Address).AmountOf(denom)
		if !held.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "issuer holds none of their own currency"), nil, nil
		}

		amount, err := simtypes.RandPositiveInt(r, held)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to generate a burn amount"), nil, nil
		}

		msg.Issuer = issuer.Address.String()
		msg.Denom = denom
		msg.Amount = amount.String()

		txCtx := simulation.OperationInput{
			R:               r,
			App:             app,
			TxGen:           txGen,
			Cdc:             nil,
			Msg:             msg,
			CoinsSpentInMsg: sdk.NewCoins(sdk.NewCoin(denom, amount)),
			Context:         ctx,
			SimAccount:      issuer,
			AccountKeeper:   ak,
			Bankkeeper:      bk,
			ModuleName:      types.ModuleName,
		}

		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}
