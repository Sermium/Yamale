package paymsg

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/paymsg/types"
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
					RpcMethod: "ListParticipantApplication",
					Use:       "list-participant-application",
					Short:     "List all participantApplication",
				},
				{
					RpcMethod:      "GetParticipantApplication",
					Use:            "get-participant-application [id]",
					Short:          "Gets a participantApplication",
					Alias:          []string{"show-participant-application"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "creator"}},
				},
				{
					RpcMethod: "ListApprovedParticipant",
					Use:       "list-approved-participant",
					Short:     "List all approvedParticipant",
				},
				{
					RpcMethod:      "GetApprovedParticipant",
					Use:            "get-approved-participant [id]",
					Short:          "Gets a approvedParticipant",
					Alias:          []string{"show-approved-participant"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "participant"}},
				},
				{
					RpcMethod: "ListPaymentRecord",
					Use:       "list-payment-record",
					Short:     "List all paymentRecord",
				},
				{
					RpcMethod:      "GetPaymentRecord",
					Use:            "get-payment-record [instructing-participant] [id]",
					Short:          "Gets a payment record; ids are unique per instructing participant, not globally",
					Alias:          []string{"show-payment-record"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "instructing_participant"}, {ProtoField: "end_to_end_id"}},
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
					RpcMethod:      "ApplyParticipant",
					Use:            "apply-participant [code] [name]",
					Short:          "Send a apply-participant tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "code"}, {ProtoField: "name"}},
				},
				{
					RpcMethod: "SendPayment",
					Use:       "send-payment [end-to-end-id] [instructing-participant] [instructed-participant] [creditor] [denom] [amount] [purpose-code] [remittance-information]",
					Short:     "Send a send-payment tx",
					Long: "Send a send-payment tx.\n\n" +
						"purpose-code and remittance-information are the ISO 20022 free-text fields,\n" +
						"and they are written to the ledger in the clear, forever, with no way to\n" +
						"erase them afterwards. Pass them empty and use --metadata-hash instead: the\n" +
						"detail is held off-chain and the chain records only the hash, which is what\n" +
						"lets a party prove later which payload this payment carried. Setting both\n" +
						"the hash and the plaintext is refused.\n\n" +
						"  --settlement-jurisdiction NG\n\n" +
						"names the authority that may act on the payment and the regulator that can\n" +
						"open its payload. It is optional today and required once governance turns\n" +
						"on the module's require_settlement_jurisdiction parameter.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "end_to_end_id"}, {ProtoField: "instructing_participant"}, {ProtoField: "instructed_participant"}, {ProtoField: "creditor"}, {ProtoField: "denom"}, {ProtoField: "amount"}, {ProtoField: "purpose_code"}, {ProtoField: "remittance_information"}},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"metadata_hash":           {Name: "metadata-hash", Usage: "base64 SHA-256 of the off-chain metadata payload, in place of the plaintext fields"},
						"settlement_jurisdiction": {Name: "settlement-jurisdiction", Usage: "ISO 3166-1 alpha-2 country whose authority settles this payment, e.g. NG"},
					},
				},
				{
					RpcMethod:      "RegisterCustomer",
					Use:            "register-customer [customer] [registered]",
					Short:          "Record, or remove, an account as banking with you",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "customer"}, {ProtoField: "registered"}},
				},
				{
					RpcMethod: "ApproveParticipant",
					Skip:      true, // authority gated; only callable via a governance proposal
				},
			},
		},
	}
}
