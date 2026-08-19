package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/paymsg module sentinel errors
var (
	ErrInvalidSigner          = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrApplicationExists      = errors.Register(ModuleName, 1101, "a participant application already exists for this address")
	ErrApplicationNotFound    = errors.Register(ModuleName, 1102, "participant application not found")
	ErrApplicationNotPending  = errors.Register(ModuleName, 1103, "participant application is not pending")
	ErrNotApprovedParticipant = errors.Register(ModuleName, 1104, "address is not an approved participant")
	ErrPaymentExists          = errors.Register(ModuleName, 1105, "a payment with this end-to-end id already exists")
	ErrInvalidAmount          = errors.Register(ModuleName, 1106, "invalid coin amount")
	ErrNotACustomer           = errors.Register(ModuleName, 1107, "the debtor does not bank with the instructing participant")
	ErrInvalidPaymentField    = errors.Register(ModuleName, 1108, "payment field is missing or outside its ISO 20022 limit")

	ErrInvalidSettlementJurisdiction = errors.Register(ModuleName, 1109, "settlement jurisdiction is missing or is not an ISO 3166-1 alpha-2 code")
	ErrInvalidMetadata               = errors.Register(ModuleName, 1110, "payment metadata payload or its hash is malformed")
	ErrConfidentialAmountUnavailable = errors.Register(ModuleName, 1111, "confidential amounts are reserved but not yet verified by this chain")
)
