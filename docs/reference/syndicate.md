<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/syndicate

## Transactions

### MsgAbortDeal

`/blockchain.syndicate.v1.MsgAbortDeal`

Signed by the `operator` field.

MsgAbortDeal refunds everybody in full.

In full including the fees, because no service was delivered. A platform that keeps a sourcing fee on a deal that never happened is charging for the attempt, and an investor who discovers that once does not commit again.

| Field | Type | Description |
| --- | --- | --- |
| `operator` | string |  |
| `deal_id` | uint64 |  |
| `reason` | string |  |

### MsgBuy

`/blockchain.syndicate.v1.MsgBuy`

Signed by the `buyer` field.

MsgBuy fills a listing.

Both legs in one message, which is the whole point: ownership transfers automatically on completion of sale, so nobody holds the other party's asset while waiting for their own. A two-message version — pay, then ask the seller to deliver — reintroduces exactly the counterparty risk a secondary market for an illiquid private position cannot absorb.

Buy fills a listing: settlement one way, tokens the other, in one block.

| Field | Type | Description |
| --- | --- | --- |
| `buyer` | string |  |
| `listing_id` | uint64 |  |

### MsgCancelListing

`/blockchain.syndicate.v1.MsgCancelListing`

Signed by the `seller` field.

MsgCancelListing withdraws an offer and returns the escrowed tokens.

| Field | Type | Description |
| --- | --- | --- |
| `seller` | string |  |
| `listing_id` | uint64 |  |

### MsgClaim

`/blockchain.syndicate.v1.MsgClaim`

Signed by the `holder` field.

MsgClaim withdraws what a holder has accrued, without giving up the position.

| Field | Type | Description |
| --- | --- | --- |
| `holder` | string |  |
| `deal_id` | uint64 |  |

### MsgCloseDeal

`/blockchain.syndicate.v1.MsgCloseDeal`

Signed by the `operator` field.

MsgCloseDeal funds the company and issues the tokens.

| Field | Type | Description |
| --- | --- | --- |
| `operator` | string |  |
| `deal_id` | uint64 |  |

### MsgCommit

`/blockchain.syndicate.v1.MsgCommit`

Signed by the `investor` field.

MsgCommit escrows an investor's cheque.

The money moves now, not at close. A commitment that is only a promise lets a deal close on paper and fail to fund, which is the failure that ends the platform's relationship with the partner firm — and that relationship is the only asset the platform actually has.

| Field | Type | Description |
| --- | --- | --- |
| `investor` | string |  |
| `deal_id` | uint64 |  |
| `amount` | Coin |  |

### MsgDistribute

`/blockchain.syndicate.v1.MsgDistribute`

Signed by the `payer` field.

MsgDistribute pays a dividend or exit proceeds into a deal.

Permissionless as to WHO pays: the money arrives from wherever it arrives, and refusing a payment because the sender was not on a list would strand it. What it does control is where the money goes, which is pro-rata to whoever holds the token at this moment — no chain of historical owners, no reconciliation of private secondary sales.

| Field | Type | Description |
| --- | --- | --- |
| `payer` | string |  |
| `deal_id` | uint64 |  |
| `amount` | Coin |  |
| `final` | bool | True when this is the exit. The deal becomes REALISED, trading stops, and every token is a claim on a known fixed sum — which is what it actually is at that point, and pretending otherwise leaves a market quoting a price for a position that no longer changes. |

### MsgList

`/blockchain.syndicate.v1.MsgList`

Signed by the `seller` field.

MsgList offers part of a position for sale.

| Field | Type | Description |
| --- | --- | --- |
| `seller` | string |  |
| `deal_id` | uint64 |  |
| `buyer` | string | Empty for an open listing, which the first eligible buyer takes. Set for a private sale to a named counterparty — the shape that actually happens, and whose administrative cost is what a paper syndicate cannot absorb. |
| `quantity` | string |  |
| `price` | Coin |  |

### MsgOnboardInvestor

`/blockchain.syndicate.v1.MsgOnboardInvestor`

Signed by the `operator` field.

MsgOnboardInvestor records completed KYC/AML, or withdraws it.

