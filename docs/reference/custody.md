<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/custody

## Transactions

### MsgAttestDeposit

`/blockchain.custody.v1.MsgAttestDeposit`

Signed by the `attestor` field.

AttestDeposit records that a deposit was seen on the source chain.

| Field | Type | Description |
| --- | --- | --- |
| `attestor` | string |  |
| `denom` | string |  |
| `recipient` | string |  |
| `amount` | string |  |
| `external_ref` | string |  |

### MsgRegisterAsset

`/blockchain.custody.v1.MsgRegisterAsset`

Signed by the `authority` field.

RegisterAsset lists an asset this chain will issue claims on. Governance.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `denom` | string |  |
| `source_chain` | string |  |
| `symbol` | string |  |
| `exponent` | uint32 |  |

### MsgReportReserve

`/blockchain.custody.v1.MsgReportReserve`

Signed by the `attestor` field.

ReportReserve states what the custodian holds against an asset.

| Field | Type | Description |
| --- | --- | --- |
| `attestor` | string |  |
| `denom` | string |  |
| `held` | string |  |

### MsgRequestRedemption

`/blockchain.custody.v1.MsgRequestRedemption`

Signed by the `redeemer` field.

RequestRedemption burns a claim and queues the payout.

| Field | Type | Description |
| --- | --- | --- |
| `redeemer` | string |  |
| `denom` | string |  |
| `amount` | string |  |
| `destination` | string |  |

### MsgSetAttestor

`/blockchain.custody.v1.MsgSetAttestor`

Signed by the `authority` field.

SetAttestor appoints or removes an attestor. Governance.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `attestor` | string |  |
| `active` | bool |  |

### MsgSettleRedemption

`/blockchain.custody.v1.MsgSettleRedemption`

Signed by the `attestor` field.

SettleRedemption records that the payout was made.

| Field | Type | Description |
| --- | --- | --- |
| `attestor` | string |  |
| `redemption_id` | string |  |
| `settled_ref` | string |  |

### MsgUpdateParams

