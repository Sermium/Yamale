<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/oracle

## Transactions

### MsgApplyAppraiser

`/blockchain.oracle.v1.MsgApplyAppraiser`

Signed by the `creator` field.

ApplyAppraiser asks to be admitted as an independent valuer.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `name` | string |  |
| `credentials` | string |  |
| `class_ids` | repeated string |  |

### MsgApproveAppraiser

`/blockchain.oracle.v1.MsgApproveAppraiser`

Signed by the `authority` field.

ApproveAppraiser records governance's decision on an application.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `appraiser` | string |  |
| `approve` | bool |  |
| `class_ids` | repeated string | class_ids may narrow the scope the applicant asked for. Governance admitting somebody to value less than they requested should not require them to reapply. |

### MsgDelegateFeeder

`/blockchain.oracle.v1.MsgDelegateFeeder`

Signed by the `operator` field.

DelegateFeeder nominates a hot key to submit a validator's votes.

| Field | Type | Description |
| --- | --- | --- |
| `operator` | string | operator is the validator's own account, and the only signer that may change who votes on its behalf. |
| `validator` | string |  |
| `feeder` | string |  |

### MsgRevokeAppraiser

`/blockchain.oracle.v1.MsgRevokeAppraiser`

Signed by the `authority` field.

RevokeAppraiser withdraws an approved valuer's authority.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `appraiser` | string |  |
| `reason` | string |  |

### MsgSubmitAppraisal

`/blockchain.oracle.v1.MsgSubmitAppraisal`

Signed by the `appraiser` field.

SubmitAppraisal records a signed valuation of a tokenised asset.

| Field | Type | Description |
| --- | --- | --- |
| `appraiser` | string |  |
| `class_id` | string |  |
| `nft_id` | string |  |
| `value` | string |  |
| `value_denom` | string |  |
| `valued_at` | int64 | valued_at is the date the valuation reflects, not the date it was sent. |
| `method` | string |  |
| `report_uri` | string |  |
| `report_hash` | string |  |

### MsgSubmitExchangeRates

`/blockchain.oracle.v1.MsgSubmitExchangeRates`

Signed by the `feeder` field.

SubmitExchangeRates reports one validator's observed prices for the current voting round.

| Field | Type | Description |
| --- | --- | --- |
| `feeder` | string |  |
| `validator` | string | validator is the operator this report is on behalf of. |
| `rates` | repeated RateVote |  |

### MsgUpdateParams

