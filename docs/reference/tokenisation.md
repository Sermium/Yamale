<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/tokenisation

## Transactions

### MsgAttestSale

`/blockchain.tokenisation.v1.MsgAttestSale`

Signed by the `attestor` field.

MsgAttestSale signs a reported figure when the collection requires attestors.

This used to sit under "permissionless", which made the attestation threshold meetable by anybody and therefore no threshold at all.

| Field | Type | Description |
| --- | --- | --- |
| `attestor` | string |  |
| `asset_id` | uint64 |  |
| `price` | Coin |  |

### MsgClaim

`/blockchain.tokenisation.v1.MsgClaim`

Signed by the `holder` field.

MsgClaim withdraws accrued income without giving up the shareholding.

It pays a DIVIDEND TOKEN -- div/<asset_id>/<SYMBOL>, one per vehicle -- rather than the stable itself, and the stable it stands for is moved out of the vehicle's spendable ledger into its dividend reserve in the same write. MsgExchangeDividend converts it back, 1:1, whenever the holder can take it.

#### Why a token at all, rather than just paying the stable

Three things it buys, and one it deliberately does not.

1. A holder who cannot receive the stable can still be PAID. A stable on this chain may carry its own send restriction -- a KYC gate, a sanctions list, a licence the holder does not have where they live. Paying such a holder directly either fails the whole claim, which leaves them accruing forever against a vehicle that has done nothing wrong, or succeeds into an account that cannot move the money on. The dividend token is issued by this module and gated by nothing, so the entitlement always lands; cashing it is a separate act that can happen later, elsewhere, or through an assignee who is permitted to hold the stable.

2. The claim becomes AUDITABLE SEPARATELY FROM THE SETTLEMENT. "Declared and not yet cashed", per vehicle, used to be a figure that existed only spread across every holder's accrued balance and could not be read without walking the whole holder set. As a token supply it is one query, it is public, and anybody can check it against the reserve without trusting this module's own accounting.

3. It separates WHEN a holder is credited from WHEN they are paid, which is what an operating business does anyway. A holder can claim on the record date and cash months later, without the chain holding an unattributed balance in the meantime.

And the thing it deliberately does NOT buy: credit. A dividend token is never issued against a declaration that has not been funded. It is fully backed at the instant it is minted or it is not minted -- see Vault.dividend_reserve. The tempting version of this feature is a token a vehicle can issue when it has DECLARED but not yet funded, so holders hold something in the meantime. That version is refused here: a token mintable against an unfunded declaration is a claim the chain cannot honour, and the first holder to reach the exchange would be paid out of a neighbouring vehicle's reserve. What a vehicle publishes before the money arrives is a COVENANT_KIND_DIVIDEND_NOTICE -- a filing, which is honest about being an announcement.

#### Transferable, deliberately

The dividend token is an ordinary bank denomination and this module places no restriction on it. If it could not move it would be a ledger row with extra steps -- Position.accrued already is one -- and reason 1 above would collapse with it: a holder who cannot hold the stable has to be able to ASSIGN the claim to somebody who can.

The consequence, stated rather than hidden: a dividend token can be sold, and it can be sold below par by somebody who needs the money before they can reach the exchange. The chain neither prevents that nor could -- it is the same freedom that makes the token useful. What it does guarantee is that the discount is never justified by the backing: par is available from the reserve, in every status, to whoever holds the token.

Unlike the fraction token it is NOT halted at realisation and is not offered to the AMM's restricted-denom classifier. The fraction token halts because at realisation it becomes a claim on a fixed pot that a pool would still quote a stale price for. A dividend token is a claim on a fixed pot at a fixed 1:1 rate from the moment it exists and never changes, so there is no stale price for anybody to take.

| Field | Type | Description |
| --- | --- | --- |
| `holder` | string |  |
| `asset_id` | uint64 |  |

### MsgCreateCollection

`/blockchain.tokenisation.v1.MsgCreateCollection`

Signed by the `authority` field.

MsgCreateCollection brings a registry into existence. Governance only.

There is deliberately no permissionless counterpart. Every other creation path on this chain is apply-then-approve, but here there is no application to approve: a registry of deeds is not something a chain grants on request.

Governance only, because a collection decides how a sale is verified and who may mint into it.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `collection` | Collection |  |

### MsgDisputeSale

`/blockchain.tokenisation.v1.MsgDisputeSale`

Signed by the `challenger` field.

MsgDisputeSale suspends redemption and refers the figure to governance.

The bond is a basis-point fraction of the reported price -- scaled to the vehicle rather than flat, since a flat bond is trivial for a large fraud to post and prohibitive for a small holder to raise. Refunded if the dispute succeeds; otherwise forfeited to the vault, never to the issuer. Paying the issuer would reward provoking weak challenges.

Permissionless.

| Field | Type | Description |
| --- | --- | --- |
| `challenger` | string |  |
| `asset_id` | uint64 |  |
| `reason` | string |  |

### MsgExchangeDividend

`/blockchain.tokenisation.v1.MsgExchangeDividend`

Signed by the `holder` field.

MsgExchangeDividend cashes a dividend token for the stable behind it, 1:1.

#### Permissionless, and refusable on nothing but the arithmetic

Whoever holds the token exchanges it. There is no role, no admin, no pause and no status check, because by the time a dividend token exists the chain has already taken the stable out of the vehicle's spendable ledger and set it aside under this token's name. Refusing to honour it afterwards would be the chain defaulting on a payment it has already reserved the money for.

