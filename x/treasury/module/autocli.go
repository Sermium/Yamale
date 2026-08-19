package treasury

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/treasury/types"
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
					RpcMethod: "ListTreasury",
					Use:       "list-treasury",
					Short:     "List all treasuries",
				},
				{
					RpcMethod:      "GetTreasury",
					Use:            "get-treasury [id]",
					Short:          "Gets a treasury by id",
					Alias:          []string{"show-treasury"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "TreasuryBalances",
					Use:            "balances [treasury-id]",
					Short:          "Shows what a treasury holds, what is locked, and what it may spend",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}},
				},
				{
					RpcMethod: "ListLock",
					Use:       "list-lock",
					Short:     "List all locks",
				},
				{
					RpcMethod:      "GetLock",
					Use:            "get-lock [id]",
					Short:          "Gets a lock by id",
					Alias:          []string{"show-lock"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "LocksByTreasury",
					Use:            "treasury-locks [treasury-id]",
					Short:          "Lists the locks held by a treasury",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}},
				},
				{
					RpcMethod:      "LocksByBeneficiary",
					Use:            "my-locks [beneficiary]",
					Short:          "Lists the locks payable to an address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "beneficiary"}},
				},
				{
					RpcMethod:      "ClaimableAmount",
					Use:            "claimable [lock-id]",
					Short:          "Shows what a lock would release if claimed right now",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "lock_id"}},
				},
				{
					RpcMethod:      "ListRole",
					Use:            "list-role [treasury-id]",
					Short:          "Lists a treasury's role assignments",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}},
				},
				{
					RpcMethod:      "GetSpendPolicy",
					Use:            "get-policy [treasury-id] [denom]",
					Short:          "Gets the spending policy for a treasury and denom",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}, {ProtoField: "denom"}},
				},
				{
					RpcMethod:      "SpendCapacity",
					Use:            "capacity [treasury-id] [denom]",
					Short:          "Shows how much may still be spent in the current period",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}, {ProtoField: "denom"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              types.Msg_serviceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod:      "CreateTreasury",
					Use:            "create-treasury [name] [admin]",
					Short:          "Open a new treasury (admin defaults to the sender when empty)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "name"}, {ProtoField: "admin", Optional: true}},
				},
				{
					RpcMethod:      "Deposit",
					Use:            "deposit [treasury-id] [amount]",
					Short:          "Fund a treasury",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}, {ProtoField: "amount"}},
				},
				{
					RpcMethod:      "Spend",
					Use:            "spend [treasury-id] [recipient] [amount] [memo]",
					Short:          "Pay out of a treasury, within its spend policy",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}, {ProtoField: "recipient"}, {ProtoField: "amount"}, {ProtoField: "memo", Optional: true}},
				},
				{
					RpcMethod: "CreateLock",
					Use:       "create-lock [treasury-id] [beneficiary] [denom] [amount] [lock-type] [start-time] [cliff-time] [end-time] [release-intervals] [revocable]",
					Short:     "Commit treasury funds to a beneficiary on a schedule",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "treasury_id"}, {ProtoField: "beneficiary"}, {ProtoField: "denom"}, {ProtoField: "amount"},
						{ProtoField: "lock_type"}, {ProtoField: "start_time"}, {ProtoField: "cliff_time"}, {ProtoField: "end_time"},
						{ProtoField: "release_intervals"}, {ProtoField: "revocable"},
					},
				},
				{
					RpcMethod:      "ClaimLock",
					Use:            "claim [lock-id]",
					Short:          "Claim whatever has vested to you",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "lock_id"}},
				},
				{
					RpcMethod:      "RevokeLock",
					Use:            "revoke-lock [lock-id]",
					Short:          "Cancel a revocable lock, returning the unvested portion",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "lock_id"}},
				},
				{
					RpcMethod:      "AssignRole",
					Use:            "assign-role [treasury-id] [address] [role]",
					Short:          "Grant an address a role over a treasury",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}, {ProtoField: "address"}, {ProtoField: "role"}},
				},
				{
					RpcMethod:      "RevokeRole",
					Use:            "revoke-role [treasury-id] [address]",
					Short:          "Remove an address's role",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}, {ProtoField: "address"}},
				},
				{
					RpcMethod: "SetSpendPolicy",
					Skip:      true, // the policy is a nested message; use a generated tx
				},
				{
					RpcMethod:      "SetPaused",
					Use:            "set-paused [treasury-id] [paused]",
					Short:          "Freeze or unfreeze a treasury",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}, {ProtoField: "paused"}},
				},
				{
					RpcMethod:      "SetAdmin",
					Use:            "set-admin [treasury-id] [new-admin]",
					Short:          "Transfer administrative control, e.g. to an x/group policy address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "treasury_id"}, {ProtoField: "new_admin"}},
				},
			},
		},
	}
}
