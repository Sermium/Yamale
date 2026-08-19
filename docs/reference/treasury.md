<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/treasury

Programmable custody: shared funds with roles, spending policies, time locks and vesting schedules, where committed funds cannot be spent by anyone.

## Transactions

### MsgAssignRole

`/blockchain.treasury.v1.MsgAssignRole`

Signed by the `admin` field.

AssignRole grants an address a role over a treasury.

| Field | Type | Description |
| --- | --- | --- |
| `admin` | string |  |
| `treasury_id` | uint64 |  |
| `address` | string |  |
| `role` | Role |  |

### MsgClaimLock

`/blockchain.treasury.v1.MsgClaimLock`

Signed by the `beneficiary` field.

ClaimLock releases whatever has vested to the beneficiary.

| Field | Type | Description |
| --- | --- | --- |
| `beneficiary` | string |  |
| `lock_id` | uint64 |  |

### MsgCreateLock

`/blockchain.treasury.v1.MsgCreateLock`

Signed by the `admin` field.

CreateLock commits treasury funds to a beneficiary on a schedule.

| Field | Type | Description |
| --- | --- | --- |
| `admin` | string |  |
| `treasury_id` | uint64 |  |
| `beneficiary` | string |  |
| `denom` | string |  |
| `amount` | string |  |
| `lock_type` | LockType |  |
| `start_time` | int64 |  |
| `cliff_time` | int64 |  |
| `end_time` | int64 |  |
| `release_intervals` | uint64 |  |
| `revocable` | bool |  |

### MsgCreateTreasury

`/blockchain.treasury.v1.MsgCreateTreasury`

Signed by the `creator` field.

CreateTreasury opens a new treasury.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `name` | string |  |
| `admin` | string | admin defaults to the creator when empty. |

### MsgDeposit

`/blockchain.treasury.v1.MsgDeposit`

Signed by the `depositor` field.

Deposit moves funds from an account into a treasury.

| Field | Type | Description |
| --- | --- | --- |
| `depositor` | string |  |
| `treasury_id` | uint64 |  |
| `amount` | repeated Coin |  |

### MsgDisputeEscrow

`/blockchain.treasury.v1.MsgDisputeEscrow`

Signed by the `party` field.

MsgDisputeEscrow freezes a lock and refers it to the named moderator.

Either party may send it. That symmetry is what removes the need for a deadline: a seller facing a buyer who has gone quiet escalates rather than waiting forever, and a buyer who received nothing does the same.

| Field | Type | Description |
| --- | --- | --- |
| `party` | string |  |
| `lock_id` | uint64 |  |
| `reason` | string |  |

### MsgOpenEscrow

`/blockchain.treasury.v1.MsgOpenEscrow`

Signed by the `depositor` field.

--- conditional locks (escrow) ----------------------------------------

| Field | Type | Description |
| --- | --- | --- |
| `depositor` | string |  |
| `beneficiary` | string |  |
| `moderator` | string | Named at creation, never afterwards. |
| `amount` | Coin |  |
| `memo` | string | What is being bought, so a moderator reading the case later knows. |
| `amount_commitment` | bytes | 6-8 mirror MsgSpend, and matter more here than there. An escrow is two private parties and a consumer purchase: the amount is nobody else's business, and "what is being bought" is a description of a person's life written to a public ledger that cannot forget it. Unpopulated for now; numbered now because escrow is the first thing on this chain that a member of the public will use with their own money. |
| `amount_range_proof` | bytes |  |
| `metadata_hash` | bytes |  |

### MsgReleaseEscrow

`/blockchain.treasury.v1.MsgReleaseEscrow`

Signed by the `depositor` field.

MsgReleaseEscrow pays the beneficiary. Only the depositor may send it.

The buyer confirming is the whole condition. Nobody else can confirm on their behalf — not the seller, not the moderator, not the treasury admin.

| Field | Type | Description |
| --- | --- | --- |
| `depositor` | string |  |
| `lock_id` | uint64 |  |

### MsgResolveEscrow

`/blockchain.treasury.v1.MsgResolveEscrow`

Signed by the `moderator` field.

MsgResolveEscrow is the moderator deciding an open case.

Their only power, and only on a lock somebody actually disputed. A moderator who could act on a quiet lock would be a custodian with a different name.

