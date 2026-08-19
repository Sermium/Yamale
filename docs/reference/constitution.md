<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/constitution

## Transactions

### MsgProposeAmendment

`/blockchain.constitution.v1.MsgProposeAmendment`

Signed by the `authority` field.

ProposeAmendment opens a change to the invariants and starts its public delay. It is authority-gated, so a proposal is the only way in.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `invariants` | Invariants | invariants is the complete replacement set, not a delta. All fields must be supplied and all of them are validated, so an amendment that meant to move one ceiling cannot silently zero another by omitting it — and a zero here would be a divisor or a missing destination somewhere else. |
| `reason` | string | reason is the stated grounds, in plain words. |

### MsgRatifyAmendment

`/blockchain.constitution.v1.MsgRatifyAmendment`

Signed by the `validator` field.

RatifyAmendment records one validator agreeing to a pending amendment.

| Field | Type | Description |
| --- | --- | --- |
| `validator` | string | validator is the operator address in its account form. |
| `amendment_id` | uint64 |  |

### MsgWithdrawAmendment

`/blockchain.constitution.v1.MsgWithdrawAmendment`

Signed by the `authority` field.

WithdrawAmendment takes a pending amendment back before it takes effect.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `amendment_id` | uint64 |  |

## Queries

### Amendment

`GET /yamale/blockchain/constitution/v1/amendment/{id}`

Amendment queries one amendment by id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `amendment` | Amendment |  |

### Invariants

`GET /yamale/blockchain/constitution/v1/invariants`

Invariants returns the values in force. This is the query a supervisor runs to find out what the chain has promised not to change, so it answers with the whole settlement rather than a field at a time.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `invariants` | Invariants |  |

### ListAmendment

`GET /yamale/blockchain/constitution/v1/amendment`

ListAmendment queries every amendment ever opened, lapsed and withdrawn ones included.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `amendment` | repeated Amendment |  |
| `pagination` | PageResponse |  |

### Ratifications

`GET /yamale/blockchain/constitution/v1/amendment/{amendment_id}/ratifications`

