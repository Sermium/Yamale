<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/land

## Transactions

### MsgAttachDeed

`/blockchain.land.v1.MsgAttachDeed`

Signed by the `creator` field.

AttachDeed adds a document to the chain of title.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `parcel_id` | uint64 |  |
| `kind` | string |  |
| `document_hash` | string |  |
| `uri` | string |  |
| `reference` | string |  |
| `issued_on` | string |  |

### MsgAttestTransfer

`/blockchain.land.v1.MsgAttestTransfer`

Signed by the `creator` field.

AttestTransfer adds one independent attestation toward quorum.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `transfer_id` | uint64 |  |

### MsgAuthoriseFractionalisation

`/blockchain.land.v1.MsgAuthoriseFractionalisation`

Signed by the `creator` field.

AuthoriseFractionalisation is the registry's permission for a tokenisation vehicle to be opened over a parcel. Without it, x/tokenisation refuses.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string | The authority in charge of the parcel. |
| `parcel_id` | uint64 |  |
| `right` | string | What may be sold: an exploitation right, a lease, a revenue share. Not the title — the title is not for sale in pieces. |
| `max_share_bps` | uint32 | Ceiling on the fraction that may be issued, in basis points. A holder permitted to sell 40% of the rent has 4000 here and keeps the other 6000. It is compared against the share the issued tokens carry, not against what the sponsor retains. Reading it the other way round inverts the rule and authorises exactly the issuance the office meant to forbid. |
| `expires_at` | int64 | Unix seconds at which the authorisation stops being live, so a permission granted for one purpose does not sit open for years. Required, and refused if it is already in the past: an authorisation with no expiry is indistinguishable from an unset field, and defaults to forever. |
| `withdraw` | bool | Withdrawing it stops new issuance. Existing holders are not expropriated by the registry — that would be a taking, and it belongs to a court. |

### MsgCompleteTransfer

`/blockchain.land.v1.MsgCompleteTransfer`

Signed by the `creator` field.

CompleteTransfer applies a transfer that has met every condition.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `transfer_id` | uint64 |  |

### MsgFreezeParcel

`/blockchain.land.v1.MsgFreezeParcel`

Signed by the `creator` field.

FreezeParcel stops all movement, or lifts a freeze.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `parcel_id` | uint64 |  |
| `reason` | string | The grounds. Required when freezing and recorded on the parcel as a Freeze entry, not merely checked and dropped: the reason a court order gives is the only part of it the holder can act on, and a stop with no readable grounds cannot be told apart from an arbitrary one. Optional when lifting, and recorded there too. An unattributed release is as hard to question as an unexplained freeze. |
| `unfreeze` | bool |  |

### MsgObject

`/blockchain.land.v1.MsgObject`

Signed by the `creator` field.

Object halts a transfer and marks the parcel disputed.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `transfer_id` | uint64 |  |
| `reason` | string |  |

### MsgProposeTransfer

`/blockchain.land.v1.MsgProposeTransfer`

Signed by the `creator` field.

ProposeTransfer opens a transfer. Signed by the holder.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `parcel_id` | uint64 |  |
| `to` | string |  |
| `price` | string |  |

### MsgRecordEncumbrance

`/blockchain.land.v1.MsgRecordEncumbrance`

Signed by the `creator` field.

RecordEncumbrance adds or releases a lien or right of way.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `parcel_id` | uint64 |  |
| `kind` | string |  |
| `holder` | string |  |
| `detail` | string |  |
| `release` | bool | Releasing marks the existing entry rather than deleting it: an encumbrance that disappears takes with it the evidence that it ever existed. |
| `index` | uint32 |  |

### MsgRegisterAuthority

`/blockchain.land.v1.MsgRegisterAuthority`

Signed by the `authority` field.

Admitting a registry office is a governance act. If an authority could admit another authority, buying one office would buy the power to manufacture the independent attestors the quorum depends on.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | x/gov. Registry offices are admitted by the chain's governance, never by each other. |
| `office` | string |  |
| `name` | string |  |
| `jurisdiction` | string |  |
| `active` | bool |  |

