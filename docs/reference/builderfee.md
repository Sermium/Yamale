<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/builderfee

Shares a governance-set portion of transaction fees with the developer whose message type was used.

## Transactions

### MsgApproveBuilder

`/blockchain.builderfee.v1.MsgApproveBuilder`

Signed by the `authority` field.

ApproveBuilder defines the ApproveBuilder RPC. It is authority-gated (the x/gov module account) and approves or rejects a pending builder registration submitted via MsgRegisterBuilder.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `msg_type_url` | string |  |
| `approve` | bool |  |

### MsgRegisterBuilder

`/blockchain.builderfee.v1.MsgRegisterBuilder`

Signed by the `creator` field.

RegisterBuilder defines the RegisterBuilder RPC.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `msg_type_url` | string |  |
| `payout_address` | string |  |

### MsgUpdateParams

`/blockchain.builderfee.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

## Queries

### GetApprovedBuilder

`GET /yamale/blockchain/builderfee/v1/approved_builder_by_msg_type`

ListApprovedBuilder Queries a list of ApprovedBuilder items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `msg_type_url` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `approved_builder` | ApprovedBuilder |  |

### GetBuilderApplication

`GET /yamale/blockchain/builderfee/v1/builder_application_by_msg_type`

ListBuilderApplication Queries a list of BuilderApplication items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `msg_type_url` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `builder_application` | BuilderApplication |  |

### ListApprovedBuilder

`GET /yamale/blockchain/builderfee/v1/approved_builder`

ListApprovedBuilder defines the ListApprovedBuilder RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `approved_builder` | repeated ApprovedBuilder |  |
| `pagination` | PageResponse |  |

### ListBuilderApplication

`GET /yamale/blockchain/builderfee/v1/builder_application`

ListBuilderApplication defines the ListBuilderApplication RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `builder_application` | repeated BuilderApplication |  |
| `pagination` | PageResponse |  |

### Params

`GET /yamale/blockchain/builderfee/v1/params`

Parameters queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

## State

### ApprovedBuilder

ApprovedBuilder defines the ApprovedBuilder message.

| Field | Type | Description |
| --- | --- | --- |
| `msg_type_url` | string |  |
| `payout_address` | string |  |

### BuilderApplication

BuilderApplication defines the BuilderApplication message.

| Field | Type | Description |
| --- | --- | --- |
| `msg_type_url` | string |  |
| `status` | string |  |
| `payout_address` | string |  |
| `creator` | string |  |

## Parameters

Changed by governance through `MsgUpdateParams`. Defaults are the values a chain starts with at genesis.

| Parameter | Default | Description |
| --- | --- | --- |
| `builder_fee_share_bps` | `3000` | builder_fee_share_bps is the share of each tx's gas fee (in basis points, out of 10000) paid to the registered builder for the first message type in the tx that has an approved builder. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrBuilderExists` | a builder application already exists for this message type |
| 1102 | `ErrApplicationNotFound` | builder application not found |
| 1103 | `ErrApplicationNotPending` | builder application is not pending |
| 1104 | `ErrInvalidMsgTypeURL` | that is not a message type URL this chain could route |
