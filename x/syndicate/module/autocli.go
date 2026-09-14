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
				{
					RpcMethod: "Documents",
					Use:       "documents [deal-id]",
					Short:     "The papers a deal depends on \u2014 their digests, never their contents",
					Long: "The papers a deal depends on: kind, title, SHA-256 digest and " +
						"where to find it.\n\n" +
						"Verify one you have been sent with `sha256sum <file>` and compare. A " +
						"digest that matches proves the file is the document this operator " +
						"attested to at that block. It proves nothing about whether the document " +
						"says what its title claims, or whether anybody signed it.\n\n" +
						"Superseded versions are listed by default, because what was agreed at a " +
						"date is usually the question. Pass --current-only for the live set alone.",
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
					// The fee schedule arrives as a --fee-schedule JSON flag,
					// and autocli generates no description for a message-typed
					// one — so an operator meeting
					// "--fee-schedule blockchain.syndicate.v1.FeeSchedule (json)"
					// with nothing after it has to read the proto to find out
					// what to type. The example is here because it is the one
					// thing that command cannot be run without.
					//
					// It is called --fee-schedule and not --fees because
					// --fees is the SDK's transaction fee, present on every tx
					// command. A proto field named `fees` makes pflag define
					// one name twice, which panics while the root command is
					// built — killing the binary on `version` and on `start`,
					// not merely on this command. See tx.proto.
					Long: "Puts an allocation in front of the investor base.\n\n" +
						"The fee schedule is basis points, and the entry fees (sourcing plus " +
						"syndication) come out of the raise at the close while the management " +
						"fee comes off each distribution as it is paid:\n\n" +
						"  --fee-schedule '{\"sourcing_bps\":200,\"syndication_bps\":100," +
						"\"management_bps\":2000}'\n\n" +
						"Both are capped by governance; see `query syndicate params`.",
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
					RpcMethod: "AttachDocument",
					// Deliberately no positional args. An operator is a group account
					// by construction, so this message is submitted inside a group
					// proposal as JSON rather than typed as a transaction, and the
					// command exists mainly so that --help documents the fields
					// somebody has to write into that JSON.
					Use:   "attach-document",
					Short: "Records the digest of a paper this deal depends on. Never the paper itself",
					Long: "Records the SHA-256 digest of a document \u2014 term sheet, subscription " +
						"agreement, shareholders' agreement, accounts, exit resolution \u2014 so that " +
						"a file produced later can be proved to be, or not to be, the one that " +
						"was agreed.\n\n" +
						"The document itself is never sent. A subscription agreement names its " +
						"investor and their cheque and a diligence pack holds a private " +
						"company's accounts; either on a public chain is a permanent disclosure " +
						"made on behalf of people who did not consent to it.\n\n" +
						"The operator is a group, so this goes in as a proposal:\n\n" +
						"  {\"@type\":\"/blockchain.syndicate.v1.MsgAttachDocument\",\n" +
						"   \"operator\":\"<operator group>\",\"deal_id\":\"3\",\n" +
						"   \"kind\":\"DOCUMENT_KIND_SUBSCRIPTION_AGREEMENT\",\n" +
						"   \"title\":\"Subscription agreement (EN)\",\n" +
						"   \"sha256\":\"<base64 of the 32 raw digest bytes>\",\n" +
						"   \"uri\":\"ipfs://...\",\"language\":\"en\",\"supersedes\":\"0\"}\n\n" +
						"Proposal JSON carries bytes as base64, not hex:\n\n" +
						"  sha256sum -b doc.pdf | cut -d' ' -f1 | xxd -r -p | base64 -w0",
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
