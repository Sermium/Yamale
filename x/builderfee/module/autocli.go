package builderfee

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/builderfee/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the parameters of the module",
				},
				{
					RpcMethod: "ListBuilderApplication",
					Use:       "list-builder-application",
					Short:     "List all builderApplication",
				},
				{
					RpcMethod:      "GetBuilderApplication",
					Use:            "get-builder-application [id]",
					Short:          "Gets a builderApplication",
					Alias:          []string{"show-builder-application"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "msg_type_url"}},
				},
				{
					RpcMethod: "ListApprovedBuilder",
					Use:       "list-approved-builder",
					Short:     "List all approvedBuilder",
				},
				{
					RpcMethod:      "GetApprovedBuilder",
					Use:            "get-approved-builder [id]",
					Short:          "Gets a approvedBuilder",
					Alias:          []string{"show-approved-builder"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "msg_type_url"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              types.Msg_serviceDesc.ServiceName,
			EnhanceCustomCommand: true, // only required if you want to use the custom command
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod:      "RegisterBuilder",
					Use:            "register-builder [msg-type-url] [payout-address]",
					Short:          "Send a register-builder tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "msg_type_url"}, {ProtoField: "payout_address"}},
				},
				{
					RpcMethod: "ApproveBuilder",
					Skip:      true, // authority gated; only callable via a governance proposal
				},
			},
		},
	}
}
