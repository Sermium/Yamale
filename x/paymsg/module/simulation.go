package paymsg

import (
	"fmt"
	"math/rand"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	paymsgsimulation "yamale/blockchain/x/paymsg/simulation"
	"yamale/blockchain/x/paymsg/types"
)

// numSeededParticipants is how many payment service providers start approved.
// SendPayment needs at least two distinct ones on the instruction, and the gov
// simulation cannot be relied on to approve any (see ProposalMsgs below), so
// without a seeded set no payment would ever settle during a run.
const numSeededParticipants = 4

// GenerateGenesisState creates a randomized GenState of the module.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	paymsgGenesis := types.GenesisState{
		Params: types.DefaultParams(),
	}

	for i := 0; i < numSeededParticipants && i < len(simState.Accounts); i++ {
		acc := simState.Accounts[i]
		paymsgGenesis.ApprovedParticipantMap = append(
			paymsgGenesis.ApprovedParticipantMap,
			types.ApprovedParticipant{
				Participant: acc.Address.String(),
				Code:        fmt.Sprintf("%08d", i+1),
				Name:        fmt.Sprintf("Simulated Bank %d", i+1),
			},
		)
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(&paymsgGenesis)
}

// RegisterStoreDecoder registers a decoder.
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns the all the gov module operations with their respective weights.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	operations := make([]simtypes.WeightedOperation, 0)
	const (
		opWeightMsgApplyParticipant          = "op_weight_msg_apply_participant"
		defaultWeightMsgApplyParticipant int = 100
	)

	var weightMsgApplyParticipant int
	simState.AppParams.GetOrGenerate(opWeightMsgApplyParticipant, &weightMsgApplyParticipant, nil,
		func(_ *rand.Rand) {
			weightMsgApplyParticipant = defaultWeightMsgApplyParticipant
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgApplyParticipant,
		paymsgsimulation.SimulateMsgApplyParticipant(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))
	const (
		opWeightMsgSendPayment          = "op_weight_msg_send_payment"
		defaultWeightMsgSendPayment int = 100
	)

	var weightMsgSendPayment int
	simState.AppParams.GetOrGenerate(opWeightMsgSendPayment, &weightMsgSendPayment, nil,
		func(_ *rand.Rand) {
			weightMsgSendPayment = defaultWeightMsgSendPayment
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgSendPayment,
		paymsgsimulation.SimulateMsgSendPayment(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))

	// Weighted well above payments: a payment can only be sent by somebody the
	// instructing participant acts for, so without a steady supply of
	// registered customers the payment operation has nobody to sign as and
	// never exercises anything.
	const (
		opWeightMsgRegisterCustomer          = "op_weight_msg_register_customer"
		defaultWeightMsgRegisterCustomer int = 60
	)

	var weightMsgRegisterCustomer int
	simState.AppParams.GetOrGenerate(opWeightMsgRegisterCustomer, &weightMsgRegisterCustomer, nil,
		func(_ *rand.Rand) {
			weightMsgRegisterCustomer = defaultWeightMsgRegisterCustomer
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgRegisterCustomer,
		paymsgsimulation.SimulateMsgRegisterCustomer(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))

	return operations
}

// ProposalMsgs returns msgs used for governance proposals for simulations.
//
// MsgApproveParticipant only accepts the gov module account as its signer, so
// it cannot be a weighted operation. Registering it here is what lets the
// simulation reach two approved participants, which SendPayment requires.
func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	const (
		opWeightMsgApproveParticipant          = "op_weight_msg_approve_participant"
		defaultWeightMsgApproveParticipant int = 100
	)

	return []simtypes.WeightedProposalMsg{
		simulation.NewWeightedProposalMsg(
			opWeightMsgApproveParticipant,
			defaultWeightMsgApproveParticipant,
			func(r *rand.Rand, ctx sdk.Context, _ []simtypes.Account) sdk.Msg {
				participant, found := randomPendingParticipant(ctx, am, r)
				if !found {
					return nil
				}
				return &types.MsgApproveParticipant{
					Authority:   authtypes.NewModuleAddress(types.GovModuleName).String(),
					Participant: participant,
					// Mostly approve, so payment traffic can actually flow.
					Approve: r.Intn(10) != 0,
				}
			},
		),
	}
}

// randomPendingParticipant picks an applicant awaiting a governance decision.
func randomPendingParticipant(ctx sdk.Context, am AppModule, r *rand.Rand) (string, bool) {
	iter, err := am.keeper.ParticipantApplication.Iterate(ctx, new(collections.Range[string]))
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
			pending = append(pending, a.Creator)
		}
	}
	if len(pending) == 0 {
		return "", false
	}
	return pending[r.Intn(len(pending))], true
}
