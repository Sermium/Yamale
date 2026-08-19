package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/emission module sentinel errors
var (
	ErrInvalidSigner = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrInvalidState  = errors.Register(ModuleName, 1101, "emission state contains an invalid provisions amount")
)
