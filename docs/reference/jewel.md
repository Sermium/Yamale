<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/jewel

A register of jewels under admitted jewelers: each piece an x/nft token that changes owner only by offer and acceptance or by a claim filed with its ownership card and approved by the shop, with owner-consented benefits, a stolen flag, and the office's reassignment of a lost key behind a 30-day public veto window.

## Transactions

### MsgAcceptBusinessMove

`/blockchain.jewel.v1.MsgAcceptBusinessMove`

Signed by the `new_business` field.

MsgAcceptBusinessMove is the message that carries out AcceptBusinessMove. The proposed account becomes the jeweler's business account, and every jewel of this jeweler the old business account holds, its stock, moves to it. A stock jewel keeps its ownership card and any pending claim, and loses any open offer or pending reassignment, which were made against the old account. The old account keeps whatever staff place it had. Refused when the new account already owns jewels of this jeweler, which would silently become stock.

| Field | Type | Description |
| --- | --- | --- |
| `new_business` | string | new_business is the account named in the proposal, the signer. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler whose business account moves. |

### MsgAcceptOffer

`/blockchain.jewel.v1.MsgAcceptOffer`

Signed by the `recipient` field.

MsgAcceptOffer is the message that carries out AcceptOffer.

AcceptOffer takes a jewel offered to the signer's account.

| Field | Type | Description |
| --- | --- | --- |
| `recipient` | string | recipient is the account the offer names, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to take. |

### MsgAdmitJeweler

`/blockchain.jewel.v1.MsgAdmitJeweler`

Signed by the `office` field.

MsgAdmitJeweler is the message that carries out AdmitJeweler.

AdmitJeweler admits a business and opens its collection. Office only.

| Field | Type | Description |
| --- | --- | --- |
| `office` | string | office is the register's office, the signer. |
| `name` | string | name is the business's display name. |
| `business` | string | business is the account that will hold the stock and manage staff. |

### MsgAmendJewel

`/blockchain.jewel.v1.MsgAmendJewel`

Signed by the `staff` field.

MsgAmendJewel is the message that carries out AmendJewel. A jewel carries at most 50 amendments, each with a note of at most 500 bytes.

AmendJewel appends a correction to a jewel's record.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string | staff is a staff member of the jewel's own jeweler, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to correct. |
| `record_hash` | string | record_hash is the SHA-256, in lower-case hex, of the corrected record file. |
| `record_uri` | string | record_uri is where the corrected record file can be fetched; at most 512 characters. |
| `note` | string | note says what was corrected; required, at most 500 bytes. |

### MsgApproveClaim

`/blockchain.jewel.v1.MsgApproveClaim`

Signed by the `staff` field.

MsgApproveClaim is the message that carries out ApproveClaim. The jewel moves to the claimer, and its card and claim are spent.

The shop's staff only.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string | staff is the jeweler's business account or a member of its staff, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel whose pending claim to approve. |

### MsgAttachBenefit

`/blockchain.jewel.v1.MsgAttachBenefit`

Signed by the `staff` field.

MsgAttachBenefit is the message that carries out AttachBenefit.

AttachBenefit adds a benefit to a jewel.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string | staff is a staff member of the jewel's own jeweler, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to attach it to. |
| `kind` | string | kind is the service as the jeweler words it, e.g. "cleaning"; at most 80 characters. |
| `allowance` | uint32 | allowance is how many uses fit in one period; at least 1. |
| `period` | int64 | period is the rolling window the allowance applies to, in seconds; 0 means the whole life. |
| `expires_at` | int64 | expires_at is when the benefit ends, in unix seconds; 0 means never. |

### MsgBuyJewel

`/blockchain.jewel.v1.MsgBuyJewel`

Signed by the `buyer` field.

MsgBuyJewel is the message that carries out BuyJewel. The buyer pays the price: the seller receives it less the fee, and the fee goes to x/jewel's account. The jewel moves to the buyer in the same step, spending any card and claim. Refused unless price is exactly the listing's, so a price changed after the buyer looked cannot be charged.

| Field | Type | Description |
| --- | --- | --- |
| `buyer` | string | buyer pays and receives the jewel; the signer. |
| `jewel_id` | uint64 | jewel_id is the listed jewel. |
| `price` | Coin | price is the price the buyer saw; it must equal the listing's. |

### MsgCancelOffer

`/blockchain.jewel.v1.MsgCancelOffer`

Signed by the `owner` field.

MsgCancelOffer is the message that carries out CancelOffer.

CancelOffer closes an open offer.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string | owner is the jewel's owner, or a staff member while it is stock; the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel whose offer to close. |

### MsgClaimOffer

`/blockchain.jewel.v1.MsgClaimOffer`

Signed by the `claimer` field.

MsgClaimOffer is the message that carries out ClaimOffer.

