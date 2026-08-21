package netting

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"

	"yamale/blockchain/x/netting/types"
)

// This module deliberately generates no random operations, and the reason is
// specific rather than a copy of the neighbouring modules' reasoning.
//
// Every message here is gated on the sender being an approved participant, and
// approval comes from x/paymsg through a governance vote that simulations
// essentially never carry to a passing tally. A random account submitting an
// obligation is refused, every time, and the simulator reads a
// delivered-but-failed message as a fault in the chain — so the operations
// would contribute a stream of false failures and no coverage at all.
//
// Seeding approvals at genesis, which is how this repository solves that
// elsewhere, would not help either: the interesting behaviour is a *window* of
// obligations between institutions that have prefunded reserves, and a
// simulation that submitted them at random amounts between random accounts
// would spend its time hitting the net debit cap rather than exercising the
// netting.
//
// What is exercised instead: genesis is generated and imported like every other
// module's, so the import/export and determinism runs cover this module's state
// — including the derived locked figures, which are the part an import can get
// wrong. The netting itself is covered by property tests in the keeper package,
// against a real bank keeper, over randomised streams of obligations with fixed
// seeds. That is the same coverage a simulation would give, aimed at this
// module rather than at whichever module a refused message landed in.

// GenerateGenesisState creates a randomized GenesisState of the module.
//
// Netting stays switched off, which is also the chain's default. A simulated
// chain with netting on and no approved participants would open and close empty
// windows for the whole run, which looks like coverage and is not.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(types.DefaultGenesis())
}

// RegisterStoreDecoder registers a decoder.
func (AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns none, for the reason at the top of this file.
func (AppModule) WeightedOperations(_ module.SimulationState) []simtypes.WeightedOperation {
	return nil
}

// ProposalMsgs returns none, for the same reason the rest of this chain's
// modules do: governance proposals essentially never pass under simulation, so
// a proposal-shaped operation contributes noise rather than coverage. The
// authority check on UpdateParams is covered by a keeper test, which asserts on
// the rejection rather than hoping to observe one.
func (AppModule) ProposalMsgs(_ module.SimulationState) []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{}
}
