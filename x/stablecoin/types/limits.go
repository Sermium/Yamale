package types

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Limits on a currency registration.
//
// Registering is permissionless and the denom becomes a store key, so every
// field here is attacker-chosen text the chain keeps forever, priced at one
// transaction fee. None of these bounds constrain a real currency: a ticker is
// a handful of characters and a description is a sentence.
const (
	MaxDisplayDenomLength = 32
	MaxNameLength         = 64
	MaxSymbolLength       = 16
	MaxDescriptionLength  = 256

	// MaxExponent is well above any real currency's smallest unit — the native
	// token uses 6 — while staying inside what a display scale can represent.
	MaxExponent = 18
)

// ValidateCurrency checks a registration before it becomes permanent state.
//
// The denom check is the one that matters most: it is the store key, and it
// later reaches sdk.NewCoin, which panics rather than erroring on a denom the
// coin type cannot represent. An approved currency that panics on every mint
// would be permanently unusable with no visible cause.
func ValidateCurrency(denom, displayDenom, name, symbol, description string, exponent uint64) error {
	if err := sdk.ValidateDenom(denom); err != nil {
		return errorsmod.Wrapf(ErrInvalidCurrency, "invalid denom %q: %s", denom, err)
	}
	if exponent > MaxExponent {
		return errorsmod.Wrapf(ErrInvalidCurrency, "exponent must be at most %d, got %d", MaxExponent, exponent)
	}

	for _, field := range []struct {
		name     string
		value    string
		max      int
		required bool
	}{
		{"display_denom", displayDenom, MaxDisplayDenomLength, true},
		{"name", name, MaxNameLength, true},
		{"symbol", symbol, MaxSymbolLength, true},
		// A description helps governance decide but is not needed to render an
		// amount, so it is bounded without being required.
		{"description", description, MaxDescriptionLength, false},
	} {
		if field.required && field.value == "" {
			return errorsmod.Wrapf(ErrInvalidCurrency, "%s must be set", field.name)
		}
		if len(field.value) > field.max {
			return errorsmod.Wrapf(ErrInvalidCurrency,
				"%s must be at most %d characters, got %d", field.name, field.max, len(field.value))
		}
	}
	return nil
}
