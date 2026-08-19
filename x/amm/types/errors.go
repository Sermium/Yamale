package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/amm module sentinel errors
var (
	ErrInvalidSigner       = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrSameDenom           = errors.Register(ModuleName, 1101, "pool denoms must be different")
	ErrInvalidAmount       = errors.Register(ModuleName, 1102, "invalid coin amount")
	ErrInvalidSwapFee      = errors.Register(ModuleName, 1109, "swap fee is outside the permitted range")
	ErrInvalidDenom        = errors.Register(ModuleName, 1110, "invalid denom")
	ErrPoolNotFound        = errors.Register(ModuleName, 1103, "pool not found")
	ErrDenomNotInPool      = errors.Register(ModuleName, 1104, "denom is not part of this pool")
	ErrInsufficientDeposit = errors.Register(ModuleName, 1105, "deposit does not cover the required proportional amount")
	ErrInsufficientShares  = errors.Register(ModuleName, 1106, "insufficient LP shares")
	ErrSlippage            = errors.Register(ModuleName, 1107, "swap output is below the minimum requested amount")
)