ClaimOffer takes a jewel offered to a claim key.

| Field | Type | Description |
| --- | --- | --- |
| `claimer` | string | claimer is the account that receives the jewel, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to claim. |
| `claim_pub_key` | bytes | claim_pub_key is the compressed secp256k1 public key of the claim key. |
| `claim_signature` | bytes | claim_signature signs ClaimSignBytes(chain id, jewel id, the offer's expires_at, claimer): "yamale-jewel-claim/v2:<chain>:<jewel>:<expires>:<claimer>". |

### MsgClearStolen

`/blockchain.jewel.v1.MsgClearStolen`

Signed by the `owner` field.

MsgClearStolen is the message that carries out ClearStolen.

ClearStolen lifts the flag.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string | owner is the jewel's owner, or a staff member while it is stock; the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to clear. |

### MsgCompleteReassignment

`/blockchain.jewel.v1.MsgCompleteReassignment`

Signed by the `creator` field.

MsgCompleteReassignment is the message that carries out CompleteReassignment.

CompleteReassignment moves the jewel once the window has passed. Anyone.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string | creator is any account, the signer; it pays the fee. |
| `jewel_id` | uint64 | jewel_id is the jewel whose reassignment to complete. |

### MsgFileClaim

`/blockchain.jewel.v1.MsgFileClaim`

Signed by the `claimer` field.

MsgFileClaim is the message that carries out FileClaim. It records a pending claim for the shop's staff to approve or reject; nothing moves yet.

FileClaim records a buyer's claim, signed with the card's secret.

| Field | Type | Description |
| --- | --- | --- |
| `claimer` | string | claimer is the account the jewel goes to once the shop approves, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to claim. |
| `card_pub_key` | bytes | card_pub_key is the compressed secp256k1 public key of the card, 33 bytes; it must be the jewel's current card. |
| `card_signature` | bytes | card_signature signs CardClaimSignBytes(chain id, jewel id, card key, claimer): "yamale-jewel-card/v1:<chain>:<jewel>:<card key hex>:<claimer>". |

### MsgIssueCard

`/blockchain.jewel.v1.MsgIssueCard`

Signed by the `staff` field.

MsgIssueCard is the message that carries out IssueCard. It sets the jewel's ownership card, replacing any earlier one, and drops a pending claim that was signed with the earlier card. Refused unless the jewel is stock of an active jeweler and not reported stolen.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string | staff is the jeweler's business account or a member of its staff, the signer. |
| `jewel_id` | uint64 | jewel_id is the stock jewel the card is for. |
| `card_key` | string | card_key is the card's compressed secp256k1 public key in hex, 66 characters. Upper-case hex is accepted and stored in lower case. The secret it belongs to is printed on the card and nowhere else. |

### MsgListJewel

`/blockchain.jewel.v1.MsgListJewel`

Signed by the `owner` field.

MsgListJewel is the message that carries out ListJewel. Listing again replaces the price. Refused for a stolen jewel, a jewel with a reassignment pending, a suspended jeweler's stock, a zero price, and a currency outside params.sale_denoms.

ListJewel puts a jewel up for sale on the marketplace, or changes its price.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string | owner is the jewel's owner, or a member of its jeweler's staff while the jewel is stock; the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to sell. |
| `price` | Coin | price is what a buyer pays. |

### MsgOfferJewel

`/blockchain.jewel.v1.MsgOfferJewel`

Signed by the `owner` field.

MsgOfferJewel is the message that carries out OfferJewel.

OfferJewel opens an offer to an account or a claim key.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string | owner is the jewel's owner, or a staff member while it is stock. |
| `jewel_id` | uint64 | jewel_id is the jewel to offer. |
| `to` | string | to is the recipient's account; set this or claim_key, not both. |
| `claim_key` | string | claim_key is a one-time key's compressed secp256k1 public key in hex, for a recipient with no account yet. |

### MsgProposeBusinessMove

`/blockchain.jewel.v1.MsgProposeBusinessMove`

Signed by the `business` field.

MsgProposeBusinessMove is the message that carries out ProposeBusinessMove. The shop's business account names the account that is to take its place. Nothing moves until that account accepts, within offer_ttl; proposing again replaces the proposal, and an empty new_business withdraws it. Refused while the jeweler is suspended, and for an account that is already another jeweler's business account or staff.

| Field | Type | Description |
| --- | --- | --- |
| `business` | string | business is the jeweler's current business account, the signer. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler whose business account moves. |
| `new_business` | string | new_business is the account to move to; empty withdraws the proposal. |

### MsgProposeReassignment

`/blockchain.jewel.v1.MsgProposeReassignment`

Signed by the `office` field.

MsgProposeReassignment is the message that carries out ProposeReassignment.

ProposeReassignment opens the public window. Office only.

| Field | Type | Description |
| --- | --- | --- |
| `office` | string | office is the register's office, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to move. |
| `to` | string | to is the account to move it to. |
| `reason` | string | reason is the office's public account of why; required. |

### MsgRedeemBenefit

`/blockchain.jewel.v1.MsgRedeemBenefit`

Signed by the `staff` field.

MsgRedeemBenefit is the message that carries out RedeemBenefit.

RedeemBenefit records one use, with the owner's signed voucher.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string | staff is a staff member of the benefit's jeweler, the signer. |
| `voucher` | Voucher | voucher is what the owner's app signed, shown to staff as a code. |
| `owner_pub_key` | bytes | owner_pub_key is the compressed secp256k1 public key of the owner. |
| `owner_signature` | bytes | owner_signature signs VoucherSignBytes(chain id, voucher). |

### MsgRegisterJewel

`/blockchain.jewel.v1.MsgRegisterJewel`

Signed by the `staff` field.

MsgRegisterJewel is the message that carries out RegisterJewel.

RegisterJewel mints a jewel into its jeweler's stock.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string | staff is the staff member registering it, the signer. |
| `inscription` | string | inscription is the laser inscription as typed; separators and case are normalised. |
| `record_hash` | string | record_hash is the SHA-256, in lower-case hex, of the record file. |
| `record_uri` | string | record_uri is where the record file can be fetched; at most 512 characters. |

### MsgReinstateJeweler

`/blockchain.jewel.v1.MsgReinstateJeweler`

Signed by the `office` field.

MsgReinstateJeweler is the message that carries out ReinstateJeweler.

ReinstateJeweler lets a suspended jeweler act again. Office only.

| Field | Type | Description |
| --- | --- | --- |
| `office` | string | office is the register's office, the signer. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler to reinstate. |

### MsgRejectClaim

`/blockchain.jewel.v1.MsgRejectClaim`

Signed by the `staff` field.

MsgRejectClaim is the message that carries out RejectClaim. The card stays valid, so the real buyer can file with it afterwards.

The shop's staff only.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string | staff is the jeweler's business account or a member of its staff, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel whose pending claim to remove. |

### MsgReportStolen

`/blockchain.jewel.v1.MsgReportStolen`

Signed by the `owner` field.

MsgReportStolen is the message that carries out ReportStolen.

ReportStolen flags a jewel and closes its offer and any pending claim.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string | owner is the jewel's owner, or a staff member while it is stock; the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to flag. |

### MsgSetStaff

`/blockchain.jewel.v1.MsgSetStaff`

Signed by the `business` field.

MsgSetStaff is the message that carries out SetStaff.

SetStaff adds and removes the people who act for a jeweler.

| Field | Type | Description |
| --- | --- | --- |
| `business` | string | business is the jeweler's business account, the signer. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler whose staff change. |
| `add` | repeated string | add are accounts to act for the jeweler; none may work for another. |
| `remove` | repeated string | remove are accounts that stop acting for it, at once. |

### MsgSuspendJeweler

`/blockchain.jewel.v1.MsgSuspendJeweler`

Signed by the `office` field.

MsgSuspendJeweler is the message that carries out SuspendJeweler.

SuspendJeweler stops a jeweler acting: no registration, no sale of its stock, and its staff's offers die. Office only.

| Field | Type | Description |
| --- | --- | --- |
| `office` | string | office is the register's office, the signer. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler to suspend. |
| `reason` | string | reason is the office's public account of why; required. |

### MsgUnlistJewel

`/blockchain.jewel.v1.MsgUnlistJewel`

Signed by the `owner` field.

MsgUnlistJewel is the message that carries out UnlistJewel.

UnlistJewel takes it off the marketplace.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string | owner is the jewel's owner, or staff while it is stock; the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel to take off the marketplace. |

### MsgUpdateParams

`/blockchain.jewel.v1.MsgUpdateParams`

Signed by the `authority` field.

MsgUpdateParams is the message that carries out UpdateParams.

UpdateParams sets the settings and appoints the office. Governance only; the office must be a group account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the governance module account, the only signer accepted. |
| `params` | Params | params replace the settings whole; office must be a group policy account. |

### MsgVetoReassignment

`/blockchain.jewel.v1.MsgVetoReassignment`

Signed by the `owner` field.

MsgVetoReassignment is the message that carries out VetoReassignment.

VetoReassignment closes it. The current owner only.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string | owner is the jewel's current owner, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel whose reassignment to refuse. |

### MsgWithdrawClaim

`/blockchain.jewel.v1.MsgWithdrawClaim`

Signed by the `claimer` field.

MsgWithdrawClaim is the message that carries out WithdrawClaim.

WithdrawClaim removes the signer's own claim.

| Field | Type | Description |
| --- | --- | --- |
| `claimer` | string | claimer is the account that filed the claim, the signer. |
| `jewel_id` | uint64 | jewel_id is the jewel whose claim to withdraw. |

### MsgWithdrawSaleFees

`/blockchain.jewel.v1.MsgWithdrawSaleFees`

Signed by the `authority` field.

MsgWithdrawSaleFees is the message that carries out WithdrawSaleFees.

WithdrawSaleFees pays marketplace fees out of x/jewel's account. Governance only.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is governance, the signer. |
| `to` | string | to is the account paid. |
| `amount` | repeated Coin | amount is what is paid out of the fees held. |

## Queries

### ClaimsByJeweler

`GET /yamale/blockchain/jewel/v1/jewelers/{jeweler_id}/claims`

ClaimsByJeweler lists a jeweler's pending claims, each with its jewel, in jewel id order: what its staff have to approve or reject.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `jeweler_id` | uint64 | jeweler_id is the jeweler whose pending claims to list. |
| `pagination` | PageRequest | pagination pages through the claims in jewel id order. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `claims` | repeated ClaimEntry | claims are this page of pending claims. |
| `pagination` | PageResponse | pagination is the key to the next page. |

### Jewel

`GET /yamale/blockchain/jewel/v1/jewel/{id}`

Jewel returns one jewel with its owner, open offer, pending reassignment, benefits, whether it has an ownership card, and any pending claim.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id is the jewel to show. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jewel` | Jewel | jewel is the register's record of it. |
| `owner` | string | owner is its current owner, read from x/nft. |
| `offer` | Offer | offer is set while an offer is open, expired or not. |
| `reassignment` | Reassignment | reassignment is set while one is pending. |
| `benefits` | repeated Benefit | benefits are the benefits attached to it. |
| `has_card` | bool | has_card is set while the jewel has an ownership card. The card's key is not shown here: only the printed card needs it. |
| `claim` | Claim | claim is set while a buyer's claim waits for the shop. |
| `listing` | Listing | listing is set while the jewel is for sale on the marketplace. |

### Jeweler

`GET /yamale/blockchain/jewel/v1/jeweler/{id}`

Jeweler returns one jeweler.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id is the jeweler to show. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jeweler` | Jeweler | jeweler is the jeweler asked for. |

### Jewelers

`GET /yamale/blockchain/jewel/v1/jewelers`

Jewelers lists the jewelers, suspended ones included.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest | pagination pages through the jewelers in id order. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jewelers` | repeated Jeweler | jewelers are this page of jewelers. |
| `pagination` | PageResponse | pagination is the key to the next page. |

### JewelsByInscription

`GET /yamale/blockchain/jewel/v1/inscription/{inscription}`

JewelsByInscription finds every jewel carrying an inscription, whichever jeweler registered it. At most 50.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `inscription` | string | inscription is the code as read off the metal; it is normalised before the search. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jewels` | repeated Jewel | jewels are every jewel carrying it, at most 50. |

### Listings

`GET /yamale/blockchain/jewel/v1/listings`

Listings lists the jewels for sale on the marketplace, each with its listing, in jewel id order.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest | pagination pages through the listings in jewel id order. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `listings` | repeated ListingEntry | listings are this page of jewels for sale. |
| `pagination` | PageResponse | pagination is the key to the next page. |

### Params

`GET /yamale/blockchain/jewel/v1/params`

Params returns the settings and the office.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params are the register's settings, office included. |

## State

### Amendment

Amendment is a correction appended to a jewel's record. The original and every correction stay visible. A jewel carries at most 50, each with a note of at most 500 bytes: the amendments live inside the jewel's record, which every change of owner rewrites.

| Field | Type | Description |
| --- | --- | --- |
| `record_hash` | string | record_hash is the SHA-256, in lower-case hex, of the corrected record file. |
| `record_uri` | string | record_uri is where the corrected record file can be fetched; at most 512 characters. |
| `note` | string | note says what was corrected and why, in the staff member's words; at most 500 bytes. |
| `staff` | string | staff is the staff member who appended it. |
| `time` | int64 | time is the block time, in unix seconds. |

### Benefit

Benefit is a service a jeweler attaches to a jewel. It passes with the jewel to each new owner.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id is the benefit's number, assigned by the chain; never 0. |
| `jewel_id` | uint64 | jewel_id is the jewel it is attached to; it passes with the jewel to each owner. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler whose staff honour it: the jewel's own jeweler. |
| `kind` | string | kind is the service as the jeweler words it, e.g. "cleaning". |
| `allowance` | uint32 | allowance is how many uses fit in one period. |
| `period` | int64 | period is the rolling window, in seconds, the allowance applies to. Zero means the allowance covers the benefit's whole life. |
| `expires_at` | int64 | expires_at is when the benefit ends, in unix seconds. Zero means it does not expire. |
| `uses` | repeated int64 | uses are the block times, in unix seconds, of every use. |

### Card

Card is a jewel's ownership card: a printed QR code holding a secret, of which the chain keeps only the public key. A jewel has at most one card. It is issued while the jewel is stock, and any change of owner spends it.

| Field | Type | Description |
| --- | --- | --- |
| `key` | string | key is the lower-case hex compressed secp256k1 public key of the card's secret, the same form as Offer.claim_key. |
| `issued_by` | string | issued_by is the account that issued it: the business account or a member of its staff. |
| `issued_at` | int64 | issued_at is the block time it was issued, in unix seconds. |

### Claim

Claim is a buyer's pending claim on a stock jewel, filed with its card and waiting for the shop's staff to approve or reject it. A jewel has at most one pending claim.

| Field | Type | Description |
| --- | --- | --- |
| `claimer` | string | claimer is the account the jewel goes to if the shop approves. |
| `card_key` | string | card_key is the key of the card the claim was signed with. Issuing a new card drops a claim made with the old one. |
| `filed_at` | int64 | filed_at is the block time the claim was filed, in unix seconds. |

### ClaimEntry

ClaimEntry is one pending claim with the jewel it is on.

| Field | Type | Description |
| --- | --- | --- |
| `jewel` | Jewel | jewel is the register's record of the claimed jewel. |
| `claim` | Claim | claim is the pending claim. |

### EventBenefitAttached

EventBenefitAttached is emitted when a jeweler attaches a benefit to a jewel.

| Field | Type | Description |
| --- | --- | --- |
| `benefit_id` | uint64 | benefit_id is the new benefit's id. |
| `jewel_id` | uint64 | jewel_id is the jewel it is attached to. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler that honours it. |

### EventBenefitRedeemed

EventBenefitRedeemed is emitted for each use of a benefit.

| Field | Type | Description |
| --- | --- | --- |
| `benefit_id` | uint64 | benefit_id is the benefit used. |
| `jewel_id` | uint64 | jewel_id is the jewel it belongs to. |
| `staff` | string | staff is the staff member who recorded the use. |

### EventBusinessMoveProposed

EventBusinessMoveProposed is emitted when a business account proposes a move, or withdraws one (new_business empty).

| Field | Type | Description |
| --- | --- | --- |
| `jeweler_id` | uint64 | jeweler_id is the jeweler whose business account is to move. |
| `new_business` | string | new_business is the account named, or empty when withdrawn. |
| `until` | int64 | until is when the proposal lapses, block time in unix seconds. |

### EventBusinessMoved

EventBusinessMoved is emitted when the named account accepts: it is now the jeweler's business account, holding the stock.

| Field | Type | Description |
| --- | --- | --- |
| `jeweler_id` | uint64 | jeweler_id is the jeweler whose business account moved. |
| `from` | string | from is the old business account. |
| `to` | string | to is the new business account. |
| `jewels` | uint64 | jewels is how many stock jewels moved with it. |

### EventCardIssued

EventCardIssued is emitted when a stock jewel gets an ownership card. A pending claim made with an earlier card is dropped with it, without an event of its own.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel the card is for. |
| `by` | string | by is the account that issued it: the business account or staff. |

### EventClaimApproved

EventClaimApproved is emitted when the shop approves a claim. The same transaction emits EventJewelTransferred with kind "card".

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel that moved. |
| `claimer` | string | claimer is the account it moved to. |
| `by` | string | by is the account that approved it: the business account or staff. |

### EventClaimFiled

EventClaimFiled is emitted when a buyer files a claim with the card.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel claimed. |
| `claimer` | string | claimer is the account the jewel goes to if the shop approves. |

### EventClaimRejected

EventClaimRejected is emitted when the shop rejects a claim. The card stays valid.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel whose claim was rejected. |
| `by` | string | by is the account that rejected it: the business account or staff. |

### EventClaimWithdrawn

EventClaimWithdrawn is emitted when a claimer withdraws their own claim.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel whose claim was withdrawn. |
| `claimer` | string | claimer is the account that withdrew it. |

### EventJewelAmended

EventJewelAmended is emitted when a correction is appended to a jewel's record.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel corrected. |
| `staff` | string | staff is the staff member who appended the correction. |

### EventJewelListed

EventJewelListed is emitted when a jewel is put up for sale, or its price changes.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel listed. |
| `seller` | string | seller is its owner. |
| `price` | Coin | price is the asking price. |

### EventJewelRegistered

EventJewelRegistered is emitted when a jewel is minted into a jeweler's stock.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the new jewel's id. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler that registered it. |
| `staff` | string | staff is the staff member who registered it. |

### EventJewelSold

EventJewelSold is emitted when a buyer pays for a listed jewel. The same transaction emits EventJewelTransferred with kind "sale".

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel sold. |
| `seller` | string | seller received the price less the fee. |
| `buyer` | string | buyer paid and now owns it. |
| `price` | Coin | price is what the buyer paid. |
| `fee` | Coin | fee is the part of the price kept as the marketplace fee. |

### EventJewelTransferred

EventJewelTransferred is emitted on every change of owner. Counting them is the jewel's owner count; their block times are the dates of each change.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel that changed owner. |
| `from` | string | from is the previous owner. |
| `to` | string | to is the new owner. |
| `kind` | string | kind is how it moved: "accept" (an offer to an account), "claim" (an offer to a claim key), "card" (a claim filed with the ownership card and approved by the shop) or "reassign" (the office's reassignment after its veto window). |

### EventJewelUnlisted

EventJewelUnlisted is emitted when its owner takes a jewel off the marketplace. A listing ended by a change of owner or a theft report emits no event of its own; the transfer or the report says it.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel withdrawn. |

### EventJewelerAdmitted

EventJewelerAdmitted is emitted when the office admits a jeweler.

| Field | Type | Description |
| --- | --- | --- |
| `jeweler_id` | uint64 | jeweler_id is the new jeweler's id. |
| `business` | string | business is the jeweler's business account, which holds its stock. |

### EventJewelerReinstated

EventJewelerReinstated is emitted when the office lets a suspended jeweler act again.

| Field | Type | Description |
| --- | --- | --- |
| `jeweler_id` | uint64 | jeweler_id is the reinstated jeweler. |

### EventJewelerSuspended

EventJewelerSuspended is emitted when the office suspends a jeweler.

| Field | Type | Description |
| --- | --- | --- |
| `jeweler_id` | uint64 | jeweler_id is the suspended jeweler. |

### EventOfferCancelled

EventOfferCancelled is emitted when an open offer is withdrawn.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel whose offer was withdrawn. |
| `by` | string | by is the account that withdrew it. |

### EventOfferMade

EventOfferMade is emitted when an offer opens.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel offered. |
| `from` | string | from is the owner the offer speaks for. |
| `maker` | string | maker is the account that signed the offer: from itself, or a staff member offering the jeweler's stock. |
| `to` | string | to is the recipient's account; empty when the offer is to a claim key. |
| `expires_at` | int64 | expires_at is when the offer lapses, in unix seconds. |

### EventReassignmentProposed

EventReassignmentProposed is emitted when the office opens a reassignment's veto window.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel to be moved. |
| `from` | string | from is the owner it would be taken from. |
| `to` | string | to is the account it would go to. |
| `window_ends` | int64 | window_ends is when the owner's veto window closes, in unix seconds. |

### EventReassignmentVetoed

EventReassignmentVetoed is emitted when the owner refuses a reassignment.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel that stays put. |
| `owner` | string | owner is the owner who vetoed it. |

### EventStaffChanged

EventStaffChanged is emitted when a jeweler's business account adds or removes staff. It lists only the changes that took effect.

| Field | Type | Description |
| --- | --- | --- |
| `jeweler_id` | uint64 | jeweler_id is the jeweler whose staff changed. |
| `added` | repeated string | added are the accounts that now act for the jeweler. |
| `removed` | repeated string | removed are the accounts that no longer act for it. |

### EventStolenCleared

EventStolenCleared is emitted when the stolen flag is lifted.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel cleared. |
| `by` | string | by is the account that lifted the flag. |

### EventStolenReported

EventStolenReported is emitted when a jewel is reported stolen.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel reported. |
| `by` | string | by is the account that reported it: the owner, or staff while it is stock. |

### GenesisCard

GenesisCard is one jewel's ownership card.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel the card is for. |
| `card` | Card | card is the card. |

### GenesisClaim

GenesisClaim is one jewel's pending claim.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel claimed. |
| `claim` | Claim | claim is the claim. |

### GenesisListing

GenesisListing is one marketplace listing, by jewel.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel for sale. |
| `listing` | Listing | listing is its price and seller. |

### Jewel

Jewel is one registered piece. Its token is x/nft class jewel-<jeweler_id>, id jewel-<id>. Ownership is x/nft's and is not copied here.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id is the jewel's number, assigned by the chain at registration; never 0. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler that registered it, and whose collection holds its token. |
| `inscription` | string | inscription is the normalised laser inscription: upper case, separators removed. |
| `record_hash` | string | record_hash is the SHA-256, in lower-case hex, of the record file (photos, materials, origin) as first registered. |
| `record_uri` | string | record_uri is where that record file can be fetched; at most 512 characters. |
| `registered_at` | int64 | registered_at is the block time of registration, in unix seconds. |
| `owner_count` | uint32 | owner_count counts owners since the jeweler sold it. Zero in stock. |
| `stolen` | bool | stolen is set while the owner reports it stolen; no offer or reassignment opens then. |
| `amendments` | repeated Amendment | amendments are the corrections staff appended, oldest first; at most 50. |
| `owner_since` | repeated int64 | owner_since holds, for each owner since the jeweler sold it, the block time in unix seconds at which they became the owner: the date of each change of owner, oldest first. It has owner_count entries; the date the jeweler's stock began is registered_at. |

### Jeweler

Jeweler is a business the office has admitted.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id is the jeweler's number, assigned by the chain at admission; never 0. |
| `name` | string | name is the business's display name, as the office admitted it. |
| `business` | string | business holds the jeweler's stock and manages its staff. |
| `staff` | repeated string | staff act for the jeweler. A person works for one jeweler at a time. |
| `status` | JewelerStatus | status says whether the jeweler may act; the office sets it. |
| `pending_business` | string | pending_business is the account the business account has proposed to hand the shop to; empty when no move is waiting. Nothing changes until that account accepts. |
| `pending_business_until` | int64 | pending_business_until is when the proposed move lapses, block time in unix seconds; 0 when none is waiting. |

### Listing

Listing is a jewel its owner has put up for sale on the marketplace. Any account may buy it at this price, in one transaction that pays the seller and moves the jewel. Any change of owner, or a theft report, ends it.

| Field | Type | Description |
| --- | --- | --- |
| `seller` | string | seller is the owner who listed it. |
| `price` | Coin | price is what the buyer pays, in a currency params.sale_denoms allows. The seller receives it less params.sale_fee_bps. |
| `listed_at` | int64 | listed_at is the block time it was listed, in unix seconds. |

### ListingEntry

ListingEntry is one jewel for sale with its listing.

| Field | Type | Description |
| --- | --- | --- |
| `jewel` | Jewel | jewel is the register's record of the jewel. |
| `listing` | Listing | listing is its price and seller. |

### Offer

Offer proposes handing a jewel to someone. Exactly one of to and claim_key is set.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel offered. |
| `from` | string | from is the owner the offer speaks for. It can be taken only while the jewel still belongs to from. |
| `to` | string | to is the recipient, when they already have an account. |
| `claim_key` | string | claim_key is the lower-case hex compressed secp256k1 public key of a one-time key sent by email, when the recipient has no account yet. The claimer proves it by signing their own address with it. |
| `expires_at` | int64 | expires_at is when the offer lapses, in unix seconds. |
| `maker` | string | maker is the account that signed the offer: equal to from when the owner made it, or the staff member who offered the jeweler's stock. A stock offer can be taken only while its maker is still staff of an active jeweler. |

### Reassignment

Reassignment is the office's proposal to move a jewel whose owner has lost their key. The owner may veto it until window_ends.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel to be moved. |
| `from` | string | from is the owner when it was proposed. A later change of owner voids it. |
| `to` | string | to is the account the office proposes to move it to. |
| `reason` | string | reason is the office's public account of why, written when it proposed. |
| `window_ends` | int64 | window_ends is when the veto window closes, in unix seconds. The owner may still veto after it, until someone completes the reassignment. |

### UsedNonce

UsedNonce is a voucher already spent.

| Field | Type | Description |
| --- | --- | --- |
| `benefit_id` | uint64 | benefit_id is the benefit the voucher was for. |
| `nonce` | string | nonce is the voucher's single-use nonce. |

### Voucher

Voucher is what the owner's app signs to consent to one use of a benefit.

| Field | Type | Description |
| --- | --- | --- |
| `benefit_id` | uint64 | benefit_id is the benefit to use. |
| `jewel_id` | uint64 | jewel_id is the benefit's jewel; it must match the benefit's. |
| `nonce` | string | nonce is single-use, at most 64 characters. |
| `expires_at` | int64 | expires_at is when the voucher stops being valid, in unix seconds; at most voucher_max_ttl after the block that redeems it. |

## Value types

### JewelerStatus

JewelerStatus says whether a jeweler may still act.

| Value | Meaning |
| --- | --- |
| `JEWELER_STATUS_UNSPECIFIED` | Never stored. |
| `JEWELER_ACTIVE` | May register jewels, sell stock and honour benefits. |
| `JEWELER_SUSPENDED` | Its jewels and their histories stay. It registers nothing, and its benefits are no longer honoured. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1 | `ErrNotOffice` | only the register's office may do this |
| 10 | `ErrStaffTaken` | this person already works for another jeweler |
| 11 | `ErrNotStaff` | you do not work for this jeweler |
| 12 | `ErrBareSendOpen` | this chain still lets an NFT be sent without an offer and acceptance; disable /cosmos.nft.v1beta1.MsgSend in x/circuit first |
| 13 | `ErrInvalidInscription` | the inscription must be 4 to 40 letters and digits; spaces, dashes, dots, slashes and underscores are ignored |
| 14 | `ErrInscriptionTaken` | this jeweler has already registered a jewel with this inscription |
| 15 | `ErrInvalidRecord` | the record needs its SHA-256 fingerprint in lower-case hex and a location of at most 512 characters |
| 16 | `ErrNoJewel` | no such jewel |
| 17 | `ErrNotOwner` | only the jewel's owner may do this |
| 18 | `ErrOfferOpen` | this jewel already has an open offer; cancel it first |
| 19 | `ErrNoOffer` | this jewel has no open offer |
| 2 | `ErrNoOffice` | no office is appointed yet; governance must appoint one before any jeweler is admitted |
| 20 | `ErrOfferExpired` | this offer has expired; ask the owner for a new one |
| 21 | `ErrOfferTarget` | an offer names either an account or a claim key, exactly one |
| 22 | `ErrSelfOffer` | the recipient already holds this jewel |
| 23 | `ErrNotRecipient` | this offer was made to someone else |
| 24 | `ErrBadClaim` | the claim key or its signature does not match this offer |
| 25 | `ErrStolen` | this jewel is reported stolen |
| 26 | `ErrNotStolen` | this jewel is not reported stolen |
| 27 | `ErrInvalidBenefit` | a benefit needs a kind of at most 80 characters, an allowance of at least 1, a period of zero or more, and an expiry in the future or none |
| 28 | `ErrNoBenefit` | no such benefit |
| 29 | `ErrBenefitExpired` | this benefit has expired |
| 3 | `ErrOfficeNotGroup` | the office must be a group account, so its decisions need several signatures |
| 30 | `ErrAllowanceUsed` | this benefit's allowance for the period is used up |
| 31 | `ErrBadVoucher` | the voucher is not valid; ask the owner to show a fresh code |
| 32 | `ErrVoucherUsed` | this voucher has already been used |
| 33 | `ErrReassignmentOpen` | a reassignment of this jewel is already pending |
| 34 | `ErrNoReassignment` | no reassignment of this jewel is pending |
| 35 | `ErrWindowOpen` | the owner's veto window has not closed yet |
| 36 | `ErrReassignmentStale` | the jewel changed owner after the reassignment was proposed |
| 37 | `ErrNoReason` | a reason is required |
| 38 | `ErrInvalidSigner` | only governance may change these settings |
| 39 | `ErrOfferStale` | the jewel changed owner after this offer was made |
| 4 | `ErrInvalidAddress` | that is not a valid account |
| 40 | `ErrNoteTooLong` | an amendment's note is at most 500 bytes; put the detail in the record file |
| 41 | `ErrTooManyAmendments` | this jewel has 50 amendments, the most it may carry; put further corrections in the record file of the last one |
| 42 | `ErrNoCard` | this jewel has no ownership card; ask the shop to print one |
| 43 | `ErrBadCardKey` | the card key must be a compressed secp256k1 public key in hex |
| 44 | `ErrCardNotLive` | this is not the jewel's current ownership card; the shop may have printed a newer one |
| 45 | `ErrBadCardClaim` | the card's signature does not match this claim |
| 46 | `ErrNotStock` | this jewel is no longer in the shop's stock |
| 47 | `ErrClaimPending` | a claim on this jewel is already waiting for the shop |
| 48 | `ErrNoClaim` | no claim on this jewel is waiting for the shop |
| 49 | `ErrNotClaimer` | this claim was filed by someone else |
| 5 | `ErrNoName` | a name is required |
| 50 | `ErrShopSelfClaim` | the shop's own business account cannot claim its stock |
| 51 | `ErrNoBusinessMove` | no move of this shop's business account is waiting for this account |
| 52 | `ErrBusinessMoveLapsed` | the proposed move of this shop's business account has lapsed; ask the shop to propose it again |
| 53 | `ErrAccountTied` | this account already runs a jeweler's business or works for another jeweler |
| 54 | `ErrBusinessOwnsPieces` | this account already owns pieces from this shop; as its business account they would become stock |
| 55 | `ErrMoveToSelf` | this is already the shop's business account |
| 56 | `ErrNotListed` | this jewel is not for sale |
| 57 | `ErrPriceMoved` | the price has changed since you looked; check the new price and buy again |
| 58 | `ErrDenomNotForSale` | jewels cannot be sold in this currency on the marketplace |
| 59 | `ErrNoPrice` | a price must be more than zero |
| 6 | `ErrBusinessTaken` | this business account already belongs to a jeweler |
| 60 | `ErrBuyOwn` | you already own this jewel |
| 7 | `ErrNoJeweler` | no such jeweler |
| 8 | `ErrJewelerSuspended` | this jeweler is suspended and can no longer act |
| 9 | `ErrNotBusiness` | only the jeweler's business account may manage its staff |
