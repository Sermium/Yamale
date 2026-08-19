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

### MsgUpdateParams

`/blockchain.validatorgov.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

## Queries

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

## Value types

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