`/blockchain.oracle.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

## Queries

### Appraisal

`GET /yamale/blockchain/oracle/v1/appraisal`

Appraisal queries the current valuation of one tokenised asset.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `class_id` | string |  |
| `nft_id` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `appraisal` | Appraisal |  |
| `stale` | bool | stale is true when the valuation is older than max_appraisal_age_seconds, measured from valued_at rather than from when it was submitted. |
| `age_seconds` | int64 |  |
| `appraiser_still_approved` | bool | appraiser_still_approved is false when the valuer's authority has since been withdrawn. The appraisal remains valid history, but a consumer should weigh it differently. |

### AppraisalHistory

`GET /yamale/blockchain/oracle/v1/appraisal_history`

AppraisalHistory queries every valuation ever recorded for one asset, newest first.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `class_id` | string |  |
| `nft_id` | string |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `appraisals` | repeated Appraisal |  |
| `pagination` | PageResponse |  |

### ExchangeRate

`GET /yamale/blockchain/oracle/v1/rate_by_denom`

ExchangeRate queries the agreed price of one denom.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `rate` | ExchangeRate |  |
| `stale` | bool | stale is true when the rate is older than max_rate_age_seconds. A stale rate is still returned — hiding it would leave a caller unable to tell "no feed" from "feed stopped" — but it must not be acted on. |
| `age_seconds` | int64 |  |

### ExchangeRates

`GET /yamale/blockchain/oracle/v1/rate`

ExchangeRates queries every agreed price.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `rates` | repeated ExchangeRate |  |
| `pagination` | PageResponse |  |

### FeederDelegation

`GET /yamale/blockchain/oracle/v1/feeder/{validator}`

FeederDelegation queries which account votes for a validator.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `validator` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `feeder` | string | feeder is the validator's own account when no delegation has been made. |

### GetAppraiser

`GET /yamale/blockchain/oracle/v1/appraiser/{address}`

GetAppraiser queries one valuer.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `appraiser` | Appraiser |  |

### ListAppraiser

`GET /yamale/blockchain/oracle/v1/appraiser`

ListAppraiser queries every valuer and applicant.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `appraisers` | repeated Appraiser |  |
| `pagination` | PageResponse |  |

### MissCounters

`GET /yamale/blockchain/oracle/v1/miss`

MissCounters queries how reliably each validator has been reporting.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `counters` | repeated MissCounter |  |
| `pagination` | PageResponse |  |

### Params

`GET /yamale/blockchain/oracle/v1/params`

Params queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

## State

### Appraisal

Appraisal is one valuer's signed opinion of what a specific tokenised asset is worth.

It is bound to an NFT — the token that represents the asset — rather than to a denom, because each real-world asset is individual. Two invoices from the same issuer are not interchangeable, and a fungible price for them would be a fiction.

| Field | Type | Description |
| --- | --- | --- |
| `class_id` | string | class_id and nft_id identify the asset in x/nft. |
| `nft_id` | string |  |
| `value` | string | value is what the asset is worth, in base units of value_denom. |
| `value_denom` | string |  |
| `appraiser` | string | appraiser is the address that signed this valuation. |
| `valued_at` | int64 | valued_at is the date the valuation reflects, in Unix seconds. This is the date of the inspection or the NAV, which is not the same as when it was submitted — a monthly NAV published late is still a month-end number, and treating the two as the same would overstate how current it is. |
| `submitted_at` | int64 | submitted_at is when the chain recorded it. |
| `submitted_height` | uint64 |  |
| `method` | string | method describes how the number was reached, e.g. "RICS Red Book", "discounted cash flow", "administrator NAV". |
| `report_uri` | string | report_uri points at the signed valuation document, and report_hash pins its contents so the on-chain number and the off-chain report cannot drift apart unnoticed. |
| `report_hash` | string |  |
| `superseded` | bool | superseded is true once a newer appraisal for the same asset exists. Old appraisals are retained rather than overwritten, because the history of what an asset was said to be worth, and by whom, is exactly what an auditor needs after a dispute. |

### Appraiser

Appraiser is an independent party that governance has admitted to value real-world assets.

Independence is the whole point: the party that values the collateral must not be the party that benefits from the valuation. The chain cannot verify independence, so it does the next best thing — it records who governance admitted, what they were admitted to value, and attributes every number they sign.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |
| `name` | string | name is the firm or individual, for the audit trail. |
| `credentials` | string | credentials points at the licence, registration or accreditation that governance relied on when admitting them. |
| `class_ids` | repeated string | class_ids limits what they may value: an NFT class each corresponds to an asset type. Empty means every class, which governance should grant rarely. |
| `status` | AppraiserStatus |  |
| `approved_at_height` | uint64 |  |

### ExchangeRate

ExchangeRate is the chain's agreed price for one denom, in terms of the quote symbol set in the module's parameters.

The rate prices one *display* unit, not one base unit — 1 YML, not 1 uyml — because that is the number a person quotes and the number a feeder reads off a market. Consumers scale by the denom's exponent.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `rate` | string | rate is a decimal string, e.g. "0.4213". |
| `updated_at` | int64 | updated_at is when this rate was agreed, in Unix seconds. Staleness is judged from here, so a feed that stops reporting stops being usable rather than silently freezing at its last value. |
| `updated_height` | int64 |  |
| `voting_power_bps` | uint64 | voting_power is the share of stake, in basis points, that contributed to this median. A rate agreed by a bare quorum deserves less trust than one agreed by everybody, and consumers can see the difference. |

### ExchangeRateVote

ExchangeRateVote is one validator's report for the current vote period.

| Field | Type | Description |
| --- | --- | --- |
| `validator` | string |  |
| `denom` | string |  |
| `rate` | string |  |

### FeederDelegation

FeederDelegation lets a validator nominate a hot key to submit its votes.

Without this a validator would have to sign with its operator key every vote period, which means keeping that key online — the one key whose compromise costs the most. The delegate can only vote; it cannot move stake, change commission, or do anything else on the validator's behalf.

| Field | Type | Description |
| --- | --- | --- |
| `validator` | string |  |
| `feeder` | string |  |

### MissCounter

MissCounter records how many vote periods a validator failed to report in.

It is recorded rather than punished. On a small permissioned network an automatic slash mostly punishes the operator whose VM rebooted, and the social layer — a known set of accountable validators — handles absence better than an incentive that fires during an outage. Governance can add a penalty later; it cannot easily undo one that fired wrongly.

| Field | Type | Description |
| --- | --- | --- |
| `validator` | string |  |
| `misses` | uint64 |  |
| `windows` | uint64 |  |

### RateVote

RateVote is one reported price within a submission.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `rate` | string |  |

## Value types

### AppraiserStatus

AppraiserStatus mirrors the application lifecycle used elsewhere on the chain.

| Value | Meaning |
| --- | --- |
| `APPRAISER_STATUS_UNSPECIFIED` | APPRAISER_STATUS_UNSPECIFIED is the unset default. |
| `APPRAISER_STATUS_PENDING` | APPRAISER_STATUS_PENDING is awaiting a governance decision. |
| `APPRAISER_STATUS_APPROVED` | APPRAISER_STATUS_APPROVED may sign appraisals within its scope. |
| `APPRAISER_STATUS_REJECTED` | APPRAISER_STATUS_REJECTED was refused, and may apply again. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrUnknownValidator` | not a bonded validator |
| 1102 | `ErrNotTheFeeder` | signer is not the feeder for this validator |
| 1103 | `ErrDenomNotAccepted` | denom is not in the accepted set |
| 1104 | `ErrInvalidRate` | exchange rate must be a positive decimal |
| 1105 | `ErrNotAnAppraiser` | address is not an approved appraiser |
| 1106 | `ErrOutOfScope` | appraiser is not approved for this asset class |
| 1107 | `ErrAssetNotFound` | no such tokenised asset |
| 1108 | `ErrApplicationExists` | an appraiser application already exists for this address |
| 1109 | `ErrApplicationMissing` | appraiser application not found |
| 1110 | `ErrNotPending` | appraiser application is not pending |
| 1111 | `ErrInvalidValuation` | invalid valuation amount |
| 1112 | `ErrValuationInFuture` | valuation date is in the future |
| 1113 | `ErrRateUnavailable` | no usable exchange rate for this denom |
| 1114 | `ErrAppraisalMissing` | no appraisal for this asset |
| 1115 | `ErrStale` | value is too old to be relied on |
| 1116 | `ErrLimitReached` | exceeds a configured maximum |
