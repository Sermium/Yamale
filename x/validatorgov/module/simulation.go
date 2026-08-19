package validatorgov

import (
	"math/rand"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	validatorgovsimulation "yamale/blockchain/x/validatorgov/simulation"
	"yamale/blockchain/x/validatorgov/types"
)

// GenerateGenesisState creates a randomized GenState of the module.
//
// Every simulation account starts on the approved allowlist. The stock x/staking
// simulation submits MsgCreateValidator from arbitrary accounts, and this
// chain's ante gate rejects any that governance has not approved — which the
// simulator reports as an undeliverable transaction and treats as a fatal
// error. Seeding the allowlist models a chain whose operators were onboarded
// through the genesis ceremony, so staking behaves normally under simulation.
// The gate itself is covered directly by x/validatorgov/ante's unit tests.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	approved := make([]types.ApprovedValidator, len(simState.Accounts))
	for i, acc := range simState.Accounts {
		approved[i] = types.ApprovedValidator{
			Candidate: acc.Address.String(),
			Approved:  "true",
		}
	}

	// Built from DefaultGenesis rather than from a bare struct, so the rotation
	// count comes along. A genesis that left it at zero would be corrected on
	// import, which is exactly the kind of silent correction the import/export
	// simulation exists to catch.
	validatorgovGenesis := types.DefaultGenesis()
	validatorgovGenesis.ApprovedValidatorMap = approved
	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(validatorgovGenesis)
}

// RegisterStoreDecoder registers a decoder.
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns the all the gov module operations with their respective weights.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	operations := make([]simtypes.WeightedOperation, 0)
	const (
		opWeightMsgApplyValidator          = "op_weight_msg_apply_validator"
		defaultWeightMsgApplyValidator int = 100
	)

	var weightMsgApplyValidator int
	simState.AppParams.GetOrGenerate(opWeightMsgApplyValidator, &weightMsgApplyValidator, nil,
		func(_ *rand.Rand) {
			weightMsgApplyValidator = defaultWeightMsgApplyValidator
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgApplyValidator,
		validatorgovsimulation.SimulateMsgApplyValidator(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))

	return operations
}

// ProposalMsgs returns msgs used for governance proposals for simulations.
//
// MsgApproveValidator only accepts the gov module account as its signer, so it
// cannot be a weighted operation. Registering it here is what lets a simulated
// candidate ever pass the ante gate and submit a MsgCreateValidator.
func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	const (
		opWeightMsgApproveValidator          = "op_weight_msg_approve_validator"
		defaultWeightMsgApproveValidator int = 100
	)

	return []simtypes.WeightedProposalMsg{
		simulation.NewWeightedProposalMsg(
			opWeightMsgApproveValidator,
			defaultWeightMsgApproveValidator,
			func(r *rand.Rand, ctx sdk.Context, _ []simtypes.Account) sdk.Msg {
				candidate, found := randomPendingCandidate(ctx, am, r)
				if !found {
					return nil
				}
				return &types.MsgApproveValidator{
					Authority: authtypes.NewModuleAddress(types.GovModuleName).String(),
					Candidate: candidate,
					// Mostly approve, so the validator set can actually grow.
					Approve: r.Intn(10) != 0,
				}
			},
		),
	}
}

// randomPendingCandidate picks a candidate awaiting a governance decision.
func randomPendingCandidate(ctx sdk.Context, am AppModule, r *rand.Rand) (string, bool) {
	iter, err := am.keeper.ValidatorApplication.Iterate(ctx, new(collections.Range[string]))
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
			pending = append(pending, a.Candidate)
		}
	}
	if len(pending) == 0 {
		return "", false
	}
	return pending[r.Intn(len(pending))], true
}
