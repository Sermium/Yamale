package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/enforcement module sentinel errors
var (
	ErrInvalidSigner    = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrUnknownValidator = errors.Register(ModuleName, 1101, "not a bonded validator")
	ErrCaseNotFound     = errors.Register(ModuleName, 1102, "no such case")
	ErrCaseClosed       = errors.Register(ModuleName, 1103, "case is no longer open")
	ErrAlreadyVoted     = errors.Register(ModuleName, 1104, "this validator has already voted on this case")
	ErrAlreadyFrozen    = errors.Register(ModuleName, 1105, "an open case already freezes this address")
	ErrNotTheOpener     = errors.Register(ModuleName, 1106, "only the validator that opened the case may withdraw it")
	ErrFrozen           = errors.Register(ModuleName, 1107, "account is frozen by an enforcement case")
	ErrEvidenceRequired = errors.Register(ModuleName, 1108, "a seizure case requires evidence")
	ErrInvalidCase      = errors.Register(ModuleName, 1109, "case is not valid")
	ErrNotSeizure       = errors.Register(ModuleName, 1110, "case did not order a seizure")
	ErrNotPassed        = errors.Register(ModuleName, 1111, "case has not passed")
	ErrProtectedAddress = errors.Register(ModuleName, 1112, "address cannot be frozen or seized")
	ErrLimitReached     = errors.Register(ModuleName, 1113, "exceeds a configured maximum")
	// Code 1114 is retired and must never be reused.
	//
	// It was ErrNoEmergencyAuthority — "no emergency authority is configured" —
	// raised when the emergency_authority parameter was empty, and deliberately
	// distinct from ErrInvalidSigner because "the founders' group did not sign
	// this" and "this chain has no founders' group" are different facts. The
	// parameter is gone; the emergency path is a role grant, so an empty chain
	// refuses with the perimeter's own ErrOutOfScope, which says which country
	// the signer's grant did not reach and is the more useful of the two.
	//
	// Left as a comment rather than deleted, because an error code is part of
	// this module's published interface. A client that special-cased 1114 should
	// find out that it retired, and a future error given that number would tell
	// every one of them something false.

	// ErrLegalInstrumentRequired is separate from ErrEvidenceRequired because
	// the two are different failures and telling somebody the wrong one sends
	// them to fetch the wrong document. Evidence is what the chain was shown;
	// an instrument is what a court or a regulator ordered.
	ErrLegalInstrumentRequired = errors.Register(ModuleName, 1115, "a seizure case requires an external legal instrument")

	// ErrNoOmbudsman is distinct from ErrInvalidSigner for the same reason
	// ErrNoEmergencyAuthority is: "you are not the ombudsman" and "this chain
	// has no ombudsman" are different facts.
	ErrNoOmbudsman = errors.Register(ModuleName, 1116, "no ombudsman is appointed")

	// ErrOmbudsmanCannotInitiate is what every path that opens or advances a
	// case returns to the ombudsman. It is its own error so that an attempt is
	// legible in a log as what it was rather than as a generic rejection: the
	// office that can only stop things tried to start one.
	ErrOmbudsmanCannotInitiate = errors.Register(ModuleName, 1117,
		"the ombudsman may only stop cases, never open, vote on, or advance one")

	// ErrSeizureCapReached is returned by nothing a user sends — it is carried
	// on the event a deferred seizure emits — but it is registered so the
	// deferral has a stable, documented identity rather than a string.
	ErrSeizureCapReached = errors.Register(ModuleName, 1118,
		"this seizure would breach the rolling cap on what may be taken per window")

	// ErrNotHeld is returned when something is asked of a case that is not
	// waiting out its delay.
	ErrNotHeld = errors.Register(ModuleName, 1119, "case is not waiting to be carried out")
)
