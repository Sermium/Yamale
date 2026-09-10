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
		&MsgEnrolHerd{},
		&MsgIdentifyAnimal{},
		&MsgCertifyAnimal{},
		&MsgRecordHealthEvent{},
		&MsgProposeTransfer{},
		&MsgAcceptTransfer{},
		&MsgAttestTransfer{},
		&MsgObjectTransfer{},
		&MsgCompleteTransfer{},
		&MsgRetireAnimal{},
		&MsgCancelTransfer{},
		&MsgResolveObjection{},
		&MsgFreezeAnimal{},
		&MsgReleaseAnimal{},
	)

	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