The chain does not perform the checks and this message does not claim it did. It records that a named, accountable operator says they were performed, against a file reference somebody can later be asked to produce. That is the only honest thing a chain can offer here, and it is worth more than nothing: the assertion is dated, attributable and cannot be backdated.

| Field | Type | Description |
| --- | --- | --- |
| `operator` | string |  |
| `investor` | string |  |
| `reference` | string | The operator's own file reference. NEVER the documents, and never an identity number: a national identifier written to a public chain is a permanent disclosure that no later correction can withdraw. |
| `jurisdiction` | string |  |
| `active` | bool |  |

### MsgOpenDeal

`/blockchain.syndicate.v1.MsgOpenDeal`

Signed by the `operator` field.

MsgOpenDeal puts an allocation in front of the investor base.

| Field | Type | Description |
| --- | --- | --- |
| `operator` | string |  |
| `reference` | string |  |
| `company` | string |  |
| `partner` | string |  |
| `drawdown_account` | string |  |
| `settlement_denom` | string |  |
| `target` | string |  |
| `minimum` | string |  |
| `minimum_ticket` | string |  |
| `fees` | FeeSchedule |  |
| `closes_at` | int64 | When commitments stop. Bounded at both ends by the module parameters. |

### MsgRegisterOperator

`/blockchain.syndicate.v1.MsgRegisterOperator`

Signed by the `authority` field.

MsgRegisterOperator admits a platform operator, or strikes one off.

Governance, and an x/group account, for the same reason every other registry on this chain demands both: an operator opens deals, decides they have closed and moves the money to a drawdown account. Each of those on one key is one key away from a platform that takes money and does not invest it.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `operator` | string |  |
| `name` | string |  |
| `jurisdiction` | string | ISO 3166-1 alpha-2 of the jurisdiction the platform entity operates from — which the proposal itself says is likely to be distinct from where the investee companies are. |
| `active` | bool |  |

### MsgUpdateParams

`/blockchain.syndicate.v1.MsgUpdateParams`

Signed by the `authority` field.

MsgUpdateParams sets the platform-wide ceilings.

Governance only.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

### MsgWithdrawCommitment

`/blockchain.syndicate.v1.MsgWithdrawCommitment`

Signed by the `investor` field.

MsgWithdrawCommitment takes an escrowed cheque back.

Unconditional while the deal is OPEN, and that is not generosity. Money committed is out of the investor's hands and earning nothing, against a deal that may never happen; the right to withdraw is what makes committing early rational, and a syndicate whose early commitments are hostages does not fill.

| Field | Type | Description |
| --- | --- | --- |
| `investor` | string |  |
| `deal_id` | uint64 |  |

## Queries

### Commitments

`GET /yamale/blockchain/syndicate/v1/commitments/{deal_id}`

Commitments lists what has been committed to one deal.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `deal_id` | uint64 |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `commitments` | repeated Commitment |  |
| `pagination` | PageResponse |  |

### Deal

`GET /yamale/blockchain/syndicate/v1/deal/{id}`

Deal returns one syndication.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `deal` | Deal |  |

### DealByRef

`GET /yamale/blockchain/syndicate/v1/deal_by_ref`

DealByRef finds one by the platform's own reference.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `reference` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `deal` | Deal |  |

### Deals

`GET /yamale/blockchain/syndicate/v1/deals`

Deals lists them.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `deals` | repeated Deal |  |
| `pagination` | PageResponse |  |

### Entitlement

`GET /yamale/blockchain/syndicate/v1/entitlement/{deal_id}/{holder}`

Entitlement is what one holder could claim right now, settled and unsettled together — the number a statement would show, computed rather than reported.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `deal_id` | uint64 |  |
| `holder` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `balance` | string | The token balance the entitlement is computed over. |
| `claimable` | Coin | Settled and unsettled together, which is what a statement would show. A response that reported only the settled part would show zero for a holder who has never transferred, which is most of them. |
| `share_bps` | uint32 | The holder's share of the deal in basis points, so a person can read it without dividing two eighteen-digit numbers. |

### Investor

`GET /yamale/blockchain/syndicate/v1/investor/{address}`

