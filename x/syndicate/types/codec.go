package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterInterfaces registers every message the syndication layer accepts.
func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgRegisterOperator{},
		&MsgOnboardInvestor{},
		&MsgOpenDeal{},
		&MsgCommit{},
		&MsgWithdrawCommitment{},
		&MsgCloseDeal{},
		&MsgAbortDeal{},
		&MsgDistribute{},
		&MsgClaim{},
		&MsgList{},
		&MsgCancelListing{},
		&MsgBuy{},
		&MsgAttachDocument{},
	)

	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