That is also why it stays open at STATUS_REALISED and at STATUS_CLOSED, when the fraction token can no longer move at all. Trading halts because a share in a sold vehicle is a claim on a fixed pot a market would misprice; a dividend token is a debt that crystallised before the sale and has nothing to do with what the vehicle fetched. A holder who claimed in March and had not got round to exchanging by the time the warehouse sold in September is paid in full in October, out of money that was never available to redemption.

The exchange is NOT routed through x/treasury. See the keeper for why.

| Field | Type | Description |
| --- | --- | --- |
| `holder` | string | Whoever holds the tokens -- not necessarily the shareholder who claimed them, because the dividend token is transferable on purpose. |
| `asset_id` | uint64 |  |
| `amount` | string | How many to cash. Named explicitly rather than inferred from the balance, so a holder cashing part of a position does not have to move the rest out of the way first. |

### MsgFinaliseSale

`/blockchain.tokenisation.v1.MsgFinaliseSale`

Signed by the `caller` field.

MsgFinaliseSale opens redemption on an asset whose reported price has cleared verification and its challenge window.

Anybody may send it, and that is deliberate rather than lax. Every condition it checks is already fixed by the report and the clock, so the sender decides nothing; what they contribute is the gas. If only the sponsor could finalise, a sponsor could decline to — and shareholders unable to exit until the party holding their money chooses to let them is the position this whole instrument exists to avoid.

A crank rather than an EndBlocker sweep, because iterating every reported sale on every block is a denial-of-service surface that grows with usage.

FinaliseSale is the permissionless crank that opens redemption, once the window has passed and the holders' share has actually been paid in.

| Field | Type | Description |
| --- | --- | --- |
| `caller` | string |  |
| `asset_id` | uint64 |  |

### MsgFractionalise

`/blockchain.tokenisation.v1.MsgFractionalise`

Signed by the `owner` field.

MsgFractionalise mints the shareholding against title.

Supply is fixed here and can never grow. Holding title confers no right to mint more: an owner who could dilute shareholders by issuing fractions against an asset they had already sold interests in is the whole fraud in one message handler.

Title holder.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string |  |
| `asset_id` | uint64 |  |
| `symbol` | string |  |
| `supply` | string |  |
| `holder_share_bps` | uint32 | Share of the asset's economics the tokens carry. The sponsor keeps the rest. Never editable afterwards. For an asset over an x/land parcel this is the figure the registry's max_share_bps caps: it is what is being sold, not what is retained. |
| `income_denom` | string | Denomination income and proceeds will be paid in. |

### MsgFundVault

`/blockchain.tokenisation.v1.MsgFundVault`

Signed by the `funder` field.

MsgFundVault pays income in. holder_share_bps of it reaches the index; the remainder is the sponsor's and is never taken from them.

The holders' share is collected and the rest stays with the funder.

| Field | Type | Description |
| --- | --- | --- |
| `funder` | string |  |
| `asset_id` | uint64 |  |
| `amount` | Coin | The gross income for the period. The chain collects holder_share_bps of it and leaves the rest where it is, which is what the sentence above has always said and what the response reports. |

### MsgMintAsset

`/blockchain.tokenisation.v1.MsgMintAsset`

Signed by the `minter` field.

MsgMintAsset records title. Only the collection's appointed authority.

A mint names its recipient, so title is attributed at creation and never exists unattributed. There is no self-mint-then-transfer path, because that path is where an authority laundering assets to itself stops being distinguishable from an authority doing its job.

| Field | Type | Description |
| --- | --- | --- |
| `minter` | string |  |
| `collection_id` | string |  |
| `owner` | string |  |
| `uri` | string |  |
| `parcel_id` | uint64 | The x/land parcel this asset is title over, or 0 when it is not land. Checked here as well as at fractionalisation, because it is cheap: a mint naming a parcel that does not exist, or one held by somebody other than the named owner, is refused outright rather than left to fail later when shares are being sold and there is money on the other side of it. |
| `sector` | Sector | What the tokenised thing does. Optional only so that a mint written before this field existed still decodes; a mint that names no sector is accepted and the asset carries SECTOR_UNSPECIFIED, which an interface shows as "not stated" rather than guessing. |

### MsgPaySaleProceeds

`/blockchain.tokenisation.v1.MsgPaySaleProceeds`

Signed by the `payer` field.

MsgPaySaleProceeds pays the holders' share of a reported price into the vault, which is what finalisation waits on.

Anyone may pay: the obligation is the sponsor's, but a sponsor who has gone quiet should not be able to strand every holder, and money arriving is never the problem. Overpayment is refused rather than accepted and stranded.

| Field | Type | Description |
| --- | --- | --- |
| `payer` | string |  |
| `asset_id` | uint64 |  |
| `amount` | Coin |  |

### MsgPublishCovenant

`/blockchain.tokenisation.v1.MsgPublishCovenant`

Signed by the `authority` field.

MsgPublishCovenant files an undertaking against a tokenisation.

Signed by the collection authority: the party that answers for the vehicle. A holder able to publish against it could file an announcement in the operator's name, on a register whose value is that its filings are attributable.

Accepted in any status. Accounts, resolutions and material events arrive after a sale as readily as before one, and a register that stopped accepting them at realisation would be missing the filings a later dispute is about.

