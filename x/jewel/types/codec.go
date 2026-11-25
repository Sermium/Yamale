package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterInterfaces registers every message the register accepts.
func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgAdmitJeweler{},
		&MsgSuspendJeweler{},
		&MsgReinstateJeweler{},
		&MsgSetStaff{},
		&MsgRegisterJewel{},
		&MsgAmendJewel{},
		&MsgOfferJewel{},
		&MsgCancelOffer{},
		&MsgAcceptOffer{},
		&MsgClaimOffer{},
		&MsgReportStolen{},
		&MsgClearStolen{},
		&MsgAttachBenefit{},
		&MsgRedeemBenefit{},
		&MsgProposeReassignment{},
		&MsgVetoReassignment{},
		&MsgCompleteReassignment{},
		&MsgIssueCard{},
		&MsgFileClaim{},
		&MsgApproveClaim{},
		&MsgRejectClaim{},
		&MsgWithdrawClaim{},
	)

	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
