package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/oracle module sentinel errors
var (
	ErrInvalidSigner      = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrUnknownValidator   = errors.Register(ModuleName, 1101, "not a bonded validator")
	ErrNotTheFeeder       = errors.Register(ModuleName, 1102, "signer is not the feeder for this validator")
	ErrDenomNotAccepted   = errors.Register(ModuleName, 1103, "denom is not in the accepted set")
	ErrInvalidRate        = errors.Register(ModuleName, 1104, "exchange rate must be a positive decimal")
	ErrNotAnAppraiser     = errors.Register(ModuleName, 1105, "address is not an approved appraiser")
	ErrOutOfScope         = errors.Register(ModuleName, 1106, "appraiser is not approved for this asset class")
	ErrAssetNotFound      = errors.Register(ModuleName, 1107, "no such tokenised asset")
	ErrApplicationExists  = errors.Register(ModuleName, 1108, "an appraiser application already exists for this address")
	ErrApplicationMissing = errors.Register(ModuleName, 1109, "appraiser application not found")
	ErrNotPending         = errors.Register(ModuleName, 1110, "appraiser application is not pending")
	ErrInvalidValuation   = errors.Register(ModuleName, 1111, "invalid valuation amount")
	ErrValuationInFuture  = errors.Register(ModuleName, 1112, "valuation date is in the future")
	ErrRateUnavailable    = errors.Register(ModuleName, 1113, "no usable exchange rate for this denom")
	ErrAppraisalMissing   = errors.Register(ModuleName, 1114, "no appraisal for this asset")
	ErrStale              = errors.Register(ModuleName, 1115, "value is too old to be relied on")
	ErrLimitReached       = errors.Register(ModuleName, 1116, "exceeds a configured maximum")
)
