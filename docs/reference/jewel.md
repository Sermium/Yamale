<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/jewel

## Transactions

### MsgAcceptOffer

`/blockchain.jewel.v1.MsgAcceptOffer`

Signed by the `recipient` field.

MsgAcceptOffer is the message that carries out AcceptOffer.

AcceptOffer takes a jewel offered to the signer's account.

| Field | Type | Description |
| --- | --- | --- |
| `recipient` | string |  |
| `jewel_id` | uint64 |  |

### MsgAdmitJeweler

`/blockchain.jewel.v1.MsgAdmitJeweler`

Signed by the `office` field.

MsgAdmitJeweler is the message that carries out AdmitJeweler.

AdmitJeweler admits a business and opens its collection. Office only.

| Field | Type | Description |
| --- | --- | --- |
| `office` | string |  |
| `name` | string |  |
| `business` | string |  |

### MsgAmendJewel

`/blockchain.jewel.v1.MsgAmendJewel`

Signed by the `staff` field.

MsgAmendJewel is the message that carries out AmendJewel.

AmendJewel appends a correction to a jewel's record.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string |  |
| `jewel_id` | uint64 |  |
| `record_hash` | string |  |
| `record_uri` | string |  |
| `note` | string |  |

### MsgAttachBenefit

`/blockchain.jewel.v1.MsgAttachBenefit`

Signed by the `staff` field.

MsgAttachBenefit is the message that carries out AttachBenefit.

AttachBenefit adds a benefit to a jewel.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string |  |
| `jewel_id` | uint64 |  |
| `kind` | string |  |
| `allowance` | uint32 |  |
| `period` | int64 |  |
| `expires_at` | int64 |  |

### MsgCancelOffer

`/blockchain.jewel.v1.MsgCancelOffer`

Signed by the `owner` field.

MsgCancelOffer is the message that carries out CancelOffer.

CancelOffer closes an open offer.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string |  |
| `jewel_id` | uint64 |  |

### MsgClaimOffer

`/blockchain.jewel.v1.MsgClaimOffer`

Signed by the `claimer` field.

MsgClaimOffer is the message that carries out ClaimOffer.

ClaimOffer takes a jewel offered to a claim key.

| Field | Type | Description |
| --- | --- | --- |
| `claimer` | string |  |
| `jewel_id` | uint64 |  |
| `claim_pub_key` | bytes | claim_pub_key is the compressed secp256k1 public key of the claim key. |
| `claim_signature` | bytes | claim_signature signs ClaimSignBytes(chain id, jewel id, claimer). |

### MsgClearStolen

`/blockchain.jewel.v1.MsgClearStolen`

Signed by the `owner` field.

MsgClearStolen is the message that carries out ClearStolen.

ClearStolen lifts the flag.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string |  |
| `jewel_id` | uint64 |  |

### MsgCompleteReassignment

`/blockchain.jewel.v1.MsgCompleteReassignment`

Signed by the `creator` field.

MsgCompleteReassignment is the message that carries out CompleteReassignment.

CompleteReassignment moves the jewel once the window has passed. Anyone.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `jewel_id` | uint64 |  |

### MsgOfferJewel

`/blockchain.jewel.v1.MsgOfferJewel`

Signed by the `owner` field.

MsgOfferJewel is the message that carries out OfferJewel.

OfferJewel opens an offer to an account or a claim key.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string | owner is the jewel's owner, or a staff member while it is stock. |
| `jewel_id` | uint64 |  |
| `to` | string |  |
| `claim_key` | string |  |

### MsgProposeReassignment

`/blockchain.jewel.v1.MsgProposeReassignment`

Signed by the `office` field.

MsgProposeReassignment is the message that carries out ProposeReassignment.

ProposeReassignment opens the public window. Office only.

| Field | Type | Description |
| --- | --- | --- |
| `office` | string |  |
| `jewel_id` | uint64 |  |
| `to` | string |  |
| `reason` | string |  |

### MsgRedeemBenefit

`/blockchain.jewel.v1.MsgRedeemBenefit`

