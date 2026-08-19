package stablecoin

import (
	"math/rand"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	stablecoinsimulation "yamale/blockchain/x/stablecoin/simulation"
	"yamale/blockchain/x/stablecoin/types"
)

// GenerateGenesisState creates a randomized GenState of the module.
//
// A few currencies start with an approved issuer drawn from the simulation
// accounts. In principle the ProposalMsgs path below could grant those rights
// during the run, but the gov simulation almost never carries a proposal
// through to a passing vote, so relying on it would leave MintCoin, BurnCoin
// and every AMM operation permanently no-op — the AMM needs a second denom in
// circulation before it can build a pool. Seeding here models a chain whose
// issuers were onboarded before the window being simulated.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	stablecoinGenesis := types.GenesisState{
		Params: types.DefaultParams(),
	}

	// Leave the tail of the list unclaimed so RegisterCurrency still has
	// something to apply for.
	const numSeededIssuers = 3
	for i := 0; i < numSeededIssuers && i < len(stablecoinsimulation.SimulatableCurrencies); i++ {
		if len(simState.Accounts) == 0 {
			break
		}
		issuer := simState.Accounts[i%len(simState.Accounts)]
		stablecoinGenesis.ApprovedIssuerMap = append(
			stablecoinGenesis.ApprovedIssuerMap,
			types.ApprovedIssuer{
				Denom:  stablecoinsimulation.SimulatableCurrencies[i].Denom,
				Issuer: issuer.Address.String(),
			},
		)
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(&stablecoinGenesis)
}

// RegisterStoreDecoder registers a decoder.
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns the all the gov module operations with their respective weights.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	operations := make([]simtypes.WeightedOperation, 0)
	const (
		opWeightMsgRegisterCurrency          = "op_weight_msg_register_currency"
		defaultWeightMsgRegisterCurrency int = 100
	)

	var weightMsgRegisterCurrency int
	simState.AppParams.GetOrGenerate(opWeightMsgRegisterCurrency, &weightMsgRegisterCurrency, nil,
		func(_ *rand.Rand) {
			weightMsgRegisterCurrency = defaultWeightMsgRegisterCurrency
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgRegisterCurrency,
		stablecoinsimulation.SimulateMsgRegisterCurrency(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgMintCoin          = "op_weight_msg_mint_coin"
		defaultWeightMsgMintCoin int = 100
	)

	var weightMsgMintCoin int
	simState.AppParams.GetOrGenerate(opWeightMsgMintCoin, &weightMsgMintCoin, nil,
		func(_ *rand.Rand) {
			weightMsgMintCoin = defaultWeightMsgMintCoin
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgMintCoin,
		stablecoinsimulation.SimulateMsgMintCoin(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgBurnCoin          = "op_weight_msg_burn_coin"
		defaultWeightMsgBurnCoin int = 100
	)

	var weightMsgBurnCoin int
	simState.AppParams.GetOrGenerate(opWeightMsgBurnCoin, &weightMsgBurnCoin, nil,
		func(_ *rand.Rand) {
			weightMsgBurnCoin = defaultWeightMsgBurnCoin
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgBurnCoin,
		stablecoinsimulation.SimulateMsgBurnCoin(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))

	return operations
}

// ProposalMsgs returns msgs used for governance proposals for simulations.
//
// MsgApproveIssuer only accepts the gov module account as its signer, so it
// cannot be a weighted operation: the only way to exercise it is as the
// payload of a governance proposal. Registering it here is what lets the
// simulation ever reach an approved issuer, and therefore MintCoin, BurnCoin
// and the AMM pools that need a second denom.
func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	const (
		opWeightMsgApproveIssuer          = "op_weight_msg_approve_issuer"
		defaultWeightMsgApproveIssuer int = 100
	)

	return []simtypes.WeightedProposalMsg{
		simulation.NewWeightedProposalMsg(
			opWeightMsgApproveIssuer,
			defaultWeightMsgApproveIssuer,
			func(r *rand.Rand, ctx sdk.Context, _ []simtypes.Account) sdk.Msg {
				denom, found := randomPendingDenom(ctx, am, r)
				if !found {
					return nil
				}
				return &types.MsgApproveIssuer{
					Authority: authtypes.NewModuleAddress(types.GovModuleName).String(),
					Denom:     denom,
					// Mostly approve, so the simulation builds up a working
					// set of currencies rather than rejecting everything.
					Approve: r.Intn(10) != 0,
				}
			},
		),
	}
}

// randomPendingDenom picks a currency awaiting a governance decision.
func randomPendingDenom(ctx sdk.Context, am AppModule, r *rand.Rand) (string, bool) {
	iter, err := am.keeper.IssuerApplication.Iterate(ctx, new(collections.Range[string]))
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
			pending = append(pending, a.Denom)
		}
	}
	if len(pending) == 0 {
		return "", false
	}
	return pending[r.Intn(len(pending))], true
}
