package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/constitution module sentinel errors
var (
	ErrInvalidSigner = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")

	// ErrInvariantViolation is what another module returns when a governance
	// proposal tries to change a value this one holds. It is registered here,
	// not there, so that every module refusing on constitutional grounds
	// refuses with the same error and a client can recognise it.
	ErrInvariantViolation = errors.Register(ModuleName, 1101, "this value is fixed at genesis and cannot be changed by a parameter update")

	ErrAmendmentNotFound = errors.Register(ModuleName, 1102, "no such amendment")
	ErrAmendmentClosed   = errors.Register(ModuleName, 1103, "amendment is no longer pending")
	ErrAlreadyRatified   = errors.Register(ModuleName, 1104, "this validator has already ratified this amendment")
	ErrUnknownValidator  = errors.Register(ModuleName, 1105, "not a bonded validator")
	ErrNoInvariants      = errors.Register(ModuleName, 1106, "this chain has no constitution")
)