Signed by the `staff` field.

MsgRedeemBenefit is the message that carries out RedeemBenefit.

RedeemBenefit records one use, with the owner's signed voucher.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string |  |
| `voucher` | Voucher |  |
| `owner_pub_key` | bytes | owner_pub_key is the compressed secp256k1 public key of the owner. |
| `owner_signature` | bytes | owner_signature signs VoucherSignBytes(chain id, voucher). |

### MsgRegisterJewel

`/blockchain.jewel.v1.MsgRegisterJewel`

Signed by the `staff` field.

MsgRegisterJewel is the message that carries out RegisterJewel.

RegisterJewel mints a jewel into its jeweler's stock.

| Field | Type | Description |
| --- | --- | --- |
| `staff` | string |  |
| `inscription` | string |  |
| `record_hash` | string |  |
| `record_uri` | string |  |

### MsgReinstateJeweler

`/blockchain.jewel.v1.MsgReinstateJeweler`

Signed by the `office` field.

MsgReinstateJeweler is the message that carries out ReinstateJeweler.

ReinstateJeweler lets a suspended jeweler act again. Office only.

| Field | Type | Description |
| --- | --- | --- |
| `office` | string |  |
| `jeweler_id` | uint64 |  |

### MsgReportStolen

`/blockchain.jewel.v1.MsgReportStolen`

Signed by the `owner` field.

MsgReportStolen is the message that carries out ReportStolen.

ReportStolen flags a jewel and closes its offer.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string |  |
| `jewel_id` | uint64 |  |

### MsgSetStaff

`/blockchain.jewel.v1.MsgSetStaff`

Signed by the `business` field.

MsgSetStaff is the message that carries out SetStaff.

SetStaff adds and removes the people who act for a jeweler.

| Field | Type | Description |
| --- | --- | --- |
| `business` | string |  |
| `jeweler_id` | uint64 |  |
| `add` | repeated string |  |
| `remove` | repeated string |  |

### MsgSuspendJeweler

`/blockchain.jewel.v1.MsgSuspendJeweler`

Signed by the `office` field.

MsgSuspendJeweler is the message that carries out SuspendJeweler.

SuspendJeweler stops a jeweler acting. Office only.

| Field | Type | Description |
| --- | --- | --- |
| `office` | string |  |
| `jeweler_id` | uint64 |  |
| `reason` | string |  |

### MsgUpdateParams

`/blockchain.jewel.v1.MsgUpdateParams`

Signed by the `authority` field.

MsgUpdateParams is the message that carries out UpdateParams.

UpdateParams sets the settings and appoints the office. Governance only; the office must be a group account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

### MsgVetoReassignment

`/blockchain.jewel.v1.MsgVetoReassignment`

Signed by the `owner` field.

MsgVetoReassignment is the message that carries out VetoReassignment.

VetoReassignment closes it. The current owner only.

| Field | Type | Description |
| --- | --- | --- |
| `owner` | string |  |
| `jewel_id` | uint64 |  |

## Queries

### Jewel

`GET /yamale/blockchain/jewel/v1/jewel/{id}`

Jewel returns one jewel with its owner, open offer, pending reassignment and benefits.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jewel` | Jewel |  |
| `owner` | string |  |
| `offer` | Offer | offer is set while an offer is open, expired or not. |
| `reassignment` | Reassignment | reassignment is set while one is pending. |
| `benefits` | repeated Benefit |  |

### Jeweler

`GET /yamale/blockchain/jewel/v1/jeweler/{id}`

Jeweler returns one jeweler.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jeweler` | Jeweler |  |

### Jewelers

`GET /yamale/blockchain/jewel/v1/jewelers`

Jewelers lists the jewelers, suspended ones included.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jewelers` | repeated Jeweler |  |
| `pagination` | PageResponse |  |

### JewelsByInscription

`GET /yamale/blockchain/jewel/v1/inscription/{inscription}`

JewelsByInscription finds every jewel carrying an inscription, whichever jeweler registered it. At most 50.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `inscription` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jewels` | repeated Jewel |  |

