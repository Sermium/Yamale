<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/emission

Replaces the standard mint module with a fixed, decaying issuance schedule that converges on a capped supply.

## Transactions

### MsgUpdateParams

`/blockchain.emission.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | params defines the module parameters to update. NOTE: All parameters must be supplied. |

## Queries

### GetEmissionState

`GET /yamale/blockchain/emission/v1/emission_state`

Queries a EmissionState by index.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `emission_state` | EmissionState |  |

### Params

`GET /yamale/blockchain/emission/v1/params`

Parameters queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

## State

### EmissionState

EmissionState defines the EmissionState message.

| Field | Type | Description |
| --- | --- | --- |
| `current_provisions_per_block` | string |  |
| `last_reduction_period` | uint64 |  |

## Parameters

Changed by governance through `MsgUpdateParams`. Defaults are the values a chain starts with at genesis.

| Parameter | Default | Description |
| --- | --- | --- |
| `reduction_period_in_blocks` | `100` | reduction_period_in_blocks is how many blocks make up one emission period; provisions_per_block is cut by reduction_factor at the start of each new period. |
| `reduction_factor` | `0.666666666666666667` | reduction_factor (e.g. "0.666666666666666667" for a 1/3 cut every period) applied to provisions_per_block at each period boundary. |
| `genesis_provisions_per_block` | `3333333333333` | genesis_provisions_per_block is the per-block emission amount (in the native base denom) during period 0, before any reduction is applied. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrInvalidState` | emission state contains an invalid provisions amount |
