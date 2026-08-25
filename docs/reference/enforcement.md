<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/enforcement

## Transactions

### MsgEmergencyFreeze

`/blockchain.enforcement.v1.MsgEmergencyFreeze`

Signed by the `authority` field.

EmergencyFreeze lets a country's enforcement authority stop an account in one block, without waiting for a validator to open a case.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority must hold ROLE_ENFORCEMENT_AUTHORITY covering the country the target account is recorded in. It used to be a single address named in the module's parameters, chain-wide, able to freeze anything. The perimeter is the difference: an authority that needs to stop an account outside its own country needs the authority of that country, urgently or otherwise. Skipping the jurisdiction check because the situation is urgent would make the one path that acts on a single signature also the one path with no territorial limit. |
| `target` | string |  |
| `reason` | string | reason is the grounds, in words the accused can read. Required, the same as for a validator's case: acting in an emergency is not a reason to leave the record blank, it is the reason the record matters. |
| `evidence_uri` | string |  |
| `evidence_hash` | string |  |

### MsgEmergencyRelease

`/blockchain.enforcement.v1.MsgEmergencyRelease`

Signed by the `authority` field.

EmergencyRelease lets it let the account go again, just as fast.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority must hold ROLE_ENFORCEMENT_AUTHORITY covering the country the case's target is recorded in. |
| `case_id` | uint64 |  |
| `reason` | string | reason is why it was released, kept beside the original accusation. The case is not deleted: a freeze that was lifted as a mistake is part of the record of how this power has been used. |

### MsgOmbudsmanVeto

`/blockchain.enforcement.v1.MsgOmbudsmanVeto`

Signed by the `ombudsman` field.

OmbudsmanVeto stops a case that has not taken anything yet. The only message the ombudsman may sign, and the only thing it can do.

| Field | Type | Description |
| --- | --- | --- |
| `ombudsman` | string | ombudsman must equal the ombudsman parameter. |
| `case_id` | uint64 |  |
| `reason` | string | reason is why the case was stopped, kept beside the original accusation. Required: an office whose refusals need no grounds is not accountable either, and this one is a check rather than a privilege. |

### MsgOpenCase

`/blockchain.enforcement.v1.MsgOpenCase`

Signed by the `opener` field.

OpenCase accuses an address, and freezes it while the validators decide.

| Field | Type | Description |
| --- | --- | --- |
| `opener` | string | opener is the account that signs: either a bonded validator's account address — the key it signs with, not its operator address — or the account holding ROLE_ENFORCEMENT_AUTHORITY over the target's country. What gets recorded on the case differs, deliberately. A validator is recorded by its operator address, because that is the name the accusation is legible under; an office is recorded by the account itself, which is the group policy address a role-holders query can be read against. Both are identities somebody can look up, which is the property that matters: an accusation with no visible author is not an accusation. |
| `target` | string |  |
| `action` | CaseAction |  |
| `reason` | string | reason is the grounds, in words the accused can read. |
| `evidence_uri` | string | evidence_uri and evidence_hash point at and pin the evidence held off-chain. Required for a seizure unless the parameters say otherwise. |
| `evidence_hash` | string |  |
| `legal_instrument` | LegalInstrument | legal_instrument names the external authority the seizure is carried out under and pins its content. Required for a seizure, always, with no parameter that turns it off — a requirement governance can vote away is a default, and this one is meant to be a requirement. Ignored for a freeze, which takes nothing and has to be openable the minute a theft is noticed. |

### MsgReverseCase

`/blockchain.enforcement.v1.MsgReverseCase`

Signed by the `authority` field.

ReverseCase is governance overturning a passed case: the appeal. It lifts the freeze and records the reversal, and it is deliberately a slower instrument than the one that imposed the freeze, because it is the one used when the chain got it wrong.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `case_id` | uint64 |  |
| `reason` | string | reason is why it was overturned, kept beside the original accusation. |

### MsgSweep

`/blockchain.enforcement.v1.MsgSweep`

Signed by the `sender` field.

