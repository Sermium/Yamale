package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterInterfaces registers the module's messages with the interface
// registry, which is what lets a client encode them and the router find them.
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterAlias{},
		&MsgRotateAlias{},
		&MsgSetJurisdiction{},
		&MsgRegisterViewingKey{},
		&MsgRevokeViewingKey{},
		&MsgAppointRegulator{},
		&MsgGrantAuditor{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}

// RegisterLegacyAminoCodec registers the module's messages for legacy Amino
// signing, which is what a hardware wallet still uses.
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterAlias{}, "blockchain/x/alias/MsgRegisterAlias", nil)
	cdc.RegisterConcrete(&MsgRotateAlias{}, "blockchain/x/alias/MsgRotateAlias", nil)
	cdc.RegisterConcrete(&MsgSetJurisdiction{}, "blockchain/x/alias/MsgSetJurisdiction", nil)
	cdc.RegisterConcrete(&MsgRegisterViewingKey{}, "blockchain/x/alias/MsgRegisterViewingKey", nil)
	cdc.RegisterConcrete(&MsgRevokeViewingKey{}, "blockchain/x/alias/MsgRevokeViewingKey", nil)
	cdc.RegisterConcrete(&MsgAppointRegulator{}, "blockchain/x/alias/MsgAppointRegulator", nil)
	cdc.RegisterConcrete(&MsgGrantAuditor{}, "blockchain/x/alias/MsgGrantAuditor", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "blockchain/x/alias/MsgUpdateParams", nil)
}
