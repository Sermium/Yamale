package amm

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	ammsimulation "yamale/blockchain/x/amm/simulation"
	"yamale/blockchain/x/amm/types"
)

// GenerateGenesisState creates a randomized GenState of the module.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	accs := make([]string, len(simState.Accounts))
	for i, acc := range simState.Accounts {
		accs[i] = acc.Address.String()
	}
	ammGenesis := types.GenesisState{
		Params: types.DefaultParams(),
	}
	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(&ammGenesis)
}

// RegisterStoreDecoder registers a decoder.
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns the all the gov module operations with their respective weights.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	operations := make([]simtypes.WeightedOperation, 0)
	const (
		opWeightMsgCreatePool          = "op_weight_msg_create_pool"
		defaultWeightMsgCreatePool int = 100
	)

	var weightMsgCreatePool int
	simState.AppParams.GetOrGenerate(opWeightMsgCreatePool, &weightMsgCreatePool, nil,
		func(_ *rand.Rand) {
			weightMsgCreatePool = defaultWeightMsgCreatePool
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgCreatePool,
		ammsimulation.SimulateMsgCreatePool(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgJoinPool          = "op_weight_msg_join_pool"
		defaultWeightMsgJoinPool int = 100
	)

	var weightMsgJoinPool int
	simState.AppParams.GetOrGenerate(opWeightMsgJoinPool, &weightMsgJoinPool, nil,
		func(_ *rand.Rand) {
			weightMsgJoinPool = defaultWeightMsgJoinPool
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgJoinPool,
		ammsimulation.SimulateMsgJoinPool(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgExitPool          = "op_weight_msg_exit_pool"
		defaultWeightMsgExitPool int = 100
	)

	var weightMsgExitPool int
	simState.AppParams.GetOrGenerate(opWeightMsgExitPool, &weightMsgExitPool, nil,
		func(_ *rand.Rand) {
			weightMsgExitPool = defaultWeightMsgExitPool
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgExitPool,
		ammsimulation.SimulateMsgExitPool(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgSwap          = "op_weight_msg_swap"
		defaultWeightMsgSwap int = 100
	)

	var weightMsgSwap int
	simState.AppParams.GetOrGenerate(opWeightMsgSwap, &weightMsgSwap, nil,
		func(_ *rand.Rand) {
			weightMsgSwap = defaultWeightMsgSwap
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgSwap,
		ammsimulation.SimulateMsgSwap(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))

	return operations
}

// ProposalMsgs returns msgs used for governance proposals for simulations.
func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{}
}
