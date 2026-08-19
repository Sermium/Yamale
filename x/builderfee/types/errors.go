package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/builderfee module sentinel errors
var (
	ErrInvalidSigner         = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrBuilderExists         = errors.Register(ModuleName, 1101, "a builder application already exists for this message type")
	ErrApplicationNotFound   = errors.Register(ModuleName, 1102, "builder application not found")
	ErrApplicationNotPending = errors.Register(ModuleName, 1103, "builder application is not pending")
)
