<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/netting

The tiered settlement layer: participants settle retail activity on their own books and submit only what they owe each other, netted multilaterally against prefunded reserves, with high-value items settling gross.

## Transactions

### MsgPostReserve

`/blockchain.netting.v1.MsgPostReserve`

Signed by the `participant` field.

PostReserve prefunds a participant's settlement reserve.

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string | participant is the approved institution posting the reserve, and the only valid signer. |
| `amount` | repeated Coin | amount may name several currencies at once; each is reserved separately, because netting never crosses currencies. |

### MsgSubmitObligation

`/blockchain.netting.v1.MsgSubmitObligation`

Signed by the `from_participant` field.

SubmitObligation records what one participant owes another.

| Field | Type | Description |
| --- | --- | --- |
| `from_participant` | string | from_participant owes and signs; to_participant is owed. |
| `to_participant` | string |  |
| `denom` | string | denom and amount are the currency and the figure owed. |
| `amount` | string |  |
| `batch_hash` | bytes | batch_hash is SHA-256 over the salted retail batch this figure summarises. Required, 32 bytes; see Obligation.batch_hash. |

### MsgUpdateParams

`/blockchain.netting.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

### MsgWithdrawReserve

`/blockchain.netting.v1.MsgWithdrawReserve`

Signed by the `participant` field.

WithdrawReserve takes back the part of a reserve that is not committed.

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string | participant is the institution withdrawing, and the only valid signer. |
| `amount` | repeated Coin | amount is what to withdraw, per currency. |

## Queries

### CurrentCycle

`GET /yamale/blockchain/netting/v1/cycle/current`

CurrentCycle queries the open netting window and when it closes.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `cycle` | Cycle | cycle is the open window, with the per-currency totals accumulated so far. |
| `closes_at_height` | int64 | closes_at_height is the block whose end blocker will settle it. Zero when netting is switched off, because then no block ever will. |

### Cycle

`GET /yamale/blockchain/netting/v1/cycle/{id}`

Cycle queries one window and what it settled, currency by currency.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id is the cycle number. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `cycle` | Cycle | cycle is the window and its per-currency outcomes. |
| `compression` | repeated DenomCompression | compression_bps is, per currency and in basis points, the share of the gross figure that netting removed: 9000 means nine tenths of the value submitted never had to be funded. It is reported by the chain rather than computed by the caller so that everyone quotes the same number, and it is zero for a currency that submitted nothing — a ratio whose denominator is zero is not a small number, it is not a number, and dividing by it here would take the query process down. |

### HeldSlices

`GET /yamale/blockchain/netting/v1/held`

HeldSlices queries every currency slice that failed to settle and is waiting to be retried. It is the operational alarm: on a healthy chain it is always empty, and anything in it is money that participants believe is settled and is not.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `held` | repeated HeldSlice | held is the list of stuck currency slices. |

### Params

`GET /yamale/blockchain/netting/v1/params`

Params queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

### ParticipantObligations

`GET /yamale/blockchain/netting/v1/obligations/{participant}/{cycle_id}`

ParticipantObligations queries the obligations one participant is a party to in one cycle, in either direction.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string | participant is the institution, on either side of the obligation. |
| `cycle_id` | uint64 | cycle_id is the window to look in. |
| `pagination` | PageRequest | pagination is the usual page request. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `obligations` | repeated Obligation | obligations is the page, oldest id first. |
| `pagination` | PageResponse | pagination is the usual page response. |

### Position

`GET /yamale/blockchain/netting/v1/position/{participant}`

Position queries one participant's reserve, its committed part and its running position in the open window.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string | participant is the institution. It is required: this module has no endpoint that walks every participant. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `entries` | repeated PositionEntry | entries is the per-currency standing. |

## State

### Cycle

Cycle is one netting window: the period obligations join, and the record of what happened when it closed.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id is the cycle number, from 1. Zero is never a cycle, because in proto3 an id of zero is indistinguishable from an unset field and "settled in cycle 0" would be unreadable to every client that received it. |
| `opened_at_height` | int64 | opened_at_height is the first block whose obligations join this cycle, and closed_at_height the block its end blocker settled it in. closed_at_height is zero while the cycle is open. |
| `closed_at_height` | int64 |  |
| `status` | CycleStatus | status is the cycle's outcome, taken from the per-currency outcomes below: settled only when every one of them settled. |
| `outcomes` | repeated DenomOutcome | outcomes carries one entry per currency that saw activity, created when the first obligation in that currency arrives and completed at close. Kept on the cycle rather than in its own store because it is a record rather than a control: a wrong figure here misreports how much compression a cycle achieved, and can never let anybody withdraw money they owe. |

### DenomCompression

DenomCompression is how much netting removed in one currency.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string | denom is the currency, compression_bps the share removed in basis points. |
| `compression_bps` | uint64 |  |

### DenomOutcome

DenomOutcome is the per-currency slice of a cycle. Currencies settle independently: there is no cross-currency netting on this chain, because offsetting a euro debit against a naira credit is an FX trade, and an FX trade priced by a settlement system is a position that system is taking.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string | denom is the currency this slice covers. |
| `status` | DenomStatus | status is what became of it. |
| `gross_amount` | string | gross_amount is the sum of every netted obligation submitted in this currency, and net_amount the sum actually discharged when the cycle settled. The ratio between them is the compression the cycle achieved, and it is the number that says whether tiering was worth doing. net_amount stays zero while the cycle is open and while it is held. |
| `net_amount` | string |  |
| `obligation_count` | uint64 | obligation_count is how many netted obligations made up gross_amount. |
| `hold_reason` | string | hold_reason records why a held slice was refused, in words an operator can act on. Empty unless status is held. |

### DenomPolicy

DenomPolicy is the netting rule for one currency.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string | denom is the currency this policy governs. |
| `gross_threshold` | string | gross_threshold is the amount at or above which a single obligation bypasses netting and settles gross in the block it was submitted in. High-value items settling individually is the point rather than a concession. It is how every RTGS system in the world is built, it is what a supervisor expects to be able to see item by item, and it keeps the largest exposures out of the deferred window entirely — the netting cycle then carries only amounts whose failure nobody would call systemic. Zero means every amount is at or above it, so the currency settles entirely gross. Same reasoning as an absent policy, and the same safe direction. |

### EventCycleHeld

EventCycleHeld is emitted when a currency slice is refused and nothing in it moves. It is the loudest thing this module can say: every obligation in the slice is still owed, at its original amount, to its original counterparty, and every participant in it is carrying an exposure past the moment it expected to be discharged.

| Field | Type | Description |
| --- | --- | --- |
| `cycle_id` | uint64 | cycle_id and denom identify the slice, reason says why it was refused. |
| `denom` | string |  |
| `reason` | string |  |

### EventCycleSettled

EventCycleSettled is emitted once per currency slice that cleared.

| Field | Type | Description |
| --- | --- | --- |
| `cycle_id` | uint64 | cycle_id and denom identify the slice. |
| `denom` | string |  |
| `gross_amount` | string | gross_amount is what was submitted and net_amount what had to be funded. The pair is the compression, published at the moment it was achieved. |
| `net_amount` | string |  |
| `obligation_count` | uint64 | obligation_count is how many items made up the gross figure, and participant_count how many institutions held a position in the slice. |
| `participant_count` | uint64 |  |
| `retried` | bool | retried is set when this slice had been held and cleared on a later attempt, so a watcher can tell a normal close from a recovery. |

### EventObligationSubmitted

EventObligationSubmitted is emitted for every obligation, netted or gross.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id, cycle_id and mode identify the obligation and say what became of it. |
| `cycle_id` | uint64 |  |
| `mode` | SettlementMode |  |
| `from_participant` | string | from_participant, to_participant, denom and amount are the obligation. |
| `to_participant` | string |  |
| `denom` | string |  |
| `amount` | string |  |

### EventReserveChanged

EventReserveChanged is emitted when a participant prefunds or withdraws.

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string | participant and denom identify the reserve, amount is the balance after the change, and deposited says which direction it moved in. |
| `denom` | string |  |
| `amount` | string |  |
| `deposited` | bool |  |

### HeldSlice

HeldSlice is one currency in one cycle that did not settle.

| Field | Type | Description |
| --- | --- | --- |
| `cycle_id` | uint64 | cycle_id and denom identify the slice, reason says why it was refused, and held_since_height when it first was — which is the number that says how long participants have been carrying an exposure they did not agree to carry for this long. |
| `denom` | string |  |
| `reason` | string |  |
| `held_since_height` | int64 |  |

### Obligation

Obligation is one participant's admitted debt to another, as submitted.

It is an interbank figure, never a customer payment. A participant runs its own retail ledger, matches its customers' instructions there, and submits to this chain only what it owes another institution as a result. That is the whole privacy argument of the tiered design: a customer payment that never reaches the chain cannot be read off it by a competitor bank, and no amount of cryptography is needed to keep a record that was never written.

| Field | Type | Description |
| --- | --- | --- |
| `cycle_id` | uint64 | cycle_id is the window this obligation joined. A gross obligation records the cycle that was open when it settled, so it can be found alongside the netted ones, but it took no part in that cycle's positions. |
| `id` | uint64 | id is the obligation number, from 1, unique across the chain. |
| `from_participant` | string | from_participant owes, to_participant is owed. Both must be approved participants at the moment of submission, and from_participant is the only valid signer: an institution's debt is not something a third party may declare on its behalf. |
| `to_participant` | string |  |
| `denom` | string | denom and amount are the currency and the figure owed. Amount is strictly positive; a negative obligation would be the opposite obligation wearing a disguise, and would let one participant move another's position. |
| `amount` | string |  |
| `batch_hash` | bytes | batch_hash is SHA-256 over the retail batch this figure summarises, held off-chain by the submitting participant. Required, and required at 32 bytes. An obligation with no link to what it summarises is a number an institution can neither be held to nor reconcile against, which is exactly the auditability that gets given up when items stop being recorded individually. This is a new module with no history to replay, so the rule can be absolute rather than parameterised. The batch must carry a random salt before it is hashed, for the same reason x/paymsg's metadata hash does: a small, guessable payload hashed without one is not a fingerprint, it is a lookup table. |
| `mode` | SettlementMode | mode is whether this settled gross on submission or joined the netting. |
| `submitted_at_height` | int64 | submitted_at_height is the block it arrived in. |

### Position

Position is one participant's running multilateral net position in one currency for one cycle. Negative means the participant owes the system, positive that the system owes it, and the two always sum to zero across every participant in a currency.

| Field | Type | Description |
| --- | --- | --- |
| `cycle_id` | uint64 | cycle_id, denom and participant are the position's identity. |
| `denom` | string |  |
| `participant` | string |  |
| `amount` | string | amount is signed. A position of exactly zero is never stored — it is removed instead — so that state derived by replaying obligations and state restored from a genesis file are the same bytes. A zero written where an import writes nothing is how import and export stop agreeing. |

### PositionEntry

PositionEntry is a participant's standing in one currency.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string | denom is the currency. |
| `reserve` | string | reserve is everything prefunded, locked the part committed to positions in windows that have not settled, and available what may be withdrawn now. available is always reserve minus locked and is never negative. |
| `locked` | string |  |
| `available` | string |  |
| `net_position` | string | net_position is the signed running position in the open window: negative when the participant owes the system, positive when it is owed. |

### Reserve

Reserve is what a participant has prefunded into the module account, per currency. It is the collateral behind everything it is allowed to owe.

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string | participant is the institution that posted it, denom the currency, amount the balance held on its behalf. |
| `denom` | string |  |
| `amount` | string |  |

## Value types

### CycleStatus

CycleStatus is where a netting window has got to.

| Value | Meaning |
| --- | --- |
| `CYCLE_STATUS_UNSPECIFIED` | CYCLE_STATUS_UNSPECIFIED is the unset default and is never valid. |
| `CYCLE_STATUS_OPEN` | CYCLE_STATUS_OPEN is the window obligations are currently joining. Exactly one cycle is open at a time. |
| `CYCLE_STATUS_SETTLED` | CYCLE_STATUS_SETTLED means every currency in the window cleared. |
| `CYCLE_STATUS_HELD` | CYCLE_STATUS_HELD means at least one currency in the window did not clear and is waiting to be retried. The obligations in it are untouched and still owed at their original amounts — see DenomOutcome.hold_reason. |

### DenomStatus

DenomStatus is the outcome for one currency inside one cycle.

| Value | Meaning |
| --- | --- |
| `DENOM_STATUS_UNSPECIFIED` | DENOM_STATUS_UNSPECIFIED is the unset default and is never valid. |
| `DENOM_STATUS_OPEN` | DENOM_STATUS_OPEN means the cycle has not closed yet. |
| `DENOM_STATUS_SETTLED` | DENOM_STATUS_SETTLED means every position in this currency was discharged against the participants' reserves, in one indivisible step. |
| `DENOM_STATUS_HELD` | DENOM_STATUS_HELD means the settlement was refused and nothing moved. |

### SettlementMode

SettlementMode is how one obligation was disposed of.

| Value | Meaning |
| --- | --- |
| `SETTLEMENT_MODE_UNSPECIFIED` | SETTLEMENT_MODE_UNSPECIFIED is the unset default and is never valid. |
| `SETTLEMENT_MODE_GROSS` | SETTLEMENT_MODE_GROSS means the amount moved between the two participants in the block the obligation was submitted in, as an ordinary transfer. Nothing about it is deferred and it takes no part in any cycle's netting. |
| `SETTLEMENT_MODE_NET` | SETTLEMENT_MODE_NET means the obligation joined the open cycle and altered both participants' running positions. It settles when the cycle closes. |

## Parameters

Changed by governance through `MsgUpdateParams`. Defaults are the values a chain starts with at genesis.

| Parameter | Default | Description |
| --- | --- | --- |
| `cycle_blocks` | `0` | cycle_blocks is how many blocks a netting window stays open before the end blocker closes it and settles the positions in it. Zero means netting is switched off, and it means that everywhere: no obligation is ever admitted to a window, every submission settles gross, and the end blocker returns before it computes anything. That reading is chosen rather than treating zero as "every block", because this value is a divisor — the cycle boundary is `height % cycle_blocks` — and a divisor that arrives as zero from a hand-written genesis panics inside an end blocker, which is not a failed transaction but a chain that will not produce another block. Params.Validate() is not the guard that matters here: it is one gate an operator can skip, and it has been skipped on this chain before. The end blocker checks the value at the point it would divide by it. One consequence of that reading is not obvious and is the reason it is written down. Setting this to zero stops the end blocker before it closes anything, so a window that is already open with positions in it never settles: the obligations stay owed, the collateral behind them stays committed, and the participants holding it cannot withdraw. Held slices stop being retried for the same reason. Nothing is lost — setting a positive value again closes the window at the next boundary — but between the two proposals every participant in that window is carrying an exposure with no settlement date, which is the state deferred net settlement exists to avoid. Switch netting off between windows, not during one, and check the current cycle's positions are empty before proposing it. |
| `denom_policies` | `[]` | denom_policies decides, per currency, what nets and what settles gross. A currency with no policy nets nothing: every obligation in it settles gross, immediately. That is the safe direction to fail in — gross settlement is what the chain does today, moves the money in the block it was instructed in, and creates no interbank exposure for anyone to be left holding. Netting is a deliberate governance act, taken currency by currency, never a default that arrives with a new denom. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrNotApprovedParticipant` | not an approved participant |
| 1102 | `ErrInvalidAmount` | invalid amount |
| 1103 | `ErrInvalidBatchHash` | batch_hash must be 32 bytes of SHA-256 over the salted batch |
| 1104 | `ErrSelfObligation` | a participant cannot owe itself |
| 1105 | `ErrNetDebitCapExceeded` | obligation would take the participant's net debit beyond its posted reserve |
| 1106 | `ErrReserveCommitted` | reserve is committed to positions that have not settled |
| 1107 | `ErrInsufficientReserve` | insufficient reserve |
| 1108 | `ErrCycleNotFound` | no such cycle |
| 1109 | `ErrPositionsUnbalanced` | net positions in a currency do not sum to zero |
| 1110 | `ErrNettingDisabled` | netting is disabled |
