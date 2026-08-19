package types

import sdkerrors "cosmossdk.io/errors"

// Errors are numbered from 2; code 1 is reserved for the SDK's internal error.
var (
	ErrAlreadyRegistered = sdkerrors.Register(ModuleName, 2,
		"this account already holds an identifier")
	ErrNotRegistered = sdkerrors.Register(ModuleName, 3,
		"this account holds no identifier")
	ErrNotFound = sdkerrors.Register(ModuleName, 4,
		"no account holds that identifier")
	ErrMalformedID = sdkerrors.Register(ModuleName, 5,
		"that is not a well-formed identifier")
	ErrInvalidParams = sdkerrors.Register(ModuleName, 6,
		"invalid parameters")
	// Exhausted cannot happen at 32^8 with a nonce that keeps incrementing, and
	// the loop that would spin forever if it did must still have a way out.
	ErrExhausted = sdkerrors.Register(ModuleName, 7,
		"could not derive an unused identifier")
	ErrInvalidSigner = sdkerrors.Register(ModuleName, 8,
		"invalid authority for this message")
	// The refusal the perimeter is built on. Not a permissive default and not a
	// placeholder country: an account whose jurisdiction nobody has recorded
	// gets no identifier, so there is no account on the rail whose perimeter is
	// unknown and no state for anyone to reason about or exploit.
	ErrNoJurisdiction = sdkerrors.Register(ModuleName, 9,
		"this account has no recorded jurisdiction")
	ErrInvalidCountry = sdkerrors.Register(ModuleName, 10,
		"that is not an assigned ISO 3166-1 alpha-2 country code")
	// Raised when a participant that did not onboard the account, or that is no
	// longer approved, tries to record where it is. The country is evidence
	// gathered by whoever performed the KYC; anyone else asserting it is a
	// guess wearing the same clothes.
	ErrNotTheRecorder = sdkerrors.Register(ModuleName, 11,
		"only the account's approved participant or a foundation administrator may record its jurisdiction")
	// A participant may record a country once. Changing one already recorded is
	// an authority's act, because a participant able to rewrite it could move a
	// customer out from under the authority investigating them.
	ErrJurisdictionSet = sdkerrors.Register(ModuleName, 12,
		"this account's jurisdiction is already recorded; only a foundation administrator may correct it")
)
