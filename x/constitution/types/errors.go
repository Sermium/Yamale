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

	// ErrConstitutionalInvariant is what the ante gate returns when a
	// transaction would put the chain in a state the constitution forbids —
	// as opposed to ErrInvariantViolation, which is a proposal trying to change
	// the constitution itself. The two are separate codes because they need
	// different answers: one is "you cannot change this rule", the other is
	// "this action breaks a rule you did not try to change".
	ErrConstitutionalInvariant = errors.Register(ModuleName, 1107, "this would break a value the constitution fixes")
)
