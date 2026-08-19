package amm

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/amm/types"
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
					RpcMethod: "ListPool",
					Use:       "list-pool",
					Short:     "List all pool",
				},
				{
					RpcMethod:      "GetPool",
					Use:            "get-pool [id]",
					Short:          "Gets a pool by id",
					Alias:          []string{"show-pool"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
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
					RpcMethod:      "CreatePool",
					Use:            "create-pool [denom-a] [amount-a] [denom-b] [amount-b] [swap-fee-bps]",
					Short:          "Send a create-pool tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom_a"}, {ProtoField: "amount_a"}, {ProtoField: "denom_b"}, {ProtoField: "amount_b"}, {ProtoField: "swap_fee_bps"}},
				},
				{
					RpcMethod:      "JoinPool",
					Use:            "join-pool [pool-id] [amount-a] [amount-b]",
					Short:          "Send a join-pool tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "pool_id"}, {ProtoField: "amount_a"}, {ProtoField: "amount_b"}},
				},
				{
					RpcMethod:      "ExitPool",
					Use:            "exit-pool [pool-id] [shares]",
					Short:          "Send a exit-pool tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "pool_id"}, {ProtoField: "shares"}},
				},
				{
					RpcMethod:      "Swap",
					Use:            "swap [pool-id] [token-in-denom] [token-in-amount] [token-out-denom] [min-amount-out]",
					Short:          "Send a swap tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "pool_id"}, {ProtoField: "token_in_denom"}, {ProtoField: "token_in_amount"}, {ProtoField: "token_out_denom"}, {ProtoField: "min_amount_out"}},
				},
			},
		},
	}
}
