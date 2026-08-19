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
		&MsgRegisterAsset{},
		&MsgSetAttestor{},
		&MsgAttestDeposit{},
		&MsgReportReserve{},
		&MsgRequestRedemption{},
		&MsgSettleRedemption{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}

// RegisterLegacyAminoCodec registers the module's messages for legacy Amino
// signing, which is what a hardware wallet still uses.
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterAsset{}, "blockchain/x/custody/MsgRegisterAsset", nil)
	cdc.RegisterConcrete(&MsgSetAttestor{}, "blockchain/x/custody/MsgSetAttestor", nil)
	cdc.RegisterConcrete(&MsgAttestDeposit{}, "blockchain/x/custody/MsgAttestDeposit", nil)
	cdc.RegisterConcrete(&MsgReportReserve{}, "blockchain/x/custody/MsgReportReserve", nil)
	cdc.RegisterConcrete(&MsgRequestRedemption{}, "blockchain/x/custody/MsgRequestRedemption", nil)
	cdc.RegisterConcrete(&MsgSettleRedemption{}, "blockchain/x/custody/MsgSettleRedemption", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "blockchain/x/custody/MsgUpdateParams", nil)
}