Investor reports whether an address has been onboarded, which is the question a secondary buyer needs answered before they bid.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `onboarded` | bool |  |
| `investor` | Investor |  |

### Listings

`GET /yamale/blockchain/syndicate/v1/listings/{deal_id}`

Listings are the secondary market for one deal.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `deal_id` | uint64 |  |
| `live_only` | bool | When true, only listings that can still be filled. |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `listings` | repeated Listing |  |
| `pagination` | PageResponse |  |

### Operators

`GET /yamale/blockchain/syndicate/v1/operators`

Operators lists the admitted platform operators.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `operators` | repeated Operator |  |
| `pagination` | PageResponse |  |

### Params

`GET /yamale/blockchain/syndicate/v1/params`

Params returns the platform-wide ceilings.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

## State

### Commitment

Commitment is one investor's promise, and their money, while a deal is open.

The money is real and escrowed from the moment of commitment. A promise that is only a promise means a deal can close on paper and fail to fund, which is the failure mode that destroys a syndication platform's relationship with the partner firm — the one asset it actually has.

| Field | Type | Description |
| --- | --- | --- |
| `deal_id` | uint64 |  |
| `investor` | string |  |
| `amount` | string |  |
| `committed_at` | int64 |  |

### Deal

Deal is one syndicated allocation into one company.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | Assigned by the chain. Never reused. |
| `reference` | string | The platform's own reference, so this can be reconciled with the paper and the partner's own records. Unique and indexed. |
| `company` | string | The company the money ends up in, in words. The chain holds no opinion about it and cannot verify it; it is here so a holder can see what they own without leaving the record. |
| `partner` | string | The partner VC or growth-equity firm whose allocation this is. Recorded because the entire premise of the syndicate is riding alongside somebody who did the diligence, and an investor is entitled to know who that was. |
| `drawdown_account` | string | Where the net capital goes at close. The partner's own drawdown account, or an escrow agent's — the platform never holds it after close. |
| `operator` | string | The platform operator: a group account, so opening a deal, closing it and aborting it each need several signatures inside the operator. A platform that can take money on one key is one key away from taking it. |
| `settlement_denom` | string | The settlement currency. One denomination for the whole deal, because accounting, reporting and investor administration across several African currencies is the administrative burden the platform exists to remove. USD or a USD-pegged stablecoin in practice. |
| `target` | string | What the allocation is. A deal that does not reach its minimum is aborted and everybody is refunded; a deal cannot take more than its target. |
| `minimum` | string |  |
| `minimum_ticket` | string | The smallest cheque. The whole point is that it is a fraction of what the partner firm would require, but it is not zero: a syndicate of ten thousand dust positions costs more to administer than it raises. |
| `fees` | FeeSchedule |  |
| `closes_at` | int64 | When commitments stop being accepted. After this the deal can be closed if it met its minimum, and must be aborted if it did not. |
| `state` | DealState |  |
| `raised` | string | Committed so far, while OPEN. After close this is the amount actually raised, and it is what the token supply was computed from. |
| `denom` | string | The deal token: a bank denomination, one per deal, whose holder is the owner of that proportion of the syndicate's position. Empty until close. A denomination that exists before the deal is funded is a claim on nothing, and somebody would trade it. |
| `supply` | string | Issued at close: one unit per smallest unit of settlement raised, so a holder's proportion is their balance over the supply and no rounding decision is hidden anywhere. |
| `distribution_index` | string | Cumulative distributions per token, scaled — the index a holder's entitlement is computed against. See the keeper's Settle for why an index rather than a snapshot: it is what lets a distribution be pro-rata to whoever holds the token at that moment without anybody tracking a chain of historical owners through private secondary sales. |
| `distributed` | string | Paid in and paid out over the life of the deal, for reporting. |
| `opened_at` | int64 |  |
| `closed_at` | int64 |  |

### FeeSchedule

Fees, in basis points, at the three points the platform earns.

Declared on the deal at creation and fixed there. A fee schedule an operator can change after the money is committed is not a schedule, it is a discretion — and the investor committed against the first number.

