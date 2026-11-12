package types

import "cosmossdk.io/errors"

// Errors are worded for the person at the counter or on the phone, not for
// the node operator: each says what is wrong in terms they can act on.
var (
	ErrNotOffice        = errors.Register(ModuleName, 1, "only the register's office may do this")
	ErrNoOffice         = errors.Register(ModuleName, 2, "no office is appointed yet; governance must appoint one before any jeweler is admitted")
	ErrOfficeNotGroup   = errors.Register(ModuleName, 3, "the office must be a group account, so its decisions need several signatures")
	ErrInvalidAddress   = errors.Register(ModuleName, 4, "that is not a valid account")
	ErrNoName           = errors.Register(ModuleName, 5, "a name is required")
	ErrBusinessTaken    = errors.Register(ModuleName, 6, "this business account already belongs to a jeweler")
	ErrNoJeweler        = errors.Register(ModuleName, 7, "no such jeweler")
	ErrJewelerSuspended = errors.Register(ModuleName, 8, "this jeweler is suspended and can no longer act")
	ErrNotBusiness      = errors.Register(ModuleName, 9, "only the jeweler's business account may manage its staff")
	ErrStaffTaken       = errors.Register(ModuleName, 10, "this person already works for another jeweler")
	ErrNotStaff         = errors.Register(ModuleName, 11, "you do not work for this jeweler")
	ErrBareSendOpen     = errors.Register(ModuleName, 12,
		"this chain still lets an NFT be sent without an offer and acceptance; disable /cosmos.nft.v1beta1.MsgSend in x/circuit first")
	ErrInvalidInscription = errors.Register(ModuleName, 13,
		"the inscription must be 4 to 40 letters and digits; spaces, dashes, dots, slashes and underscores are ignored")
	ErrInscriptionTaken = errors.Register(ModuleName, 14, "this jeweler has already registered a jewel with this inscription")
	ErrInvalidRecord    = errors.Register(ModuleName, 15,
		"the record needs its SHA-256 fingerprint in lower-case hex and a location of at most 512 characters")
	ErrNoJewel        = errors.Register(ModuleName, 16, "no such jewel")
	ErrNotOwner       = errors.Register(ModuleName, 17, "only the jewel's owner may do this")
	ErrOfferOpen      = errors.Register(ModuleName, 18, "this jewel already has an open offer; cancel it first")
	ErrNoOffer        = errors.Register(ModuleName, 19, "this jewel has no open offer")
	ErrOfferExpired   = errors.Register(ModuleName, 20, "this offer has expired; ask the owner for a new one")
	ErrOfferTarget    = errors.Register(ModuleName, 21, "an offer names either an account or a claim key, exactly one")
	ErrSelfOffer      = errors.Register(ModuleName, 22, "the recipient already holds this jewel")
	ErrNotRecipient   = errors.Register(ModuleName, 23, "this offer was made to someone else")
	ErrBadClaim       = errors.Register(ModuleName, 24, "the claim key or its signature does not match this offer")
	ErrStolen         = errors.Register(ModuleName, 25, "this jewel is reported stolen")
	ErrNotStolen      = errors.Register(ModuleName, 26, "this jewel is not reported stolen")
	ErrInvalidBenefit = errors.Register(ModuleName, 27,
		"a benefit needs a kind of at most 80 characters, an allowance of at least 1, a period of zero or more, and an expiry in the future or none")
	ErrNoBenefit         = errors.Register(ModuleName, 28, "no such benefit")
	ErrBenefitExpired    = errors.Register(ModuleName, 29, "this benefit has expired")
	ErrAllowanceUsed     = errors.Register(ModuleName, 30, "this benefit's allowance for the period is used up")
	ErrBadVoucher        = errors.Register(ModuleName, 31, "the voucher is not valid; ask the owner to show a fresh code")
	ErrVoucherUsed       = errors.Register(ModuleName, 32, "this voucher has already been used")
	ErrReassignmentOpen  = errors.Register(ModuleName, 33, "a reassignment of this jewel is already pending")
	ErrNoReassignment    = errors.Register(ModuleName, 34, "no reassignment of this jewel is pending")
	ErrWindowOpen        = errors.Register(ModuleName, 35, "the owner's veto window has not closed yet")
	ErrReassignmentStale = errors.Register(ModuleName, 36, "the jewel changed owner after the reassignment was proposed")
	ErrNoReason          = errors.Register(ModuleName, 37, "a reason is required")
	ErrInvalidSigner     = errors.Register(ModuleName, 38, "only governance may change these settings")
	ErrOfferStale        = errors.Register(ModuleName, 39, "the jewel changed owner after this offer was made")
	ErrNoteTooLong       = errors.Register(ModuleName, 40, "an amendment's note is at most 500 bytes; put the detail in the record file")
	ErrTooManyAmendments = errors.Register(ModuleName, 41,
		"this jewel has 50 amendments, the most it may carry; put further corrections in the record file of the last one")
)
