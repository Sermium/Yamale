package constitution

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"

	"yamale/blockchain/x/constitution/types"
)

// This module generates no random operations, and one carefully seeded genesis.
//
// # Why the genesis has to be seeded at all
//
// DefaultInvariants() leaves enforcement_recovery_destination empty, and that is
// correct: it is an address, no address compiled into a binary is anybody's
// foundation, and Invariants.Validate() refuses an empty one because a chain
// that can pass a seizure with nowhere to send what it takes is worse than a
// chain that will not start.
//
// The consequence is that this module's DefaultGenesis does not validate, which
// is fine for a real launch — the operator supplies the address — and fatal for
// the simulator, which builds its genesis from module defaults. Without this
// file the whole simulation suite died in InitChain before block one:
//
//	panic: failed to initialize constitution genesis state: constitution genesis
//	is invalid, refusing to start: enforcement_recovery_destination must name the
//	foundation account
//
// That is why the 2026-09-03 audit reported no simulation runs. The runs were
// not skipped, they were impossible, and the failure named this module while
// looking like a fault in whichever module the simulator happened to touch
// first. x/enforcement had already hit exactly this and seeded its own
// destination for the same reason; the constitutional copy of the same field
// was missed.
//
// # Why no operations
//
// The two messages here are an amendment proposal and its ratification. An
// amendment needs 80% of voting power and then waits out a 120,960-block delay,
// so under simulation it is a message that is submitted, never ratified, and
// never enacted — noise rather than coverage. The threshold, the delay and the
// refusal paths are covered by the keeper tests, which assert on the outcome
// instead of hoping the random walk produces one.

// GenerateGenesisState creates a randomized GenesisState of the module.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	genesis := types.DefaultGenesis()

	// The one field with no honest default. Seeded from the simulated chain's
	// own accounts so that the recovery destination is an address that exists
	// on it — a made-up bech32 string would validate here and then fail the
	// first time a seizure tried to pay out to it.
	if len(simState.Accounts) > 0 {
		genesis.Invariants.EnforcementRecoveryDestination = simState.Accounts[0].Address.String()
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(genesis)
}

// RegisterStoreDecoder registers a decoder.
func (AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns none, for the reason at the top of this file.
func (AppModule) WeightedOperations(_ module.SimulationState) []simtypes.WeightedOperation {
	return nil
}

// ProposalMsgs returns none. An amendment that needs four fifths of voting
// power and a nine-day delay is not something a random walk ever completes, and
// a proposal-shaped operation that never enacts tests the submission path only.
func (AppModule) ProposalMsgs(_ module.SimulationState) []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{}
}
