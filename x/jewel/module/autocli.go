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
			},
		},
	}
}
