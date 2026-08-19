package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterInterfaces registers every message the registry accepts.
func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgRegisterAuthority{},
		&MsgRegisterParcel{},
		&MsgProposeTransfer{},
		&MsgValidateTransfer{},
		&MsgAttestTransfer{},
		&MsgObject{},
		&MsgCompleteTransfer{},
		&MsgRecordEncumbrance{},
		&MsgFreezeParcel{},
		&MsgAttachDeed{},
		&MsgSetRestriction{},
		&MsgAuthoriseFractionalisation{},
	)

	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