`/blockchain.custody.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams sets the module parameters. Governance.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

## Queries

### Assets

`GET /yamale/blockchain/custody/v1/assets`

Assets lists everything this chain issues claims on.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `assets` | repeated Asset |  |

### Deposit

`GET /yamale/blockchain/custody/v1/deposit`

Deposit returns one deposit by id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `deposit` | Deposit |  |

### Params

`GET /yamale/blockchain/custody/v1/params`

Params returns the module parameters.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

### Redemption

`GET /yamale/blockchain/custody/v1/redemption/{id}`

Redemption returns one redemption by id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `redemption` | Redemption |  |

### Solvency

`GET /yamale/blockchain/custody/v1/solvency`

Solvency answers "are we holding what we owe", per asset.

Deliberately public and deliberately computed rather than reported: if the number cannot be checked by anyone, the whole arrangement rests on trusting the operator, and this is a chain whose purpose is not having to.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `solvency` | repeated Solvency |  |

## State

### Asset

Asset is one externally-custodied thing this chain can issue a claim on.

The claim and the asset are the same unit, always. Deposit one ETH, hold one yeth, redeem one ETH. That identity is the whole design: where the asset and the liability are the same unit there is no price in the arrangement, so there is no position for the treasury to be wrong about.

The alternative — minting the native token against foreign collateral — is a written put the treasury is short, permanently and unhedged. See docs/guides/custody.md.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string | denom on this chain, e.g. "yeth". |
| `source_chain` | string | chain the real asset sits on, e.g. "ethereum". Informational: this module never talks to it. |
| `symbol` | string | symbol and exponent for display. ETH needs 18, which x/stablecoin already permits. |
| `exponent` | uint32 |  |
| `paused` | bool | paused stops new issuance without stopping redemption. Redemption must never be pausable by the operator — an issuer who can stop you leaving is not a custodian. |

### Attestation

Attestation is one attestor's statement that a deposit happened.

Held individually rather than as a counter so that who said what stays on the record. A threshold reached by two attestors is a fact about those two, and if one is later found compromised, every mint they contributed to is identifiable.

| Field | Type | Description |
| --- | --- | --- |
| `deposit_id` | string |  |
| `attestor` | string |  |
| `attested_at_height` | int64 |  |

### Deposit

Deposit is an external payment being turned into a claim on this chain.

| Field | Type | Description |
| --- | --- | --- |
| `id` | string |  |
| `denom` | string |  |
| `recipient` | string | recipient of the minted claim. |
| `amount` | string |  |
| `external_ref` | string | external_ref is the transaction on the source chain. Unique per asset: the same deposit must never be creditable twice, and this is what makes a replay detectable rather than merely unlikely. |
| `status` | DepositStatus |  |
| `created_at_height` | int64 |  |

### Redemption

Redemption is a claim being burned to release the real asset.

The burn happens when the redemption is requested, not when it is paid. Leaving the claim in circulation while the asset is being sent would let the same claim be spent again and redeemed twice.

| Field | Type | Description |
| --- | --- | --- |
| `id` | string |  |
| `denom` | string |  |
| `redeemer` | string |  |
| `amount` | string |  |
| `destination` | string | destination on the source chain. Opaque to this module. |
| `status` | RedemptionStatus |  |
| `requested_at_height` | int64 |  |
| `payable_at_height` | int64 | payable_at_height is when the delay expires. Stored rather than computed so a parameter change cannot retroactively move an existing redemption. |
| `settled_ref` | string | external_ref of the payout, once settled. |

### Reserve

Reserve is what the custodian says it holds against an asset.

Attested rather than derived: the chain cannot see another chain's balances, so this is a signed statement, and it is only as good as the attestors. What the chain *can* do is compare it with what it has issued, which is what makes "are we solvent" answerable by anyone rather than by the operator.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `held` | string |  |
| `as_of_height` | int64 |  |
| `attestor` | string |  |

### Solvency

Solvency is the answer to the only question that matters, computed rather than asserted: issued versus held, per asset.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `issued` | string | issued is the chain's own total supply of the claim. The chain knows this for certain. |
| `held` | string | held is the last attested reserve. The chain takes this on trust. |
| `reserve_age_blocks` | int64 | reserve_age_blocks is how stale that statement is. A reserve figure with no age is a number somebody can leave unchanged while the money leaves. |
| `solvent` | bool |  |

## Value types

### DepositStatus

DepositStatus is where a deposit stands.

| Value | Meaning |
| --- | --- |
| `DEPOSIT_STATUS_UNSPECIFIED` | DEPOSIT_STATUS_UNSPECIFIED is the unset zero value. |
| `DEPOSIT_STATUS_PENDING` | waiting for enough attestors. |
| `DEPOSIT_STATUS_CREDITED` | threshold reached and the claim minted. |
| `DEPOSIT_STATUS_REJECTED` | refused by governance before crediting. |

### RedemptionStatus

RedemptionStatus is where a redemption stands.

| Value | Meaning |
| --- | --- |
| `REDEMPTION_STATUS_UNSPECIFIED` | REDEMPTION_STATUS_UNSPECIFIED is the unset zero value. |
| `REDEMPTION_STATUS_PENDING` | REDEMPTION_STATUS_PENDING is burned and waiting out the delay. |
| `REDEMPTION_STATUS_SETTLED` | REDEMPTION_STATUS_SETTLED is paid on the source chain. |
| `REDEMPTION_STATUS_CANCELLED` | cancelled before payout; the claim is minted back to the redeemer. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 10 | `ErrAlreadySettled` | that redemption has already been settled |
| 11 | `ErrInvalidAmount` | amount must be positive |
| 12 | `ErrInvalidParams` | invalid parameters |
| 13 | `ErrInvalidSigner` | invalid authority for this message |
| 2 | `ErrUnknownAsset` | no such asset is registered for custody |
| 3 | `ErrAssetExists` | that asset is already registered |
| 4 | `ErrNotAttestor` | this account is not an appointed attestor |
| 5 | `ErrAlreadyAttested` | this attestor has already attested to that deposit |
| 6 | `ErrDuplicateRef` | that external reference has already been credited |
| 7 | `ErrIssuancePaused` | issuance is paused for that asset |
| 8 | `ErrNotFound` | no such record |
| 9 | `ErrNotPayableYet` | this redemption is still inside its delay window |
