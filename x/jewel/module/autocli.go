package jewel

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/jewel/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Shows the office and how long offers, vouchers and veto windows last"},
				{RpcMethod: "Jeweler", Use: "jeweler [id]", Short: "Shows one jeweler: name, business account, staff and status",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Jewelers", Use: "jewelers", Short: "Lists the jewelers, suspended ones included"},
				{RpcMethod: "Jewel", Use: "jewel [id]", Short: "What the verification page shows: the record, the owner, any open offer, pending reassignment and benefits",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "JewelsByInscription", Use: "inscription [code]", Short: "Finds the jewels carrying an inscription, as read through a loupe",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "inscription"}}},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "UpdateParams", Skip: true}, // governance only
				{RpcMethod: "AdmitJeweler", Use: "admit-jeweler [name] [business]", Short: "The office admits a jeweler",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "name"}, {ProtoField: "business"}}},
				{RpcMethod: "SuspendJeweler", Use: "suspend-jeweler [jeweler-id] [reason]", Short: "The office stops a jeweler acting",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jeweler_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "ReinstateJeweler", Use: "reinstate-jeweler [jeweler-id]", Short: "The office lets a suspended jeweler act again",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jeweler_id"}}},
				{RpcMethod: "SetStaff", Use: "set-staff [jeweler-id]", Short: "Adds (--add) and removes (--remove) the jeweler's staff",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jeweler_id"}}},
				{RpcMethod: "RegisterJewel", Use: "register [inscription] [record-sha256] [record-uri]", Short: "Registers a jewel into the jeweler's stock",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "inscription"}, {ProtoField: "record_hash"}, {ProtoField: "record_uri"}}},
				{RpcMethod: "AmendJewel", Use: "amend [jewel-id] [record-sha256] [record-uri] [note]", Short: "Appends a correction to a jewel's record",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}, {ProtoField: "record_hash"}, {ProtoField: "record_uri"}, {ProtoField: "note"}}},
				{RpcMethod: "OfferJewel", Use: "offer [jewel-id]", Short: "Offers a jewel to an account (--to) or a claim key (--claim-key)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "CancelOffer", Use: "cancel-offer [jewel-id]", Short: "Closes the open offer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "AcceptOffer", Use: "accept [jewel-id]", Short: "Takes a jewel offered to you",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "ClaimOffer", Skip: true}, // needs a claim-key signature: tools/jewelsign
				{RpcMethod: "ReportStolen", Use: "report-stolen [jewel-id]", Short: "Flags a jewel as stolen and closes its offer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "ClearStolen", Use: "clear-stolen [jewel-id]", Short: "Lifts the stolen flag",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "AttachBenefit", Use: "attach-benefit [jewel-id] [kind] [allowance]", Short: "Attaches a benefit (--period, --expires-at in unix seconds)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}, {ProtoField: "kind"}, {ProtoField: "allowance"}}},
				{RpcMethod: "RedeemBenefit", Skip: true}, // needs the owner's voucher signature: tools/jewelsign
				{RpcMethod: "ProposeReassignment", Use: "propose-reassignment [jewel-id] [to] [reason]", Short: "The office proposes moving a jewel whose owner lost their key",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}, {ProtoField: "to"}, {ProtoField: "reason"}}},
				{RpcMethod: "VetoReassignment", Use: "veto-reassignment [jewel-id]", Short: "The owner refuses a proposed reassignment",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "CompleteReassignment", Use: "complete-reassignment [jewel-id]", Short: "Anyone completes a reassignment once its window has passed",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
			},
		},
	}
}
