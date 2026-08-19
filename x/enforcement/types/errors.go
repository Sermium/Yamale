package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/enforcement module sentinel errors
var (
	ErrInvalidSigner    = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrUnknownValidator = errors.Register(ModuleName, 1101, "not a bonded validator")
	ErrCaseNotFound     = errors.Register(ModuleName, 1102, "no such case")
	ErrCaseClosed       = errors.Register(ModuleName, 1103, "case is no longer open")
	ErrAlreadyVoted     = errors.Register(ModuleName, 1104, "this validator has already voted on this case")
	ErrAlreadyFrozen    = errors.Register(ModuleName, 1105, "an open case already freezes this address")
	ErrNotTheOpener     = errors.Register(ModuleName, 1106, "only the validator that opened the case may withdraw it")
	ErrFrozen           = errors.Register(ModuleName, 1107, "account is frozen by an enforcement case")
	ErrEvidenceRequired = errors.Register(ModuleName, 1108, "a seizure case requires evidence")
	ErrInvalidCase      = errors.Register(ModuleName, 1109, "case is not valid")
	ErrNotSeizure       = errors.Register(ModuleName, 1110, "case did not order a seizure")
	ErrNotPassed        = errors.Register(ModuleName, 1111, "case has not passed")
	ErrProtectedAddress = errors.Register(ModuleName, 1112, "address cannot be frozen or seized")
	ErrLimitReached     = errors.Register(ModuleName, 1113, "exceeds a configured maximum")
	// ErrNoEmergencyAuthority is deliberately distinct from ErrInvalidSigner:
	// "the founders' group did not sign this" and "this chain has no founders'
	// group" are different facts, and telling somebody the first when the second
	// is true sends them looking for a key that does not exist.
	ErrNoEmergencyAuthority = errors.Register(ModuleName, 1114, "no emergency authority is configured")
)
