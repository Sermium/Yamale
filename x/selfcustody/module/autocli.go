package selfcustody

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/selfcustody/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
//
// Written for the two people who will type these. One is a holder who wants
// their key and is being told to wait; the other is a support engineer trying
// to establish, in a dispute, exactly what somebody agreed to and when. Both
// are better served by a sentence that says what the command is for than by a
// restatement of the method name.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "How long the wait is, and the version and hash of the warning now in force",
				},
				{
					RpcMethod:      "Disclosure",
					Use:            "disclosure [account]",
					Short:          "Whether this account's private key has been handed to its holder — and so whether the platform still stands behind it",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "account"}},
				},
				{
					RpcMethod:      "Request",
					Use:            "request [account]",
					Short:          "A pending request, when it may be confirmed, and when it lapses",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "account"}},
				},
				{
					RpcMethod: "Count",
					Use:       "count",
					Short:     "How many accounts have taken their keys, and how many are waiting — no names",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "RequestDisclosure",
					Use:       "request-disclosure [acknowledgement-version] [acknowledgement-hash]",
					Short:     "Start the wait to take your own private key. Read the warning first; this is not a seed phrase",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "acknowledgement_version"},
						{ProtoField: "acknowledgement_hash"},
					},
				},
				{
					RpcMethod:      "CancelDisclosure",
					Use:            "cancel-disclosure [reason]",
					Short:          "Stop a pending request. Use this if you did not make it",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "reason"}},
				},
				{
					RpcMethod: "ConfirmDisclosure",
					Use:       "confirm-disclosure [acknowledgement-version] [acknowledgement-hash]",
					Short:     "Complete the pathway. After this the platform's custody of this key ends and cannot be restored",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "acknowledgement_version"},
						{ProtoField: "acknowledgement_hash"},
					},
				},
				{
					RpcMethod: "UpdateParams",
					Skip:      true,
				},
			},
		},
	}
}
