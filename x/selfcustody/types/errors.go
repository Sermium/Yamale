package types

import "cosmossdk.io/errors"

// Errors are worded for the person being refused, who is an account holder
// trying to take their own key and is entitled to know exactly why they cannot
// yet.
var (
	ErrNotAuthority = errors.Register(ModuleName, 1,
		"this message is governance-only")

	ErrAlreadyDisclosed = errors.Register(ModuleName, 2,
		"this account's key has already been disclosed; it cannot be disclosed twice and it cannot be undone")
	ErrRequestPending = errors.Register(ModuleName, 3,
		"a disclosure is already pending for this account")
	ErrNoRequest = errors.Register(ModuleName, 4,
		"there is no pending disclosure request for this account")
	ErrTooSoon = errors.Register(ModuleName, 5,
		"the waiting period has not run; this is the time in which a stolen device can be noticed")
	ErrRequestExpired = errors.Register(ModuleName, 6,
		"this request has lapsed and must be made again")

	ErrWrongAcknowledgement = errors.Register(ModuleName, 7,
		"that is not the warning this chain publishes; you were shown something else")
	ErrInvalidAccount = errors.Register(ModuleName, 8, "that is not a valid account")

	ErrInvalidParams = errors.Register(ModuleName, 9, "these parameters cannot be enforced")
)