PublishCovenant files an undertaking against a tokenisation: accounts, a structure change, a dividend notice, an assembly. The digest, never the document.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `asset_id` | uint64 |  |
| `kind` | CovenantKind |  |
| `title` | string |  |
| `sha256` | bytes | Exactly 32 bytes. Not a hex string: two spellings of one digest that do not compare equal are worse than no digest. |
| `uri` | string |  |
| `period_start` | int64 | The period covered, unix seconds; both zero for a document covering a moment rather than a span. |
| `period_end` | int64 |  |
| `acts_by` | int64 | When a holder must have acted by. Zero when nothing is asked of them. |
| `supersedes` | uint64 | The covenant this corrects. Zero if it corrects nothing. Must belong to the same asset and must not already have been superseded -- two corrections of one filing leave two current versions and no rule for choosing. |

### MsgRecordSubscription

`/blockchain.tokenisation.v1.MsgRecordSubscription`

Signed by the `authority` field.

MsgRecordSubscription records what an investor paid for tokens they hold.

#### Why this message has to exist

A token balance says what an account holds and never what it paid. Tokens move by ordinary bank send, which carries no price, so without an explicit record there is nothing to compute a return against — and an interface then either shows no return at all, or invents an entry price, which is worse.

#### Who may sign it, and why it is not the investor

The collection's authority: the party that took the money. An investor able to state their own cost could state a low one and publish a flattering return beside a real balance, on a register whose whole value is that its figures are not the holder's word.

#### What it does NOT claim

That the money was received, or received in this denomination, or at arm's length. It records an assertion by a named, accountable party, dated and attributable — the same thing a KYC reference is, and worth the same: not proof, but evidence somebody can be asked about.

Additive. Recording a second subscription for the same holder adds to both the basis and the quantity, so an investor who subscribed twice at two prices carries a weighted average, which is what an average cost is.

RecordSubscription states what an investor paid for tokens they hold, so a return can be computed against something rather than assumed.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | The collection authority for the asset. Not the investor. |
| `asset_id` | uint64 |  |
| `investor` | string |  |
| `quantity` | string | Tokens this subscription covers. Refused above the investor's balance: a basis covering more tokens than exist in the account would report a return on tokens nobody holds. |
| `paid` | string | What was paid for them, in the vault's income denomination. The denomination is taken from the vault rather than named here, so a subscription cannot be recorded in a currency the vehicle does not pay in. |

### MsgRedeem

`/blockchain.tokenisation.v1.MsgRedeem`

Signed by the `holder` field.

MsgRedeem burns tokens and pays their share in the same step.

The burn *is* the claim. A design that burns and then expects a later claim strands the money of everyone slow, asleep or dead, and leaves the chain holding funds it can no longer attribute. Supply falls to zero as holders claim, and title burns when it reaches zero.

| Field | Type | Description |
| --- | --- | --- |
| `holder` | string |  |
| `asset_id` | uint64 |  |
| `amount` | string |  |

### MsgReportSale

`/blockchain.tokenisation.v1.MsgReportSale`

Signed by the `reporter` field.

MsgReportSale states what the asset sold for. It does not open redemption.

The figure is recorded, is public, and sits for the collection's challenge window. Verification alone would not be enough: redemption is irreversible, so without a window verification only means the theft needed two signatures.

| Field | Type | Description |
| --- | --- | --- |
| `reporter` | string |  |
| `asset_id` | uint64 |  |
| `price` | Coin |  |
| `evidence_uri` | string |  |

### MsgResolveDispute

`/blockchain.tokenisation.v1.MsgResolveDispute`

Signed by the `authority` field.

MsgResolveDispute is governance deciding the contested figure.

An empty corrected price upholds what was reported.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `asset_id` | uint64 |  |
| `corrected_price` | Coin | Empty upholds the reported figure and forfeits the bond. |

### MsgSetCollectionAttestors

`/blockchain.tokenisation.v1.MsgSetCollectionAttestors`

Signed by the `authority` field.

MsgSetCollectionAttestors replaces the register of accounts that may attest a sale reported under this collection.

Governance only, like CreateCollection and SetCollectionAuthority beside it. If the seller could appoint the accounts that check the seller, the register would restate the problem rather than fix it.

It replaces rather than appends, because a register that could only grow would let one careless proposal dilute a careful appointment for good.

Attestations already recorded against a reported sale are not revisited. They were made by an appointed attestor at the moment they were made, and rewriting history to match a later appointment would let an authority manufacture or destroy a quorum after the fact — which is a worse power than the one this message exists to constrain.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `collection_id` | string |  |
| `attestors` | repeated string |  |

### MsgSetCollectionAuthority

`/blockchain.tokenisation.v1.MsgSetCollectionAuthority`

Signed by the `authority` field.

MsgSetCollectionAuthority appoints or revokes who may mint into a collection.

Revocation stops future mints and touches nothing already issued. Deciding an existing asset was wrongly issued is a seizure, and seizures go through x/enforcement where two thirds decide -- not through a registry keeper.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `collection_id` | string |  |
| `new_authority` | string | Empty revokes. A collection with no authority refuses mints; it does not fall back to governance. |

### MsgTransferAsset

`/blockchain.tokenisation.v1.MsgTransferAsset`

Signed by the `owner` field.

MsgTransferAsset moves title. The shareholding is untouched.

A buyer takes an encumbered asset subject to its shareholders, and the obligation to fund the vault moves with title.

The fraction tokens are ordinary balances and stay where they are — selling the vehicle does not sell its shareholders out from under them.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string |  |
| `asset_id` | uint64 |  |
| `recipient` | string |  |

### MsgUpdateParams

`/blockchain.tokenisation.v1.MsgUpdateParams`