### MsgRegisterParcel

`/blockchain.land.v1.MsgRegisterParcel`

Signed by the `creator` field.

RegisterParcel records a parcel for the first time.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `geometry_hash` | string |  |
| `cadastral_ref` | string |  |
| `holder` | string |  |

### MsgSetRestriction

`/blockchain.land.v1.MsgSetRestriction`

Signed by the `creator` field.

SetRestriction imposes or lifts a limit on what may be done with a parcel.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `parcel_id` | uint64 |  |
| `kind` | string |  |
| `value` | string |  |
| `detail` | string |  |
| `lift` | bool | Lifting marks the restriction rather than removing it, so the record still shows the land was once constrained and who released it. |
| `index` | uint32 |  |

### MsgUpdateParams

`/blockchain.land.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams sets the quorum and challenge window. Governance only.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

### MsgValidateTransfer

`/blockchain.land.v1.MsgValidateTransfer`

Signed by the `creator` field.

ValidateTransfer records the jurisdiction's own validation.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `transfer_id` | uint64 |  |

## Queries

### Authorities

`GET /yamale/blockchain/land/v1/authorities`

Authorities lists the registry offices governance has admitted.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `authorities` | repeated Authority |  |
| `pagination` | PageResponse |  |

### FractionalisationAuthority

`GET /yamale/blockchain/land/v1/fractionalisation_authority/{parcel_id}`

FractionalisationAuthority returns the registry's standing permission to fractionalise a parcel, and whether it is still live.

Public for the same reason every other read here is: somebody being sold a share in a piece of land must be able to check, before paying, that the registry ever permitted the sale and that the permission has not been withdrawn or run out.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `parcel_id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `authorisation` | FractionalisationAuthority |  |
| `live` | bool | False when the permission has been withdrawn, has expired against the current block time, or the parcel now carries a restriction forbidding fractionalisation. Answered here rather than left to the caller so that a wallet and the keeper cannot disagree about what "live" means. |

### Params

`GET /yamale/blockchain/land/v1/params`

Params returns the quorum and challenge window currently in force.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

### Parcel

`GET /yamale/blockchain/land/v1/parcel/{id}`

Parcel returns one parcel by its chain id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `parcel` | Parcel |  |

### ParcelByGeometry

`GET /yamale/blockchain/land/v1/parcel_by_geometry`

ParcelByGeometry exposes the uniqueness check, so a surveyor can ask "is this ground already registered" before a second title is ever proposed.

In the query string for the same reason as ParcelByRef. The keeper accepts any non-empty string as a survey hash and deliberately does not impose an encoding, so a registry that records its hashes in base64 — an alphabet that includes '/' — would find this route unreachable for exactly the parcels it was asked about. A lookup that works until somebody changes their hash encoding is a lookup nobody can rely on.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `geometry_hash` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `parcel` | Parcel |  |

### ParcelByRef

`GET /yamale/blockchain/land/v1/parcel_by_ref`

ParcelByRef finds a parcel by the registry's own reference — the number written on the paper somebody is actually holding.

The reference travels in the query string rather than as a path segment, and that is not a style choice. Registry references contain slashes — ACC/GA/2019/00412 is an ordinary one — and a path template binds `{field}` to a single URL segment. A reference with slashes in it therefore matches no route at all, percent-encoded or raw, which made the module's primary lookup the one lookup that could not be performed over REST.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `cadastral_ref` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `parcel` | Parcel |  |

### ParcelsByHolder

`GET /yamale/blockchain/land/v1/parcels_by_holder/{holder}`

ParcelsByHolder lists what one account holds.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `holder` | string |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `parcels` | repeated Parcel |  |
| `pagination` | PageResponse |  |

### PendingTransfers

`GET /yamale/blockchain/land/v1/pending_transfers`

PendingTransfers lists transfers awaiting completion. The public list is what makes the challenge window meaningful: an objection cannot be filed against a transfer nobody can see.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `transfers` | repeated Transfer |  |
| `pagination` | PageResponse |  |

### Transfer

`GET /yamale/blockchain/land/v1/transfer/{id}`

