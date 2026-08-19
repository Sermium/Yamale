package oracle

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	oraclesimulation "yamale/blockchain/x/oracle/simulation"
	"yamale/blockchain/x/oracle/types"
)

// GenerateGenesisState creates a randomized GenState of the module.
//
// No rates are seeded. The simulation's validators can agree them under their
// own steam within a round or two, and seeding would hide the case that
// matters most — a chain that starts with no price at all, where every consumer
// has to distinguish "no feed" from "feed stopped".
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	oracleGenesis := types.DefaultGenesis()
	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(oracleGenesis)
}

// RegisterStoreDecoder registers a decoder.
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns all the oracle module operations with their
// respective weights.
//
// Voting dominates because that is what the module actually does every minute
// of its life; delegation is rare because in production it happens when a key
// is rotated, not continuously.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	operations := make([]simtypes.WeightedOperation, 0, 3)

	ops := []struct {
		key       string
		weight    int
		operation simtypes.Operation
	}{
		{"op_weight_msg_submit_exchange_rates", 100, oraclesimulation.SimulateMsgSubmitExchangeRates(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
		{"op_weight_msg_delegate_feeder", 10, oraclesimulation.SimulateMsgDelegateFeeder(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
		{"op_weight_msg_apply_appraiser", 20, oraclesimulation.SimulateMsgApplyAppraiser(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig)},
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
// Admitting a valuer is gov-gated, but it is not simulated through a proposal:
// gov proposals essentially never pass under simulation, so the operation would
// contribute nothing but noise. Appraisal submission is likewise absent — it
// requires an NFT to value, and no simulated message can mint one, so seeding an
// approved appraiser would only produce a valuer with nothing to value.
func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{}
}