| Field | Type | Description |
| --- | --- | --- |
| `moderator` | string |  |
| `lock_id` | uint64 |  |
| `pay_beneficiary` | bool | true pays the beneficiary, false returns the money to the depositor. There is no third option: a case that stays open forever is the failure this whole mechanism exists to prevent. |

### MsgRevokeLock

`/blockchain.treasury.v1.MsgRevokeLock`

Signed by the `admin` field.

RevokeLock cancels a revocable lock, returning the unreleased portion.

| Field | Type | Description |
| --- | --- | --- |
| `admin` | string |  |
| `lock_id` | uint64 |  |

### MsgRevokeRole

`/blockchain.treasury.v1.MsgRevokeRole`

Signed by the `admin` field.

RevokeRole removes an address's role.

| Field | Type | Description |
| --- | --- | --- |
| `admin` | string |  |
| `treasury_id` | uint64 |  |
| `address` | string |  |

### MsgSetAdmin

`/blockchain.treasury.v1.MsgSetAdmin`

Signed by the `admin` field.

SetAdmin transfers administrative control of a treasury.

| Field | Type | Description |
| --- | --- | --- |
| `admin` | string |  |
| `treasury_id` | uint64 |  |
| `new_admin` | string |  |

### MsgSetPaused

`/blockchain.treasury.v1.MsgSetPaused`

Signed by the `sender` field.

SetPaused freezes or unfreezes a treasury.

| Field | Type | Description |
| --- | --- | --- |
| `sender` | string |  |
| `treasury_id` | uint64 |  |
| `paused` | bool |  |

### MsgSetSpendPolicy

`/blockchain.treasury.v1.MsgSetSpendPolicy`

Signed by the `admin` field.

SetSpendPolicy sets the spending constraints for one denom.

| Field | Type | Description |
| --- | --- | --- |
| `admin` | string |  |
| `policy` | SpendPolicy |  |

### MsgSpend

`/blockchain.treasury.v1.MsgSpend`

Signed by the `spender` field.

Spend moves funds out of a treasury to a recipient.

| Field | Type | Description |
| --- | --- | --- |
| `spender` | string |  |
| `treasury_id` | uint64 |  |
| `recipient` | string |  |
| `amount` | repeated Coin |  |
| `memo` | string | memo records why the payment was made, for the audit trail. |
| `amount_commitment` | bytes | 6-8 are reserved for confidentiality, unpopulated and unverified. See docs/scope/confidentiality.md and MsgSendPayment, which these mirror. A treasury spend is public by default and that is deliberate: for donor disbursement, subsidy and public payroll the auditability is the product. The numbers are taken anyway, because the exception the design already anticipates — commercial escrow — is a spend like any other, and by the time it is wanted the treasuries holding real money will have written spends that cannot be re-encoded. memo is the field with a problem today rather than later: it is free text on a value transfer, which is where a payroll run acquires an employee's name, and it goes to a ledger with no erasure path. metadata_hash is where that moves. |
| `amount_range_proof` | bytes |  |
| `metadata_hash` | bytes |  |

### MsgUpdateParams

`/blockchain.treasury.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

## Queries

### ClaimableAmount

`GET /yamale/blockchain/treasury/v1/lock/{lock_id}/claimable`

ClaimableAmount reports what a lock would release if claimed right now.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `lock_id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `claimable` | string | claimable is releasable right now; vested is the cumulative amount the schedule has unlocked so far, including what was already claimed. |
| `vested` | string |  |
| `remaining` | string |  |

### GetLock

`GET /yamale/blockchain/treasury/v1/lock/{id}`

GetLock queries one lock by id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `lock` | Lock |  |

### GetSpendPolicy

`GET /yamale/blockchain/treasury/v1/treasury/{treasury_id}/policy`

GetSpendPolicy queries the spending policy for one treasury and denom.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `treasury_id` | uint64 |  |
| `denom` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `policy` | SpendPolicy |  |

### GetTreasury

`GET /yamale/blockchain/treasury/v1/treasury/{id}`

GetTreasury queries one treasury by id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `treasury` | Treasury |  |

### ListLock

`GET /yamale/blockchain/treasury/v1/lock`

ListLock queries all locks.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `lock` | repeated Lock |  |
| `pagination` | PageResponse |  |

### ListRole

`GET /yamale/blockchain/treasury/v1/treasury/{treasury_id}/roles`

ListRole queries the role assignments of one treasury.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `treasury_id` | uint64 |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `role` | repeated RoleAssignment |  |
| `pagination` | PageResponse |  |

