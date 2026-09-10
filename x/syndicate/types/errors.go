package types

import "cosmossdk.io/errors"

// Errors are numbered from 1 so a zero code is never a syndicate error, and
// worded as what the person reading them actually needs to know.
//
// The audience here is not a node operator. It is an investor who has just been
// refused, and the difference between "you are not onboarded", "this deal has
// closed" and "that is below the minimum cheque" is the difference between
// somebody fixing it in a minute and somebody emailing support.
var (
	ErrNotAuthority = errors.Register(ModuleName, 1,
		"this message is governance-only")
	ErrNotOperator = errors.Register(ModuleName, 2,
		"the signer is not an admitted platform operator")
	ErrOperatorInactive = errors.Register(ModuleName, 3,
		"this platform operator is no longer admitted")
	ErrOperatorNotGroup = errors.Register(ModuleName, 4,
		"a platform operator must be a group account, so taking money needs several signatures")
	ErrInvalidJurisdiction = errors.Register(ModuleName, 5,
		"that is not a country this chain recognises")

	ErrNotOnboarded = errors.Register(ModuleName, 6,
		"this address has not completed KYC onboarding and cannot hold a deal token")
	ErrInvestorInactive = errors.Register(ModuleName, 7,
		"this investor's onboarding has been withdrawn and they may not acquire more")
	ErrInvalidInvestor = errors.Register(ModuleName, 8, "the investor is not a valid account")

	ErrNoReference   = errors.Register(ModuleName, 9, "a deal reference is required")
	ErrRefTaken      = errors.Register(ModuleName, 10, "this deal reference is already used")
	ErrNoDeal        = errors.Register(ModuleName, 11, "no such deal")
	ErrDealClosed    = errors.Register(ModuleName, 12, "this deal is no longer open for commitments")
	ErrDealNotOpen   = errors.Register(ModuleName, 13, "this deal is not open")
	ErrDealNotFunded = errors.Register(ModuleName, 14, "this deal has not closed and issued its tokens")
	ErrWindowOpen    = errors.Register(ModuleName, 15,
		"commitments are still being accepted; this deal cannot be closed yet")
	ErrMinimumNotMet = errors.Register(ModuleName, 16,
		"the deal did not reach its minimum and must be aborted rather than closed")
	ErrOverTarget = errors.Register(ModuleName, 17,
		"that would take the deal over its target")
	ErrBelowTicket = errors.Register(ModuleName, 18,
		"that is below this deal's minimum cheque")
	ErrNoCommitment = errors.Register(ModuleName, 19, "you have not committed to this deal")
	ErrWrongDenom   = errors.Register(ModuleName, 20,
		"this deal settles in a different currency")

	ErrInvalidTerms = errors.Register(ModuleName, 21, "these deal terms cannot be met")
	ErrFeeTooHigh   = errors.Register(ModuleName, 22,
		"that fee is above the ceiling the platform's governance set")
	ErrWindowTooShort = errors.Register(ModuleName, 23,
		"a deal must stay open long enough for the investor base to see it")
	ErrWindowTooLong = errors.Register(ModuleName, 24,
		"a deal may not hold committed money longer than the platform allows")

	ErrNothingToClaim = errors.Register(ModuleName, 25, "there is nothing accrued to claim")
	ErrTradingHalted  = errors.Register(ModuleName, 26,
		"this deal has been realised; the token is a claim on a fixed sum and no longer trades")

	ErrNoListing     = errors.Register(ModuleName, 27, "no such listing")
	ErrListingClosed = errors.Register(ModuleName, 28, "this listing is no longer live")
	ErrNotSeller     = errors.Register(ModuleName, 29, "only the seller may withdraw a listing")
	ErrNotNamedBuyer = errors.Register(ModuleName, 30,
		"this listing is a private sale to a named buyer")
	ErrSelfDeal      = errors.Register(ModuleName, 31, "you cannot buy your own listing")
	ErrInvalidAmount = errors.Register(ModuleName, 32, "the amount must be positive")
)
