package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgCreateTreasury{},
		&MsgDeposit{},
		&MsgSpend{},
		&MsgCreateLock{},
		&MsgClaimLock{},
		&MsgRevokeLock{},
		&MsgOpenEscrow{},
		&MsgReleaseEscrow{},
		&MsgDisputeEscrow{},
		&MsgResolveEscrow{},
		&MsgAssignRole{},
		&MsgRevokeRole{},
		&MsgSetSpendPolicy{},
		&MsgSetPaused{},
		&MsgSetAdmin{},
	)
	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
