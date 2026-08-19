package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgUpdateParams{}, "blockchain/x/tokenisation/MsgUpdateParams", nil)
	cdc.RegisterConcrete(&MsgCreateCollection{}, "blockchain/x/tokenisation/MsgCreateCollection", nil)
	cdc.RegisterConcrete(&MsgSetCollectionAuthority{}, "blockchain/x/tokenisation/MsgSetCollectionAuthority", nil)
	cdc.RegisterConcrete(&MsgMintAsset{}, "blockchain/x/tokenisation/MsgMintAsset", nil)
	cdc.RegisterConcrete(&MsgFractionalise{}, "blockchain/x/tokenisation/MsgFractionalise", nil)
	cdc.RegisterConcrete(&MsgTransferAsset{}, "blockchain/x/tokenisation/MsgTransferAsset", nil)
	cdc.RegisterConcrete(&MsgFundVault{}, "blockchain/x/tokenisation/MsgFundVault", nil)
	cdc.RegisterConcrete(&MsgReportSale{}, "blockchain/x/tokenisation/MsgReportSale", nil)
	cdc.RegisterConcrete(&MsgAttestSale{}, "blockchain/x/tokenisation/MsgAttestSale", nil)
	cdc.RegisterConcrete(&MsgDisputeSale{}, "blockchain/x/tokenisation/MsgDisputeSale", nil)
	cdc.RegisterConcrete(&MsgResolveDispute{}, "blockchain/x/tokenisation/MsgResolveDispute", nil)
	cdc.RegisterConcrete(&MsgClaim{}, "blockchain/x/tokenisation/MsgClaim", nil)
	cdc.RegisterConcrete(&MsgRedeem{}, "blockchain/x/tokenisation/MsgRedeem", nil)
}

func RegisterInterfaces(reg cdctypes.InterfaceRegistry) {
	reg.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgCreateCollection{},
		&MsgSetCollectionAuthority{},
		&MsgMintAsset{},
		&MsgFractionalise{},
		&MsgTransferAsset{},
		&MsgFundVault{},
		&MsgReportSale{},
		&MsgAttestSale{},
		&MsgDisputeSale{},
		&MsgResolveDispute{},
		&MsgClaim{},
		&MsgRedeem{},
	)
	msgservice.RegisterMsgServiceDesc(reg, &_Msg_serviceDesc)
}