Signed by the `authority` field.

MsgUpdateParams sets the module parameters. Governance only.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

## Queries

### Asset

`GET /yamale/blockchain/tokenisation/v1/assets/{asset_id}`

Asset returns one vehicle with its vault and any reported sale, which is everything a holder needs to decide whether to redeem.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `asset` | Asset |  |
| `vault` | Vault |  |
| `sale` | SaleReport |  |

### Assets

`GET /yamale/blockchain/tokenisation/v1/assets`

Assets lists the vehicles in a collection, or across all of them.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `collection_id` | string |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `assets` | repeated Asset |  |
| `pagination` | PageResponse |  |

### Collections

`GET /yamale/blockchain/tokenisation/v1/collections`

Collections lists the registries vehicles are minted into. A collection is who may mint, how a sale is verified, and how long a reported price sits before it can be acted on.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `collections` | repeated Collection |  |
| `pagination` | PageResponse |  |

### Covenants

`GET /yamale/blockchain/tokenisation/v1/covenants/{asset_id}`

Covenants lists what has been published against one tokenisation.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `current_only` | bool | Omit filings that have been corrected. False by default: what was said at a date is usually the question, and a reader who can only see the current version cannot answer it. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `covenants` | repeated Covenant |  |

### DividendReserve

`GET /yamale/blockchain/tokenisation/v1/assets/{asset_id}/dividend-reserve`

DividendReserve is the backing check, exposed so it does not have to be taken on trust.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `dividend_denom` | string | The denomination claims against this vehicle are paid in. Empty for a vehicle that has not been fractionalised, which is not the same as one whose dividend token has zero supply. |
| `outstanding` | Coin | Dividend tokens issued and not yet cashed: what the vehicle still owes. |
| `reserve` | Coin | Stable set aside to pay them. Equal to outstanding in amount whenever the module is behaving; a difference is a bug, and a difference in the wrong direction is an exchange that will run dry. |
| `unreserved` | Coin | What the vehicle still holds that is NOT promised to anybody: the pool a future distribution can be credited against, and the pool redemption pays out of. Returned beside the other two because the useful question about a vault is which of its money is already spoken for. |

### Entitlement

`GET /yamale/blockchain/tokenisation/v1/assets/{asset_id}/entitlement/{holder}`

What an account is owed right now, including income that has accrued since its balance last moved.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `holder` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `owed` | Coin |  |

### Params

`GET /yamale/blockchain/tokenisation/v1/params`

Params returns the module parameters.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

### Position

`GET /yamale/blockchain/tokenisation/v1/assets/{asset_id}/position/{holder}`

Position returns one holder's whole record against a tokenisation.

Entitlement answers only "what could they withdraw now", as a single Coin. Everything else a holder needs to see about their own position -- what it cost, how much of it that cost covers, and the time accruing in the current period -- had no query at all, so the console was reading it out of the raw store at /store/tokenisation/key and decoding the key layout by hand. That works and it couples a browser to internal storage: a change to a key prefix, which is an implementation detail nobody would announce, silently breaks every reader.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `holder` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `position` | Position | Absent when this account has never held a share in this tokenisation. An absence, not a failure: most accounts hold most things not at all. |
| `found` | bool |  |
| `owed` | string | What could be withdrawn right now, settled and unsettled together. The same figure Entitlement returns, carried here so a screen showing a whole position does not need two round trips that could disagree. |
| `pending_token_seconds` | string | Token-seconds accrued in the CURRENT, undeclared period, brought up to the block being queried. Not money, and deliberately not converted to any. The rate is unknown until the period closes, so a figure in a currency would be a forecast standing beside a settled amount in the same units -- and a reader cannot tell those apart once both are money. |

## State

### Asset

Asset is title to one real-world thing.

Title and shareholding are separate registers. This record says who owns the asset; the tokens say who holds the economic interest. They move independently, so an asset changing operator is a non-event for shareholders.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `collection_id` | string |  |
| `owner` | string | Holder of title. Whoever holds it carries the obligation to pay income into the vault -- an obligation, never a balance they could walk away with. |
| `uri` | string |  |
| `fraction_denom` | string | The encumbrance. Non-empty means shareholders exist against this asset. It lives on the asset record rather than in an index built beside it, so there is no path where a stale view sells somebody a warehouse that already has shareholders. That is the failure which costs the chain its credibility as a registry rather than merely costing money. |
| `holder_share_bps` | uint32 | Share of the asset's economics carried by the tokens, in basis points. The sponsor keeps the rest. Fixed at fractionalisation and never editable: an issuer who could revise it would dilute every holder without minting a single token. |
| `status` | Status |  |
| `parcel_id` | uint64 | The x/land parcel this vehicle is over, or 0 when the asset is not land. Naming the parcel is what makes the registry's authorisation enforceable: fractionalising an asset with a parcel id is refused unless the registry has a live permission for that parcel, the issued share is inside the ceiling it set, and the sponsor is still the parcel's holder. Zero means none of that applies, so a warehouse receipt or a bond is untouched. The parcel is referenced, never moved. Title stays in x/land held by the same account, which is what keeps a fractionalised parcel governed by the registry rather than by whoever accumulated the tokens. |
| `sector` | Sector | What the tokenised thing does. Set at mint and never edited: a sector an issuer could revise is a sector an issuer revises after the market has priced the first one. |

### Collection

Collection is a class of real-world asset the chain will record title for: a deeds registry, a warehouse operator's receipts, a vehicle register.

