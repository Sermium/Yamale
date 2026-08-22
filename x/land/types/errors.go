package types

import "cosmossdk.io/errors"

// Errors are numbered from 1 so that a zero code is never a land error, and
// worded as what a registrar would need to be told — these reach an official
// through a UI, and "invalid request" tells them nothing they can act on.
var (
	ErrNotAuthority      = errors.Register(ModuleName, 1, "signer is not a registry office")
	ErrAuthorityInactive = errors.Register(ModuleName, 2, "this registry office is not active")

	ErrNoGeometry     = errors.Register(ModuleName, 3, "a survey hash is required")
	ErrNoCadastralRef = errors.Register(ModuleName, 4, "a cadastral reference is required")
	ErrInvalidHolder  = errors.Register(ModuleName, 5, "the holder is not a valid account")

	// The two refusals that make a parcel unique.
	ErrGeometryTitled = errors.Register(ModuleName, 6,
		"this ground is already titled")
	ErrRefTaken = errors.Register(ModuleName, 7,
		"this cadastral reference is already used")

	ErrNoParcel              = errors.Register(ModuleName, 8, "no such parcel")
	ErrNotHolder             = errors.Register(ModuleName, 9, "only the holder may propose a transfer")
	ErrParcelNotTransferable = errors.Register(ModuleName, 10,
		"this parcel cannot be transferred in its current state")
	ErrInvalidRecipient = errors.Register(ModuleName, 11, "the recipient is not a valid account")
	ErrSelfTransfer     = errors.Register(ModuleName, 12, "the recipient is already the holder")

	ErrNoTransfer       = errors.Register(ModuleName, 13, "no such transfer")
	ErrTransferClosed   = errors.Register(ModuleName, 14, "this transfer is already complete")
	ErrTransferDisputed = errors.Register(ModuleName, 15, "this transfer has been objected to")

	ErrWrongJurisdiction = errors.Register(ModuleName, 16,
		"only the office holding this parcel may validate it")
	ErrAlreadyValidated = errors.Register(ModuleName, 17, "already validated")

	// The independence rule, stated as an error because it is the mechanism.
	ErrNotIndependent = errors.Register(ModuleName, 18,
		"an attestor from the parcel's own office is not independent")
	ErrAlreadyAttested = errors.Register(ModuleName, 19, "this office has already attested")

	ErrNotValidated        = errors.Register(ModuleName, 20, "not yet validated by the office in charge")
	ErrNoQuorum            = errors.Register(ModuleName, 21, "not enough independent attestations")
	ErrChallengeWindowOpen = errors.Register(ModuleName, 22,
		"the challenge window has not closed yet")

	ErrNoReason = errors.Register(ModuleName, 23, "an objection must give a reason")
)

// Errors for the deed, restriction and fractionalisation surface.
var (
	ErrNotGovernance     = errors.Register(ModuleName, 24, "this message may only come from governance")
	ErrNoDocument        = errors.Register(ModuleName, 25, "a document hash is required")
	ErrNoRestriction     = errors.Register(ModuleName, 26, "no such restriction")
	ErrNoRestrictionKind = errors.Register(ModuleName, 27, "a restriction kind is required")
	ErrNoEncumbrance     = errors.Register(ModuleName, 28, "no such encumbrance")
	ErrNotFrozen         = errors.Register(ModuleName, 29, "this parcel is not frozen")
	ErrBadShareCeiling   = errors.Register(ModuleName, 30,
		"the share ceiling must be between 1 and 10000 basis points")
	// A restriction forbidding fractionalisation outranks an office's
	// permission — otherwise recording restrictions would be decorative.
	ErrFractionalisationForbidden = errors.Register(ModuleName, 31,
		"a restriction on this parcel forbids fractionalisation")
)

// Errors guarding the authorisation record itself.
var (
	// Zero is both "unset" and, if read as "no expiry", a permission that never
	// lapses. Refused so that neither reading can happen.
	ErrBadExpiry = errors.Register(ModuleName, 33,
		"the authorisation must expire at a time in the future")
	ErrNoAuthorisation = errors.Register(ModuleName, 34,
		"this parcel has no fractionalisation authorisation to withdraw")
)

// ErrOfficeNotGroup refuses a registry office that is a single key.
var ErrOfficeNotGroup = errors.Register(ModuleName, 32,
	"a registry office must be a group account, so its decisions need several signatures")

// ErrInvalidJurisdiction refuses an office admitted for somewhere that is not a
// country.
//
// The jurisdiction on an office record is what the perimeter check is made
// against, so a free-text value there is a perimeter no grant can ever cover:
// the office would be admitted, look admitted, and be unable to register a
// single parcel. Refused at admission so the failure is visible in the proposal
// rather than the first time a registrar tries to do their job.
var ErrInvalidJurisdiction = errors.Register(ModuleName, 35,
	"a registry office's jurisdiction must be an assigned ISO 3166-1 alpha-2 country code")