### ListTreasury

`GET /yamale/blockchain/treasury/v1/treasury`

ListTreasury queries all treasuries.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `treasury` | repeated Treasury |  |
| `pagination` | PageResponse |  |

### LocksByBeneficiary

`GET /yamale/blockchain/treasury/v1/beneficiary/{beneficiary}/locks`

LocksByBeneficiary queries the locks payable to one address, so a beneficiary can find what is owed to them without knowing any treasury id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `beneficiary` | string |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `lock` | repeated Lock |  |
| `pagination` | PageResponse |  |

### LocksByTreasury

`GET /yamale/blockchain/treasury/v1/treasury/{treasury_id}/locks`

LocksByTreasury queries the locks held by one treasury.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `treasury_id` | uint64 |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `lock` | repeated Lock |  |
| `pagination` | PageResponse |  |

### Params

`GET /yamale/blockchain/treasury/v1/params`

Params queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

### SpendCapacity

`GET /yamale/blockchain/treasury/v1/treasury/{treasury_id}/capacity`

SpendCapacity reports how much may still be spent in the current period.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `treasury_id` | uint64 |  |
| `denom` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `remaining_this_period` | string | remaining_this_period is what the period limit still allows; available is what the treasury actually holds unlocked. A spend is bounded by whichever is smaller, which is why both are reported. |
| `available` | string |  |
| `per_transaction_limit` | string |  |
| `period_resets_at` | int64 |  |

### TreasuryBalances

`GET /yamale/blockchain/treasury/v1/treasury/{treasury_id}/balances`

