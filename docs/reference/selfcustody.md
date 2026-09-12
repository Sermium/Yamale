<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/selfcustody

## Transactions

### MsgCancelDisclosure

`/blockchain.selfcustody.v1.MsgCancelDisclosure`

Signed by the `account` field.

MsgCancelDisclosure stops a pending request.

Available to the holder at any time before confirmation, and it is the thing a person whose phone was stolen reaches for once the notice reaches their other devices.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `reason` | string |  |

### MsgConfirmDisclosure

`/blockchain.selfcustody.v1.MsgConfirmDisclosure`

Signed by the `account` field.

MsgConfirmDisclosure completes the pathway.

The acknowledgement is required AGAIN, and deliberately. The holder read the warning when they asked; the waiting period exists so they can change their mind, and a confirmation that did not re-state what they were agreeing to would turn the second step into a formality. If governance has published a new warning in the meantime, the old acknowledgement no longer matches and they must read the new one — which is the correct outcome, because the terms they are accepting have changed.

ConfirmDisclosure ends it. After this the key may be released and the platform's custody obligations over this account are discharged.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `acknowledgement_version` | string |  |
| `acknowledgement_hash` | string |  |

### MsgRequestDisclosure

`/blockchain.selfcustody.v1.MsgRequestDisclosure`

Signed by the `account` field.

MsgRequestDisclosure asks for the account's own private key.

Signed by the account, which is the only signature that could mean anything here: the request is a statement about the signer's own property, and no operator attestation could substitute for it.

The acknowledgement hash must match the one governance published. A request naming a different hash is refused rather than recorded, because it means the holder was shown some other text — and the entire evidentiary value of this record is that we know exactly what they read.

RequestDisclosure starts the waiting period.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `acknowledgement_version` | string |  |
| `acknowledgement_hash` | string |  |

### MsgUpdateParams

`/blockchain.selfcustody.v1.MsgUpdateParams`

Signed by the `authority` field.

MsgUpdateParams sets the delay and the acknowledgement text.

Governance only.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

## Queries

### Count

`GET /yamale/blockchain/selfcustody/v1/count`

Count is how many accounts have taken their keys. Published so the rate is visible — an operator quietly pushing customers out of custody should be observable — without naming anybody.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `disclosed` | uint64 |  |
| `pending` | uint64 |  |

### Disclosure

`GET /yamale/blockchain/selfcustody/v1/disclosure/{account}`

Disclosure reports whether one account's key has been taken.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `disclosed` | bool |  |
| `disclosure` | Disclosure |  |

### Params

`GET /yamale/blockchain/selfcustody/v1/params`

Params returns the delay and the acknowledgement text now in force.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

### Request

`GET /yamale/blockchain/selfcustody/v1/request/{account}`

Request reports a pending request and when it becomes confirmable.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `pending` | bool |  |
| `request` | Request |  |
| `eligible_at` | int64 | When the holder may confirm, and when the request lapses. Both computed rather than stored, so a change to the delay applies to requests already in flight — which is the honest behaviour when governance decides the waiting period was too short. |
| `expires_at` | int64 |  |

## State

### Disclosure

Disclosure is the permanent record that one account's key was taken.

There is no message that removes one, and that is not an oversight. A key cannot be un-disclosed. Re-sharing an account produces new shares under the SAME public key — which is exactly why a password reset does not change the address and does not retire the x/alias identifier — so the exported key keeps signing for ever. Nothing the operator or the holder can subsequently do makes this account safe again; the only remedy is to move the funds to a new account, and the record says so rather than implying a repair exists.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `requested_at` | int64 | When the holder first asked, and when they confirmed. Both are kept: the gap between them is the evidence that the waiting period was served, and it is the first thing anybody reviewing a disputed export will look for. |
| `disclosed_at` | int64 |  |
| `disclosed_at_height` | int64 | The height, because a timestamp is a claim about a clock and a height is a position in a chain other people also hold. |
| `acknowledgement_version` | string | Exactly which warning was shown. The version is for a human reading a dispute; the hash is what makes the claim checkable, because a version string can be reused over edited text and a hash cannot. |
| `acknowledgement_hash` | string |  |

### EventDisclosed

EventDisclosed is emitted once, when custody of an account's key ends.

It names the acknowledgement version and hash so that an indexer, a counterparty or an auditor can establish exactly what was agreed from the event stream alone, without reconstructing state at that height.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `requested_at` | int64 |  |
| `disclosed_at` | int64 |  |
| `disclosed_at_height` | int64 |  |
| `acknowledgement_version` | string |  |
| `acknowledgement_hash` | string |  |

### EventDisclosureCancelled

EventDisclosureCancelled is emitted when a request is stopped.

Also worth notifying. A cancellation the holder did not make is the signal that somebody with access to the account is watching for their own request to go through, and the holder should be told that as loudly as they were told about the request.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `cancelled_at` | int64 |  |
| `reason` | string |  |

### EventDisclosureRequested

EventDisclosureRequested is emitted when a holder starts the pathway.

This is the one every enrolled device should turn into a push notification, an email and an SMS. It carries the deadlines rather than making the reader fetch parameters and do arithmetic, because the question it has to answer at a glance is "how long do I have to stop this".

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `requested_at` | int64 |  |
| `eligible_at` | int64 | When the holder may confirm — and therefore the moment by which somebody who did not make this request must have cancelled it. |
| `expires_at` | int64 | When the request lapses on its own. |

### Request

Request is a disclosure that has been asked for and not yet confirmed.

Removed when the holder confirms or cancels, so this collection holds only live requests. The waiting period exists for one specific person: the account holder whose phone was taken. They cannot be helped by the custodian's usual defences once a key is out — a freeze does not bind a key somebody holds in plaintext, and recovery re-shares the same public key — so the only protection available is time plus notice, and this is the time.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `requested_at` | int64 |  |
| `acknowledgement_version` | string |  |
| `acknowledgement_hash` | string |  |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1 | `ErrNotAuthority` | this message is governance-only |
| 2 | `ErrAlreadyDisclosed` | this account's key has already been disclosed; it cannot be disclosed twice and it cannot be undone |
| 3 | `ErrRequestPending` | a disclosure is already pending for this account |
| 4 | `ErrNoRequest` | there is no pending disclosure request for this account |
| 5 | `ErrTooSoon` | the waiting period has not run; this is the time in which a stolen device can be noticed |
| 6 | `ErrRequestExpired` | this request has lapsed and must be made again |
| 7 | `ErrWrongAcknowledgement` | that is not the warning this chain publishes; you were shown something else |
| 8 | `ErrInvalidAccount` | that is not a valid account |
| 9 | `ErrInvalidParams` | these parameters cannot be enforced |
