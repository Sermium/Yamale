<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/validatorgov

Restricts the validator set to candidates that governance has admitted, enforced before a create-validator transaction is accepted.

## Transactions

### MsgApplyValidator

`/blockchain.validatorgov.v1.MsgApplyValidator`

Signed by the `creator` field.

ApplyValidator defines the ApplyValidator RPC.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `moniker` | string |  |
| `description` | string |  |
| `legal_entity_id` | string | legal_entity_id identifies the applying entity. |
| `beneficial_owner_id` | string | beneficial_owner_id identifies whoever ultimately owns it. Where an entity has no owner above it, this is the entity's own identifier — stated rather than left blank, so that "nobody owns us" is a claim somebody signed. |
| `jurisdiction` | string | jurisdiction is an ISO 3166-1 alpha-2 code from the assigned list. |

### MsgApproveOperatorRecovery

`/blockchain.validatorgov.v1.MsgApproveOperatorRecovery`

Signed by the `authority` field.

ApproveOperatorRecovery defines the ApproveOperatorRecovery RPC. It is authority-gated by the same account that admits validators, because recovering one should be exactly as hard as admitting one.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `rotation_id` | uint64 | rotation_id is the recovery being decided. |
| `approve` | bool | approve is the decision. |

### MsgApproveValidator

`/blockchain.validatorgov.v1.MsgApproveValidator`

Signed by the `authority` field.

ApproveValidator defines the ApproveValidator RPC. It is authority-gated (the x/gov module account) and approves or rejects a pending validator application submitted via ApplyValidator.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `candidate` | string |  |
| `approve` | bool |  |

### MsgAttestOwnership

`/blockchain.validatorgov.v1.MsgAttestOwnership`

Signed by the `creator` field.

AttestOwnership defines the AttestOwnership RPC: the operator re-signing for who is behind it, which is what keeps a declaration from going stale.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string | creator is the approved operator address, in its account form. |
| `legal_entity_id` | string |  |
| `beneficial_owner_id` | string |  |
| `jurisdiction` | string |  |

### MsgCancelOperatorRotation

`/blockchain.validatorgov.v1.MsgCancelOperatorRotation`

Signed by the `creator` field.

CancelOperatorRotation defines the CancelOperatorRotation RPC, which lets the current operator withdraw a rotation before it takes effect.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string | creator is the current operator address. |
| `rotation_id` | uint64 | rotation_id is the rotation being withdrawn. |

### MsgProposeOperatorRecovery

`/blockchain.validatorgov.v1.MsgProposeOperatorRecovery`

Signed by the `creator` field.

ProposeOperatorRecovery defines the ProposeOperatorRecovery RPC: the recovery path, openable by anybody and inert until approved.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string | creator is whoever noticed, which is anybody. |
| `current_operator` | string | current_operator is the operator address whose key is claimed to be lost. |
| `new_operator` | string | new_operator is the address proposed to take over. |
| `reason` | string | reason is the stated grounds, in plain words. Required: a claim that somebody's key is gone should have to say how that is known. |

### MsgRotateOperator

`/blockchain.validatorgov.v1.MsgRotateOperator`

Signed by the `creator` field.

RotateOperator defines the RotateOperator RPC: the planned path, signed by the operator being replaced.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string | creator is the current operator address. |
| `new_operator` | string | new_operator is the address that takes over after the delay. |

### MsgSetValidatorPower

`/blockchain.validatorgov.v1.MsgSetValidatorPower`

Signed by the `authority` field.

SetValidatorPower defines the SetValidatorPower RPC. It is authority-gated and moves how many seats one admitted validator holds.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `validator` | string | validator is the operator address, in its account form. |
| `seats` | uint64 | seats is the power the validator is to carry. Zero is refused: a validator with no seats is one that has been removed, and removing one should say so rather than arrive as a power update that happens to be empty. |

### MsgUpdateParams