Collections are chain-level constructs. There is no permissionless message that creates one, because a registry of deeds is not something a chain grants on request -- governance creates it and appoints who may mint into it.

| Field | Type | Description |
| --- | --- | --- |
| `id` | string |  |
| `authority` | string | The only address permitted to mint assets into this collection. An unset or revoked authority does not fall back to governance: the collection refuses mints outright. The failure mode of a permissive default here is unlimited issuance of title to things nobody owns. |
| `verification` | VerificationMode | How a sale price reported against assets in this collection is checked. |
| `attestation_threshold` | uint32 | Attestor threshold when verification is VERIFY_ATTESTORS. Refused below 2: one attestor is not a threshold, it is a single point of unlimited theft. |
| `challenge_window_seconds` | int64 | How long a reported sale price sits before redemption opens. Calibrated to the asset class rather than a chain-wide constant -- a bond's redemption was fixed at issuance and needs days, a unique building needs a month. See docs/guides/tokenisation.md. |
| `dispute_bond_bps` | uint32 | Fraction of the reported sale price a challenger must bond, in basis points. Scales with the vehicle rather than being flat: a flat bond is trivial for a large fraud to post and prohibitive for a small holder. |
| `attestors` | repeated string | attestors is the register of accounts that may attest a sale reported under this collection. Required when verification is VERIFY_ATTESTORS, and it must hold at least attestation_threshold entries: a threshold higher than the register is one no honest sale can ever meet, which turns every vehicle in the collection into a one-way door. #### Why a register rather than nothing Without one, AttestSale accepts a signature from any address at all, and a sponsor meets any threshold with N fresh keys for the cost of the gas. The threshold is the whole of what stands between a shareholder and a sale reported below what was received, so a threshold anybody can meet is not a protection, it is the appearance of one. #### Who sets it, which is the whole of why it works Governance, and only governance — the same signer that creates the collection in the first place. A sponsor cannot appoint their own attestors, so they cannot manufacture a quorum, and that is the difference between this register and a decorative one. What the chain still cannot decide is whether an account governance appointed is genuinely independent of the seller. That is not a fact the chain holds. What it can do is make the appointment a visible, deliberate act by a signer the seller does not control, which is the same arrangement x/land uses for the offices that attest a transfer. |

### Covenant

Covenant is an undertaking published against a tokenisation: the digest of a document, what kind of document it is, and when it was filed.

#### Why this is not the same as the asset's uri

The uri is one string fixed at mint. An operating business publishes for as long as it exists -- accounts every year, a notice before every assembly, a disclosure whenever something breaks -- and none of that can be known at the moment title is created.

#### The chain holds the digest and never the document

A set of accounts names customers, salaries and counterparties. Writing it to a public chain is a permanent disclosure made on behalf of people who did not consent to it. What is stored is a SHA-256 digest, a kind, a title, a period and a location.

What that is worth, exactly: it proves a document with these contents existed at this block and was published by the party that controls the collection. It proves nothing about whether the accounts are true, whether the assembly was held, or whether the file can still be fetched.

#### Append-only

Never deleted, never edited. An operator who could withdraw a disclosure after publishing it has a register that says whatever is convenient today. A correction is a new covenant naming the one it supersedes, exactly as a deal document is.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `asset_id` | uint64 |  |
| `kind` | CovenantKind |  |
| `title` | string | What it is called, for a person reading a list. |
| `sha256` | bytes | SHA-256 of the document. Exactly 32 bytes; the algorithm is fixed rather than carried in a field, because an algorithm field is an invitation to attach an MD5 and call it proof. |
| `uri` | string | Where it can be fetched. May be empty and may stop working; the chain makes no promise about it. |
| `period_start` | int64 | The period this covers, as unix seconds. Both zero for a document that covers a moment rather than a span -- a material event, a resolution. Carried because "the 2026 accounts" published late and "the 2027 accounts" published early are the same filing date and different documents, and a holder checking whether a year is missing needs the period rather than the filing. |
| `period_end` | int64 |  |
| `acts_by` | int64 | When a holder must have acted by, as unix seconds. Zero when nothing is required of them. Set for an assembly notice, and it is the field that makes a deadline visible as a deadline. |
| `published_by` | string | The account that published it: the collection authority at the time. |
| `published_at` | int64 |  |
| `supersedes` | uint64 | The covenant this replaces, and the one that replaced it. Same shape and same reason as a deal document: a correction goes forward and the earlier version stays readable, because a dispute is usually about what was said at a date rather than what is said now. |
| `superseded_by` | uint64 |  |

### PeriodRate

PeriodRate is what one distribution paid, kept so a holder who settles long afterwards is still paid at the rate of the period they held through.

Two figures rather than one, and the second is what removes a loop:

`rate` is money per token-second, and pays a PARTIAL period -- the stretch a holder was present for before or after their balance moved.

