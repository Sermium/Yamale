package enforcement

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/enforcement/types"
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
					RpcMethod:      "GetCase",
					Use:            "case [id]",
					Short:          "Shows one case, its grounds, its evidence and every vote on it",
					Alias:          []string{"show-case"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod: "ListCase",
					Use:       "list-case",
					Short:     "Lists every case ever opened, resolved or not",
				},
				{
					RpcMethod: "OpenCases",
					Use:       "open-cases",
					Short:     "Lists the cases still waiting to be voted on",
				},
				{
					RpcMethod:      "CaseVotes",
					Use:            "votes [case-id]",
					Short:          "Shows how each validator voted, and what the case still needs to pass",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "case_id"}},
				},
				{
					RpcMethod:      "FreezeStatus",
					Use:            "freeze-status [address]",
					Short:          "Shows whether an address may send, and on what grounds if not",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod: "ListFreeze",
					Use:       "list-freeze",
					Short:     "Lists every frozen address",
				},
				{
					RpcMethod: "Recovered",
					Use:       "recovered",
					Short:     "Shows what this module has taken in total, and how often",
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
					RpcMethod: "ReverseCase",
					Skip:      true, // skipped because authority gated: reversal is a governance proposal
				},
				{
					// Skipped for a different reason from the ones above: these are
					// signed by the founders' group policy, so they are executed
					// through an x/group proposal rather than by a key at a
					// terminal. The message JSON is in the enforcement guide.
					RpcMethod: "EmergencyFreeze",
					Skip:      true,
				},
				{
					RpcMethod: "EmergencyRelease",
					Skip:      true,
				},
				{
					RpcMethod: "OpenCase",
					Use:       "open-case [opener] [target] [action]",
					Short:     "Accuse an address and freeze it while the validators decide",
					Long: "Accuse an address and freeze it while the validators decide.\n\n" +
						"The freeze takes effect in this block. It expires by itself if the case is\n" +
						"never resolved, and the account is released the moment the case is rejected,\n" +
						"withdrawn or expires.\n\n" +
						"action is freeze or seize. A seizure needs evidence:\n" +
						"  --evidence-uri https://... --evidence-hash <sha256>",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "opener"},
						{ProtoField: "target"},
						{ProtoField: "action"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"reason":        {Name: "reason", Usage: "the grounds for the case, in words the accused can read"},
						"evidence_uri":  {Name: "evidence-uri", Usage: "where the evidence is held"},
						"evidence_hash": {Name: "evidence-hash", Usage: "SHA-256 of the evidence, so a later edit can be shown"},
					},
				},
				{
					RpcMethod: "VoteCase",
					Use:       "vote-case [voter] [case-id] [option]",
					Short:     "Vote yes, no or abstain on an open case",
					Long: "Vote yes, no or abstain on an open case.\n\n" +
						"voter is the validator's own account — the key it signs with. Its operator\n" +
						"address and its voting power are read from the staking module.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "voter"},
						{ProtoField: "case_id"},
						{ProtoField: "option"},
					},
				},
				{
					RpcMethod: "WithdrawCase",
					Use:       "withdraw-case [opener] [case-id]",
					Short:     "Take back a case you opened, releasing the account",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "opener"},
						{ProtoField: "case_id"},
					},
				},
				{
					RpcMethod: "Sweep",
					Use:       "sweep [sender] [case-id]",
					Short:     "Collect what a passed seizure can now reach",
					Long: "Collect what a passed seizure can now reach.\n\n" +
						"Anyone may send this and nobody gains by it: the destination is fixed by the\n" +
						"module's parameters. It exists because a seizure against staked funds is not\n" +
						"finished on the day it passes — the stake was unbonded then, and arrives when\n" +
						"the unbonding period ends.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "sender"},
						{ProtoField: "case_id"},
					},
				},
			},
		},
	}
}
