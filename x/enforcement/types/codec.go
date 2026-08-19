package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgOpenCase{},
		&MsgVoteCase{},
		&MsgWithdrawCase{},
		&MsgSweep{},
		&MsgReverseCase{},
		&MsgEmergencyFreeze{},
		&MsgEmergencyRelease{},
		&MsgOmbudsmanVeto{},
	)
	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
