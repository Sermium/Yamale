package market

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/market/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
//
// The short descriptions are written for the person who will actually type
// these: a holder trying to sell a stake in a building. "Creates a listing"
// tells them nothing; "name your price and let the first buyer take it" tells
// them which of the three mechanisms they meant.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "How deep one order may sweep the book, and how long an auction may run",
				},
				{
					RpcMethod:      "Listing",
					Use:            "listing [id]",
					Short:          "One fixed-price offer: the parcel, the price, and whether it is still there",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "Listings",
					Use:            "listings [denom]",
					Short:          "Every fixed-price offer over a share, including the filled ones — which are the only record of what it has fetched",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}},
				},
				{
					RpcMethod:      "Auction",
					Use:            "auction [id]",
					Short:          "One auction: the parcel, the reserve, the standing bid and when it closes",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "Auctions",
					Use:            "auctions [denom]",
					Short:          "Every auction over a share",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}},
				},
				{
					RpcMethod:      "Order",
					Use:            "order [id]",
					Short:          "One book order: what was asked for, and what is left of it",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "Book",
					Use:            "book [denom]",
					Short:          "What is bid and offered for a share right now, best price first",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "denom"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // governance only
				},
				{
					RpcMethod: "List",
					Use:       "list [denom] [quantity] [price]",
					Short:     "Offer a parcel of shares for a fixed total; the first buyer takes it",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom"},
						{ProtoField: "quantity"},
						{ProtoField: "price"},
					},
				},
				{
					RpcMethod:      "Buy",
					Use:            "buy [listing-id]",
					Short:          "Take a listing whole: your money one way, the shares the other, in one transaction",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "listing_id"}},
				},
				{
					RpcMethod:      "CancelListing",
					Use:            "cancel-listing [listing-id]",
					Short:          "Withdraw your offer and take the shares back",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "listing_id"}},
				},
				{
					RpcMethod: "OpenAuction",
					Use:       "open-auction [denom] [quantity] [reserve] [closes-at]",
					Short:     "Let the market say what a parcel is worth by a deadline; use a zero reserve to take whatever it fetches",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom"},
						{ProtoField: "quantity"},
						{ProtoField: "reserve"},
						{ProtoField: "closes_at"},
					},
				},
				{
					RpcMethod: "Bid",
					Use:       "bid [auction-id] [amount]",
					Short:     "Beat the standing bid; your money is held until somebody beats yours",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "auction_id"},
						{ProtoField: "amount"},
					},
				},
				{
					RpcMethod:      "SettleAuction",
					Use:            "settle-auction [auction-id]",
					Short:          "Close out an auction past its deadline — anybody may do this, including the winner",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "auction_id"}},
				},
				{
					RpcMethod:      "CancelAuction",
					Use:            "cancel-auction [auction-id]",
					Short:          "Withdraw an auction nobody has bid on yet",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "auction_id"}},
				},
				{
					RpcMethod: "PlaceOrder",
					Use:       "place-order [denom] [side] [quantity] [unit-price]",
					Short:     "Cross what you can against the book and rest the remainder; the price is per share, and it is a limit",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "denom"},
						{ProtoField: "side"},
						{ProtoField: "quantity"},
						{ProtoField: "unit_price"},
					},
				},
				{
					RpcMethod:      "CancelOrder",
					Use:            "cancel-order [order-id]",
					Short:          "Withdraw a resting order and take back what it still holds",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "order_id"}},
				},
			},
		},
	}
}