Sweep collects whatever a passed seizure can now reach. Permissionless and repeatable: funds that were staked arrive later, when unbonding matures, and somebody has to be able to collect them without another vote.

| Field | Type | Description |
| --- | --- | --- |
| `sender` | string | sender pays for the transaction and gets nothing for it. Anyone may send this: the destination is fixed by the parameters, so there is nothing to gain by being the one who calls it and nothing to lose by letting somebody else. |
| `case_id` | uint64 |  |

### MsgUpdateParams

`/blockchain.enforcement.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

### MsgVoteCase

`/blockchain.enforcement.v1.MsgVoteCase`

Signed by the `voter` field.

VoteCase records one validator's judgement.

| Field | Type | Description |
| --- | --- | --- |
| `voter` | string | voter is the validator's account address, the key it signs with. Its operator address and its voting power are read from the staking module. |
| `case_id` | uint64 |  |
| `option` | VoteOption |  |

### MsgWithdrawCase

`/blockchain.enforcement.v1.MsgWithdrawCase`

Signed by the `opener` field.

WithdrawCase takes back a case before the vote ends, lifting its freeze.

| Field | Type | Description |
| --- | --- | --- |
| `opener` | string | opener is the account that opened the case: a validator's account address, or the enforcement authority's own address. It is resolved the same way it was when the case was opened, so whichever of the two forms was recorded is the one that matches. |
| `case_id` | uint64 |  |

## Queries

### CaseVotes

`GET /yamale/blockchain/enforcement/v1/case/{case_id}/votes`

CaseVotes queries how each validator voted on a case.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `votes` | repeated Vote |  |
| `yes_power` | int64 | The tallies as they stand, so a caller does not have to add up the votes and reach a different answer from the chain's. |
| `no_power` | int64 |  |
| `abstain_power` | int64 |  |
| `total_power_at_open` | int64 |  |
| `required_power` | int64 | required_power is what yes_power has to reach for the case to pass. |

### FreezeStatus

`GET /yamale/blockchain/enforcement/v1/freeze/{address}`

FreezeStatus answers whether one address can send, and if not, why.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `frozen` | bool |  |
| `freeze` | Freeze | freeze and case are set only when frozen. Together they are the answer to "why", which is the question this query exists for. |
| `case` | Case |  |

### GetCase

`GET /yamale/blockchain/enforcement/v1/case/{id}`

GetCase queries one case, with its votes.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `case` | Case |  |
| `votes` | repeated Vote |  |

### HeldCases

`GET /yamale/blockchain/enforcement/v1/case/held`

HeldCases queries the seizures that have been agreed and are waiting out their delay. This is the list an ombudsman reads: everything still stoppable at no cost to anybody, and how long there is left to stop it.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `case` | repeated Case |  |

### ListCase

`GET /yamale/blockchain/enforcement/v1/case`

ListCase queries every case ever opened, resolved or not.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `case` | repeated Case |  |
| `pagination` | PageResponse |  |

### ListFreeze

`GET /yamale/blockchain/enforcement/v1/freeze`

ListFreeze queries every frozen address.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `freeze` | repeated Freeze |  |
| `pagination` | PageResponse |  |

### OpenCases

`GET /yamale/blockchain/enforcement/v1/case/open`

OpenCases queries the cases still being voted on.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `case` | repeated Case |  |

### Params

`GET /yamale/blockchain/enforcement/v1/params`

Params queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

### Recovered

`GET /yamale/blockchain/enforcement/v1/recovered`

Recovered totals what this module has taken, across every case. It is the number that says how much this power has actually been used.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `total` | repeated Coin |  |
| `cases_opened` | uint64 | cases_passed and cases_opened put that total in proportion. |
| `cases_passed` | uint64 |  |

### SeizureWindow

`GET /yamale/blockchain/enforcement/v1/window`

SeizureWindow queries how much of the rolling cap is left.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `window_start_height` | int64 |  |
| `current_height` | int64 |  |
| `seized` | repeated Coin | seized is the total inside the window; cap is the parameter it is measured against; remaining is cap minus seized, floored at zero and carrying only the denominations the cap names. |
| `cap` | repeated Coin |  |
| `remaining` | repeated Coin |  |
| `seizure_count` | uint64 | seizure_count and max_seizures are the other half of the cap: the one that binds every denomination, including ones the value cap does not name. |
| `max_seizures` | uint64 |  |

## State

### Case

Case is an accusation against one address, and everything the chain did about it. Cases are never deleted: the record of who was frozen, on whose word, on what evidence, and by whose votes is the only thing that makes this power answerable.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `target` | string | target is the account accused. |
| `opener` | string | opener is the validator operator that opened the case, or the emergency authority when `emergency` is set. Not typed as a validator address for exactly that reason: an emergency case is opened by a group policy account, and rendering it as an operator address would name a validator that does not exist. |
| `action` | CaseAction |  |
| `status` | CaseStatus |  |
| `reason` | string | reason is the stated grounds, in plain words, for whoever is accused. |
| `evidence_uri` | string | evidence_uri points at the evidence held off-chain; evidence_hash is the SHA-256 of what it pointed at when the case was opened, so a document that is later edited can be shown to have been edited. |
| `evidence_hash` | string |  |
| `opened_at_height` | int64 |  |
| `voting_ends_at_height` | int64 |  |
| `resolved_at_height` | int64 |  |
| `total_power_at_open` | int64 | total_power_at_open is the bonded power the threshold is measured against. Frozen at open so that power leaving the set mid-vote cannot lower the bar for what is already being decided. |
| `yes_power` | int64 |  |
| `no_power` | int64 |  |
| `abstain_power` | int64 |  |
| `recovered` | repeated Coin | recovered is everything this case has taken so far. It grows as unbonding funds arrive and are swept, which is why it is a running total rather than a single figure written once. |
| `sweep_complete` | bool | sweep_complete is set when a seizure has nothing left to collect: the account is empty and no unbonding remains. Until then the case stays sweepable by anyone. |
| `emergency` | bool | emergency marks a case opened by the emergency authority rather than by a validator. It changes nothing about how the case is decided — the validators still confirm or refuse it on the same terms — and exists so that nobody reading the record has to work out from an address whether the founders acted directly. |
| `legal_instrument` | LegalInstrument | legal_instrument is the external authority a seizure is carried out under. Required for a seizure and empty for a freeze: a freeze takes nothing and is meant to be openable in the minute a theft is noticed, which is not a minute in which anybody has a court order. |
| `execute_at_height` | int64 | execute_at_height is when a held seizure may be carried out — the height the delay its size earned runs to. Zero on any case that is not a seizure the validators have passed. |
| `assessed_value` | repeated Coin | assessed_value is what the target was found to hold when the case was decided: balance, stake and unbonding together. It is the figure the delay was sized from and the figure charged against the rolling cap, recorded on the case so that "why did this one wait a week" is answerable from the record rather than from a re-run of the arithmetic against state that has since moved. |

### EventCaseHeld

EventCaseHeld is emitted when a seizure is agreed and starts waiting out its delay. This is the block in which anybody who wants to object still can, so it carries the height the objection window closes at and the value the delay was sized from.

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |
| `target` | string |  |
| `assessed_value` | repeated Coin | assessed_value is what the target was found to hold: balance, stake and unbonding together. |
| `execute_at_height` | int64 |  |
| `delay_blocks` | uint64 |  |

### EventCaseOpened

EventCaseOpened is emitted when a case is opened and its target frozen.

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |
| `target` | string |  |
| `opener` | string |  |
| `action` | CaseAction |  |
| `reason` | string |  |
| `voting_ends_at_height` | int64 |  |
| `emergency` | bool | emergency is set when the founders' group opened it directly rather than a validator. Anyone watching for this module's events should be able to tell the two apart without fetching the case. |

### EventCaseResolved

EventCaseResolved is emitted once, when a case reaches its final status — including when it expires with nobody having voted.

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |
| `target` | string |  |
| `status` | CaseStatus |  |
| `yes_power` | int64 |  |
| `no_power` | int64 |  |
| `required_power` | int64 |  |

### EventCaseVetoed

EventCaseVetoed is emitted when the ombudsman stops a case.

Its own event rather than an EventCaseResolved with a different status, because "the office outside the validator set refused this" is the single most important thing that can happen in this module and it should not have to be inferred from an enum by whoever is watching.

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |
| `target` | string |  |
| `ombudsman` | string |  |
| `reason` | string |  |
| `was_held` | bool | was_held distinguishes a veto that stopped a seizure the validators had already agreed to from one that stopped a case still being argued. They are very different acts by the same office. |

### EventCaseVoted

EventCaseVoted is emitted for each validator's vote.

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |
| `validator` | string |  |
| `option` | VoteOption |  |
| `power` | int64 |  |

### EventFreezeLifted

EventFreezeLifted is emitted whenever an address becomes able to send again, whatever the reason — expiry, rejection, withdrawal or reversal.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |
| `case_id` | uint64 |  |
| `status` | CaseStatus |  |

### EventSeized

EventSeized is emitted every time a sweep collects something, which for a target with staked funds is more than once.

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |
| `target` | string |  |
| `destination` | string |  |
| `collected` | repeated Coin |  |
| `complete` | bool | complete is set on the sweep that finishes the job: nothing liquid left, nothing staked, nothing unbonding. |

### EventSeizureDeferred

EventSeizureDeferred is emitted when a seizure comes due and the rolling cap refuses it.

Emitted every time it is refused, not once. A case that is quietly waiting is indistinguishable from a case that has been forgotten, and the difference matters to the person whose account is still frozen.

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |
| `target` | string |  |
| `retry_at_height` | int64 | retry_at_height is the next height at which the window could have room — when the oldest seizure in it falls out. |
| `reason` | string | reason says which limit refused it, in words, so that nobody has to reconstruct the arithmetic from the parameters to find out. |

### Freeze

Freeze is the fact that stops the money moving.

It is kept as its own record rather than being derived by scanning cases, because it is read on every single transfer the chain processes.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |
| `case_id` | uint64 | case_id is the case that put it there — so an account holder who is refused a transfer can be told which case to read, rather than "no". |
| `expires_at_height` | int64 | expires_at_height is the height the freeze lapses at, for the provisional freeze that comes with opening a case. Zero means it does not expire on its own: the validators voted for it, and lifting it takes a decision. |
| `frozen_at_height` | int64 |  |

### LegalInstrument

LegalInstrument is the external authority a seizure is carried out under.

This is deliberately not the evidence fields. Evidence is why the chain believes the allegation; an instrument is who, outside this chain, ordered that something be done about it. Keeping them in one pair of fields would let a case satisfy its authority requirement by attaching its own investigation report, which is exactly the substitution the requirement exists to prevent.

There is no URI here, and that is the design rather than an omission. A link is a document somebody controls: whoever hosts it can change it, take it down, or never have had it. What is stored instead is an identifier that names the instrument in the world — the issuing body and its own reference number — so that verification means going to that body's register, plus a hash that pins the content of what was served. A reader with the reference can find the instrument without this chain's help; a reader with the hash can prove the copy they were shown is the one the case was opened on. Neither depends on anyone keeping a web server up.

| Field | Type | Description |
| --- | --- | --- |
| `issuing_authority` | string | issuing_authority names the body that issued it, as it names itself — "High Court of Kenya at Nairobi", "Bank of Ghana". Free text because the set of courts and supervisors in the world is not enumerable in a proto file, and a wrong enum would be worse than an honest string. |
| `reference` | string | reference is the instrument's own identifier in the issuer's register: the case number, the direction number, the warrant number. This is the half that makes the instrument findable by somebody who does not trust this chain. |
| `kind` | LegalInstrumentKind |  |
| `hash` | string | hash is the SHA-256, lowercase hex, of the instrument as it was served. It pins the content: an order that is later amended can be shown to have been amended, and a copy produced afterwards can be checked against what the validators actually voted on. |
| `issued_at` | int64 | issued_at is when the instrument was issued, as Unix seconds. Refused if it is in the future relative to the block: an order dated tomorrow has not been issued, and a case that claims one is either mistaken or manufactured. |

### SeizureDelayTier

SeizureDelayTier is one step in the schedule that decides how long a seizure waits between being decided and being carried out.

A tier matches when the value the case assessed reaches its threshold in that denomination, and a case takes the longest delay of every tier it matches. Longest rather than first, so the schedule does not depend on the order governance happened to write it in — an ordering bug in a parameter list is invisible until the day it lets somebody's life savings move at the speed meant for pocket change.

| Field | Type | Description |
| --- | --- | --- |
| `threshold` | Coin | threshold is the smallest amount, in base units, that falls into this tier. |
| `delay_blocks` | uint64 | delay_blocks is how long a case in this tier waits after the vote before anything moves. |

### SeizureRecord

SeizureRecord is one executed seizure, kept so the rolling window can be summed without replaying the chain.

Records fall out of the window by height, and the number that can be inside one is bounded by the cap itself — which is what keeps both the sum and the pruning bounded no matter how long the chain has been running.

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |
| `height` | int64 | height is when the seizure executed. It is the first component of the key, so the window is a range scan over recent heights rather than a filter over every seizure there has ever been. |
| `amount` | repeated Coin | amount is what this seizure counted for against the cap: the value assessed when the case was decided, or what execution actually moved if that was larger. Taking the larger of the two is what stops a deposit arriving during the hold from being taken outside the window's arithmetic. |

### Vote

Vote is one validator's judgement on one case, with the power it carried when it was cast.

| Field | Type | Description |
| --- | --- | --- |
| `case_id` | uint64 |  |
| `validator` | string |  |
| `option` | VoteOption |  |
| `power` | int64 | power is recorded as it was at the moment of voting. Reading it live at resolution instead would let a validator vote and then change what their vote meant. |

## Value types

### CaseAction

CaseAction is what a case asks the chain to do.

| Value | Meaning |
| --- | --- |
| `CASE_ACTION_UNSPECIFIED` | CASE_ACTION_UNSPECIFIED is the unset default and is never valid. |
| `CASE_ACTION_FREEZE` | CASE_ACTION_FREEZE stops the account from sending anything. Nothing is taken and nothing is redistributed; the funds stay where they are and stay the account's. |
| `CASE_ACTION_SEIZE` | CASE_ACTION_SEIZE freezes the account and, if the case passes, moves what it holds to the recovery destination. Delegations are unbonded so that staked funds come back and can be recovered as they mature. |

### CaseStatus

CaseStatus is where a case has got to.

| Value | Meaning |
| --- | --- |
| `CASE_STATUS_UNSPECIFIED` | CASE_STATUS_UNSPECIFIED is the unset default and is never valid. |
| `CASE_STATUS_VOTING` | CASE_STATUS_VOTING is open. The target is frozen for as long as it stays that way, or until the provisional freeze expires. |
| `CASE_STATUS_PASSED` | CASE_STATUS_PASSED means the validators agreed. A freeze case ends here with the account frozen indefinitely; a seizure case sweeps and then keeps sweeping as unbonding funds arrive. |
| `CASE_STATUS_REJECTED` | CASE_STATUS_REJECTED means they did not, and the freeze is lifted. |
| `CASE_STATUS_EXPIRED` | CASE_STATUS_EXPIRED means the voting period ended without the threshold being reached — including the ordinary case where nobody voted at all. The freeze is lifted. This is deliberately not the same outcome as rejection: silence is not a finding, and the record should not claim it was. |
| `CASE_STATUS_WITHDRAWN` | CASE_STATUS_WITHDRAWN means whoever opened it took it back before the vote ended, which also lifts the freeze. |
| `CASE_STATUS_REVERSED` | CASE_STATUS_REVERSED means an authority overturned the case rather than the validators deciding it: governance reversing one that passed, or the emergency authority releasing an account it or a validator had frozen. Any freeze is lifted. What was already seized is not returned by this module — that takes a transfer from the recovery destination, by whoever controls it, and pretending otherwise would be a lie told by an enum value. |
| `CASE_STATUS_HELD` | CASE_STATUS_HELD is a seizure the validators have agreed to that has not been carried out yet: it is waiting out the delay its size earned. It is a status of its own rather than a flag on PASSED because it is the only window in which the seizure can still be stopped without anything having to be given back. The ombudsman's veto lives here, and so does governance's chance to reverse a case before it costs anybody anything. A reader who cannot tell "decided" from "done" cannot tell those apart. The account stays frozen throughout, and the freeze no longer lapses: the set has decided, so there is nothing left for a lapse to protect against. |
| `CASE_STATUS_VETOED` | CASE_STATUS_VETOED means the ombudsman stopped the case. Any freeze is lifted and nothing is taken. Distinct from REJECTED and from REVERSED on purpose. Rejected is the validator set disagreeing; reversed is an authority undoing something that already happened; vetoed is one office outside the set refusing to let it happen at all. Collapsing them would hide which check actually caught the case, and that is the only thing the record of a stopped case is for. |

### LegalInstrumentKind

LegalInstrumentKind is the sort of external authority a seizure rests on.

It is a closed list rather than free text because the list is the point: a seizure on this chain is the execution of something a court, a regulator or a magistrate already ordered, and "other" would let a case name its own paperwork as its authority.

| Value | Meaning |
| --- | --- |
| `LEGAL_INSTRUMENT_KIND_UNSPECIFIED` | LEGAL_INSTRUMENT_KIND_UNSPECIFIED is the unset default and is never valid. |
| `LEGAL_INSTRUMENT_KIND_COURT_ORDER` | LEGAL_INSTRUMENT_KIND_COURT_ORDER is an order of a court. |
| `LEGAL_INSTRUMENT_KIND_REGULATORY_DIRECTION` | LEGAL_INSTRUMENT_KIND_REGULATORY_DIRECTION is a direction issued by a financial supervisor under its own statutory power — a central bank's directive, a financial intelligence unit's freezing direction. |
| `LEGAL_INSTRUMENT_KIND_WARRANT` | LEGAL_INSTRUMENT_KIND_WARRANT is a warrant issued in a criminal matter. |

### VoteOption

VoteOption is how a validator voted.

| Value | Meaning |
| --- | --- |
| `VOTE_OPTION_UNSPECIFIED` | VOTE_OPTION_UNSPECIFIED is the unset default and is never valid. A vote has to say something. |
| `VOTE_OPTION_YES` | VOTE_OPTION_YES supports the case: freeze, or freeze and seize. |
| `VOTE_OPTION_NO` | VOTE_OPTION_NO opposes it. |
| `VOTE_OPTION_ABSTAIN` | VOTE_OPTION_ABSTAIN counts towards nothing. It exists so a validator can put on the record that they saw the case and declined to judge it, which is a different statement from not voting. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrUnknownValidator` | not a bonded validator |
| 1102 | `ErrCaseNotFound` | no such case |
| 1103 | `ErrCaseClosed` | case is no longer open |
| 1104 | `ErrAlreadyVoted` | this validator has already voted on this case |
| 1105 | `ErrAlreadyFrozen` | an open case already freezes this address |
| 1106 | `ErrNotTheOpener` | only the validator that opened the case may withdraw it |
| 1107 | `ErrFrozen` | account is frozen by an enforcement case |
| 1108 | `ErrEvidenceRequired` | a seizure case requires evidence |
| 1109 | `ErrInvalidCase` | case is not valid |
| 1110 | `ErrNotSeizure` | case did not order a seizure |
| 1111 | `ErrNotPassed` | case has not passed |
| 1112 | `ErrProtectedAddress` | address cannot be frozen or seized |
| 1113 | `ErrLimitReached` | exceeds a configured maximum |
| 1115 | `ErrLegalInstrumentRequired` | a seizure case requires an external legal instrument |
| 1116 | `ErrNoOmbudsman` | no ombudsman is appointed |
| 1117 | `ErrOmbudsmanCannotInitiate` | the ombudsman may only stop cases, never open, vote on, or advance one |
| 1118 | `ErrSeizureCapReached` | this seizure would breach the rolling cap on what may be taken per window |
| 1119 | `ErrNotHeld` | case is not waiting to be carried out |
