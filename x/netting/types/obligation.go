package types

import (
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BatchHashLength is SHA-256's output size, and the only length a batch hash
// may be.
//
// Enforced rather than merely documented because a hash of the wrong length is
// not a hash of anything — it is a field somebody filled in to get past a
// check, and the whole audit story of a netted system rests on being able to
// go from a chain figure back to the items it summarises.
const BatchHashLength = 32

// ValidateObligationFields checks everything about an obligation that can be
// decided without reading state.
//
// Separated out so it runs first, before any store read: refusing a malformed
// message should cost a few comparisons rather than a walk through the
// participant registry.
func ValidateObligationFields(from, to, denom string, amount math.Int, batchHash []byte) error {
	if from == to {
		// Not merely useless. A self-obligation adds and subtracts the same
		// figure from one position, so it changes nothing while inflating the
		// gross total the cycle reports — which is the number the compression
		// claim is computed from. Free, unlimited, and it makes the system look
		// better than it is.
		return errorsmod.Wrapf(ErrSelfObligation, "%s", from)
	}
	if err := sdk.ValidateDenom(denom); err != nil {
		return errorsmod.Wrap(ErrInvalidAmount, err.Error())
	}
	if amount.IsNil() || !amount.IsPositive() {
		return errorsmod.Wrapf(ErrInvalidAmount, "amount must be positive, got %s", amount)
	}
	if len(batchHash) != BatchHashLength {
		return errorsmod.Wrapf(ErrInvalidBatchHash, "got %d bytes", len(batchHash))
	}
	return nil
}

// CompressionBps is how much of a cycle's gross value netting removed, in basis
// points: 10000 means nothing had to be funded at all, 0 that netting achieved
// nothing.
//
// The divisor is guarded here rather than at the two call sites, because a
// cycle with no obligations in a currency is an ordinary state — it happens on
// the first block after every close — and a ratio with a zero denominator is
// not a small number, it is a panic. Reported as zero compression, which is the
// honest answer for a window that compressed nothing because it carried
// nothing.
func CompressionBps(gross, net math.Int) uint64 {
	if gross.IsNil() || net.IsNil() || !gross.IsPositive() {
		return 0
	}
	if net.IsNegative() {
		return 0
	}
	if net.GTE(gross) {
		return 0
	}
	removed := gross.Sub(net).Mul(math.NewInt(10_000)).Quo(gross)
	return removed.Uint64()
}
