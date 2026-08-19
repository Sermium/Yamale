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

EmergencyFreeze lets the founders' group stop an account in one block, without waiting for a validator to open a case.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority must equal the emergency_authority parameter. |
| `target` | string |  |
| `reason` | string | reason is the grounds, in words the accused can read. Required, the same as for a validator's case: acting in an emergency is not a reason to leave the record blank, it is the reason the record matters. |
| `evidence_uri` | string |  |
| `evidence_hash` | string |  |

### MsgEmergencyRelease

`/blockchain.enforcement.v1.MsgEmergencyRelease`

Signed by the `authority` field.

EmergencyRelease lets them let it go again, just as fast.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `case_id` | uint64 |  |
| `reason` | string | reason is why it was released, kept beside the original accusation. The case is not deleted: a freeze that was lifted as a mistake is part of the record of how this power has been used. |

### MsgOpenCase

`/blockchain.enforcement.v1.MsgOpenCase`

Signed by the `opener` field.

OpenCase accuses an address, and freezes it while the validators decide.

| Field | Type | Description |
| --- | --- | --- |
| `opener` | string | opener is the account address of a bonded validator — the key it signs with, not its operator address. The chain derives the operator from it and records that on the case, so the accusation is attributed to the validator rather than to an address nobody recognises. |
| `target` | string |  |
| `action` | CaseAction |  |
| `reason` | string | reason is the grounds, in words the accused can read. |
| `evidence_uri` | string | evidence_uri and evidence_hash point at and pin the evidence held off-chain. Required for a seizure unless the parameters say otherwise. |
| `evidence_hash` | string |  |

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
| `opener` | string | opener is the account address of the validator that opened the case. |
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

### Freeze

Freeze is the fact that stops the money moving.

It is kept as its own record rather than being derived by scanning cases, because it is read on every single transfer the chain processes.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |
| `case_id` | uint64 | case_id is the case that put it there — so an account holder who is refused a transfer can be told which case to read, rather than "no". |
| `expires_at_height` | int64 | expires_at_height is the height the freeze lapses at, for the provisional freeze that comes with opening a case. Zero means it does not expire on its own: the validators voted for it, and lifting it takes a decision. |
| `frozen_at_height` | int64 |  |

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
| 1114 | `ErrNoEmergencyAuthority` | no emergency authority is configured |