Transfer returns one transfer, including who signed it and when.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `transfer` | Transfer |  |

### TransfersByParcel

`GET /yamale/blockchain/land/v1/transfers_by_parcel/{parcel_id}`

TransfersByParcel returns every transfer ever proposed for a parcel, including the abandoned and the disputed. This is the history that makes a theft arguable afterwards.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `parcel_id` | uint64 |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `transfers` | repeated Transfer |  |
| `pagination` | PageResponse |  |

## State

### Authority

Authority is a registry office permitted to register and validate.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |
| `name` | string |  |
| `jurisdiction` | string | Where this office may act. A parcel is validated by the office whose jurisdiction it falls in, and attested by offices elsewhere. |
| `active` | bool |  |

### Deed

Deed is one document in the chain of title.

| Field | Type | Description |
| --- | --- | --- |
| `kind` | string | What it is: grant, sale, inheritance, court order, survey. |
| `document_hash` | string | Hash of the document the registry holds. Proves which paper this refers to without publishing a scan that may carry somebody's personal details. |
| `uri` | string | Where the registry serves it from, for anyone entitled to read it. |
| `reference` | string | The registry's reference for the document, and its date in the paper world. |
| `issued_on` | string |  |
| `recorded_at` | int64 |  |

### Encumbrance

Encumbrance is a claim against a parcel that is not ownership.

| Field | Type | Description |
| --- | --- | --- |
| `kind` | string |  |
| `holder` | string |  |
| `detail` | string |  |
| `recorded_at` | int64 |  |
| `released` | bool | Released rather than deleted: an encumbrance that vanishes from the record takes with it the evidence that it ever constrained the title. |

### FractionalisationAuthority

FractionalisationAuthority is a registry office's standing permission for a tokenisation vehicle to be opened over one parcel.

It is a record of its own rather than a flag on the parcel because x/tokenisation has to be able to answer "may this be issued, and how much of it" at every issuance, long after the grant. A permission that existed only as a message in a past block is a permission nothing can enforce, which is how the ceiling and the expiry come to be decorative.

One per parcel: granting again replaces the terms rather than accumulating permissions, because a parcel carrying two ceilings has the higher one.

| Field | Type | Description |
| --- | --- | --- |
| `parcel_id` | uint64 |  |
| `right` | string | What may be sold: an exploitation right, a lease, a revenue share. Not the title — the title is not for sale in pieces and never leaves this module. |
| `max_share_bps` | uint32 | Ceiling on the fraction that may be issued, in basis points. A holder permitted to sell 40% of the rent has 4000 here and keeps the other 6000. |
| `expires_at` | int64 | Unix seconds. Past this the authorisation is not live and x/tokenisation refuses to issue against it, so a permission granted for one purpose does not sit open for years. |
| `granted_by` | string | The office that granted it, so the decision can be argued with. |
| `granted_at` | int64 |  |
| `withdrawn` | bool | Withdrawn rather than deleted, for the same reason a lifted restriction is marked rather than removed: a permission that vanishes takes with it the evidence that the registry ever gave it, and who gave it. Withdrawal stops new issuance and expropriates nobody. |
| `withdrawn_at` | int64 |  |

### Freeze

Freeze is one order stopping every dealing with a parcel, and the grounds the office gave for it.

Kept as a list on the parcel rather than as a single current entry, because a parcel frozen, released and frozen again by a different office is the exact history somebody contesting the second freeze needs to be able to show.

| Field | Type | Description |
| --- | --- | --- |
| `reason` | string | Why the land was stopped: the court order, the fraud inquiry, the succession dispute. Required by the keeper, and stored because a stop whose grounds cannot be read is indistinguishable from an arbitrary one. |
| `imposed_by` | string | The office that imposed it, so it can be argued with. |
| `imposed_at` | int64 |  |
| `lifted` | bool |  |
| `lifted_by` | string | The office that lifted it and the grounds it gave, recorded for the same reason as the freeze itself: releasing land is as consequential a decision as stopping it, and an unattributed release is one nobody can question. |
| `lift_reason` | string |  |
| `lifted_at` | int64 |  |