### Params

`GET /yamale/blockchain/jewel/v1/params`

Params returns the settings and the office.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

## State

### Amendment

Amendment is a correction appended to a jewel's record. The original and every correction stay visible.

| Field | Type | Description |
| --- | --- | --- |
| `record_hash` | string |  |
| `record_uri` | string |  |
| `note` | string |  |
| `staff` | string |  |
| `time` | int64 | time is the block time, in unix seconds. |

### Benefit

Benefit is a service a jeweler attaches to a jewel. It passes with the jewel to each new owner.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `jewel_id` | uint64 |  |
| `jeweler_id` | uint64 |  |
| `kind` | string | kind is the service as the jeweler words it, e.g. "cleaning". |
| `allowance` | uint32 | allowance is how many uses fit in one period. |
| `period` | int64 | period is the rolling window, in seconds, the allowance applies to. Zero means the allowance covers the benefit's whole life. |
| `expires_at` | int64 | expires_at is in unix seconds. Zero means it does not expire. |
| `uses` | repeated int64 | uses are the block times, in unix seconds, of every use. |

### Jewel

Jewel is one registered piece. Its token is x/nft class jewel-<jeweler_id>, id jewel-<id>. Ownership is x/nft's and is not copied here.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `jeweler_id` | uint64 |  |
| `inscription` | string | inscription is the normalised laser inscription: upper case, separators removed. |
| `record_hash` | string | record_hash is the SHA-256, in lower-case hex, of the record file (photos, materials, origin) as first registered. |
| `record_uri` | string |  |
| `registered_at` | int64 | registered_at is the block time of registration, in unix seconds. |
| `owner_count` | uint32 | owner_count counts owners since the jeweler sold it. Zero in stock. |
| `stolen` | bool |  |
| `amendments` | repeated Amendment |  |

### Jeweler

Jeweler is a business the office has admitted.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `name` | string |  |
| `business` | string | business holds the jeweler's stock and manages its staff. |
| `staff` | repeated string | staff act for the jeweler. A person works for one jeweler at a time. |
| `status` | JewelerStatus |  |

### Offer

Offer proposes handing a jewel to someone. Exactly one of to and claim_key is set.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 |  |
| `from` | string | from is the owner when the offer was made. |
| `to` | string | to is the recipient, when they already have an account. |
| `claim_key` | string | claim_key is the lower-case hex compressed secp256k1 public key of a one-time key sent by email, when the recipient has no account yet. The claimer proves it by signing their own address with it. |
| `expires_at` | int64 | expires_at is in unix seconds. |

### Reassignment

Reassignment is the office's proposal to move a jewel whose owner has lost their key. The owner may veto it until window_ends.

| Field | Type | Description |
| --- | --- | --- |
| `jewel_id` | uint64 |  |
| `from` | string | from is the owner when it was proposed. A later change of owner voids it. |
| `to` | string |  |
| `reason` | string |  |
| `window_ends` | int64 | window_ends is in unix seconds. |

### UsedNonce

UsedNonce is a voucher already spent.

| Field | Type | Description |
| --- | --- | --- |
| `benefit_id` | uint64 |  |
| `nonce` | string |  |

### Voucher

Voucher is what the owner's app signs to consent to one use of a benefit.

| Field | Type | Description |
| --- | --- | --- |
| `benefit_id` | uint64 |  |
| `jewel_id` | uint64 |  |
| `nonce` | string | nonce is single-use, at most 64 characters. |
| `expires_at` | int64 | expires_at is in unix seconds. |

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
| 4 | `ErrInvalidAddress` | that is not a valid account |
| 5 | `ErrNoName` | a name is required |
| 6 | `ErrBusinessTaken` | this business account already belongs to a jeweler |
| 7 | `ErrNoJeweler` | no such jeweler |
| 8 | `ErrJewelerSuspended` | this jeweler is suspended and can no longer act |
| 9 | `ErrNotBusiness` | only the jeweler's business account may manage its staff |
