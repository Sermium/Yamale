package builderfee

import (
	"math/rand"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	builderfeesimulation "yamale/blockchain/x/builderfee/simulation"
	"yamale/blockchain/x/builderfee/types"
)

// GenerateGenesisState creates a randomized GenState of the module.
//
// Two message types start with an approved builder so the post-handler fee
// split runs against real transactions during the simulation; the gov
// simulation cannot be relied on to approve any (see ProposalMsgs below).
// The remaining simulatable message types are left unclaimed so
// RegisterBuilder still has something to apply for.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	builderfeeGenesis := types.GenesisState{
		Params: types.DefaultParams(),
	}

	const numSeededBuilders = 2
	for i := 0; i < numSeededBuilders && i < len(builderfeesimulation.SimulatableMsgTypeURLs); i++ {
		if len(simState.Accounts) == 0 {
			break
		}
		payout := simState.Accounts[i%len(simState.Accounts)]
		builderfeeGenesis.ApprovedBuilderMap = append(
			builderfeeGenesis.ApprovedBuilderMap,
			types.ApprovedBuilder{
				MsgTypeUrl:    builderfeesimulation.SimulatableMsgTypeURLs[i],
				PayoutAddress: payout.Address.String(),
			},
		)
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(&builderfeeGenesis)
}

// RegisterStoreDecoder registers a decoder.
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns the all the gov module operations with their respective weights.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	operations := make([]simtypes.WeightedOperation, 0)
	const (
		opWeightMsgRegisterBuilder          = "op_weight_msg_register_builder"
		defaultWeightMsgRegisterBuilder int = 100
	)

	var weightMsgRegisterBuilder int
	simState.AppParams.GetOrGenerate(opWeightMsgRegisterBuilder, &weightMsgRegisterBuilder, nil,
		func(_ *rand.Rand) {
			weightMsgRegisterBuilder = defaultWeightMsgRegisterBuilder
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgRegisterBuilder,
		builderfeesimulation.SimulateMsgRegisterBuilder(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))

	return operations
}

// ProposalMsgs returns msgs used for governance proposals for simulations.
//
// MsgApproveBuilder only accepts the gov module account as its signer, so it
// cannot be a weighted operation. Registering it here is what activates the
// post-handler fee split during a simulation run.
func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	const (
		opWeightMsgApproveBuilder          = "op_weight_msg_approve_builder"
		defaultWeightMsgApproveBuilder int = 100
	)

	return []simtypes.WeightedProposalMsg{
		simulation.NewWeightedProposalMsg(
			opWeightMsgApproveBuilder,
			defaultWeightMsgApproveBuilder,
			func(r *rand.Rand, ctx sdk.Context, _ []simtypes.Account) sdk.Msg {
				msgTypeURL, found := randomPendingBuilder(ctx, am, r)
				if !found {
					return nil
				}
				return &types.MsgApproveBuilder{
					Authority:  authtypes.NewModuleAddress(types.GovModuleName).String(),
					MsgTypeUrl: msgTypeURL,
					// Mostly approve, so the fee split is actually exercised.
					Approve: r.Intn(10) != 0,
				}
			},
		),
	}
}

// randomPendingBuilder picks an application awaiting a governance decision.
func randomPendingBuilder(ctx sdk.Context, am AppModule, r *rand.Rand) (string, bool) {
	iter, err := am.keeper.BuilderApplication.Iterate(ctx, new(collections.Range[string]))
	if err != nil {
		return "", false
	}
	defer iter.Close()

	applications, err := iter.Values()
	if err != nil {
		return "", false
	}

	pending := make([]string, 0, len(applications))
	for _, a := range applications {
		if a.Status == types.StatusPending {
			pending = append(pending, a.MsgTypeUrl)
		}
	}
	if len(pending) == 0 {
		return "", false
	}
	return pending[r.Intn(len(pending))], true
}