### Parcel

Parcel is a piece of ground that can be owned exactly once.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | Assigned by the chain. Never reused, never reassigned. |
| `geometry_hash` | string | Hash of the surveyed boundary (GeoJSON or the cadastral document). The survey itself is too large for a block and often too sensitive to publish; the hash proves which survey this title refers to without disclosing it. This field is the uniqueness constraint. The keeper indexes it, and registering a parcel whose hash already exists fails — that refusal is the whole "cannot be owned twice" guarantee. |
| `cadastral_ref` | string | The registry's own human reference, so this record can be reconciled with the paper world it has to coexist with for a very long time. |
| `holder` | string | Exactly one account. Deliberately not a list: co-ownership is a legal arrangement between people, expressed by the account being a group account, not by the chain holding several owners and having to rank them. |
| `authority` | string | The registry office whose jurisdiction this parcel falls in. |
| `status` | Status |  |
| `encumbrances` | repeated Encumbrance | Mortgages, liens, rights of way. Recorded because a title shown without its encumbrances is a lie that gets somebody's house taken. |
| `registered_at` | int64 | Block height of first registration. The audit trail starts here, including for the initial seeding of the registry, which is a political act and should be as auditable as everything after it. |
| `deeds` | repeated Deed | The deed documents themselves, as metadata on the token. A parcel is an NFT: one indivisible token, one holder, and everything a reader needs to understand the title travelling with it. Deeds are listed rather than embedded — a scan of a 1974 grant is megabytes and often contains personal data, so the chain carries its hash and a pointer, and the registry serves the document itself. |
| `restrictions` | repeated Restriction | What may lawfully be done with this land, set by the registry. These are the reason the registry stays in the loop after issuance. A parcel can be fractionalised later — an exploitation right sold in shares, rents flowing to holders — but only within these limits, and the land service must be able to see and stop a breach. Without them, tokenisation becomes a way to sell around the law. |
| `vehicle_id` | uint64 | Set when a tokenisation vehicle has been opened over this parcel. The parcel itself never moves into the vehicle: the title stays here, held by the same account, and the vehicle sells rights *referencing* it. That separation is what keeps a fractionalised parcel still governed by the registry rather than by whoever accumulated the tokens. |
| `freezes` | repeated Freeze | Every freeze ever imposed on this parcel, and the grounds given for it. The keeper refuses a freeze without a reason, so recording the status without the reason threw away the only part a court order actually consists of. A holder shown "your land is frozen" with no readable grounds cannot tell an inquiry from an extortion, and cannot argue with either — which is precisely the unaccountable act this module is written against. Lifting marks the entry rather than removing it, like a released encumbrance and a lifted restriction: a freeze that vanishes takes with it the evidence that an office ever stopped this land, and who did it. |

### Restriction

Restriction is a limit on what may be done with a parcel.

Kept as data rather than code because the limits differ by country and by parcel, and a chain that hard-codes one country's land law is a chain only that country can use.

| Field | Type | Description |
| --- | --- | --- |
| `kind` | string | agricultural_use_only, no_fractionalisation, foreign_ownership_capped, heritage_protected, minimum_parcel_size, customary_tenure. |
| `value` | string | The limit itself, where one applies — a percentage cap, a minimum size. |
| `detail` | string |  |
| `imposed_by` | string | The office that imposed it, so it can be argued with. |
| `imposed_at` | int64 |  |
| `lifted` | bool |  |

### Transfer

Transfer is a proposed change of holder, and the record of who agreed to it.