| Field | Type | Description |
| --- | --- | --- |
| `sourcing_bps` | uint32 | Charged at close, for securing and structuring access to the allocation. Taken from the raise, so an investor's cheque buys slightly less than its face value in ownership, which is what the reporting has to show. |
| `syndication_bps` | uint32 | Charged at close, for structuring and closing the syndicate itself. |
| `management_bps` | uint32 | Charged on the way OUT, per distribution, for the ongoing administration of the syndicate. Not accrued per year against committed capital, and the difference matters. An annual management fee on an illiquid position has to be collected from somebody — either by calling more capital, which a syndicate cannot do, or by selling down the position, which it cannot either. Charging it against distributions means the platform is paid when and only when the investors are, which is both collectable and the right incentive. |

### Investor

Investor is somebody the platform has onboarded.

Every investor completes KYC/AML before joining the platform or taking part in any syndicate, and this is that record. The chain does not perform the checks and must not pretend to: it records that a named, accountable operator says they were performed, with a date and a reference somebody can be asked to produce.

The record is what the deal token's transfer rule consults. That is the part worth being precise about: a token whose transfer is gated is not a bearer instrument, and an investor who buys one on a secondary market is buying into the same onboarding requirement as everyone else. If the gate lived only in the platform's user interface it would bind exactly nobody, because the token is a bank denomination and a bank send does not go through a user interface.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |
| `onboarded_by` | string | Who onboarded them. An operator group account, so the attestation is a decision of an institution rather than of one employee. |
| `reference` | string | The operator's own file reference for the KYC pack. Free text, and never the documents themselves: a national identity number on a public chain is a permanent disclosure that no later correction can withdraw. |
| `jurisdiction` | string | ISO 3166-1 alpha-2. The jurisdiction question is the one this platform says out loud is unsettled, and holding the answer per investor is what lets a later rule be applied without re-onboarding anybody. |
| `active` | bool | Struck off investors keep their record — they may still hold and still be paid, because their money is already in a company and the chain has no right to strand it — but they may not acquire more. |
| `onboarded_at` | int64 |  |

### Listing

Listing is a holder selling part of their position.

The liquidity the whole model rests on: an investor is not locked in for the life of the investment. Two shapes, and both are here because the private one is what actually happens — an angel sells to somebody they know, and the administrative cost of that sale is what a paper syndicate cannot absorb.

An OPEN listing names nobody and the first eligible buyer takes it. A PRIVATE listing names its buyer, and nobody else can fill it.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `deal_id` | uint64 |  |
| `seller` | string |  |
| `buyer` | string | Empty for an open listing; the named counterparty for a private one. |
| `quantity` | string | How many deal tokens, and the total asked for them in the settlement denomination. A total rather than a unit price, because a unit price plus a quantity is two numbers that can disagree after rounding, and the disagreement lands on whoever is paying. |
| `price` | Coin |  |
| `state` | ListingState |  |
| `filled_by` | string |  |
| `listed_at` | int64 |  |
| `filled_at` | int64 |  |

### Operator

Operator is the platform entity: the syndication and investor-management layer, admitted by governance.

An x/group account, and the requirement is not ceremony. An operator opens deals, decides they have closed, moves escrowed money to a drawdown account and attests that investors passed KYC. Each of those on a single key is one key away from a platform that takes money and does not invest it — and the investors here are precisely the people with no practical recourse, spread across jurisdictions, holding a position they cannot value.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |
| `name` | string |  |
| `jurisdiction` | string | Where the platform entity itself operates from, which the proposal notes is likely to be distinct from where the investee companies are based. |
| `active` | bool | Struck off operators keep their record. Every deal names the operator that opened it, and "was this platform admitted at the time" has to stay answerable long after somebody has stopped being admitted. |
| `admitted_at` | int64 |  |

### Position

Position is one holder's place in one deal's distribution index.

`index` is the value of the deal's cumulative index the last time this holder's entitlement was settled. Entitlement is balance * (deal.index - position.index), which is only correct if the position is settled against the balance actually held at the time — which is why transfers settle both sides BEFORE the balances move.

