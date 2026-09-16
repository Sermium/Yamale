package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterInterfaces registers every message the market accepts.
func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgList{},
		&MsgBuy{},
		&MsgCancelListing{},
		&MsgOpenAuction{},
		&MsgBid{},
		&MsgSettleAuction{},
		&MsgCancelAuction{},
		&MsgPlaceOrder{},
		&MsgCancelOrder{},
	)

	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