`cumulative_per_token` is the running total of income per token after this distribution. A holder who was present for several WHOLE periods is paid for all of them by subtracting one of these from another, so settling never walks the periods in between however many there are.

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `period_index` | uint64 |  |
| `amount` | string | What this distribution paid, and the total token-seconds it is divided over. Two integers rather than one pre-divided rate, and the reason is precision. A rate stored as `amount / token_seconds` and later multiplied back by a holder's seconds loses the division twice: measured, a holder owed exactly 100,000 was paid 99,999. Kept this way the credit is `amount * held_seconds / token_seconds` -- one truncation, at the end, favouring the vault the way every other rounding in this module does. |
| `token_seconds` | string | Supply times the period's length: the denominator every holder's share of this distribution is taken over. Zero for a period of no length, which is what makes that case detectable rather than a division by zero. |
| `per_token` | string | What THIS distribution paid per token: amount / supply. Used when the period had no length at all -- two distributions in one block. Nobody held for any time, so token-seconds cannot apportion it and the only sensible answer is the old rule, by balance. Without this the money is written into the cumulative figure and credited to nobody, which is how a distribution silently strands in the vault. |
| `cumulative_per_token` | string | Income per token accumulated over the vehicle's life, after this one. |
| `ended_at` | int64 | When the period this paid for ended. The period it covers is [previous.ended_at, ended_at]. |

### Position

Position is one account's place in the index.

Settled whenever the balance moves, so what is owed is always balance * (cumulative_per_token - last_index).

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `holder` | string |  |
| `last_index` | string |  |
| `accrued` | string | Settled but not yet withdrawn. |
| `cost_basis` | string | What this holder PAID, in the vault's income denomination, for the tokens `cost_qty` covers. #### Why two fields and not one A holding can be partly covered. Nine million tokens of which five million were subscribed at a recorded price and four million arrived as a plain transfer from somebody with no basis is the ordinary case, not an edge one. With a single field the interface must either average a known cost with an unknown one, or report a return on the whole holding that is only true of part of it. With two it can say: this much returned on this much invested, and this much not covered. #### Why it cannot simply be derived A token balance records what an account holds and never what it paid, and tokens move by ordinary bank send, which carries no price. So basis is only as good as the events allowed to write it: a recorded subscription, where the party that took the money states the figure, and a marketplace fill, where the price is in the trade and nobody has to assert anything. #### Zero is never assumed A transfer from a holder with no basis leaves the recipient with none, and an interface must show that as COST NOT RECORDED rather than as cost zero. Cost zero reports an infinite return, which is the most flattering and most wrong figure this module could produce. |
| `cost_qty` | string | How many tokens `cost_basis` was paid for. Never larger than the balance: a transfer out reduces both proportionally. |
| `token_seconds` | string | Token-seconds accrued in ONE period and not yet turned into money. This is what makes a mid-period sale split the dividend: the seller stops accruing at the moment of sale and the buyer starts, so each is paid for the time they actually held rather than for being present when the cheque was declared. |
| `token_seconds_period` | uint64 | Which period those seconds belong to. Carried because the rate is only known when the period closes, so seconds accrued in period 3 must be paid at period 3's rate however long the holder waits to settle. |
| `last_touch` | int64 | When this position last accrued. Every balance change touches it, which is what keeps the integral of balance over time exact rather than sampled. |

### SaleReport

SaleReport is a reported price inside or past its challenge window.

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `price` | Coin |  |
| `reporter` | string |  |
| `reported_at` | Timestamp |  |
| `claimable_at` | Timestamp | Opens redemption once passed, absent a dispute. |
| `attestors` | repeated string | Attestors who have signed this figure, when the mode requires them. |
| `disputed` | bool |  |
| `proceeds_paid` | Coin | The holders' share of the price, actually paid into the vault. A reported price is a claim about an off-chain sale. Crediting the index from the claim alone distributes money the vehicle never received, and the shared module account makes the shortfall somebody else's. Finalisation waits on this reaching what the holders are owed. |
| `challenger` | string | Who challenged the figure, and what they posted to do it. Held so the bond has somewhere to go: returned when the challenge was right, forfeited to the vault when it was not. |
| `bond` | Coin |  |

### Vault

Vault holds what is owed to shareholders and the running index that says how much each token has earned.

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `cumulative_per_token` | string | Income per token, accumulated over the vehicle's life, scaled by 1e18. An index rather than a snapshot because a snapshot is snipeable: buy from the pool at H-1, be holder of record at H, sell at H+1, and collect a quarter's rent for two blocks of exposure with no capital at risk. An index is time-weighted, so two blocks earn two blocks. It also removes a halt: there is no holder set to iterate. |
| `funded` | repeated Coin | Total gross income ever declared against the vehicle, for the solvency view. Larger than what the module holds for it, because only the holders' share is ever collected. |
| `denom` | string | The denomination income arrives in. |
| `period_index` | uint64 | How many distributions this vault has made. A PERIOD is the span between two distributions, and it is what pro-rata temporis apportions. Zero means none yet: everything since period_start is the first period. |
| `period_start` | int64 | When the current period began: the last distribution, or the moment the vault started accruing. For a vehicle that existed before pro-rata temporis, this is the UPGRADE HEIGHT's time and not issuance. The chain has no record of when balances moved before that, so token-seconds for the past cannot be reconstructed, and assuming everyone held from issuance would credit a buyer from last week with a year of accrual. The discontinuity is real and is the honest option; see docs/superpowers/specs. |
| `held` | repeated Coin | What the module account actually holds on this vehicle's behalf and has not yet promised to anybody. One module account serves every vehicle, so a balance is not an ownership record: without this ledger a vehicle that funded nothing pays its holders out of a different vehicle's vault, and the bank keeper has no way to object. Every credit is a transfer in, every debit is a payout, and no payout is made that this does not cover. A dividend claimed leaves here and lands in `dividend_reserve`. It has to leave: money still counted here is money the next distribution can be credited against, and crediting a distribution against stable already owed to a dividend token is how the exchange runs dry. |
| `dividend_reserve` | repeated Coin | The stable set aside to honour dividend tokens already issued against this vehicle, and nothing else. #### Why this is a separate field rather than a subtraction A dividend token the chain can mint beyond what the vehicle holds is a promise the chain cannot keep. The guarantee is kept by construction: the ONLY path that mints a dividend token is a claim, and a claim moves exactly its own amount from `held` into here in the same write. So for every vehicle, at every height, supply(div/<asset>/<SYMBOL>) == dividend_reserve.AmountOf(denom) and the exchange, which burns and pays 1:1 out of this figure alone, cannot run dry against a correctly-funded vehicle. Held as a count rather than derived from the token supply because the supply is bank state and this is the ledger that has to refuse a payout BEFORE the bank keeper is asked — the same reason `held` exists beside the module account balance. #### Why it is per vehicle For exactly the reason `held` is. One module account holds every vehicle's money, so an exchange that checked only the module balance would let vehicle A's dividend token be cashed out of vehicle B's reserve, and the bank keeper would have no way to notice. #### It survives realisation, and is never swept Redemption pays out of `held`, which by construction excludes this. So a holder who claimed and never exchanged is still paid after the vehicle is sold, after it is CLOSED, and after the last fraction token is burned. No rule reclaims it, because the only rule that could — "after N seconds, give it to somebody else" — has no correct somebody else: the money is already owed to a named token, and the chain has no basis for deciding its holder has stopped wanting it. |

