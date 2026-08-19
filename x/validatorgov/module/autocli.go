package validatorgov

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/validatorgov/types"
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
					RpcMethod: "ListValidatorApplication",
					Use:       "list-validator-application",
					Short:     "List all validatorApplication",
				},
				{
					RpcMethod:      "GetValidatorApplication",
					Use:            "get-validator-application [id]",
					Short:          "Gets a validatorApplication",
					Alias:          []string{"show-validator-application"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "candidate"}},
				},
				{
					RpcMethod: "ListApprovedValidator",
					Use:       "list-approved-validator",
					Short:     "List all approvedValidator",
				},
				{
					RpcMethod:      "GetApprovedValidator",
					Use:            "get-approved-validator [id]",
					Short:          "Gets a approvedValidator",
					Alias:          []string{"show-approved-validator"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "candidate"}},
				},
				{
					RpcMethod: "ListOperatorRotation",
					Use:       "list-operator-rotation",
					Short:     "List every operator rotation ever opened",
				},
				{
					RpcMethod:      "GetOperatorRotation",
					Use:            "get-operator-rotation [id]",
					Short:          "Gets an operator rotation by id",
					Alias:          []string{"show-operator-rotation"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "PendingOperatorRotation",
					Use:            "pending-operator-rotation [operator]",
					Short:          "Shows the rotation open against an operator, if there is one",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "current_operator"}},
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
					RpcMethod:      "ApplyValidator",
					Use:            "apply-validator [moniker] [description]",
					Short:          "Send a apply-validator tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "moniker"}, {ProtoField: "description"}},
				},
				{
					RpcMethod: "ApproveValidator",
					Skip:      true, // authority gated; only callable via a governance proposal
				},
				{
					RpcMethod:      "RotateOperator",
					Use:            "rotate-operator [new-operator]",
					Short:          "Rotate your validator's operator key to a new address",
					Long:           "Signed by the operator being replaced. Takes effect after the planned rotation delay.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "new_operator"}},
				},
				{
					RpcMethod:      "ProposeOperatorRecovery",
					Use:            "propose-operator-recovery [current-operator] [new-operator] [reason]",
					Short:          "Propose recovering a validator whose operator key is lost",
					Long:           "Openable by anybody, and inert until the same offices that admit validators approve it. The named operator cancels it by signing anything at all.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "current_operator"}, {ProtoField: "new_operator"}, {ProtoField: "reason"}},
				},
				{
					RpcMethod: "ApproveOperatorRecovery",
					Skip:      true, // authority gated; only callable via a governance proposal
				},
				{
					RpcMethod:      "CancelOperatorRotation",
					Use:            "cancel-operator-rotation [rotation-id]",
					Short:          "Withdraw a rotation against your own operator address before it takes effect",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "rotation_id"}},
				},
			},
		},
	}
}