It is kept after completion, not deleted. The history of who signed what and when is the receipt that a dispossessed owner currently does not have, and deleting it to save space would remove the only thing that makes the theft arguable afterwards.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `parcel_id` | uint64 |  |
| `from` | string |  |
| `to` | string |  |
| `price` | string | Declared consideration. Recorded for the audit trail; the chain does not move it. Land paid for off-chain is the common case and pretending otherwise would make the record false. |
| `validated` | bool | Set when the jurisdiction's own office validates. |
| `validated_by` | string |  |
| `attestors` | repeated string | Independent registrars who have attested, one entry each. The keeper refuses an attestation from the proposing authority: an attestor from the same office is not independent, and allowing it collapses the whole mechanism back to a single bribe. |
| `quorum_at` | int64 | When quorum was reached. The challenge window runs from here, not from proposal, so the public clock starts only once the transfer is real. |
| `objected_by` | string | First objection, if any. One is enough to stop everything. |
| `objection_reason` | string |  |
| `proposed_at` | int64 |  |
| `completed_at` | int64 |  |

## Value types

### Status

Status is where a parcel is in its life. A parcel that is not REGISTERED cannot begin a transfer, which is what makes FROZEN and DISPUTED effective: they are not warnings, they are stops.

| Value | Meaning |
| --- | --- |
| `STATUS_UNSPECIFIED` | Never set on a stored parcel; the zero value only appears on a message that forgot to set one, and the keeper rejects it. |
| `STATUS_REGISTERED` | Held by exactly one account, and movable. |
| `STATUS_TRANSFER_PENDING` | A transfer is under way. No second transfer may start. |
| `STATUS_DISPUTED` | Somebody objected. Terminal for the transfer, and an entry point for a court. The chain preserves the evidence and does not adjudicate. |
| `STATUS_FROZEN` | Stopped by an authority plus quorum — a court order, a fraud inquiry. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1 | `ErrNotAuthority` | signer is not a registry office |
| 10 | `ErrParcelNotTransferable` | this parcel cannot be transferred in its current state |
| 11 | `ErrInvalidRecipient` | the recipient is not a valid account |
| 12 | `ErrSelfTransfer` | the recipient is already the holder |
| 13 | `ErrNoTransfer` | no such transfer |
| 14 | `ErrTransferClosed` | this transfer is already complete |
| 15 | `ErrTransferDisputed` | this transfer has been objected to |
| 16 | `ErrWrongJurisdiction` | only the office holding this parcel may validate it |
| 17 | `ErrAlreadyValidated` | already validated |
| 18 | `ErrNotIndependent` | an attestor from the parcel's own office is not independent |
| 19 | `ErrAlreadyAttested` | this office has already attested |
| 2 | `ErrAuthorityInactive` | this registry office is not active |
| 20 | `ErrNotValidated` | not yet validated by the office in charge |
| 21 | `ErrNoQuorum` | not enough independent attestations |
| 22 | `ErrChallengeWindowOpen` | the challenge window has not closed yet |
| 23 | `ErrNoReason` | an objection must give a reason |
| 24 | `ErrNotGovernance` | this message may only come from governance |
| 25 | `ErrNoDocument` | a document hash is required |
| 26 | `ErrNoRestriction` | no such restriction |
| 27 | `ErrNoRestrictionKind` | a restriction kind is required |
| 28 | `ErrNoEncumbrance` | no such encumbrance |
| 29 | `ErrNotFrozen` | this parcel is not frozen |
| 3 | `ErrNoGeometry` | a survey hash is required |
| 30 | `ErrBadShareCeiling` | the share ceiling must be between 1 and 10000 basis points |
| 31 | `ErrFractionalisationForbidden` | a restriction on this parcel forbids fractionalisation |
| 32 | `ErrOfficeNotGroup` | a registry office must be a group account, so its decisions need several signatures |
| 33 | `ErrBadExpiry` | the authorisation must expire at a time in the future |
| 34 | `ErrNoAuthorisation` | this parcel has no fractionalisation authorisation to withdraw |
| 35 | `ErrInvalidJurisdiction` | a registry office's jurisdiction must be an assigned ISO 3166-1 alpha-2 country code |
| 4 | `ErrNoCadastralRef` | a cadastral reference is required |
| 5 | `ErrInvalidHolder` | the holder is not a valid account |
| 6 | `ErrGeometryTitled` | this ground is already titled |
| 7 | `ErrRefTaken` | this cadastral reference is already used |
| 8 | `ErrNoParcel` | no such parcel |
| 9 | `ErrNotHolder` | only the holder may propose a transfer |