## Value types

### CovenantKind

CovenantKind is what an operator is publishing, so a holder can tell an obligation from an announcement.

The distinction is the point. A general assembly notice with a date is something a holder must ACT on and can miss; a set of accounts is something they read. A register that filed both as "document" makes the one with a deadline as easy to overlook as the one without.

| Value | Meaning |
| --- | --- |
| `COVENANT_KIND_UNSPECIFIED` |  |
| `COVENANT_KIND_FINANCIAL_STATEMENTS` | Annual or interim accounts. The undertaking most holders were sold. |
| `COVENANT_KIND_STRUCTURE_CHANGE` | A change to the structure itself: a new subsidiary, a redomiciliation, a change of control at the operating company. What a holder bought into is not what it was. |
| `COVENANT_KIND_DIVIDEND_NOTICE` | A distribution declared: the amount, the period it covers, and when it will be paid. Published BEFORE the money moves, so a holder can check the payment against what was announced rather than the other way round. |
| `COVENANT_KIND_GENERAL_ASSEMBLY` | A general assembly: the date, the agenda, the resolutions. Carries a deadline, which is why it is not filed as a report. |
| `COVENANT_KIND_RESOLUTION` | The minutes and resolutions of one, after the fact. |
| `COVENANT_KIND_INDEPENDENT_REPORT` | An auditor's opinion, a valuation, a technical certification -- work by a third party that the operator did not do and cannot revise. |
| `COVENANT_KIND_MATERIAL_EVENT` | Anything the operator is obliged to disclose promptly and that fits none of the above: litigation, a covenant breach, a material contract lost. |

### Sector

Sector is what the tokenised thing DOES, so an investor can filter a market by it and a regulator can recognise the description.

ISIC Rev. 4 sections, not a list invented here. A securities offering is described to a regulator in a classification they already use, and a bespoke list has to be mapped onto one eventually -- by somebody with less context than whoever wrote it. The sections are A to U; the names below are the ones a reader recognises rather than the standard's own wording.

An enum rather than free text for the reason DocumentKind is an enum: this is what a marketplace filters on, and "Timber / timber / Bois / forestry products" is four sectors to a machine and one to a person.

| Value | Meaning |
| --- | --- |
| `SECTOR_UNSPECIFIED` | Never stored on an asset minted after this field existed. Assets minted before it carry this, and that is an absence rather than a claim: it means the chain was not asked, not that the sector is unknown to anybody. |
| `SECTOR_AGRICULTURE` | A. Agriculture, forestry and fishing -- including a standing concession. |
| `SECTOR_EXTRACTION` | B. Mining and quarrying. |
| `SECTOR_MANUFACTURING` | C. Manufacturing, which is anything that transforms a material. A sawmill is here rather than in A: what is sold is what leaves the mill. |
| `SECTOR_UTILITIES` | D and E. Electricity, gas, water and waste. |
| `SECTOR_CONSTRUCTION` | F. Construction. |
| `SECTOR_TRADE` | G. Wholesale and retail trade. |
| `SECTOR_TRANSPORT` | H. Transport and storage. |
| `SECTOR_HOSPITALITY` | I. Accommodation and food service. |
| `SECTOR_TECHNOLOGY` | J. Information and communication. |
| `SECTOR_FINANCIAL` | K. Financial and insurance activities. |
| `SECTOR_REAL_ESTATE` | L. Real estate. Distinct from an asset being OVER registered land: a warehouse operator is real estate, and a sawmill built on a titled parcel is not. |
| `SECTOR_SERVICES` | M and N. Professional, scientific, technical and administrative services. |
| `SECTOR_PUBLIC_AND_SOCIAL` | O to U. Public administration, education, health, arts, and the rest. |

### Status

Status is the vehicle's life. It ends; this is a closed-end instrument, not a perpetual registry.