TreasuryBalances reports total, locked and available amounts per denom.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `treasury_id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `balances` | repeated DenomBalance |  |

## State

### DenomBalance

DenomBalance is one denom's position in a treasury.

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `total` | string |  |
| `locked` | string |  |
| `available` | string | available is total minus locked: what may actually be spent. |

### Lock

Lock commits treasury funds to a beneficiary on a schedule.

Creating a lock does not transfer anything; it moves funds from the treasury's available balance into its locked balance. The beneficiary pulls what has vested by claiming, so the treasury never needs to run a scheduler and a beneficiary who never claims costs the chain nothing.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `treasury_id` | uint64 |  |
| `beneficiary` | string | beneficiary is the only address that may claim from this lock. |
| `denom` | string |  |
| `total_amount` | string | total_amount is the full commitment; released_amount is how much of it the beneficiary has already claimed. The difference is what remains locked. |
| `released_amount` | string |  |
| `start_time` | int64 | start_time is when the schedule begins, in Unix seconds. |
| `cliff_time` | int64 | cliff_time is the earliest moment anything may be claimed. Before it, a vesting lock releases nothing at all — that is the whole point of a cliff. Set equal to start_time for no cliff. |
| `end_time` | int64 | end_time is when the lock is fully released. |
| `release_intervals` | uint64 | release_intervals splits vesting into discrete tranches rather than a continuous drip: 4 over a year means quarterly. Zero or one means continuous, releasing proportionally to elapsed time. |
| `lock_type` | LockType |  |
| `revocable` | bool | revocable lets the admin cancel the lock and return the *unreleased* portion to available. Already-vested funds are never clawed back, whether or not the beneficiary has claimed them — a promise that can be withdrawn retroactively is not a promise. |
| `active` | bool | active is false once the lock is fully claimed or revoked. Inactive locks are retained rather than deleted so the disbursement history stays auditable. |
| `created_at_height` | uint64 |  |
| `depositor` | string | depositor funded the lock and is the only account that may release it. Held separately from the treasury's admin on purpose: in an escrow the buyer is not an administrator of anything, and an admin who could release somebody else's escrow would be the trusted party this design exists to remove. |
| `moderator` | string | moderator decides a disputed lock, and is named when the lock is created rather than chosen afterwards. Both parties see who will judge them before they commit money, which is the whole of what makes this honest. A moderator has exactly one power: deciding an *open* case. They cannot release, refund, or touch a lock nobody has disputed. |
| `dispute` | DisputeState |  |
| `dispute_reason` | string | Why the case was opened, kept because a moderator judging between two strangers has nothing else to go on. |
| `dispute_opened_by` | string | Which side escalated. "The seller cannot get confirmed" and "the buyer says it never came" are different disputes with the same shape. |

### RoleAssignment

RoleAssignment grants an address a role over one treasury.

| Field | Type | Description |
| --- | --- | --- |
| `treasury_id` | uint64 |  |
| `address` | string |  |
| `role` | Role |  |

### SpendPolicy

SpendPolicy constrains what a ROLE_SPENDER may move out of a treasury in one denom, without needing an admin decision per payment.

The policy is what makes a spender role safe to hand out: it bounds the blast radius of a compromised operational key to one period's limit, rather than the whole treasury.

| Field | Type | Description |
| --- | --- | --- |
| `treasury_id` | uint64 |  |
| `denom` | string |  |
| `per_transaction_limit` | string | per_transaction_limit caps a single spend. Empty means no cap. |
| `period_limit` | string | period_limit caps the total spent within one window, and period_seconds is the window length. Empty period_limit means no cap. The window is fixed, not rolling: it resets when period_seconds elapses since the window opened, which keeps the accounting to a single stored value per denom. |
| `period_seconds` | uint64 |  |
| `allowlist` | repeated string | allowlist restricts destinations. Empty means any destination is allowed; non-empty means only these are. |
| `blocklist` | repeated string | blocklist forbids destinations outright. It is checked after the allowlist, so an address in both is denied — the safer reading of a contradiction. |

### SpendWindow

SpendWindow tracks consumption of a SpendPolicy's period_limit.

| Field | Type | Description |
| --- | --- | --- |
| `treasury_id` | uint64 |  |
| `denom` | string |  |
| `spent` | string | spent is the amount already used in the current window. |
| `window_start` | int64 | window_start is when the current window opened, in Unix seconds. |

### Treasury

Treasury is a pool of funds under programmable control.

Funds deposited into a treasury are held by the treasury module account, not by the admin's own address. That indirection is what makes the locked/available split enforceable: because no ordinary bank transfer can reach the balance, the only way out is a treasury message, and every treasury message checks the available balance first.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `name` | string | name is a human label, for display only. |
| `admin` | string | admin manages roles, policies and locks. Point this at an x/group policy address to get M-of-N control with a full on-chain approval trail; a plain account address gives single-key control. |
| `paused` | bool | paused halts spending and claiming without unwinding any state, so a compromise can be contained while the admins decide what to do. |
| `created_at_height` | uint64 |  |

### TreasuryBalance

TreasuryBalance is the module's ledger of what one treasury holds in one denom, and how much of it is already committed to locks.

available = total - locked, and it is `available` that every outbound path checks. Once funds back a lock they cannot be spent by anyone, including the admin and including a proposal that clears its signing threshold.

| Field | Type | Description |
| --- | --- | --- |
| `treasury_id` | uint64 |  |
| `denom` | string |  |
| `total` | string |  |
| `locked` | string |  |

## Value types

### DisputeState

DisputeState tracks a conditional lock that somebody has escalated.

| Value | Meaning |
| --- | --- |
| `DISPUTE_STATE_NONE` |  |
| `DISPUTE_STATE_OPEN` | Frozen. Neither release nor refund happens until the moderator decides. |
| `DISPUTE_STATE_RESOLVED` |  |

### LockType

LockType selects how a lock releases its funds over time.

| Value | Meaning |
| --- | --- |
| `LOCK_TYPE_UNSPECIFIED` | LOCK_TYPE_UNSPECIFIED is the unset default and is never valid. |
| `LOCK_TYPE_TIME` | LOCK_TYPE_TIME releases the whole amount at once, when end_time passes. Use it for a simple escrow or a delayed disbursement. |
| `LOCK_TYPE_VESTING` | LOCK_TYPE_VESTING releases progressively between cliff_time and end_time. Use it for grants, payroll and token allocations. |
| `LOCK_TYPE_CONDITIONAL` | LOCK_TYPE_CONDITIONAL releases when the depositor says so, not when a clock says so. This is escrow: a buyer commits the money, the seller ships knowing it exists, and the funds move on confirmation. Deliberately no deadline. An automatic release rewards precisely the seller who ships nothing and waits, and it lands hardest on buyers who are ill, travelling or offline. Instead either party may open a case, which is why silence is not a weapon: the seller escalates rather than waiting forever. |

### Role

Role is what an address may do to a treasury.

Proposing, voting and executing are deliberately absent: on this chain that machinery belongs to x/group, which already records every approval and rejection as an attributable on-chain event. Setting a treasury's admin to a group policy composes the two — the group decides, the treasury enforces.

| Value | Meaning |
| --- | --- |
| `ROLE_UNSPECIFIED` | ROLE_UNSPECIFIED is the unset default and grants nothing. |
| `ROLE_ADMIN` | ROLE_ADMIN may manage roles and policies, create locks, and revoke revocable locks. Equivalent to the treasury's admin address. |
| `ROLE_SPENDER` | ROLE_SPENDER may move funds out directly, without an admin decision, so long as the spend policy allows it. This is the role that makes routine operational payments possible without a governance round trip. |
| `ROLE_PAUSER` | ROLE_PAUSER may pause and unpause the treasury. Held separately from admin so an emergency responder can freeze funds without also being able to move them. |

## Parameters

Changed by governance through `MsgUpdateParams`. Defaults are the values a chain starts with at genesis.

| Parameter | Default | Description |
| --- | --- | --- |
| `max_locks_per_treasury` | `500` | max_locks_per_treasury bounds how many active locks one treasury may hold. Releasing funds walks a treasury's locks, so an unbounded count would let anyone make that walk arbitrarily expensive. |
| `max_role_assignments_per_treasury` | `100` | max_role_assignments_per_treasury bounds the size of a treasury's access control list, for the same reason. |
| `min_lock_seconds` | `60` | min_lock_seconds is the shortest duration a lock may run for. A lock that ends the moment it is created commits nothing and only wastes state. |
| `max_spend_policy_addresses` | `200` | max_spend_policy_addresses bounds the combined size of a spend policy's allowlist and blocklist. Both are scanned on every spend and stored indefinitely, so leaving them unbounded would let a treasury make its own payments arbitrarily expensive to validate and bloat state while doing it. |
| `fee_operating_treasury_id` | `0` | fee_operating_treasury_id names the treasury that network fees are paid into on a deployment that has no native token. The destination is a treasury rather than an address because fees are operating income of the deployment, and income that lands in somebody's account is spendable by whoever holds that key. A treasury puts it behind the same roles, spend policies and M-of-N group control as every other committed balance, and leaves the audit trail those produce. Nothing reads this unless route_fees_to_operating_treasury is set. |
| `route_fees_to_operating_treasury` | `false` | route_fees_to_operating_treasury turns fee routing on. It is a separate field rather than a zero sentinel on the id above, because in proto3 a uint64 of 0 is indistinguishable from a field nobody set, and whether zero names a treasury is x/treasury's numbering convention rather than this parameter's. Ids start at one today, so zero would work; if that ever changed, every chain that had not configured routing would start paying its fees into whichever treasury was created first, and nothing here would have changed to say so. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrTreasuryNotFound` | treasury not found |
| 1102 | `ErrUnauthorized` | signer is not authorized to perform this action on this treasury |
| 1103 | `ErrInvalidAmount` | invalid coin amount |
| 1104 | `ErrInsufficientFunds` | treasury does not have enough available balance |
| 1105 | `ErrLockNotFound` | lock not found |
| 1106 | `ErrInvalidSchedule` | invalid lock schedule |
| 1107 | `ErrLockInactive` | lock is no longer active |
| 1108 | `ErrNotRevocable` | lock is not revocable |
| 1109 | `ErrNothingToClaim` | nothing has vested yet |
| 1110 | `ErrTreasuryPaused` | treasury is paused |
| 1111 | `ErrSpendLimit` | spend exceeds the policy limit |
| 1112 | `ErrDestinationDenied` | destination is not permitted by the spend policy |
| 1113 | `ErrLimitReached` | treasury has reached a configured maximum |
| 1114 | `ErrInvalidRole` | invalid role |
| 1115 | `ErrSelfEscrow` | an escrow cannot pay its own depositor |
| 1116 | `ErrModeratorIsParty` | the moderator cannot be the buyer or the seller |
| 1117 | `ErrNotEscrow` | that lock is not an escrow |
| 1118 | `ErrNotDepositor` | only the depositor may release an escrow |
| 1119 | `ErrNotParty` | only the buyer or the seller may open a case |
| 1120 | `ErrNotModerator` | only the named moderator may decide this case |
| 1121 | `ErrEscrowDisputed` | this escrow is under review and cannot be released directly |
| 1122 | `ErrAlreadyDisputed` | a case is already open on this escrow |
| 1123 | `ErrNoOpenCase` | there is no open case on this escrow |
| 1124 | `ErrNoReason` | a case must say what happened |
| 1125 | `ErrLockClosed` | that lock is already settled |
