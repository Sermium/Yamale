package syndicate

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/syndicate/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
//
// The short descriptions are written for the person who will actually type
// these: an operator running a raise, and an investor checking what they own.
// "Queries a deal by identifier" tells them nothing; "how full the book is, and
// what the fees will take" tells them which command to reach for.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "The ceilings governance put on fees and on how long a raise may hold money",
				},
				{
					RpcMethod: "Operators",
					Use:       "operators",
					Short:     "The admitted platform operators",
				},
				{
					RpcMethod:      "Investor",
					Use:            "investor [address]",
					Short:          "Whether an address has completed onboarding — what a secondary buyer needs before bidding",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "address"}},
				},
				{
					RpcMethod:      "Deal",
					Use:            "deal [id]",
					Short:          "One syndication: the company, the partner, the terms and how full the book is",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod:      "DealByRef",
					Use:            "deal-by-ref [reference]",
					Short:          "Finds a deal by the platform's own file reference",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "reference"}},
				},
				{
					RpcMethod: "Deals",
					Use:       "deals",
					Short:     "Every syndication the platform has opened",
				},
				{
					RpcMethod:      "Commitments",
					Use:            "commitments [deal-id]",
					Short:          "Who has committed what to an open deal",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "deal_id"}},
				},
				{
					RpcMethod: "Entitlement",
					Use:       "entitlement [deal-id] [holder]",
					Short:     "What a holder owns and what they could claim right now",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "deal_id"},
						{ProtoField: "holder"},
					},
				},
				{
					RpcMethod:      "Listings",
					Use:            "listings [deal-id]",
					Short:          "The secondary market for one deal",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "deal_id"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "OnboardInvestor",
					Use:       "onboard [investor] [reference] [jurisdiction] [active]",
					Short:     "Records that this operator completed KYC on somebody. Never the documents — only that it was done, and when",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "investor"},
						{ProtoField: "reference"},
						{ProtoField: "jurisdiction"},
						{ProtoField: "active"},
					},
				},
				{
					RpcMethod: "OpenDeal",
					Use:       "open-deal [reference] [company] [partner] [drawdown-account] [settlement-denom] [target] [minimum] [minimum-ticket] [closes-at]",
					Short:     "Puts an allocation in front of the investor base",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "reference"},
						{ProtoField: "company"},
						{ProtoField: "partner"},
						{ProtoField: "drawdown_account"},
						{ProtoField: "settlement_denom"},
						{ProtoField: "target"},
						{ProtoField: "minimum"},
						{ProtoField: "minimum_ticket"},
						{ProtoField: "closes_at"},
					},
				},
				{
					RpcMethod: "Commit",
					Use:       "commit [deal-id] [amount]",
					Short:     "Escrows your cheque. The money moves now, and you may withdraw it in full while the deal is open",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "deal_id"},
						{ProtoField: "amount"},
					},
				},
				{
					RpcMethod:      "WithdrawCommitment",
					Use:            "withdraw [deal-id]",
					Short:          "Takes your escrowed cheque back, in full",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "deal_id"}},
				},
				{
					RpcMethod:      "CloseDeal",
					Use:            "close-deal [deal-id]",
					Short:          "Funds the company and issues the tokens. Operator only",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "deal_id"}},
				},
				{
					RpcMethod: "AbortDeal",
					Use:       "abort-deal [deal-id] [reason]",
					Short:     "Refunds everybody in full, fees included. Anybody may call it once a deal has missed its minimum",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "deal_id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "Distribute",
					Use:       "distribute [deal-id] [amount] [final]",
					Short:     "Pays a dividend or exit proceeds pro-rata to whoever holds the token now",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "deal_id"},
						{ProtoField: "amount"},
						{ProtoField: "final"},
					},
				},
				{
					RpcMethod:      "Claim",
					Use:            "claim [deal-id]",
					Short:          "Withdraws what you have accrued, without giving up the position",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "deal_id"}},
				},
				{
					RpcMethod: "List",
					Use:       "list [deal-id] [buyer] [quantity] [price]",
					Short:     "Offers part of your position for sale. Leave the buyer empty for an open listing",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "deal_id"},
						{ProtoField: "buyer"},
						{ProtoField: "quantity"},
						{ProtoField: "price"},
					},
				},
				{
					RpcMethod:      "CancelListing",
					Use:            "cancel-listing [listing-id]",
					Short:          "Withdraws an offer and returns the escrowed tokens",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "listing_id"}},
				},
				{
					RpcMethod:      "Buy",
					Use:            "buy [listing-id]",
					Short:          "Fills a listing. Payment and tokens move together, in one block",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "listing_id"}},
				},
				{
					RpcMethod: "RegisterOperator",
					Skip:      true, // governance only
				},
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // governance only
				},
			},
		},
	}
}
