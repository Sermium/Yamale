package land

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/land/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the attestation quorum and the challenge window now in force",
				},
				{
					RpcMethod:      "Parcel",
					Use:            "parcel [id]",
					Short:          "Shows one parcel: its holder, status, encumbrances, restrictions and deeds",
					Alias:          []string{"show-parcel"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod: "ParcelByRef",
					Use:       "parcel-by-ref [cadastral-ref]",
					Short:     "Finds a parcel by the reference written on the paper somebody is holding",
					Long: "Finds a parcel by the reference written on the paper somebody is holding.\n\n" +
						"cadastral-ref is the registry's own human reference, not the chain id. This is\n" +
						"the lookup a citizen at a counter can actually perform, because it is the only\n" +
						"number on their document.",
					Example:        "blockchaind query land parcel-by-ref YM-KIN-2024-00187",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "cadastral_ref"}},
				},
				{
					RpcMethod: "ParcelByGeometry",
					Use:       "parcel-by-geometry [geometry-hash]",
					Short:     "Shows which parcel already holds a survey, if any — run this before registering",
					Long: "Shows which parcel already holds a survey, if any.\n\n" +
						"geometry-hash is the hash of the surveyed boundary (the GeoJSON or the\n" +
						"cadastral document), not the document itself. It is the uniqueness constraint:\n" +
						"registration fails if this hash is already titled, so a surveyor should ask\n" +
						"here first rather than discover it in a failed transaction.\n\n" +
						"An empty result means the ground is untitled on this chain.",
					Example:        "blockchaind query land parcel-by-geometry 9f2b1c...e04a",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "geometry_hash"}},
				},
				{
					RpcMethod:      "Transfer",
					Use:            "transfer [id]",
					Short:          "Shows one transfer: who proposed it, who validated, who attested, who objected",
					Alias:          []string{"show-transfer"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}},
				},
				{
					RpcMethod: "TransfersByParcel",
					Use:       "transfers-by-parcel [parcel-id]",
					Short:     "Lists every transfer ever proposed for a parcel, including abandoned and disputed ones",
					Long: "Lists every transfer ever proposed for a parcel, including abandoned and\n" +
						"disputed ones.\n\n" +
						"This is the whole chain of title, kept rather than pruned. It is the history\n" +
						"that makes a theft arguable afterwards, so read it before believing a title.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "parcel_id"}},
				},
				{
					RpcMethod: "PendingTransfers",
					Use:       "pending-transfers",
					Short:     "Lists transfers awaiting completion — the ones an objection can still stop",
				},
				{
					RpcMethod: "Authorities",
					Use:       "authorities",
					Short:     "Lists the registry offices governance has admitted, and their jurisdictions",
				},
				{
					RpcMethod:      "ParcelsByHolder",
					Use:            "parcels-by-holder [holder]",
					Short:          "Lists the parcels one account holds",
					Alias:          []string{"holdings"},
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "holder"}},
				},
				{
					RpcMethod: "FractionalisationAuthority",
					Use:       "fractionalisation-authority [parcel-id]",
					Short:     "Shows whether shares may lawfully be sold over a parcel, and up to what fraction",
					Long: "Shows whether shares may lawfully be sold over a parcel, and up to what\n" +
						"fraction.\n\n" +
						"This is the check to run before buying a share in somebody's land. `live`\n" +
						"is false when the office has withdrawn the permission, when it has run\n" +
						"out, or when the parcel now carries a restriction forbidding\n" +
						"fractionalisation — and x/tokenisation refuses to issue in each of those\n" +
						"cases, so a vehicle still selling shares against a permission that is not\n" +
						"live is selling something the chain will not mint.",
					Example:        "blockchaind query land fractionalisation-authority 7",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "parcel_id"}},
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
					// Skipped for the same reason, and it is the reason the module
					// works: an office that could admit another office would be able
					// to manufacture the independent attestors the quorum rests on.
					// Admission is a governance proposal.
					RpcMethod: "RegisterAuthority",
					Skip:      true,
				},
				{
					RpcMethod: "RegisterParcel",
					Use:       "register-parcel [geometry-hash] [cadastral-ref] [holder]",
					Short:     "Record a parcel for the first time, held by one account",
					Long: "Record a parcel for the first time, held by one account.\n\n" +
						"--from must be an active registry office, and the parcel falls in that\n" +
						"office's jurisdiction from here on. Offices are x/group accounts, so in\n" +
						"practice this is generated with --generate-only and submitted as a group\n" +
						"proposal rather than broadcast from a terminal.\n\n" +
						"geometry-hash is the hash of the surveyed boundary, not the survey — the\n" +
						"survey is too large for a block and often too sensitive to publish. It must\n" +
						"be unique, and that refusal is the whole \"cannot be owned twice\" guarantee.\n" +
						"cadastral-ref is the registry's own reference, and must also be unique.\n" +
						"holder is the account that will own it — a group account where the land is\n" +
						"held by a family or a co-operative.",
					Example: "blockchaind tx land register-parcel 9f2b1c...e04a YM-KIN-2024-00187 yml1holder... \\\n" +
						"  --from registry-kinshasa --generate-only",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "geometry_hash"},
						{ProtoField: "cadastral_ref"},
						{ProtoField: "holder"},
					},
				},
				{
					RpcMethod: "ProposeTransfer",
					Use:       "propose-transfer [parcel-id] [to] [price]",
					Short:     "Step 1 of 4: the holder opens a transfer of their own parcel",
					Long: "Step 1 of 4: the holder opens a transfer of their own parcel.\n\n" +
						"--from must be the current holder. An office cannot start the sale of\n" +
						"somebody's land, which is the first of the four separations. The parcel must\n" +
						"be REGISTERED: frozen, disputed and already-transferring parcels are stops.\n\n" +
						"price is the declared consideration, recorded for the audit trail only. The\n" +
						"chain does not move it — land paid for off-chain is the common case, and\n" +
						"pretending otherwise would make the record false.\n\n" +
						"The full flow, and it is four different signers by construction:\n" +
						"  1. propose-transfer   — the holder\n" +
						"  2. validate-transfer  — the office holding the parcel's file\n" +
						"  3. attest-transfer    — other offices, until quorum is reached\n" +
						"  4. complete-transfer  — anyone, once the challenge window has elapsed\n" +
						"Anyone at all may `object` between steps 1 and 4, which stops everything.",
					Example: "blockchaind tx land propose-transfer 7 yml1buyer... \"4200 USD\" --from seller",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "parcel_id"},
						{ProtoField: "to"},
						{ProtoField: "price", Optional: true},
					},
				},
				{
					RpcMethod: "ValidateTransfer",
					Use:       "validate-transfer [transfer-id]",
					Short:     "Step 2 of 4: the office holding the file confirms the seller is who they claim",
					Long: "Step 2 of 4: the office holding the file confirms the seller is who they claim.\n\n" +
						"--from must be the office whose jurisdiction the parcel falls in — the one\n" +
						"that can check the seller against paper the chain cannot see. Exactly one\n" +
						"validation is accepted, and it does not count toward the attestation quorum.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "transfer_id"}},
				},
				{
					RpcMethod: "AttestTransfer",
					Use:       "attest-transfer [transfer-id]",
					Short:     "Step 3 of 4: one independent office attests, moving the transfer toward quorum",
					Long: "Step 3 of 4: one independent office attests, moving the transfer toward quorum.\n\n" +
						"--from must be an active office that is NOT the one holding the parcel, unless\n" +
						"governance has set same_authority_attestation. An attestor from the same\n" +
						"office is not independent, and allowing it collapses a quorum of many offices\n" +
						"back into a single bribe. One office, one attestation.\n\n" +
						"The response reports attestations against the quorum. The challenge window\n" +
						"starts the moment quorum is reached, not at proposal — check `query land\n" +
						"params` for how long it runs.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "transfer_id"}},
				},
				{
					RpcMethod: "Object",
					Use:       "object [transfer-id] [reason]",
					Short:     "Stop a transfer and mark the parcel disputed — open to anyone, no standing needed",
					Long: "Stop a transfer and mark the parcel disputed.\n\n" +
						"Open to anyone on purpose: somebody whose land is being sold from under them\n" +
						"usually has no official relationship to prove, and requiring standing would\n" +
						"exclude exactly the people this protects.\n\n" +
						"One objection is enough and it is terminal for the transfer. The chain\n" +
						"preserves the evidence and does not adjudicate — deciding is a court's job.",
					Example: "blockchaind tx land object 12 \"the seller died in 2019; the succession is before the court\" \\\n" +
						"  --from anyone",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "transfer_id"},
						{ProtoField: "reason"},
					},
				},
				{
					RpcMethod: "CompleteTransfer",
					Use:       "complete-transfer [transfer-id]",
					Short:     "Step 4 of 4: apply a transfer that has met every condition — anyone may send this",
					Long: "Step 4 of 4: apply a transfer that has met every condition.\n\n" +
						"Callable by anyone, and it is mechanical: validated, quorum reached, challenge\n" +
						"window elapsed, no objection. If only an official could finalise, an official\n" +
						"could refuse to — and a refusal that costs a seller their sale is leverage\n" +
						"worth paying to remove.\n\n" +
						"A failure here is information, not a bug: it names the condition not yet met.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "transfer_id"}},
				},
				{
					RpcMethod: "RecordEncumbrance",
					Use:       "record-encumbrance [parcel-id]",
					Short:     "Record a mortgage, lien or right of way against a parcel, or release one",
					Long: "Record a mortgage, lien or right of way against a parcel, or release one.\n\n" +
						"--from must be the office in charge of the parcel. Two modes, hence the flags:\n" +
						"recording needs --kind and --holder; releasing needs --release and the --index\n" +
						"of the existing entry, as shown by `query land parcel [id]`.\n\n" +
						"A release marks the entry rather than deleting it. An encumbrance that\n" +
						"vanishes takes with it the evidence that it ever constrained the title, and a\n" +
						"title shown without its encumbrances is a lie that gets somebody's house\n" +
						"taken.",
					Example: "blockchaind tx land record-encumbrance 7 --kind mortgage --holder yml1bank... \\\n" +
						"  --detail \"12,000 USD over 5 years\" --from registry-kinshasa\n" +
						"blockchaind tx land record-encumbrance 7 --release --index 0 --from registry-kinshasa",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "parcel_id"}},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"kind":    {Name: "kind", Usage: "mortgage, lien, right-of-way or caveat"},
						"holder":  {Name: "holder", Usage: "the account the claim runs in favour of"},
						"detail":  {Name: "detail", Usage: "the terms, in words the owner can read"},
						"release": {Name: "release", Usage: "release the entry at --index instead of recording a new one"},
						"index":   {Name: "index", Usage: "position of the encumbrance to release, as numbered by query land parcel"},
					},
				},
				{
					RpcMethod: "FreezeParcel",
					Use:       "freeze-parcel [parcel-id] [reason]",
					Short:     "Stop all movement of a parcel, or lift a freeze with --unfreeze",
					Long: "Stop all movement of a parcel, or lift a freeze with --unfreeze.\n\n" +
						"--from must be the office in charge of the parcel, and offices are x/group\n" +
						"accounts, so the freeze already requires the office's own M-of-N. A single\n" +
						"official who can freeze land can extort its owner.\n\n" +
						"reason is required when freezing and optional when lifting, and both are\n" +
						"recorded on the parcel — `query land parcel [id]` shows every freeze it has\n" +
						"ever carried, who imposed it, and who lifted it. A stop whose grounds nobody\n" +
						"can read is one nobody can argue with, which is the point of writing it down.",
					Example: "blockchaind tx land freeze-parcel 7 \"court order 2026/114, fraud inquiry\" --from registry-kinshasa\n" +
						"blockchaind tx land freeze-parcel 7 \"inquiry closed, no finding\" --unfreeze --from registry-kinshasa",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "parcel_id"},
						{ProtoField: "reason", Optional: true},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"unfreeze": {Name: "unfreeze", Usage: "lift an existing freeze and return the parcel to REGISTERED"},
					},
				},
				{
					RpcMethod: "AttachDeed",
					Use:       "attach-deed [parcel-id] [kind] [document-hash]",
					Short:     "Add a document to a parcel's chain of title",
					Long: "Add a document to a parcel's chain of title.\n\n" +
						"--from must be the office in charge of the parcel.\n\n" +
						"kind is what the paper is: grant, sale, inheritance, court order, survey.\n" +
						"document-hash is the hash of the document the registry holds, never the\n" +
						"document — a scan of a 1974 grant is megabytes and usually carries somebody's\n" +
						"personal details. The chain carries the hash and a pointer; the registry\n" +
						"serves the document to whoever is entitled to read it.",
					Example: "blockchaind tx land attach-deed 7 inheritance 4c1f...9ab2 \\\n" +
						"  --uri https://registry.example/deeds/4c1f --reference ACT-1974-221 \\\n" +
						"  --issued-on 1974-06-02 --from registry-kinshasa",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "parcel_id"},
						{ProtoField: "kind"},
						{ProtoField: "document_hash"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"uri":       {Name: "uri", Usage: "where the registry serves the document from"},
						"reference": {Name: "reference", Usage: "the registry's own reference for the document"},
						"issued_on": {Name: "issued-on", Usage: "the document's date in the paper world"},
					},
				},
				{
					RpcMethod: "SetRestriction",
					Use:       "set-restriction [parcel-id]",
					Short:     "Impose a limit on what may be done with a parcel, or lift one",
					Long: "Impose a limit on what may be done with a parcel, or lift one.\n\n" +
						"--from must be the office in charge of the parcel. Two modes, hence the flags:\n" +
						"imposing needs --kind; lifting needs --lift and the --index of the existing\n" +
						"restriction, as shown by `query land parcel [id]`.\n\n" +
						"Known kinds: agricultural_use_only, no_fractionalisation,\n" +
						"foreign_ownership_capped, heritage_protected, minimum_parcel_size,\n" +
						"customary_tenure. They are data rather than code because land law differs by\n" +
						"country, and a chain that hard-codes one country's law is a chain only that\n" +
						"country can use. --value carries the limit where one applies: a percentage\n" +
						"cap, a minimum size.\n\n" +
						"Lifting marks the restriction rather than removing it, so the record still\n" +
						"shows the land was once constrained and which office released it.",
					Example: "blockchaind tx land set-restriction 7 --kind foreign_ownership_capped --value 4900 \\\n" +
						"  --detail \"49% ceiling on non-resident holding\" --from registry-kinshasa\n" +
						"blockchaind tx land set-restriction 7 --lift --index 2 --from registry-kinshasa",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "parcel_id"}},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"kind":   {Name: "kind", Usage: "the restriction, e.g. agricultural_use_only or heritage_protected"},
						"value":  {Name: "value", Usage: "the limit itself where one applies, e.g. a basis-point cap"},
						"detail": {Name: "detail", Usage: "the grounds, in words the owner can argue with"},
						"lift":   {Name: "lift", Usage: "lift the restriction at --index instead of imposing a new one"},
						"index":  {Name: "index", Usage: "position of the restriction to lift, as numbered by query land parcel"},
					},
				},
				{
					RpcMethod: "AuthoriseFractionalisation",
					Use:       "authorise-fractionalisation [parcel-id]",
					Short:     "Permit a tokenisation vehicle over a parcel, up to a share ceiling and an expiry",
					Long: "Permit a tokenisation vehicle over a parcel, up to a share ceiling and an expiry.\n\n" +
						"--from must be the office in charge of the parcel, and the parcel must be\n" +
						"REGISTERED. Without a live authorisation x/tokenisation refuses to open a\n" +
						"vehicle. A no_fractionalisation restriction outranks this permission.\n\n" +
						"--right names what may be sold: an exploitation right, a lease, a revenue\n" +
						"share. Not the title — the title stays in this module, held by the same\n" +
						"account, and the vehicle sells rights that reference it. That separation is\n" +
						"what keeps a fractionalised parcel governed by the registry rather than by\n" +
						"whoever accumulated the tokens.\n\n" +
						"--max-share-bps is the ceiling on the fraction that may be issued, in basis\n" +
						"points: 6000 means 60%, and 10000 is the maximum accepted. It caps the share\n" +
						"the issued tokens carry, not the share the holder keeps back.\n\n" +
						"--expires-at is a Unix timestamp in seconds and is required: it must be in\n" +
						"the future, because an authorisation with no expiry is one that sits open\n" +
						"for years, which is the thing this field exists to prevent.\n\n" +
						"--withdraw stops new issuance. It does not expropriate existing holders — that\n" +
						"is a taking, and it belongs to a court.",
					Example: "blockchaind tx land authorise-fractionalisation 7 --right exploitation \\\n" +
						"  --max-share-bps 6000 --expires-at 1790000000 --from registry-kinshasa\n" +
						"blockchaind tx land authorise-fractionalisation 7 --withdraw --from registry-kinshasa",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "parcel_id"}},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"right":         {Name: "right", Usage: "what may be sold in shares: an exploitation right, a lease, a revenue share"},
						"max_share_bps": {Name: "max-share-bps", Usage: "ceiling on the fraction that may be issued, in basis points (6000 = 60%)"},
						"expires_at":    {Name: "expires-at", Usage: "Unix seconds after which the authorisation no longer stands; required, and must be in the future"},
						"withdraw":      {Name: "withdraw", Usage: "withdraw the authorisation, stopping new issuance"},
					},
				},
			},
		},
	}
}
