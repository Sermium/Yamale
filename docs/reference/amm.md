<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/amm

A constant-product automated market maker: permissionless liquidity pools, and swaps priced by the pool's own reserves.

## Transactions

### MsgCreatePool

`/blockchain.amm.v1.MsgCreatePool`

Signed by the `creator` field.

CreatePool defines the CreatePool RPC.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `denom_a` | string |  |
| `amount_a` | string |  |
| `denom_b` | string |  |
| `amount_b` | string |  |
| `swap_fee_bps` | uint64 |  |

### MsgExitPool

`/blockchain.amm.v1.MsgExitPool`

Signed by the `sender` field.

ExitPool defines the ExitPool RPC.

| Field | Type | Description |
| --- | --- | --- |
| `sender` | string |  |
| `pool_id` | uint64 |  |
| `shares` | string |  |

### MsgJoinPool

`/blockchain.amm.v1.MsgJoinPool`

Signed by the `sender` field.

JoinPool defines the JoinPool RPC.

| Field | Type | Description |
| --- | --- | --- |
| `sender` | string |  |
| `pool_id` | uint64 |  |
| `amount_a` | string |  |
| `amount_b` | string |  |

### MsgSwap

`/blockchain.amm.v1.MsgSwap`

Signed by the `sender` field.

Swap defines the Swap RPC.

| Field | Type | Description |
| --- | --- | --- |
| `sender` | string |  |
| `pool_id` | uint64 |  |
| `token_in_denom` | string |  |
| `token_in_amount` | string |  |
| `token_out_denom` | string |  |
| `min_amount_out` | string |  |

### MsgUpdateParams

`/blockchain.amm.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

## Queries

### GetPool

`GET /yamale/blockchain/amm/v1/pool/{id}`

ListPool Queries a list of Pool items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `pool` | Pool |  |

### ListPool

`GET /yamale/blockchain/amm/v1/pool`

ListPool defines the ListPool RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `pool` | repeated Pool |  |
| `pagination` | PageResponse |  |

### Params

`GET /yamale/blockchain/amm/v1/params`

Parameters queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

## State

### Pool

Pool defines the Pool message.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `denom_a` | string |  |
| `denom_b` | string |  |
| `reserve_a` | string |  |
| `reserve_b` | string |  |
| `total_shares` | string |  |
| `swap_fee_bps` | uint64 |  |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrSameDenom` | pool denoms must be different |
| 1102 | `ErrInvalidAmount` | invalid coin amount |
| 1103 | `ErrPoolNotFound` | pool not found |
| 1104 | `ErrDenomNotInPool` | denom is not part of this pool |
| 1105 | `ErrInsufficientDeposit` | deposit does not cover the required proportional amount |
| 1106 | `ErrInsufficientShares` | insufficient LP shares |
| 1107 | `ErrSlippage` | swap output is below the minimum requested amount |
| 1109 | `ErrInvalidSwapFee` | swap fee is outside the permitted range |
| 1110 | `ErrInvalidDenom` | invalid denom |
| 1111 | `ErrWouldEmptyPool` | a pool cannot be exited completely; leave at least one share behind or nothing can ever join it again |
| 1112 | `ErrZeroOutput` | that swap would return nothing at all, so it is refused rather than settled |
| 1113 | `ErrCorruptPool` | this pool's stored reserves cannot be read as numbers |
