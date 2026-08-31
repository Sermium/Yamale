package tokenisation

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/tokenisation/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
//
// This module went without one for longer than it should have, and the effect
// was not that the commands were awkward — it was that thirteen messages had no
// caller at all. A module reachable only over raw gRPC is a module nobody
// operates, and one nobody operates is one whose refusals have never been seen
// by the people relying on them.
//
// Two messages are skipped rather than exposed, and both for the same reason as
// x/land: an authority that could be handed out from a terminal is not an
// authority. Everything else is here, including the three that lose somebody
// money if they are wrong — Fractionalise, ReportSale and Redeem — because those
// are exactly the ones an operator needs to be able to rehearse.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the dispute bond and the sale-attestation settings now in force",
				},
				{
					RpcMethod: "Collections",
					Use:       "collections",
					Short:     "Lists the vehicles' collections: who administers each, and how a sale is verified",
					Long: "Lists the collections on this chain.\n\n" +
						"A collection is the frame a class of vehicles is issued under — who may\n" +
						"mint into it, how many attestations a reported sale needs, and how long a\n" +
						"challenge window runs. Read it before believing a price reported under it:\n" +
						"a collection that verifies nothing produces prices nobody checked.",
				},
				{
					RpcMethod:      "Assets",
					Use:            "assets [collection-id]",
					Short:          "Lists the assets minted into one collection",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "collection_id"}},
				},
				{
					RpcMethod: "Asset",
					Use:       "asset [asset-id]",
					Short:     "Shows one asset with its vault balance and any sale reported against it",
					Long: "Shows one asset: the title, the vault, and the sale.\n\n" +
						"The three are returned together on purpose. An asset read without its vault\n" +
						"says nothing about whether the income it promises has actually arrived, and\n" +
						"one read without its sale report hides the number that decides what every\n" +
						"holder is owed.",
					Alias:          []string{"show-asset"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "asset_id"}},
				},
				{
					RpcMethod: "Entitlement",
					Use:       "entitlement [asset-id] [holder]",
					Short:     "Shows what one holder may claim from an asset's vault right now",
					Long: "Shows what one holder may claim from an asset's vault right now.\n\n" +
						"This is the question a shareholder actually has, and it is not answerable\n" +
						"from a token balance alone: the entitlement depends on what has been paid\n" +
						"into the vault, what this holder has already claimed, and the fraction they\n" +
						"hold at this moment.",
					Example: "blockchaind query tokenisation entitlement 3 yml1holder...",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"},
						{ProtoField: "holder"},
					},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              types.Msg_serviceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // skipped because authority gated
				},
				{
					// The one message that can overwrite a price the market has
					// already been told. It belongs to the collection's authority
					// acting on a finding, not to a flag somebody can reach for
					// while a dispute is still an argument.
					RpcMethod: "ResolveDispute",
					Skip:      true,
				},
				{
					RpcMethod: "CreateCollection",
					Use:       "create-collection",
					Short:     "Open a collection: who may mint into it, and how a sale reported under it is verified",
					Long: "Open a collection.\n\n" +
						"--from must be the account that will administer it. The collection is passed\n" +
						"as one JSON object because its fields are a single decision about how much\n" +
						"verification this class of vehicle carries, and splitting them into flags\n" +
						"invites setting one and forgetting the rest.\n\n" +
						"  id                        the collection's handle, unique on this chain\n" +
						"  authority                 the account that may mint and set the authority\n" +
						"  verification              how a reported sale is checked\n" +
						"  attestation_threshold     independent attestations a sale needs\n" +
						"  challenge_window_seconds  how long a reported sale can be disputed\n" +
						"  dispute_bond_bps          what a challenger stakes, in basis points\n\n" +
						"A threshold of zero with a window of zero is a collection where whoever\n" +
						"reports a sale sets the price unopposed. That is a legitimate choice for a\n" +
						"vehicle whose holders are all in one room, and a disastrous one for a\n" +
						"vehicle sold to the public.",
					Example: "blockchaind tx tokenisation create-collection --from registry-kinshasa \\\n" +
						"  --collection '{\"id\":\"ke-farmland\",\"authority\":\"yml1office...\"," +
						"\"attestation_threshold\":3,\"challenge_window_seconds\":604800,\"dispute_bond_bps\":500}'",
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"collection": {Name: "collection", Usage: "the collection as a JSON object; see the long help for its fields"},
					},
				},
				{
					RpcMethod: "SetCollectionAuthority",
					Use:       "set-collection-authority [collection-id] [new-authority]",
					Short:     "Hand a collection's administration to another account",
					Long: "Hand a collection's administration to another account.\n\n" +
						"--from must be the current authority. There is no confirmation step and no\n" +
						"way back: the new authority is the only account that can hand it on again.\n" +
						"Where the collection matters, the destination should be an x/group account\n" +
						"rather than a person, so that losing one key is not losing the collection.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "collection_id"},
						{ProtoField: "new_authority"},
					},
				},
				{
					RpcMethod: "SetCollectionAttestors",
					Use:       "set-collection-attestors [collection-id] [attestors]",
					Short:     "Appoint the accounts that may attest a sale reported under this collection",
					Long: "Appoint the accounts that may attest a sale reported under this collection.\n\n" +
						"Governance only, and that is the whole of why it works: if the seller could\n" +
						"appoint the accounts that check the seller, the register would restate the\n" +
						"problem rather than fix it.\n\n" +
						"attestors is a comma-separated list. It REPLACES the register rather than\n" +
						"adding to it, and it must hold at least attestation_threshold distinct\n" +
						"accounts — a threshold higher than the register is one no honest sale can\n" +
						"ever meet, which turns every vehicle in the collection into a one-way door.\n\n" +
						"Attestations already recorded against a reported sale are not revisited.\n" +
						"They were made by an appointed attestor at the time, and rewriting that to\n" +
						"match a later appointment would let governance manufacture or destroy a\n" +
						"quorum after the fact.",
					Example: "blockchaind tx tokenisation set-collection-attestors ke-farmland \n\n" +
						"  yml1auditor...,yml1registry...,yml1notary... --from gov",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "collection_id"},
						{ProtoField: "attestors", Varargs: true},
					},
				},
				{
					RpcMethod: "MintAsset",
					Use:       "mint-asset [collection-id] [owner] [uri]",
					Short:     "Mint the title of one vehicle, optionally bound to a registered parcel",
					Long: "Mint the title of one vehicle.\n\n" +
						"--from must be the collection's authority. The asset is the title: one\n" +
						"indivisible thing, owned by one account, which is what the fraction tokens\n" +
						"issued later refer back to.\n\n" +
						"--parcel-id binds the vehicle to a parcel in x/land. When it is set, the\n" +
						"chain checks that parcel carries a live fractionalisation authorisation\n" +
						"before it will let the asset be fractionalised, and the share ceiling the\n" +
						"registry set is enforced against the supply. Left unset, the asset is a\n" +
						"vehicle over something this chain does not register, and nothing is checked\n" +
						"for you.\n\n" +
						"uri points at the offering document. The chain stores the pointer, never the\n" +
						"document.",
					Example: "blockchaind tx tokenisation mint-asset ke-farmland yml1sponsor... \\\n" +
						"  https://vehicle.example/ke-0007 --parcel-id 7 --from registry-kinshasa",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "collection_id"},
						{ProtoField: "owner"},
						{ProtoField: "uri"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"parcel_id": {Name: "parcel-id", Usage: "the x/land parcel this vehicle is over; the registry's authorisation is then enforced"},
					},
				},
				{
					RpcMethod: "Fractionalise",
					Use:       "fractionalise [asset-id] [symbol] [supply]",
					Short:     "Issue the fixed shareholding over an asset — once, and never again",
					Long: "Issue the fixed shareholding over an asset.\n\n" +
						"--from must be the asset's owner. This is a closed-end vehicle: the supply is\n" +
						"fixed here and there is no second issuance, so a holder's percentage cannot\n" +
						"be diluted afterwards by anybody, including the sponsor.\n\n" +
						"--holder-share-bps is the share of the asset's economics THE TOKENS CARRY,\n" +
						"in basis points, and the sponsor keeps the rest. 4000 sells 40% and retains\n" +
						"60%. It is what is being SOLD, not what is retained, and it can never be\n" +
						"changed afterwards. Where the asset is bound to a parcel,\n" +
						"the registry's max_share_bps caps what may be issued, and this message is\n" +
						"refused outright if the authorisation has been withdrawn, has expired, or\n" +
						"the parcel has since acquired a no_fractionalisation restriction.\n\n" +
						"--income-denom is the currency the vault will pay in. Holders are paid in\n" +
						"it and in nothing else, so it should be the currency the underlying actually\n" +
						"earns.\n\n" +
						"supply is an integer count of shares, not an amount of money.",
					Example: "blockchaind tx tokenisation fractionalise 3 KEFARM 1000000 \\\n" +
						"  --holder-share-bps 4000 --income-denom uKES --from sponsor",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"},
						{ProtoField: "symbol"},
						{ProtoField: "supply"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"holder_share_bps": {Name: "holder-share-bps", Usage: "share of the economics the tokens carry, in basis points (4000 sells 40%); the sponsor keeps the rest"},
						"income_denom":     {Name: "income-denom", Usage: "the currency the vault pays holders in"},
					},
				},
				{
					RpcMethod: "TransferAsset",
					Use:       "transfer-asset [asset-id] [recipient]",
					Short:     "Move the title of a vehicle to another account",
					Long: "Move the title of a vehicle to another account.\n\n" +
						"--from must be the current owner. This moves the title only. The fraction\n" +
						"tokens are ordinary balances and stay exactly where they are, which is the\n" +
						"point: selling the vehicle does not sell its shareholders' shares out from\n" +
						"under them.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"},
						{ProtoField: "recipient"},
					},
				},
				{
					RpcMethod: "FundVault",
					Use:       "fund-vault [asset-id] [amount]",
					Short:     "Pay income into an asset's vault, where its holders can claim it",
					Long: "Pay income into an asset's vault.\n\n" +
						"Open to anybody — rent, a harvest, a dividend, whoever is actually paying.\n" +
						"The amount must be in the asset's income denomination, and it becomes\n" +
						"claimable by holders in proportion to what they hold.\n\n" +
						"Money paid in here is not the sponsor's any more. It leaves the vault only\n" +
						"through claim and redeem.",
					Example: "blockchaind tx tokenisation fund-vault 3 480000uKES --from tenant",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"},
						{ProtoField: "amount"},
					},
				},
				{
					RpcMethod: "ReportSale",
					Use:       "report-sale [asset-id] [price]",
					Short:     "Report what the underlying sold for — the number that decides every payout",
					Long: "Report what the underlying sold for.\n\n" +
						"This is where a closed-end vehicle is stolen from, and it is worth being\n" +
						"blunt about how. Every holder is paid out against this price. A sponsor who\n" +
						"reports a sale below what was really received keeps the difference and pays\n" +
						"the shareholders a proportion of a lie. Nothing about the token supply, the\n" +
						"vault or the title prevents that — the only thing that does is the\n" +
						"attestation threshold and the challenge window on the collection.\n\n" +
						"So: --evidence-uri is not decoration. It is what an attestor reads before\n" +
						"attesting and what a challenger cites when disputing.",
					Example: "blockchaind tx tokenisation report-sale 3 82000000uKES \\\n" +
						"  --evidence-uri https://registry.example/sale/2026-114 --from sponsor",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"},
						{ProtoField: "price"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"evidence_uri": {Name: "evidence-uri", Usage: "where the proof of what was received can be read"},
					},
				},
				{
					RpcMethod: "AttestSale",
					Use:       "attest-sale [asset-id] [price]",
					Short:     "Independently confirm a reported sale price, moving it toward its threshold",
					Long: "Independently confirm a reported sale price.\n\n" +
						"The price is repeated here rather than implied, so an attestor states a\n" +
						"number instead of endorsing one. Attesting to a price that differs from the\n" +
						"report is a refusal, not an amendment.\n\n" +
						"The collection decides how many independent attestations are needed before\n" +
						"the sale can finalise — read it with `query tokenisation collections`.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"},
						{ProtoField: "price"},
					},
				},
				{
					RpcMethod: "DisputeSale",
					Use:       "dispute-sale [asset-id] [reason]",
					Short:     "Challenge a reported sale price within its window, staking the collection's bond",
					Long: "Challenge a reported sale price within its window.\n\n" +
						"The challenger stakes the collection's dispute bond, in basis points of the\n" +
						"reported price. The bond exists because a free challenge is one a competitor\n" +
						"files for the delay alone; it is also why the bond should not be set so high\n" +
						"that only the sponsor can afford to object to the sponsor.\n\n" +
						"A dispute stops the payout and hands the price to the collection's authority\n" +
						"to resolve.",
					Example: "blockchaind tx tokenisation dispute-sale 3 \\\n" +
						"  \"the deed filed with the registry shows 96,000,000 KES\" --from holder",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "FinaliseSale",
					Use:       "finalise-sale [asset-id]",
					Short:     "Open redemption on a sale that has cleared its attestations and its window",
					Long: "Open redemption on a sale that has cleared its attestations and its window.\n\n" +
						"Anybody may send it, and that is deliberate rather than lax. Every condition\n" +
						"it checks is already fixed by the report and the clock, so the sender decides\n" +
						"nothing and contributes only the gas. If only the sponsor could finalise, a\n" +
						"sponsor could decline to — and shareholders unable to exit until the party\n" +
						"holding their money allows it is the position this instrument exists to\n" +
						"avoid.\n\n" +
						"A failure here names the condition not yet met: still inside the window,\n" +
						"short of its attestation threshold, or under dispute.\n\n" +
						"This message did not exist until 2026-08-27. The crank it calls was written\n" +
						"and never wired to anything, so no asset could reach REALISED and no holder\n" +
						"could ever redeem.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "asset_id"}},
				},
				{
					RpcMethod: "Claim",
					Use:       "claim [asset-id]",
					Short:     "Take the income owed to you from an asset's vault, without giving up your shares",
					Long: "Take the income owed to you from an asset's vault.\n\n" +
						"--from must hold the fraction tokens. This pays out what has accrued and\n" +
						"leaves the shares where they are, so it can be called as often as the vault\n" +
						"is funded. Check the amount first with `query tokenisation entitlement`.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "asset_id"}},
				},
				{
					RpcMethod: "Redeem",
					Use:       "redeem [asset-id] [amount]",
					Short:     "Burn shares for your part of the sale proceeds — this ends your holding",
					Long: "Burn shares for your part of the sale proceeds.\n\n" +
						"--from must hold the fraction tokens, and the shares are destroyed: this is\n" +
						"the exit, not a withdrawal. It pays against the sale price that finalised,\n" +
						"so a price still inside its challenge window, or under dispute, is not yet\n" +
						"redeemable — and that refusal is the protection, not an obstacle.\n\n" +
						"amount is a count of shares. Redeeming part of a holding is allowed.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "asset_id"},
						{ProtoField: "amount"},
					},
				},
			},
		},
	}
}
