package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgBurnCoin{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgMintCoin{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterCurrency{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgApproveIssuer{},
	)

	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRevokeIssuer{},
	)
	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
