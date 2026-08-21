package netting

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/netting/types"
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
					RpcMethod: "CurrentCycle",
					Use:       "current-cycle",
					Short:     "Shows the open netting window and the block it closes at",
				},
				{
					RpcMethod:      "Cycle",
					Use:            "cycle [id]",
					Short:          "Shows one window, what it settled per currency, and the compression it achieved",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "Position",
					Use:            "position [participant]",
					Short:          "Shows a participant's reserve, what is committed, and its running net position",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "participant"}},
				},
				{
					RpcMethod: "ParticipantObligations",
					Use:       "obligations [participant] [cycle-id]",
					Short:     "Lists the obligations a participant is party to in one window, in either direction",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "participant"},
						{ProtoField: "cycle_id"},
					},
				},
				{
					RpcMethod: "HeldSlices",
					Use:       "held",
					Short:     "Lists the currency slices that failed to settle and are waiting to be retried",
					Long: "Lists the currency slices that failed to settle and are waiting to be retried.\n\n" +
						"On a healthy chain this is empty. Anything in it is money participants were\n" +
						"expecting to have settled and which has not, at its original amounts, against\n" +
						"its original counterparties.",
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
					RpcMethod: "PostReserve",
					Use:       "post-reserve [participant] [amount]",
					Short:     "Prefund the reserve that backs everything this participant may owe",
					Long: "Prefund the reserve that backs everything this participant may owe.\n\n" +
						"The coins move into the netting module account. Nothing may be netted beyond\n" +
						"what is posted here, which is why a window can always settle.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "participant"},
						{ProtoField: "amount", Varargs: true},
					},
				},
				{
					RpcMethod: "WithdrawReserve",
					Use:       "withdraw-reserve [participant] [amount]",
					Short:     "Take back the part of the reserve that is not committed",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "participant"},
						{ProtoField: "amount", Varargs: true},
					},
				},
				{
					RpcMethod: "SubmitObligation",
					Use:       "submit-obligation [from-participant] [to-participant] [denom] [amount]",
					Short:     "Record what this participant owes another",
					Long: "Record what this participant owes another.\n\n" +
						"Whether it settles gross in this block or joins the open netting window is\n" +
						"decided by the chain from the amount and the currency's threshold, not by the\n" +
						"sender. The response says which happened.\n\n" +
						"--batch-hash is SHA-256 over the salted retail batch this figure summarises,\n" +
						"and is required: an interbank figure with no link to the items behind it is\n" +
						"one neither party can reconcile.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "from_participant"},
						{ProtoField: "to_participant"},
						{ProtoField: "denom"},
						{ProtoField: "amount"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"batch_hash": {Name: "batch-hash", Usage: "SHA-256 of the salted retail batch this obligation summarises"},
					},
				},
			},
		},
	}
}