`/blockchain.validatorgov.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

## Queries

### Concentration

`GET /yamale/blockchain/validatorgov/v1/concentration`

Concentration reports what every declared entity, owner and jurisdiction currently holds, against its ceiling.

This is the supervisor's query. Under equal seats a ceiling is a count out of a count, so the answer is meant to be checked against a list of admitted validators by somebody who is not recomputing anything — which is the whole argument for declaring ownership on-chain rather than filing it somewhere.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `groups` | repeated ConcentrationGroup |  |
| `total_power` | int64 | total_power is the denominator every share above was measured against: the power of the validators active right now. Returned alongside so that a reader can check the arithmetic rather than trust it. |
| `active_validators` | uint32 | active_validators and min_active_validators say whether the check is in a position to act at all. A breach reported while these are equal is one the chain has decided not to correct, and that distinction is invisible from the shares alone. |
| `min_active_validators` | uint32 |  |

### GetApprovedValidator

`GET /yamale/blockchain/validatorgov/v1/approved_validator/{candidate}`

ListApprovedValidator Queries a list of ApprovedValidator items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `candidate` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `approved_validator` | ApprovedValidator |  |

### GetOperatorRotation

`GET /yamale/blockchain/validatorgov/v1/operator_rotation/{id}`

GetOperatorRotation queries one rotation by id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `operator_rotation` | OperatorRotation |  |

### GetValidatorApplication

`GET /yamale/blockchain/validatorgov/v1/validator_application/{candidate}`

ListValidatorApplication Queries a list of ValidatorApplication items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `candidate` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `validator_application` | ValidatorApplication |  |

### ListApprovedValidator

`GET /yamale/blockchain/validatorgov/v1/approved_validator`

ListApprovedValidator defines the ListApprovedValidator RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `approved_validator` | repeated ApprovedValidator |  |
| `pagination` | PageResponse |  |

### ListDemotion

`GET /yamale/blockchain/validatorgov/v1/demotion`

ListDemotion queries every demotion currently in force.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `demotion` | repeated Demotion |  |
| `pagination` | PageResponse |  |

### ListOperatorRotation

`GET /yamale/blockchain/validatorgov/v1/operator_rotation`

ListOperatorRotation queries every rotation ever opened.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `operator_rotation` | repeated OperatorRotation |  |
| `pagination` | PageResponse |  |

### ListValidatorApplication

`GET /yamale/blockchain/validatorgov/v1/validator_application`

ListValidatorApplication defines the ListValidatorApplication RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `validator_application` | repeated ValidatorApplication |  |
| `pagination` | PageResponse |  |

### Params

`GET /yamale/blockchain/validatorgov/v1/params`

Parameters queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

### PendingOperatorRotation

`GET /yamale/blockchain/validatorgov/v1/pending_operator_rotation/{current_operator}`

PendingOperatorRotation queries the open rotation against one operator, if there is one. This is the query a delegator runs before deciding whether to undelegate, so it answers from the address they already know rather than from a rotation id they would have to go and find.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `current_operator` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `found` | bool | found is false when nothing is pending against the operator. Reported as a field rather than as a not-found error, because "no rotation is pending" is the answer the question has, not a failure to answer it. |
| `operator_rotation` | OperatorRotation |  |

## State

### ApprovedValidator

ApprovedValidator defines the ApprovedValidator message.

| Field | Type | Description |
| --- | --- | --- |
| `candidate` | string |  |
| `approved` | string |  |
| `declaration` | Declaration | declaration is copied from the application at approval and kept current by re-attestation. The epoch check reads it from here rather than from the application, because the application is what was asked for once and this is what is claimed now — and a ceiling has to be computed against the second. |

### ConcentrationGroup

ConcentrationGroup is one entity, owner or jurisdiction and what it currently holds. It exists for the supervisor's query rather than for the chain: under equal seats a ceiling is a count out of a count, and the point of publishing it this way is that it can be checked against a list by somebody who is not recomputing anything.

| Field | Type | Description |
| --- | --- | --- |
| `cap` | ConcentrationCap |  |
| `group` | string |  |
| `power` | int64 |  |
| `power_bps` | uint64 |  |
| `cap_bps` | uint64 |  |
| `over` | bool | over is whether the group is above its ceiling right now, which is not the same as whether anything has been demoted: a breach the set is too small to correct stays reported and uncorrected, because a cap must never be the reason a chain stops producing blocks. |

### Declaration

Declaration is who is actually behind a validator.

A concentration cap cannot be computed from addresses. Two operator keys tell you nothing about whether they answer to one bank, and the events that concentrate a validator set — an acquisition, a merger, a nationalisation — change none of the addresses involved. So the attributes are declared, and the chain enforces ceilings over what was declared.

The chain cannot detect a false declaration and does not pretend to. What it can do is make the declaration a signed statement, on the record, with a date on it, so that a lie is documented and actionable rather than deniable. That is how the rest of the financial system handles beneficial ownership, and it is the only honest thing a chain can offer here.

| Field | Type | Description |
| --- | --- | --- |
| `legal_entity_id` | string | legal_entity_id identifies the admitted entity — an LEI where the applicant has one, a national register number otherwise. Free text because there is no single global register this chain could validate against, and a format check that accepted only LEIs would exclude every entity in a jurisdiction that does not issue them. |
| `beneficial_owner_id` | string | beneficial_owner_id identifies the ultimate beneficial owner, and it is the field the caps actually turn on. Two subsidiaries of one state bank are two legal entities and one owner; a ceiling applied only to entities would let that owner take the set two seats at a time. |
| `jurisdiction` | string | jurisdiction is the ISO 3166-1 alpha-2 code of the authority the operator answers to, validated against the assigned-country list x/alias owns. A shape check is not enough: NX and QK are two letters and neither is a country, and a mistyped code would create a perimeter no authority holds and therefore a ceiling nothing is ever measured against. |
| `attested_at_height` | int64 | attested_at_height is when this declaration was last signed for. A declaration with no date cannot be stale, and a declaration that cannot be stale is one nobody has to keep true. Re-attestation is what turns silence into a visible fact: after the interval the record is reported as stale in queries and in an event at every epoch, so an ownership change that was never declared shows up as an operator who has stopped signing for its own declaration. |

### Demotion

Demotion is a validator whose power the epoch check took away, and why.

It is a record and not a punishment. Nothing is slashed, no delegation is unbonded, and it is undone automatically at the first epoch the breach has cleared — so the entity's remedy is in its own hands and does not require anybody to vote on letting it back. That is the difference between a concentration ceiling and an expulsion.

| Field | Type | Description |
| --- | --- | --- |
| `operator` | string | operator is the address in its account form, matching how the approval allowlist and the rotation records key a validator. |
| `cap` | ConcentrationCap |  |
| `group` | string | group is the declared value that breached — the entity id, the owner id or the country code — so the record says which set the validator was counted in rather than only that some ceiling was hit. |
| `group_power_bps` | uint64 | group_power_bps and cap_bps are what the group held and what it was allowed to hold, at the epoch the demotion happened. Frozen into the record because both figures are recomputed every epoch, and a demotion that could only be explained by numbers that have since moved cannot be audited at all. |
| `cap_bps` | uint64 |  |
| `demoted_at_height` | int64 |  |
| `jailed_validator` | bool | jailed_validator records that this demotion is what jailed the validator, so that restoring it un-jails only what it jailed. A validator already jailed for downtime when the epoch check ran must stay jailed when the breach clears: releasing it would turn a concentration ceiling into a way of clearing somebody else's downtime. |

### EventConcentrationUncorrected

EventConcentrationUncorrected is emitted when a group is over its ceiling and the epoch check will not act, because acting would take the active set below the floor.

This is the honest half of the design. A cap can be arithmetically unsatisfiable at a given set size, and a check that kept demoting until the ceiling held would demote the chain into a halt. So the breach is published every epoch instead, which is a problem for governance rather than a problem for block production.

| Field | Type | Description |
| --- | --- | --- |
| `cap` | ConcentrationCap |  |
| `group` | string |  |
| `group_power_bps` | uint64 |  |
| `cap_bps` | uint64 |  |
| `active_validators` | uint32 |  |
| `min_active_validators` | uint32 |  |

### EventDeclarationStale

EventDeclarationStale is emitted at each epoch for an approved validator that has not re-attested within the interval. Nothing is done about it here; the event is the doing. An ownership change that was never declared looks exactly like an operator who stopped signing for its own declaration, which is the only signal a chain can produce about a statement it cannot verify.

| Field | Type | Description |
| --- | --- | --- |
| `operator` | string |  |
| `attested_at_height` | int64 |  |
| `stale_since_height` | int64 |  |

### EventRecoveryApproved

EventRecoveryApproved is emitted when the admission quorum agrees to a recovery. This is the moment the validator is paused and the challenge window starts, so it is the moment anybody watching needs to hear about.

| Field | Type | Description |
| --- | --- | --- |
| `rotation_id` | uint64 |  |
| `current_operator` | string |  |
| `new_operator` | string |  |
| `completes_at_height` | int64 |  |
| `validator_paused` | bool | validator_paused is false when there was no validator to pause — an approved candidate who never created one — or when it was already jailed for something else. |

### EventRotationProposed

EventRotationProposed is emitted when a rotation is opened, by either path.

| Field | Type | Description |
| --- | --- | --- |
| `rotation_id` | uint64 |  |
| `current_operator` | string |  |
| `new_operator` | string |  |
| `proposer` | string |  |
| `kind` | RotationKind |  |
| `reason` | string |  |
| `completes_at_height` | int64 | completes_at_height is zero for a recovery that has not been approved yet, because no clock is running on it. |

### EventRotationResolved

EventRotationResolved is emitted once, when a rotation reaches its final status, including when it is vetoed by the old key signing.

| Field | Type | Description |
| --- | --- | --- |
| `rotation_id` | uint64 |  |
| `current_operator` | string |  |
| `new_operator` | string |  |
| `kind` | RotationKind |  |
| `status` | RotationStatus |  |

### EventValidatorDemoted

EventValidatorDemoted is emitted when the epoch check takes a validator's seats away.

| Field | Type | Description |
| --- | --- | --- |
| `operator` | string |  |
| `cap` | ConcentrationCap |  |
| `group` | string |  |
| `group_power_bps` | uint64 |  |
| `cap_bps` | uint64 |  |
| `jailed_validator` | bool | jailed_validator is false when the validator was already jailed for something else, in which case the demotion is recorded but nothing was done to it. |

### EventValidatorRestored

EventValidatorRestored is emitted when a breach clears and the seats go back. Restoration is automatic and nobody votes on it, so it needs announcing for the same reason the demotion did.

| Field | Type | Description |
| --- | --- | --- |
| `operator` | string |  |
| `cap` | ConcentrationCap |  |
| `group` | string |  |
| `unjailed_validator` | bool |  |

### OperatorRotation

OperatorRotation is one attempt to move a validator's operator key, and everything the chain did about it. Rotations are never deleted: a recovery that was vetoed is the record of somebody having claimed a key was lost when it was not, and that is precisely the history worth keeping.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | id is numbered from one. Zero is reserved because in proto3 an id of 0 is indistinguishable from an unset field, and "rotation 0" would be a record nobody could look up. |
| `current_operator` | string | current_operator is the address being replaced. It is the same address the ApprovedValidator allowlist is keyed by, which is the account form of the validator's operator address. |
| `new_operator` | string | new_operator is the address that takes over. |
| `kind` | RotationKind |  |
| `status` | RotationStatus |  |
| `proposer` | string | proposer is whoever submitted it. For a planned rotation this equals current_operator; for a recovery it is whoever noticed, which is anybody. |
| `reason` | string | reason is the stated grounds. Required on a recovery — a claim that somebody's key is gone should have to say so in words — and unused on a planned rotation, where the signature says everything. |
| `approved` | bool | approved is set once the admission quorum has agreed to a recovery. Until then a recovery is inert: it is a claim on the record, and it neither pauses the validator nor runs a clock. Otherwise anyone could pause any validator on this chain for the price of one transaction. |
| `opened_at_height` | int64 |  |
| `completes_at_height` | int64 | completes_at_height is the height the rotation takes effect at. Zero means no clock is running yet, which for a recovery is the ordinary state between being proposed and being approved. |
| `resolved_at_height` | int64 |  |
| `paused_validator` | bool | paused_validator records that this rotation is what jailed the validator, so that resolving it un-jails only what it jailed. A validator already jailed for downtime when the recovery was approved must stay jailed when the recovery ends: releasing it would turn a contested rotation into a way of clearing somebody else's downtime. |

### ValidatorApplication

ValidatorApplication defines the ValidatorApplication message.

| Field | Type | Description |
| --- | --- | --- |
| `candidate` | string |  |
| `status` | string |  |
| `declaration` | Declaration | declaration is who is behind the applicant. Carried on the application and not only on the approval, because it is what the admission vote is meant to be judging: a set asked to approve a candidate whose owner and jurisdiction it cannot see is being asked to approve an address. |

## Value types

### ConcentrationCap

ConcentrationCap is which ceiling a demotion was for. Recorded rather than inferred, because "over the entity cap" and "over the jurisdiction cap" are different facts about the same validator and the remedy is different for each: one entity can be restructured, a jurisdiction cannot.

| Value | Meaning |
| --- | --- |
| `CONCENTRATION_CAP_UNSPECIFIED` | CONCENTRATION_CAP_UNSPECIFIED is the unset default and is never valid. |
| `CONCENTRATION_CAP_ENTITY` | CONCENTRATION_CAP_ENTITY is the ceiling on one declared legal entity. |
| `CONCENTRATION_CAP_BENEFICIAL_OWNER` | CONCENTRATION_CAP_BENEFICIAL_OWNER is the ceiling on one declared ultimate beneficial owner, across every entity it owns. |
| `CONCENTRATION_CAP_JURISDICTION` | CONCENTRATION_CAP_JURISDICTION is the ceiling on the power answering to one national authority. |

### RotationKind

RotationKind is which of the two paths a rotation took, and it decides everything about how the rotation is treated. The two are separated at the type level rather than inferred from who signed, so that a reader of the record — or of an event — never has to reconstruct which set of rules applied to a rotation that already happened.

| Value | Meaning |
| --- | --- |
| `ROTATION_KIND_UNSPECIFIED` | ROTATION_KIND_UNSPECIFIED is the unset default and is never valid. |
| `ROTATION_KIND_PLANNED` | ROTATION_KIND_PLANNED was signed by the operator being replaced. Possession of the key is the whole proof, so nothing else is required of it: anyone able to submit this already controls everything it protects. |
| `ROTATION_KIND_RECOVERY` | ROTATION_KIND_RECOVERY was proposed by somebody who does not hold the key, which is also exactly what an attacker would be doing. It carries the admission quorum, the pause and the challenge window because of that. |

### RotationStatus

RotationStatus is where a rotation has got to.

| Value | Meaning |
| --- | --- |
| `ROTATION_STATUS_UNSPECIFIED` | ROTATION_STATUS_UNSPECIFIED is the unset default and is never valid. |
| `ROTATION_STATUS_PENDING` | ROTATION_STATUS_PENDING is open. A planned rotation is counting down its delay; a recovery is either waiting for the admission quorum or counting down its challenge window. |
| `ROTATION_STATUS_COMPLETED` | ROTATION_STATUS_COMPLETED means the new operator is now the operator of record. |
| `ROTATION_STATUS_CANCELLED` | ROTATION_STATUS_CANCELLED means the current operator withdrew it deliberately, by message, before it took effect. |
| `ROTATION_STATUS_VETOED` | ROTATION_STATUS_VETOED means the current operator signed a transaction while the recovery claiming their key was lost was open. It is deliberately not the same status as cancelled: cancelled is somebody changing their mind, vetoed is a claim about a lost key being disproved, and conflating the two would hide the only evidence that a recovery was false. |
| `ROTATION_STATUS_REJECTED` | ROTATION_STATUS_REJECTED means the admission quorum refused the recovery. |

## Parameters

Changed by governance through `MsgUpdateParams`. Defaults are the values a chain starts with at genesis.

| Parameter | Default | Description |
| --- | --- | --- |
| `planned_rotation_delay_blocks` | `34560` | planned_rotation_delay_blocks is how long a rotation signed by the current operator waits before it takes effect. Short on purpose: whoever submitted it already holds the key, so the delay is not there to prove anything, only to give delegators and the other validators a block window in which to see it happen. |
| `recovery_challenge_window_blocks` | `120960` | recovery_challenge_window_blocks is how long an approved recovery waits before it takes effect, during which the validator is paused and the current operator can end it by signing anything at all. Measured in days, because the thing it is defending against is somebody who does not hold the key claiming that nobody does. |
| `attestation_interval_blocks` | `6307200` | attestation_interval_blocks is how long a declaration stays fresh before the chain starts reporting it as stale. Reported, not enforced. Turning an expired attestation into a demotion would make an operator's inattention a consensus event, and the failure it would cause — a set that all forgot at once — is worse than the one it would prevent. What the chain does instead is publish the date and say so loudly, which is enough for admission governance to act on and is the most an unfalsifiable declaration can honestly support. |
| `seat_bond_amount` | `1000000` | seat_bond_amount is how many base units of the bond denomination one seat carries. Equal seats are implemented in the unit the SDK already counts. Cosmos derives consensus power from bonded tokens and there is no supported way for a second module to report a different number — only one module may return validator updates, and x/staking is that module. So a seat is a fixed quantity of stake rather than a parallel notion of power, and every threshold on this chain that reads bonded power keeps working unchanged: x/enforcement's two thirds, x/oracle's rate agreement and x/gov's tally all become seat counts by arithmetic rather than by amendment. Set to the SDK's power reduction, one seat is exactly one unit of consensus power, which is what makes a ceiling countable off a list. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrApplicationNotFound` | validator application not found |
| 1102 | `ErrApplicationNotPending` | validator application is not pending |
| 1103 | `ErrNotApprovedValidator` | address is not an approved validator operator |
| 1104 | `ErrRotationInProgress` | a rotation is already open against this operator |
| 1105 | `ErrRotationNotFound` | operator rotation not found |
| 1106 | `ErrRotationNotPending` | operator rotation is no longer pending |
| 1107 | `ErrRotationNotRecovery` | operator rotation is not a recovery |
| 1108 | `ErrRecoveryAlreadyDecided` | operator recovery has already been decided |
| 1109 | `ErrOperatorUnchanged` | the new operator is the current operator |
| 1110 | `ErrOperatorInUse` | the new operator address is already an approved validator operator |
| 1111 | `ErrNotCurrentOperator` | signer is not the current operator |
| 1112 | `ErrMissingReason` | a recovery must state its grounds |
| 1113 | `ErrInvalidDeclaration` | beneficial ownership declaration is not valid |
| 1114 | `ErrNoValidator` | no validator has been created for this operator |
| 1115 | `ErrSeatReserveEmpty` | the module's seat reserve cannot cover this power |
| 1116 | `ErrInvalidSeats` | a validator must hold at least one seat |
