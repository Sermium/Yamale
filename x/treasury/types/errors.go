package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/treasury module sentinel errors
var (
	ErrInvalidSigner     = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrTreasuryNotFound  = errors.Register(ModuleName, 1101, "treasury not found")
	ErrUnauthorized      = errors.Register(ModuleName, 1102, "signer is not authorized to perform this action on this treasury")
	ErrInvalidAmount     = errors.Register(ModuleName, 1103, "invalid coin amount")
	ErrInsufficientFunds = errors.Register(ModuleName, 1104, "treasury does not have enough available balance")
	ErrLockNotFound      = errors.Register(ModuleName, 1105, "lock not found")
	ErrInvalidSchedule   = errors.Register(ModuleName, 1106, "invalid lock schedule")
	ErrLockInactive      = errors.Register(ModuleName, 1107, "lock is no longer active")
	ErrNotRevocable      = errors.Register(ModuleName, 1108, "lock is not revocable")
	ErrNothingToClaim    = errors.Register(ModuleName, 1109, "nothing has vested yet")
	ErrTreasuryPaused    = errors.Register(ModuleName, 1110, "treasury is paused")
	ErrSpendLimit        = errors.Register(ModuleName, 1111, "spend exceeds the policy limit")
	ErrDestinationDenied = errors.Register(ModuleName, 1112, "destination is not permitted by the spend policy")
	ErrLimitReached      = errors.Register(ModuleName, 1113, "treasury has reached a configured maximum")
	ErrInvalidRole       = errors.Register(ModuleName, 1114, "invalid role")
)

// Conditional locks (escrow). Numbered on from the existing treasury errors.
var (
	ErrSelfEscrow = errors.Register(ModuleName, 1115,
		"an escrow cannot pay its own depositor")
	// A moderator who is one of the parties is not a moderator.
	ErrModeratorIsParty = errors.Register(ModuleName, 1116,
		"the moderator cannot be the buyer or the seller")
	ErrNotEscrow = errors.Register(ModuleName, 1117,
		"that lock is not an escrow")
	ErrNotDepositor = errors.Register(ModuleName, 1118,
		"only the depositor may release an escrow")
	ErrNotParty = errors.Register(ModuleName, 1119,
		"only the buyer or the seller may open a case")
	ErrNotModerator = errors.Register(ModuleName, 1120,
		"only the named moderator may decide this case")
	ErrEscrowDisputed = errors.Register(ModuleName, 1121,
		"this escrow is under review and cannot be released directly")
	ErrAlreadyDisputed = errors.Register(ModuleName, 1122,
		"a case is already open on this escrow")
	ErrNoOpenCase = errors.Register(ModuleName, 1123,
		"there is no open case on this escrow")
	ErrNoReason = errors.Register(ModuleName, 1124,
		"a case must say what happened")
	ErrLockClosed = errors.Register(ModuleName, 1125,
		"that lock is already settled")
)
