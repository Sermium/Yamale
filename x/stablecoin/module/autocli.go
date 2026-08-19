package stablecoin

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/stablecoin/types"
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
					RpcMethod: "ListIssuerApplication",
					Use:       "list-issuer-application",
					Short:     "List all issuerApplication",
				},
				{
					RpcMethod:      "GetIssuerApplication",
					Use:            "get-issuer-application [id]",
					Short:          "Gets a issuerApplication",
					Alias:          []string{"show-issuer-application"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}},
				},
				{
					RpcMethod: "ListApprovedIssuer",
					Use:       "list-approved-issuer",
					Short:     "List all approvedIssuer",
				},
				{
					RpcMethod:      "GetApprovedIssuer",
					Use:            "get-approved-issuer [id]",
					Short:          "Gets a approvedIssuer",
					Alias:          []string{"show-approved-issuer"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}},
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
					RpcMethod:      "RegisterCurrency",
					Use:            "register-currency [denom] [display-denom] [exponent] [name] [symbol] [description]",
					Short:          "Send a register-currency tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}, {ProtoField: "display_denom"}, {ProtoField: "exponent"}, {ProtoField: "name"}, {ProtoField: "symbol"}, {ProtoField: "description"}},
				},
				{
					RpcMethod:      "MintCoin",
					Use:            "mint-coin [denom] [amount] [recipient]",
					Short:          "Send a mint-coin tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}, {ProtoField: "amount"}, {ProtoField: "recipient"}},
				},
				{
					RpcMethod:      "BurnCoin",
					Use:            "burn-coin [denom] [amount]",
					Short:          "Send a burn-coin tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}, {ProtoField: "amount"}},
				},
				{
					RpcMethod: "ApproveIssuer",
					Skip:      true, // authority gated; only callable via a governance proposal
				},
			},
		},
	}
}