Ratifications lists who has ratified one amendment, which is the record that makes a validator answerable for a change it agreed to.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `amendment_id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `ratifications` | repeated Ratification |  |
| `required_power` | int64 | required_power is what the amendment still has to reach, restated here so that a reader of this response never has to recompute a threshold from a basis-point figure held in another query's answer. |

## State

### Amendment

Amendment is one attempt to change the invariants, and everything the chain did about it. Amendments are never deleted: an amendment that lapsed is the record of somebody having tried to move a ceiling, and a set of amendments filtered down to the ones that succeeded would hide exactly the pattern worth noticing.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id is numbered from one. Zero is reserved because in proto3 an id of 0 is indistinguishable from an unset field, and "amendment 0" would be a record nobody could look up. |
| `proposed` | Invariants | proposed is the complete replacement set. Whole rather than a delta, so that what the chain will hold after enactment is on the record from the day it was proposed, and nobody has to reconstruct it from a diff against a state that may itself have been amended in between. |
| `reason` | string | reason is the stated grounds, in plain words. Required: a proposal to move the numbers this chain promised not to move should have to say why. |
| `proposed_at_height` | int64 |  |
| `effective_at_height` | int64 | effective_at_height is the earliest height the amendment can take effect. It is fixed when the amendment opens, from the delay in force at that moment, so an amendment that shortens the delay cannot shorten its own. |
| `snapshot_power` | int64 | snapshot_power is the total voting power at the moment the amendment opened. Ratification is measured against this and not against the power bonded when the delay runs out, because a threshold measured against the set that remains is passed by jailing everyone who would have voted no. |
| `ratified_power` | int64 | ratified_power is the power that has ratified so far, accumulated as each ratification arrives at the weight the validator carried then. Kept as a running total rather than recomputed at enactment for the same reason: a tally recomputed at the end is a tally that can be changed by changing the set. |
| `status` | AmendmentStatus |  |
| `resolved_at_height` | int64 |  |

### EventAmendmentProposed

EventAmendmentProposed is emitted when an amendment opens.

| Field | Type | Description |
| --- | --- | --- |
| `amendment_id` | uint64 |  |
| `reason` | string |  |
| `effective_at_height` | int64 |  |
| `snapshot_power` | int64 |  |
| `required_power` | int64 |  |

### EventAmendmentRatified

EventAmendmentRatified is emitted for each validator that ratifies, carrying the running total so that anyone watching can see the threshold approaching rather than only learn it was reached.

| Field | Type | Description |
| --- | --- | --- |
| `amendment_id` | uint64 |  |
| `validator` | string |  |
| `power` | int64 |  |
| `ratified_power` | int64 |  |
| `required_power` | int64 |  |

### EventAmendmentResolved

EventAmendmentResolved is emitted once, when an amendment reaches its final status — enacted, lapsed or withdrawn.

| Field | Type | Description |
| --- | --- | --- |
| `amendment_id` | uint64 |  |
| `status` | AmendmentStatus |  |
| `ratified_power` | int64 |  |
| `required_power` | int64 |  |

### Invariants

Invariants are the values this chain fixes at genesis and will not let an ordinary governance proposal change.

Every other parameter on this chain is a mutable gov param, and that was not a theoretical weakness: recovery_destination — the single address seized assets may be sent to — was found empty on the running devnet, which means a seizure carried by two thirds of the validator set would have had nowhere to send what it took, and nobody noticed until a console happened to print it. A parameter that nothing refuses to change is a parameter nothing is checking.

What belongs here is narrow and deliberate: the numbers a majority would have an interest in moving in the moment it wanted to act. A chain that can vote to lower its own seizure threshold does not have one, and a cap enforced at an epoch the same vote can lengthen is not enforced at every epoch — which is why the epoch length is in this message and not in x/validatorgov's params.

The values are not duplicated state. x/enforcement still stores its own Params; what changed is that both its MsgUpdateParams and its InitGenesis refuse anything that disagrees with the copy here. Divergence is therefore unrepresentable rather than merely unlikely, and the modules that read these values at speed keep reading their own store.

Zero is "unset" for every field. That is why InitGenesis refuses a zero rather than substituting a default: an address compiled into a binary is not the foundation on somebody else's network, and a cap that defaulted itself into existence would be a cap nobody chose.

| Field | Type | Description |
| --- | --- | --- |
| `max_entity_power_bps` | uint64 | max_entity_power_bps is the share of voting power one legal entity may hold, in basis points. Under equal seats this is countable rather than arithmetic: with one seat per admitted validator, 2000 is "no entity holds more than a fifth of the seats", which a supervisor can check against a list without recomputing anything. |
| `max_beneficial_owner_power_bps` | uint64 | max_beneficial_owner_power_bps is the same ceiling applied to the ultimate beneficial owner behind the entities, which is the number that actually matters. Two subsidiaries of one state bank are two entities and one owner, and it is the owner that decides how they both vote. |
| `max_jurisdiction_power_bps` | uint64 | max_jurisdiction_power_bps is the ceiling on the power answering to any one national authority. It is what makes open accession possible: a sovereign is never refused, only admitted at a bounded weight, so balance is kept by arithmetic instead of by a refusal that would be read as exclusion. |
| `concentration_epoch_blocks` | uint64 | concentration_epoch_blocks is how often the ceilings above are checked and breaches corrected. It is constitutional for the same reason the ceilings are. Concentration is a state, not an event — it changes when a bank acquires a participant, when two members merge, when an operator is nationalised — so the only thing standing between a cap and a dead letter is that the check keeps running. A proposal able to set this to a billion blocks would repeal the caps without appearing to touch them. |
| `min_active_validators` | uint32 | min_active_validators is the size below which the epoch check stops demoting and starts merely reporting. A cap can be arithmetically unsatisfiable: three validators under three owners cannot all sit below a fifth of the power, and a check that kept demoting until the ceiling held would demote the chain to a halt. So the floor is explicit and a breach it cannot correct is recorded and emitted as an event instead. Enforcement must never be the thing that stops the chain — an uncorrectable breach that everyone can see is strictly better than a network that no longer produces blocks. |
| `enforcement_threshold_bps` | uint64 | enforcement_threshold_bps is x/enforcement's seizure threshold: the share of voting power that must vote yes before assets are taken. It is fixed here because it is the first number a set that wanted to seize something would have an interest in lowering, and lowering it is exactly the move that would be indistinguishable from a routine parameter change. |
| `enforcement_recovery_destination` | string | enforcement_recovery_destination is the only address seized assets may be sent to. Fixed here because a mutable destination is the difference between a recovery mechanism and a theft mechanism: the threshold decides whether funds move, this decides who ends up with them. It is an x/group policy account, so the two fields below say what that policy has to look like. Together the three describe the whole custody arrangement: this address, held by exactly foundation_custodian_count people, any foundation_signature_threshold of whom can act. |
| `enforcement_voting_period_blocks` | uint64 | enforcement_voting_period_blocks and enforcement_provisional_freeze_blocks are the two delays that make a seizure answerable. Shortening the voting period is how a supermajority becomes whoever was awake; shortening the freeze is how an account is handed back mid-case. Both are the sort of change that reads as housekeeping and is not. |
| `enforcement_provisional_freeze_blocks` | uint64 |  |
| `amendment_delay_blocks` | uint64 | amendment_delay_blocks is how long a proposed amendment sits in public before it can take effect. Weeks, not hours: the delay is the whole protection, because it is the window in which anyone who would be harmed by the change finds out it is coming while they can still act on it. |
| `amendment_threshold_bps` | uint64 | amendment_threshold_bps is the share of the voting power recorded when the amendment was proposed that must separately ratify it, in basis points. Separately, because a gov proposal passes on whatever threshold x/gov happens to carry, and x/gov's thresholds are ordinary mutable params — so an amendment ratified only through gov could be ratified by first voting gov's own numbers down. The ratification here is measured against a snapshot taken when the amendment opened, so it cannot be passed by shrinking the electorate either. |
| `foundation_custodian_count` | uint32 | foundation_custodian_count is how many people hold the account seized assets are sent to, and it is an exact number rather than a floor. Exact, because the failure it prevents is a ratchet rather than an event. A custodian leaves and the group drops to four: that is not "one short", it moves the rule from three of five to three of four, so the people who remain each hold more of the authority than the set agreed they should. Lose another and it is three of three, where every custodian holds a veto and one unreachable person freezes the account permanently. Nobody votes for that; it is arrived at by taking two individually reasonable decisions in sequence, months apart, each one obviously fine on its own. So a departure and a replacement are one decision. x/group's MsgUpdateGroupMembers carries adds and removes in the same message, which makes the swap the native operation rather than a convention, and an ante decorator refuses any update that would leave the foundation group at a different size — including MsgLeaveGroup, which is a custodian shrinking the set with nobody's agreement but their own. |
| `foundation_signature_threshold` | uint32 | foundation_signature_threshold is how many of them must sign. Constitutional rather than an ordinary group decision because the group is its own admin: without this, three custodians could vote to make it two, and the arrangement that took a ceremony to establish would be changed by the people it constrains, in one proposal, with a week's notice. Moving it now costs a proposal, three weeks in public and a four-fifths ratification — which is the right weight for "how many people it takes to move seized property". |

### Ratification

Ratification is one validator agreeing to one amendment, at the weight it carried when it agreed. Recorded individually rather than only as a total so that who ratified a change to the seizure threshold is answerable for it afterwards, which is the only sanction available against a set that votes itself more power.

| Field | Type | Description |
| --- | --- | --- |
| `amendment_id` | uint64 |  |
| `validator` | string | validator is the operator address in its account form, matching how every other record in this repository keys a validator. |
| `power` | int64 |  |
| `height` | int64 |  |

## Value types

### AmendmentStatus

AmendmentStatus is where an amendment has got to.

| Value | Meaning |
| --- | --- |
| `AMENDMENT_STATUS_UNSPECIFIED` | AMENDMENT_STATUS_UNSPECIFIED is the unset default and is never valid. |
| `AMENDMENT_STATUS_PENDING` | AMENDMENT_STATUS_PENDING is proposed, in public, and counting down. It is collecting ratifications and has changed nothing. |
| `AMENDMENT_STATUS_ENACTED` | AMENDMENT_STATUS_ENACTED means the delay ran out with enough ratified power behind it and the invariants were replaced. |
| `AMENDMENT_STATUS_LAPSED` | AMENDMENT_STATUS_LAPSED means the delay ran out without enough ratified power. It is deliberately not the same status as withdrawn: lapsed is the record of an amendment the validator set declined to ratify, and that is precisely the history worth keeping. |
| `AMENDMENT_STATUS_WITHDRAWN` | AMENDMENT_STATUS_WITHDRAWN means governance took it back before the delay ran out. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrInvariantViolation` | this value is fixed at genesis and cannot be changed by a parameter update |
| 1102 | `ErrAmendmentNotFound` | no such amendment |
| 1103 | `ErrAmendmentClosed` | amendment is no longer pending |
| 1104 | `ErrAlreadyRatified` | this validator has already ratified this amendment |
| 1105 | `ErrUnknownValidator` | not a bonded validator |
| 1106 | `ErrNoInvariants` | this chain has no constitution |
| 1107 | `ErrConstitutionalInvariant` | this would break a value the constitution fixes |