| Field | Type | Description |
| --- | --- | --- |
| `deal_id` | uint64 |  |
| `holder` | string |  |
| `index` | string |  |
| `accrued` | string | Settled and not yet withdrawn. |

## Value types

### DealState

DealState is where a syndication has got to.

| Value | Meaning |
| --- | --- |
| `DEAL_STATE_UNSPECIFIED` | Never stored on a deal that exists. A deal is created OPEN. |
| `DEAL_STATE_OPEN` | Accepting commitments. Money is escrowed in the module account, not paid to anybody, and an investor may still withdraw in full. |
| `DEAL_STATE_CLOSED` | Closed and funded. Tokens are issued, the net capital has gone to the partner's drawdown account, and commitments are no longer refundable — the money is in a company now. |
| `DEAL_STATE_ABORTED` | The target was not met, or the platform pulled the deal. Every commitment is refunded in full, including the fees, because no service was delivered. |
| `DEAL_STATE_REALISED` | Realised: the investment has been exited and the proceeds are in the distribution pot. Tokens stop trading and become a claim on a known fixed sum, which is the only honest thing they can be at that point. |

### ListingState

ListingState is where a secondary sale has got to.

| Value | Meaning |
| --- | --- |
| `LISTING_STATE_UNSPECIFIED` | Never stored on a listing that exists. A listing is created LIVE. |
| `LISTING_STATE_LIVE` | Open, and fillable by whoever it is addressed to. |
| `LISTING_STATE_FILLED` | Bought. |
| `LISTING_STATE_CANCELLED` | Withdrawn by the seller. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1 | `ErrNotAuthority` | this message is governance-only |
| 10 | `ErrRefTaken` | this deal reference is already used |
| 11 | `ErrNoDeal` | no such deal |
| 12 | `ErrDealClosed` | this deal is no longer open for commitments |
| 13 | `ErrDealNotOpen` | this deal is not open |
| 14 | `ErrDealNotFunded` | this deal has not closed and issued its tokens |
| 15 | `ErrWindowOpen` | commitments are still being accepted; this deal cannot be closed yet |
| 16 | `ErrMinimumNotMet` | the deal did not reach its minimum and must be aborted rather than closed |
| 17 | `ErrOverTarget` | that would take the deal over its target |
| 18 | `ErrBelowTicket` | that is below this deal's minimum cheque |
| 19 | `ErrNoCommitment` | you have not committed to this deal |
| 2 | `ErrNotOperator` | the signer is not an admitted platform operator |
| 20 | `ErrWrongDenom` | this deal settles in a different currency |
| 21 | `ErrInvalidTerms` | these deal terms cannot be met |
| 22 | `ErrFeeTooHigh` | that fee is above the ceiling the platform's governance set |
| 23 | `ErrWindowTooShort` | a deal must stay open long enough for the investor base to see it |
| 24 | `ErrWindowTooLong` | a deal may not hold committed money longer than the platform allows |
| 25 | `ErrNothingToClaim` | there is nothing accrued to claim |
| 26 | `ErrTradingHalted` | this deal has been realised; the token is a claim on a fixed sum and no longer trades |
| 27 | `ErrNoListing` | no such listing |
| 28 | `ErrListingClosed` | this listing is no longer live |
| 29 | `ErrNotSeller` | only the seller may withdraw a listing |
| 3 | `ErrOperatorInactive` | this platform operator is no longer admitted |
| 30 | `ErrNotNamedBuyer` | this listing is a private sale to a named buyer |
| 31 | `ErrSelfDeal` | you cannot buy your own listing |
| 32 | `ErrInvalidAmount` | the amount must be positive |
| 4 | `ErrOperatorNotGroup` | a platform operator must be a group account, so taking money needs several signatures |
| 5 | `ErrInvalidJurisdiction` | that is not a country this chain recognises |
| 6 | `ErrNotOnboarded` | this address has not completed KYC onboarding and cannot hold a deal token |
| 7 | `ErrInvestorInactive` | this investor's onboarding has been withdrawn and they may not acquire more |
| 8 | `ErrInvalidInvestor` | the investor is not a valid account |
| 9 | `ErrNoReference` | a deal reference is required |
