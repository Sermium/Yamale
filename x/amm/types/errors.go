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
	// A pool with zero reserves and zero shares is not empty, it is broken: the
	// next JoinPool divides by a zero reserve, and every swap against it
	// returns nothing. The id persists as a permanently unusable record.
	ErrWouldEmptyPool = errors.Register(ModuleName, 1111,
		"a pool cannot be exited completely; leave at least one share behind or nothing can ever join it again")
	// A swap that takes payment and returns nothing is settled, not refused,
	// unless somebody says otherwise. min_amount_out of zero used to be the
	// caller saying they did not mind.
	ErrZeroOutput = errors.Register(ModuleName, 1112,
		"that swap would return nothing at all, so it is refused rather than settled")
	// The module writes these strings itself, so a failure here means state has
	// been corrupted or a migration is wrong — and math.Int carries a nil inner
	// value on a failed parse, which panics on first use rather than erroring.
	ErrCorruptPool = errors.Register(ModuleName, 1113,
		"this pool's stored reserves cannot be read as numbers")
	// A fraction denom whose transfers can be halted cannot be pooled: a pool
	// pays both legs in one send, so a halt on one leg locks the other. This is
	// the one exception to the AMM being permissionless.
	ErrRestrictedDenom = errors.Register(ModuleName, 1114,
		"that denomination carries a transfer restriction a pool cannot survive")
)
