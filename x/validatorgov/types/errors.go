package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/validatorgov module sentinel errors
var (
	ErrInvalidSigner         = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrApplicationNotFound   = errors.Register(ModuleName, 1101, "validator application not found")
	ErrApplicationNotPending = errors.Register(ModuleName, 1102, "validator application is not pending")

	ErrNotApprovedValidator   = errors.Register(ModuleName, 1103, "address is not an approved validator operator")
	ErrRotationInProgress     = errors.Register(ModuleName, 1104, "a rotation is already open against this operator")
	ErrRotationNotFound       = errors.Register(ModuleName, 1105, "operator rotation not found")
	ErrRotationNotPending     = errors.Register(ModuleName, 1106, "operator rotation is no longer pending")
	ErrRotationNotRecovery    = errors.Register(ModuleName, 1107, "operator rotation is not a recovery")
	ErrRecoveryAlreadyDecided = errors.Register(ModuleName, 1108, "operator recovery has already been decided")
	ErrOperatorUnchanged      = errors.Register(ModuleName, 1109, "the new operator is the current operator")
	ErrOperatorInUse          = errors.Register(ModuleName, 1110, "the new operator address is already an approved validator operator")
	ErrNotCurrentOperator     = errors.Register(ModuleName, 1111, "signer is not the current operator")
	ErrMissingReason          = errors.Register(ModuleName, 1112, "a recovery must state its grounds")
)
