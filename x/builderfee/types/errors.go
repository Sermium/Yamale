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
	// msg_type_url is a store key chosen by whoever signs a permissionless
	// message, and x/stablecoin bounds its own text fields carefully in
	// types/limits.go while this module bounded nothing at all.
	ErrInvalidMsgTypeURL = errors.Register(ModuleName, 1104, "that is not a message type URL this chain could route")
)
