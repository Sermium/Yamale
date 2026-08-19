package simulation

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

// SimulateMsgRegisterCustomer has an approved participant claim an account as
// one it acts for.
//
// This is what makes payments possible at all under the customer rule, so it
// runs often enough to keep a supply of them. It also exercises the case that
// must fail: an account already banking elsewhere cannot be claimed by a second
// participant, which is the impersonation the relationship exists to prevent.
func SimulateMsgRegisterCustomer(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgRegisterCustomer{}

		participants, err := approvedParticipants(ctx, k)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read approved participants"), nil, err
		}
		if len(participants) == 0 {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "no approved participants"), nil, nil
		}

		// Only the participant may claim a customer, so the simulation has to
		// hold that participant's key.
		offset := r.Intn(len(participants))
		var signer simtypes.Account
		var participant string
		for n := range participants {
			candidate := participants[(offset+n)%len(participants)]
			addr, err := sdk.AccAddressFromBech32(candidate)
			if err != nil {
				continue
			}
			if account, ok := simtypes.FindAccount(accs, addr); ok {
				signer, participant = account, candidate
				break
			}
		}
		if participant == "" {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "no approved participant is a simulation account"), nil, nil
		}

		customer, _ := simtypes.RandomAcc(r, accs)
		customerStr := customer.Address.String()

		// An account already banking elsewhere is refused, and the simulator
		// treats a refused delivery as fatal rather than as a no-op.
		if existing, err := k.Customer.Get(ctx, customerStr); err == nil && existing.Participant != participant {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "account already banks with another participant"), nil, nil
		}

		msg.Participant = participant
		msg.Customer = customerStr
		// Occasionally end a relationship, so removal is exercised too.
		msg.Registered = r.Intn(8) != 0

		return simulation.GenAndDeliverTxWithRandFees(simulation.OperationInput{
			R:             r,
			App:           app,
			TxGen:         txGen,
			Cdc:           nil,
			Msg:           msg,
			Context:       ctx,
			SimAccount:    signer,
			AccountKeeper: ak,
			Bankkeeper:    bk,
			ModuleName:    types.ModuleName,
		})
	}
}
