package types

import (
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// NewParams creates a new Params instance.
func NewParams() Params {
	return Params{
		DefaultMintCeiling: DefaultMintCeiling,
		MintCeilings:       nil,
	}
}

// DefaultMintCeiling is the supply cap a currency gets when nothing else says
// otherwise.
//
// A figure rather than "unlimited", because unlimited is what the audit found:
// MintCoin checked the signer against the recorded issuer and then minted
// whatever it was asked for, with no cap, no per-period limit and no reserve
// check anywhere in the path. One trillion of the smallest unit is a hundred
// billion display units at a six-decimal exponent — generous for any currency
// this chain represents, and finite, which is the whole point.
var DefaultMintCeiling = math.NewInt(1_000_000_000_000_000)

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	return NewParams()
}

// Validate validates the set of params.
func (p Params) Validate() error {
	// An unset ceiling is not an invalid one. It reads as zero, and zero means
	// no minting — which is where a chain upgraded past this point lands, and
	// where it should land, until governance states a figure. Refusing the
	// params outright instead would make every genesis written before this
	// field existed unloadable, which is a far worse failure than a currency
	// that cannot be issued until somebody decides how much of it may exist.
	if !p.DefaultMintCeiling.IsNil() && p.DefaultMintCeiling.IsNegative() {
		return errorsmod.Wrap(ErrInvalidParams, "default_mint_ceiling cannot be negative")
	}
	seen := make(map[string]struct{}, len(p.MintCeilings))
	for _, c := range p.MintCeilings {
		if err := sdk.ValidateDenom(c.Denom); err != nil {
			return errorsmod.Wrapf(ErrInvalidParams, "mint ceiling for %q: %s", c.Denom, err)
		}
		if _, dup := seen[c.Denom]; dup {
			// Two ceilings for one currency is not a configuration, it is a
			// question about which one binds — and the answer would depend on
			// iteration order.
			return errorsmod.Wrapf(ErrInvalidParams, "two mint ceilings for %s", c.Denom)
		}
		seen[c.Denom] = struct{}{}
		if !c.Ceiling.IsNil() && c.Ceiling.IsNegative() {
			return errorsmod.Wrapf(ErrInvalidParams, "the mint ceiling for %s cannot be negative", c.Denom)
		}
	}
	return nil
}

// MintCeilingFor is the largest total supply of a denomination that may exist.
//
// A currency with its own entry uses that; everything else uses the default.
// Both may be zero, and zero means the currency may not be minted — which is
// how a currency is suspended without revoking its issuer, and what an
// upgraded chain sees until governance states a figure.
func (p Params) MintCeilingFor(denom string) math.Int {
	for _, c := range p.MintCeilings {
		if c.Denom == denom {
			if c.Ceiling.IsNil() {
				return math.ZeroInt()
			}
			return c.Ceiling
		}
	}
	if p.DefaultMintCeiling.IsNil() {
		return math.ZeroInt()
	}
	return p.DefaultMintCeiling
}
