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

// SimulateMsgConfirmParticipant has an account answer a participant's claim on
// it.
//
// This is the half of the relationship the account owns. A registration is one
// institution asserting something about somebody else's address, and it buys
// nothing until the account signs: assertInstructedBy refuses a payment naming
// an unconfirmed claim, so without this operation the simulated chain
// accumulates claims that no payment can ever use and the payment path stops
// being exercised at all.
//
// It refuses sometimes as well as confirming, because leaving is the escape the
// confirmation exists to provide — an account claimed without being asked has
// to be able to get out, and a simulation that only ever says yes never touches
// that path.
func SimulateMsgConfirmParticipant(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgConfirmParticipant{}

		// Only an account with a claim against it has anything to answer, and
		// the simulation has to hold that account's key to sign for it.
		type claim struct {
			account     simtypes.Account
			participant string
		}
		pending := make([]claim, 0, 8)
		if err := k.Customer.Walk(ctx, nil, func(_ string, customer types.Customer) (bool, error) {
			addr, err := sdk.AccAddressFromBech32(customer.Customer)
			if err != nil {
				return false, nil
			}
			if account, ok := simtypes.FindAccount(accs, addr); ok {
				pending = append(pending, claim{account: account, participant: customer.Participant})
			}
			return false, nil
		}); err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read customers"), nil, err
		}
		if len(pending) == 0 {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "no claim to answer"), nil, nil
		}

		chosen := pending[r.Intn(len(pending))]
		msg.Customer = chosen.account.Address.String()
		// Named, so a confirmation cannot be replayed against a claim the
		// account never read — and so this operation exercises that check.
		msg.Participant = chosen.participant
		// Mostly yes. A chain where accounts refuse more often than they agree
		// would starve the payment path of debtors, which is the thing this
		// operation exists to feed.
		msg.Confirm = r.Intn(6) != 0

		return simulation.GenAndDeliverTxWithRandFees(simulation.OperationInput{
			R:             r,
			App:           app,
			TxGen:         txGen,
			Cdc:           nil,
			Msg:           msg,
			Context:       ctx,
			SimAccount:    chosen.account,
			AccountKeeper: ak,
			Bankkeeper:    bk,
			ModuleName:    types.ModuleName,
		})
	}
}
