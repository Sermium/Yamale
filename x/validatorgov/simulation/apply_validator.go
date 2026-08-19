package simulation

import (
	"errors"
	"math/rand"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/validatorgov/keeper"
	"yamale/blockchain/x/validatorgov/types"
)

// SimulateMsgApplyValidator nominates a random account as a validator
// candidate. Accounts with an application already awaiting a decision are
// skipped, so the operation produces a steady supply of fresh applications for
// the gov proposal simulator (see the module's ProposalMsgs) rather than
// repeatedly overwriting the same pending one.
func SimulateMsgApplyValidator(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		candidate := simAccount.Address.String()

		// Owners and jurisdictions are drawn from short pools rather than
		// randomised freely, so that simulated applicants actually collide into
		// the groups the concentration ceilings are computed over. Every
		// applicant declaring a unique owner would exercise the registry and
		// never once exercise a cap.
		owner := simOwners[r.Intn(len(simOwners))]

		msg := &types.MsgApplyValidator{
			Creator:           candidate,
			Moniker:           simtypes.RandStringOfLength(r, 10),
			Description:       simtypes.RandStringOfLength(r, 20),
			LegalEntityId:     simtypes.RandStringOfLength(r, 12),
			BeneficialOwnerId: owner,
			Jurisdiction:      simJurisdictions[r.Intn(len(simJurisdictions))],
		}

		application, err := k.ValidatorApplication.Get(ctx, candidate)
		switch {
		case err == nil && application.Status == types.StatusPending:
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "candidate already has a pending application"), nil, nil
		case err != nil && !errors.Is(err, collections.ErrNotFound):
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read validator applications"), nil, err
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

// simOwners and simJurisdictions are the pools simulated applicants declare
// from. Kept small on purpose: with four owners and three countries a run of
// any length puts several validators behind one owner, which is the state a
// concentration ceiling exists for and the state a uniformly random declaration
// would almost never produce.
var (
	simOwners        = []string{"OWNER-A", "OWNER-B", "OWNER-C", "OWNER-D"}
	simJurisdictions = []string{"CH", "ZA", "SG"}
)
