package simulation

import (
	"fmt"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

// SimulateMsgApplyParticipant nominates a random account as a payment service
// provider. Addresses that already have an application or an approval are
// skipped, since ApplyParticipant rejects duplicates outright.
func SimulateMsgApplyParticipant(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		applicant := simAccount.Address.String()

		msg := &types.MsgApplyParticipant{
			Creator: applicant,
			// An ISPB-equivalent participant code: eight digits.
			Code: fmt.Sprintf("%08d", r.Intn(100000000)),
			Name: simtypes.RandStringOfLength(r, 12),
		}

		pending, err := k.ParticipantApplication.Has(ctx, applicant)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read participant applications"), nil, err
		}
		approved, err := k.ApprovedParticipant.Has(ctx, applicant)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read approved participants"), nil, err
		}
		if pending || approved {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "account has already applied"), nil, nil
		}

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