| Value | Meaning |
| --- | --- |
| `STATUS_UNSPECIFIED` | Unset. No asset is ever written in this state; it is what a zero value in a hand-edited genesis looks like. |
| `STATUS_HELD` | Title exists, no shareholders yet. |
| `STATUS_ACTIVE` | Shareholders exist. Income distributes as it arrives. |
| `STATUS_REPORTED` | A sale price has been reported and is inside its challenge window. Redemption is not open yet. |
| `STATUS_REALISED` | The price is final. Proceeds distribute exactly like income -- the sale is not a special case in the accounting, it is the last distribution and it is larger. Transfers of the token stop here. Each token is now a claim on a known fixed pot, and a pool still quoting from its reserves is a free lunch that gets taken within a block. |
| `STATUS_CLOSED` | Supply reached zero and the title was burned. |
| `STATUS_DISPUTED` | A challenge succeeded or the sale fell through. Back to ACTIVE by governance, never automatically. |

### VerificationMode

VerificationMode is how the chain learns what an asset sold for.

The issuer alone can never set it. An issuer who under-reports steals from every shareholder in one transaction, at the moment of maximum value, with no further obligation to anybody afterwards.

| Value | Meaning |
| --- | --- |
| `VERIFICATION_UNSPECIFIED` | Unset. A collection must choose how its sales are verified, so this is never a working configuration — it is what an incomplete genesis reads as. |
| `VERIFY_VALUER` | The appointed independent valuer signs the figure. x/oracle already holds this machinery for appraisals. |
| `VERIFY_ATTESTORS` | m-of-n attestors agree, m >= 2. Same shape as x/custody's deposits. |
| `VERIFY_GOVERNANCE` | Voted, with the evidence attached to the proposal. |
| `VERIFY_SCHEDULE` | The redemption amount was fixed when the vehicle was created -- par plus coupon, on a date -- so there is nothing to appraise and nobody to trust. The chain checks whether the expected amount arrived. This is the sovereign case: a state issuing a bond asks its citizens to trust arithmetic rather than a person. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 10 | `ErrInvalidShare` | holder share must be between 1 and 10000 basis points |
| 11 | `ErrAmountTooSmall` | amount is too small to divide across the shareholding |
| 12 | `ErrWrongStatus` | the asset is not in a state that allows this |
| 13 | `ErrTradingHalted` | this asset has been realised; the only remaining path is redemption |
| 14 | `ErrNotVerified` | the reported sale price has not met its verification requirement |
| 15 | `ErrStillInWindow` | the reported sale price is still inside its challenge window |
| 16 | `ErrAlreadyDisputed` | that sale is already under dispute |
| 17 | `ErrNotAttestor` | this account is not an appointed attestor for that collection |
| 18 | `ErrAlreadyAttested` | this attestor has already signed that figure |
| 19 | `ErrInvalidThreshold` | attestation threshold must be at least two |
| 2 | `ErrCollectionNotFound` | no such collection |
| 20 | `ErrInvalidParams` | invalid parameters |
| 21 | `ErrInvalidSigner` | expected the governance account as the only signer |
| 22 | `ErrInvalidAmount` | amount must be positive |
| 23 | `ErrNothingOwed` | nothing is owed to this account |
| 24 | `ErrNoSaleReported` | no sale has been reported for that asset |
| 25 | `ErrWrongDenom` | that is not the denomination this vault pays income in |
| 26 | `ErrNoParcel` | the land registry has no parcel with that id |
| 27 | `ErrNotParcelHolder` | the asset's owner is not the current holder of that parcel |
| 28 | `ErrNoLandAuthorisation` | the land registry has not authorised fractionalising that parcel |
| 29 | `ErrAuthorisationWithdrawn` | the land registry has withdrawn its authorisation to fractionalise that parcel |
| 3 | `ErrCollectionExists` | that collection already exists |
| 30 | `ErrAuthorisationExpired` | the land registry's authorisation to fractionalise that parcel has expired |
| 31 | `ErrShareCeilingExceeded` | the shares issued over that parcel would exceed the ceiling the land registry set |
| 32 | `ErrLandFractionalisationForbidden` | a restriction on that parcel forbids fractionalisation |
| 33 | `ErrNoLandRegistry` | this chain has no land registry, so an asset cannot name a parcel |
| 34 | `ErrWindowClosed` | the challenge window on that sale has closed, so it can no longer be disputed |
| 35 | `ErrVaultUnfunded` | this vehicle does not hold enough to pay that, and the money of another vehicle is not available to it |
| 36 | `ErrProceedsUnpaid` | the holders' share of the reported price has not been paid into the vault, so redemption cannot open |
| 37 | `ErrOverpayment` | that is more than the sale still owes, and the surplus would have no owner |
| 38 | `ErrInvalidSubscription` | that subscription cannot be recorded against this holding |
| 39 | `ErrInvalidCovenant` | a filing needs a kind, a title and a 32-byte SHA-256 digest of its contents |
| 4 | `ErrNoAuthority` | this collection has no minting authority |
| 40 | `ErrCovenantNotFound` | no such filing |
| 41 | `ErrCovenantSuperseded` | that filing has already been corrected; correct the current version instead |
| 42 | `ErrNoDividendDenom` | that asset has no shareholding, so it has no dividend token |
| 43 | `ErrReserveUnfunded` | this vehicle's dividend reserve cannot cover that, and the reserve of another vehicle is not available to it |
| 5 | `ErrNotAuthority` | this account is not the collection's minting authority |
| 6 | `ErrAssetNotFound` | no such asset |
| 7 | `ErrNotOwner` | this account does not hold title to that asset |
| 8 | `ErrAlreadyFractionalised` | that asset already has shareholders |
| 9 | `ErrNoShareholders` | that asset has no shareholders to credit |
