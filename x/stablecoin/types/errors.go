package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/stablecoin module sentinel errors
var (
	ErrInvalidSigner         = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrCurrencyExists        = errors.Register(ModuleName, 1101, "a currency is already registered or pending for this denom")
	ErrApplicationNotFound   = errors.Register(ModuleName, 1102, "issuer application not found")
	ErrApplicationNotPending = errors.Register(ModuleName, 1103, "issuer application is not pending")
	ErrNotApprovedIssuer     = errors.Register(ModuleName, 1104, "sender is not the approved issuer for this denom")
	ErrInvalidAmount         = errors.Register(ModuleName, 1105, "invalid coin amount")
	ErrInvalidCurrency       = errors.Register(ModuleName, 1110, "currency registration field is invalid or outside its limit")
	ErrInvalidParams         = errors.Register(ModuleName, 1112, "these parameters are not a configuration this module can act on")
	// The ceiling is on total supply, so this refuses the mint that would
	// cross it rather than the issuer who asked. A ceiling of zero means the
	// currency is suspended, which is also what an upgraded chain sees until
	// governance states a figure.
	ErrMintCeiling = errors.Register(ModuleName, 1113, "that mint would take the currency past its supply ceiling")
	// Raised only by the ante decorator on a chain with no native token, where a
	// fee in an unissued denom costs the sender nothing real.
	ErrFeeDenomNotIssued = errors.Register(ModuleName, 1111, "fee denomination has no approved issuer")
)
