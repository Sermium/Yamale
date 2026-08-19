package simulation

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/builderfee/keeper"
	"yamale/blockchain/x/builderfee/types"
)

// SimulatableMsgTypeURLs are the message types a simulated builder may claim a
// fee share on. They are real message types carried by simulated transactions,
// so an approved builder actually gets paid by the post handler.
var SimulatableMsgTypeURLs = []string{
	"/blockchain.amm.v1.MsgSwap",
	"/blockchain.amm.v1.MsgJoinPool",
	"/blockchain.paymsg.v1.MsgSendPayment",
	"/blockchain.stablecoin.v1.MsgMintCoin",
	"/cosmos.bank.v1beta1.MsgSend",
}

// SimulateMsgRegisterBuilder nominates a random account as the fee-share
// recipient for a message type that has no builder yet.
func SimulateMsgRegisterBuilder(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		payoutAccount, _ := simtypes.RandomAcc(r, accs)

		msg := &types.MsgRegisterBuilder{
			Creator:       simAccount.Address.String(),
			PayoutAddress: payoutAccount.Address.String(),
		}

		// Only a message type with neither a pending application nor an
		// approved builder can be registered.
		available, err := unclaimedMsgTypeURL(ctx, k, r)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read builder state"), nil, err
		}
		if available == "" {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "every simulatable message type already has a builder"), nil, nil
		}
		msg.MsgTypeUrl = available

		txCtx := simulation.OperationInput{
			R:             r,
			App:           app,
			TxGen:         txGen,
			Cdc:           nil,
			Msg:           msg,
			Context:       ctx,
			SimAccount:    simAccount,
			AccountKeeper: ak,
			Bankkeeper:    bk,
			ModuleName:    types.ModuleName,
		}

		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}

// unclaimedMsgTypeURL returns a message type with no builder application or
// approval yet, starting from a random offset so the choice is not biased
// toward the first entry. It returns "" when they are all taken.
func unclaimedMsgTypeURL(ctx sdk.Context, k keeper.Keeper, r *rand.Rand) (string, error) {
	offset := r.Intn(len(SimulatableMsgTypeURLs))
	for i := range SimulatableMsgTypeURLs {
		candidate := SimulatableMsgTypeURLs[(offset+i)%len(SimulatableMsgTypeURLs)]

		pending, err := k.BuilderApplication.Has(ctx, candidate)
		if err != nil {
			return "", err
		}
		approved, err := k.ApprovedBuilder.Has(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !pending && !approved {
			return candidate, nil
		}
	}
	return "", nil
}
