package treasury

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	treasurysimulation "yamale/blockchain/x/treasury/simulation"
	"yamale/blockchain/x/treasury/types"
)

// GenerateGenesisState creates a randomized GenState of the module.
//
// No treasuries are seeded: unlike the gov-gated modules, creating one is
// permissionless, so the simulation reaches a populated state under its own
// steam within the first few blocks. Seeding would only hide whether
// CreateTreasury actually works.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	treasuryGenesis := types.DefaultGenesis()
	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(treasuryGenesis)
}

// RegisterStoreDecoder registers a decoder.
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns all the treasury module operations with their
// respective weights.
//
// The weights are deliberately uneven. Creating treasuries and depositing has
// to outpace spending, or the simulation drains everything it builds and the
// interesting paths — locks reaching their cliff, claims interleaving with
// revocations — never get exercised.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	operations := make([]simtypes.WeightedOperation, 0, 8)

	ops := []struct {
		key       string
		weight    int
		operation simtypes.Operation
	}{
		{"op_weight_msg_create_treasury", 40, treasurysimulation.SimulateMsgCreateTreasury(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
		{"op_weight_msg_treasury_deposit", 100, treasurysimulation.SimulateMsgDeposit(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
		{"op_weight_msg_treasury_spend", 60, treasurysimulation.SimulateMsgSpend(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
		{"op_weight_msg_create_lock", 80, treasurysimulation.SimulateMsgCreateLock(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
		{"op_weight_msg_claim_lock", 80, treasurysimulation.SimulateMsgClaimLock(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
		{"op_weight_msg_revoke_lock", 30, treasurysimulation.SimulateMsgRevokeLock(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
		{"op_weight_msg_treasury_assign_role", 30, treasurysimulation.SimulateMsgAssignRole(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
		{"op_weight_msg_set_spend_policy", 20, treasurysimulation.SimulateMsgSetSpendPolicy(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
	}

	for _, op := range ops {
		weight := op.weight
		simState.AppParams.GetOrGenerate(op.key, &weight, nil, func(_ *rand.Rand) {
			weight = op.weight
		})
		operations = append(operations, simulation.NewWeightedOperation(weight, op.operation))
	}

	return operations
}

// ProposalMsgs returns msgs used for governance proposals for simulations.
//
// The treasury has none: nothing in it is gov-gated. Control comes from a
// treasury's own admin, which is exactly the point of pointing that admin at an
// x/group policy rather than routing treasury decisions through chain
// governance.
func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{}
}
