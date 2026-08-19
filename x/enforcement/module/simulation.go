package enforcement

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"

	"yamale/blockchain/x/enforcement/types"
)

// This module deliberately generates no random operations.
//
// Everything it does freezes an account, and a frozen account makes every other
// module's operations against it fail — a transfer, a swap, a delegation, a
// treasury deposit. The simulator treats a delivered-but-failed message as a
// fault in the chain, so a module that froze random accounts would turn the
// full-app simulation into a generator of false failures, and the first thing
// anyone would do about that is stop trusting the simulation.
//
// What is exercised instead: genesis is generated and imported like every other
// module's, so the import/export and determinism runs cover this module's state.
// The freeze, the vote and the seizure are covered by the keeper tests, against
// a real bank keeper, where a failure points at this module rather than at a
// victim of it.

// GenerateGenesisState creates a randomized GenesisState of the module.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	params := types.DefaultParams()

	// A destination is seeded so that the parameters a simulated chain starts
	// with are ones a seizure could actually run against — an empty destination
	// would make every seizure path unreachable and hide any change that broke
	// it.
	if len(simState.Accounts) > 0 {
		params.RecoveryDestination = simState.Accounts[0].Address.String()
	}

	// The delay schedule and the value cap have no defaults for the same reason
	// the destination has none: both are denominated, and no denomination
	// compiled into this binary is anybody's currency. Seeded here against the
	// simulated chain's own bond denom, because parameters that failed Validate
	// would stop the whole simulation at InitGenesis and the failure would look
	// like a bug in whatever module ran first.
	bondDenom := simState.BondDenom
	if bondDenom == "" {
		bondDenom = sdk.DefaultBondDenom
	}
	params.SeizureDelayTiers = []types.SeizureDelayTier{{
		Threshold:   sdk.NewCoin(bondDenom, math.NewInt(1_000_000_000)),
		DelayBlocks: types.DefaultSeizureDelayBlocks * 7,
	}}
	params.SeizureWindowCap = sdk.NewCoins(sdk.NewCoin(bondDenom, math.NewInt(1_000_000_000_000)))

	genesis := types.DefaultGenesis()
	genesis.Params = params

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(genesis)
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
// authority checks on UpdateParams and ReverseCase are covered by the keeper
// tests, which assert on the rejection rather than hoping to observe one.
func (AppModule) ProposalMsgs(_ module.SimulationState) []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{}
}
