package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterInterfaces registers the module's messages with the interface
// registry, which is what lets a client encode them and the router find them.
//
// The explicit list and RegisterMsgServiceDesc overlap: the second walks the Msg
// service descriptor and registers every request type in it, so it already covers
// everything above. The list is kept — and kept complete — because it is what a
// reader checks against the service, and two of these messages were missing from
// it for as long as they existed. That was survivable only by accident, and the
// same omission in RegisterLegacyAminoCodec below was not.
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterAlias{},
		&MsgRotateAlias{},
		&MsgSetJurisdiction{},
		&MsgRegisterViewingKey{},
		&MsgRevokeViewingKey{},
		&MsgAppointRegulator{},
		&MsgGrantAuditor{},
		&MsgGrantRole{},
		&MsgRevokeRole{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}

// RegisterLegacyAminoCodec registers the module's messages for legacy Amino
// signing, which is what a hardware wallet still uses.
//
// Unlike the registry above there is no service descriptor to fall back on here:
// a message absent from this list cannot be amino-signed at all, and the failure
// is a wallet refusing to display a transaction rather than an error naming the
// missing registration. MsgGrantRole and MsgRevokeRole were both missing, which
// mattered the moment a role grant stopped being governance-only — an office or a
// custodian signing one from a hardware wallet had no way to.
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterAlias{}, "blockchain/x/alias/MsgRegisterAlias", nil)
	cdc.RegisterConcrete(&MsgRotateAlias{}, "blockchain/x/alias/MsgRotateAlias", nil)
	cdc.RegisterConcrete(&MsgSetJurisdiction{}, "blockchain/x/alias/MsgSetJurisdiction", nil)
	cdc.RegisterConcrete(&MsgRegisterViewingKey{}, "blockchain/x/alias/MsgRegisterViewingKey", nil)
	cdc.RegisterConcrete(&MsgRevokeViewingKey{}, "blockchain/x/alias/MsgRevokeViewingKey", nil)
	cdc.RegisterConcrete(&MsgAppointRegulator{}, "blockchain/x/alias/MsgAppointRegulator", nil)
	cdc.RegisterConcrete(&MsgGrantAuditor{}, "blockchain/x/alias/MsgGrantAuditor", nil)
	cdc.RegisterConcrete(&MsgGrantRole{}, "blockchain/x/alias/MsgGrantRole", nil)
	cdc.RegisterConcrete(&MsgRevokeRole{}, "blockchain/x/alias/MsgRevokeRole", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "blockchain/x/alias/MsgUpdateParams", nil)
}
