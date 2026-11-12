<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/jewel

A register of jewels under admitted jewelers: each piece an x/nft token that changes owner only by offer and acceptance, with owner-consented benefits, a stolen flag, and the office's reassignment of a lost key behind a 30-day public veto window.

## Transactions

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
| `claim_signature` | bytes | claim_signature signs ClaimSignBytes(chain id, jewel id, claimer). |

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

### MsgReportStolen

`/blockchain.jewel.v1.MsgReportStolen`

Signed by the `owner` field.

MsgReportStolen is the message that carries out ReportStolen.

ReportStolen flags a jewel and closes its offer.

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

SuspendJeweler stops a jeweler acting. Office only.

| Field | Type | Description |
| --- | --- | --- |
| `office` | string | office is the register's office, the signer. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler to suspend. |
| `reason` | string | reason is the office's public account of why; required. |

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

## Queries

### Jewel

`GET /yamale/blockchain/jewel/v1/jewel/{id}`

Jewel returns one jewel with its owner, open offer, pending reassignment and benefits.

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

### EventJewelAmended

EventJewelAmended is emitted when a correction is appended to a jewel's record.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel corrected. |
| `staff` | string | staff is the staff member who appended the correction. |

### EventJewelRegistered

EventJewelRegistered is emitted when a jewel is minted into a jeweler's stock.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the new jewel's id. |
| `jeweler_id` | uint64 | jeweler_id is the jeweler that registered it. |
| `staff` | string | staff is the staff member who registered it. |

### EventJewelTransferred

EventJewelTransferred is emitted on every change of owner. Counting them is the jewel's owner count; their block times are the dates of each change.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 | jewel_id is the jewel that changed owner. |
| `from` | string | from is the previous owner. |
| `to` | string | to is the new owner. |
| `kind` | string | kind is how it moved: "accept" (an offer to an account), "claim" (an offer to a claim key) or "reassign" (the office's reassignment after its veto window). |

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
| 5 | `ErrNoName` | a name is required |
| 6 | `ErrBusinessTaken` | this business account already belongs to a jeweler |
| 7 | `ErrNoJeweler` | no such jeweler |
| 8 | `ErrJewelerSuspended` | this jeweler is suspended and can no longer act |
| 9 | `ErrNotBusiness` | only the jeweler's business account may manage its staff |
