package constitution

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/constitution/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Invariants",
					Use:       "invariants",
					Short:     "Shows the values this chain fixed at genesis and will not let a parameter update change",
				},
				{
					RpcMethod:      "Amendment",
					Use:            "amendment [id]",
					Short:          "Shows one amendment, what it would change and where it has got to",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod: "ListAmendment",
					Use:       "list-amendment",
					Short:     "Lists every amendment ever opened, lapsed and withdrawn ones included",
				},
				{
					RpcMethod:      "Ratifications",
					Use:            "ratifications [amendment-id]",
					Short:          "Shows which validators have ratified an amendment, and what it still needs",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "amendment_id"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              types.Msg_serviceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "ProposeAmendment",
					Skip:      true, // skipped because authority gated: an amendment is a governance proposal
				},
				{
					RpcMethod: "WithdrawAmendment",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod: "RatifyAmendment",
					Use:       "ratify-amendment [validator] [amendment-id]",
					Short:     "Agree, as a validator, to a pending constitutional amendment",
					Long: "Agree, as a validator, to a pending constitutional amendment.\n\n" +
						"validator is the validator's own account — the key it signs with. Its operator\n" +
						"address and its voting power are read from the staking module.\n\n" +
						"There is no way to take this back. The protection an amendment carries is the\n" +
						"delay and the threshold, not the ability to run the vote backwards.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "validator"},
						{ProtoField: "amendment_id"},
					},
				},
			},
		},
	}
}
