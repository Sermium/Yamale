package alias

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/alias/types"
)

// AutoCLIOptions builds the CLI from the proto service definitions, so the
// commands cannot drift from the messages they send.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Show the module parameters"},
				{
					RpcMethod:      "Alias",
					Use:            "resolve [id]",
					Short:          "Resolve a user ID to an address",
					Long:           "Accepts the identifier in any form: hyphenated or not, upper or lower case, with I and O where 1 and 0 belong.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "AliasOf",
					Use:            "id-of [address]",
					Short:          "Show the user ID held by an address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod:      "Retired",
					Use:            "retired [id]",
					Short:          "Report whether a user ID has been given up",
					Long:           "A retired identifier resolves to nothing and is never issued again. This tells it apart from one that never existed.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "Jurisdiction",
					Use:            "jurisdiction [address]",
					Short:          "Show the country recorded against an account",
					Long:           "An account with none holds no user ID and will not be issued one, so \"not found\" here is an answer rather than a gap.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod:      "Perimeter",
					Use:            "perimeter [country]",
					Short:          "List the accounts recorded in one country",
					Long:           "The accounts a national authority may act on, and no others. Returns jurisdiction records, not user IDs.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "country"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "RegisterAlias",
					Use:       "register",
					Short:     "Claim a user ID for your account",
					Long:      "The chain assigns the identifier; there is nothing to choose. One per account.",
				},
				{
					RpcMethod: "RotateAlias",
					Use:       "rotate",
					Short:     "Retire your user ID and take a new one",
					Long:      "For an account whose key was compromised. The old identifier is never issued again, so a payment sent to it arrives nowhere rather than with whoever took the key.",
				},
				{
					RpcMethod: "SetJurisdiction",
					Use:       "set-jurisdiction [account] [country]",
					Short:     "Record where an account is",
					Long:      "The approved participant that onboarded the account records it once. Correcting one already recorded is a foundation administrator's act, and it retires the account's user ID and issues a replacement carrying the new country — an identifier whose prefix could go stale is an identifier whose prefix can lie.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "account"},
						{ProtoField: "country"},
					},
				},
				// UpdateParams is governance-only and is submitted as a proposal
				// payload, so it is deliberately not offered as a CLI command:
				// a command that can only ever fail is a support ticket.
				{RpcMethod: "UpdateParams", Skip: true},
			},
		},
	}
}
