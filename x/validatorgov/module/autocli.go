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
				{
					RpcMethod: "Concentration",
					Use:       "concentration",
					Short:     "Shows what every entity, beneficial owner and jurisdiction holds against its ceiling",
					Long: "Shows what every entity, beneficial owner and jurisdiction holds against its ceiling.\n\n" +
						"Under equal seats these are counts, so the answer can be checked against the\n" +
						"list of admitted validators without recomputing anything. A group marked over\n" +
						"with active_validators equal to min_active_validators is a breach the chain\n" +
						"has decided not to correct, because correcting it would take the active set\n" +
						"below the floor.",
				},
				{
					RpcMethod: "ListDemotion",
					Use:       "list-demotion",
					Short:     "Lists the validators the concentration ceilings are currently holding down",
				},
				{
					RpcMethod: "JurisdictionReconciliation",
					Use:       "jurisdiction-reconciliation",
					Short:     "Compares each validator's declared country with the one the chain recorded",
					Long: "Compares each validator's declared country with the one the chain recorded.\n\n" +
						"Two registries, on purpose. A validator declares its jurisdiction when it\n" +
						"applies and signs for it; the jurisdiction registry is written by the\n" +
						"participant that onboarded the account and did the know-your-customer work,\n" +
						"or corrected by a foundation administrator, and never by the account itself.\n" +
						"They are reconciled here and merged nowhere: merging one into the other would\n" +
						"either destroy the signature that makes a false declaration an offence, or let\n" +
						"a validator choose the perimeter with the least authority watching it.\n\n" +
						"DISAGREE is not a breach and nothing is done to the validator for it. It means\n" +
						"one of two things is wrong and somebody has to find out which, because the\n" +
						"jurisdiction ceiling is computed over the declaration while every authority\n" +
						"acting on the account is scoped by the record.\n\n" +
						"UNRECORDED is reported apart from AGREE: an account nobody has placed has a\n" +
						"declaration nobody has corroborated, and the remedy is to place the account.\n\n" +
						"Every approved validator is listed, agreements included, with counts. A query\n" +
						"that answered with an empty list when all was well would be indistinguishable\n" +
						"from one pointed at an empty registry.",
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
					RpcMethod: "ApplyValidator",
					Use:       "apply-validator [moniker] [description] [legal-entity-id] [beneficial-owner-id] [jurisdiction]",
					Short:     "Apply to join the validator set, declaring who is behind you",
					Long: "Apply to join the validator set, declaring who is behind you.\n\n" +
						"All three declaration fields are required. The concentration ceilings are\n" +
						"computed over declared entities, owners and jurisdictions, so an applicant\n" +
						"who declared none would belong to no group and be bounded by no ceiling.\n\n" +
						"beneficial-owner-id is whoever ultimately owns the entity. Where nobody does,\n" +
						"repeat the entity's own identifier — \"nobody owns us\" is a claim you sign,\n" +
						"not a field you leave blank.\n\n" +
						"jurisdiction is an ISO 3166-1 alpha-2 country code.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "moniker"},
						{ProtoField: "description"},
						{ProtoField: "legal_entity_id"},
						{ProtoField: "beneficial_owner_id"},
						{ProtoField: "jurisdiction"},
					},
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
				{
					RpcMethod: "AttestOwnership",
					Use:       "attest-ownership [legal-entity-id] [beneficial-owner-id] [jurisdiction]",
					Short:     "Re-sign for who is behind your validator, so the declaration is not stale",
					Long: "Re-sign for who is behind your validator, so the declaration is not stale.\n\n" +
						"The whole declaration is restated, not just the date. That is deliberate: an\n" +
						"operator whose owner has changed and who re-attests the old values has put a\n" +
						"false statement on the record under its own key.\n\n" +
						"A declaration older than the attestation interval is reported as stale in an\n" +
						"event at every epoch. Nothing is done to the validator for it — the chain\n" +
						"cannot verify a declaration, so what it publishes is the date.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "legal_entity_id"},
						{ProtoField: "beneficial_owner_id"},
						{ProtoField: "jurisdiction"},
					},
				},
				{
					RpcMethod: "SetValidatorPower",
					Skip:      true, // authority gated; only callable via a governance proposal
				},
			},
		},
	}
}
