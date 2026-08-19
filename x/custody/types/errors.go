package types

import sdkerrors "cosmossdk.io/errors"

// Errors are numbered from 2; code 1 is reserved for the SDK's internal error.
var (
	ErrUnknownAsset = sdkerrors.Register(ModuleName, 2,
		"no such asset is registered for custody")
	ErrAssetExists = sdkerrors.Register(ModuleName, 3,
		"that asset is already registered")
	ErrNotAttestor = sdkerrors.Register(ModuleName, 4,
		"this account is not an appointed attestor")
	ErrAlreadyAttested = sdkerrors.Register(ModuleName, 5,
		"this attestor has already attested to that deposit")
	// The one that stops a deposit being credited twice.
	ErrDuplicateRef = sdkerrors.Register(ModuleName, 6,
		"that external reference has already been credited")
	ErrIssuancePaused = sdkerrors.Register(ModuleName, 7,
		"issuance is paused for that asset")
	ErrNotFound = sdkerrors.Register(ModuleName, 8,
		"no such record")
	ErrNotPayableYet = sdkerrors.Register(ModuleName, 9,
		"this redemption is still inside its delay window")
	ErrAlreadySettled = sdkerrors.Register(ModuleName, 10,
		"that redemption has already been settled")
	ErrInvalidAmount = sdkerrors.Register(ModuleName, 11,
		"amount must be positive")
	ErrInvalidParams = sdkerrors.Register(ModuleName, 12,
		"invalid parameters")
	ErrInvalidSigner = sdkerrors.Register(ModuleName, 13,
		"invalid authority for this message")
)
