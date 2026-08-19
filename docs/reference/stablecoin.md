<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/stablecoin

Governance-approved issuers for fiat-referenced currencies, with minting and redemption restricted to the approved issuer of each denom.

## Transactions

### MsgApproveIssuer

`/blockchain.stablecoin.v1.MsgApproveIssuer`

Signed by the `authority` field.

ApproveIssuer defines the ApproveIssuer RPC. It is authority-gated (the x/gov module account) and approves or rejects a pending currency registration submitted via MsgRegisterCurrency.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `denom` | string |  |
| `approve` | bool |  |

### MsgBurnCoin

`/blockchain.stablecoin.v1.MsgBurnCoin`

Signed by the `issuer` field.

BurnCoin defines the BurnCoin RPC.

| Field | Type | Description |
| --- | --- | --- |
| `issuer` | string |  |
| `denom` | string |  |
| `amount` | string |  |

### MsgMintCoin

`/blockchain.stablecoin.v1.MsgMintCoin`

Signed by the `issuer` field.

MintCoin defines the MintCoin RPC.

| Field | Type | Description |
| --- | --- | --- |
| `issuer` | string |  |
| `denom` | string |  |
| `amount` | string |  |
| `recipient` | string |  |

### MsgRegisterCurrency

`/blockchain.stablecoin.v1.MsgRegisterCurrency`

Signed by the `creator` field.

RegisterCurrency defines the RegisterCurrency RPC.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `denom` | string |  |
| `display_denom` | string |  |
| `exponent` | uint64 |  |
| `name` | string |  |
| `symbol` | string |  |
| `description` | string |  |

### MsgUpdateParams

`/blockchain.stablecoin.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

## Queries

### GetApprovedIssuer

`GET /yamale/blockchain/stablecoin/v1/approved_issuer_by_denom`

ListApprovedIssuer Queries a list of ApprovedIssuer items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `approved_issuer` | ApprovedIssuer |  |

### GetIssuerApplication

`GET /yamale/blockchain/stablecoin/v1/issuer_application_by_denom`

ListIssuerApplication Queries a list of IssuerApplication items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `issuer_application` | IssuerApplication |  |

### ListApprovedIssuer

`GET /yamale/blockchain/stablecoin/v1/approved_issuer`

ListApprovedIssuer defines the ListApprovedIssuer RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `approved_issuer` | repeated ApprovedIssuer |  |
| `pagination` | PageResponse |  |

### ListIssuerApplication

`GET /yamale/blockchain/stablecoin/v1/issuer_application`

ListIssuerApplication defines the ListIssuerApplication RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `issuer_application` | repeated IssuerApplication |  |
| `pagination` | PageResponse |  |

### Params

`GET /yamale/blockchain/stablecoin/v1/params`

Parameters queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

## State

### ApprovedIssuer

ApprovedIssuer defines the ApprovedIssuer message.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `issuer` | string |  |

### IssuerApplication

IssuerApplication defines the IssuerApplication message.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `status` | string |  |
| `creator` | string |  |
| `display_denom` | string |  |
| `exponent` | uint64 |  |
| `name` | string |  |
| `symbol` | string |  |
| `description` | string |  |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrCurrencyExists` | a currency is already registered or pending for this denom |
| 1102 | `ErrApplicationNotFound` | issuer application not found |
| 1103 | `ErrApplicationNotPending` | issuer application is not pending |
| 1104 | `ErrNotApprovedIssuer` | sender is not the approved issuer for this denom |
| 1105 | `ErrInvalidAmount` | invalid coin amount |
| 1110 | `ErrInvalidCurrency` | currency registration field is invalid or outside its limit |
| 1111 | `ErrFeeDenomNotIssued` | fee denomination has no approved issuer |
