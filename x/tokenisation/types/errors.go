package types

import sdkerrors "cosmossdk.io/errors"

// Errors are numbered from 2; code 1 is reserved for the SDK's internal error.
var (
	ErrCollectionNotFound = sdkerrors.Register(ModuleName, 2,
		"no such collection")
	ErrCollectionExists = sdkerrors.Register(ModuleName, 3,
		"that collection already exists")
	// An unset or revoked authority refuses mints rather than falling back to
	// governance. The failure mode of a permissive default is unlimited
	// issuance of title to things nobody owns.
	ErrNoAuthority = sdkerrors.Register(ModuleName, 4,
		"this collection has no minting authority")
	ErrNotAuthority = sdkerrors.Register(ModuleName, 5,
		"this account is not the collection's minting authority")
	ErrAssetNotFound = sdkerrors.Register(ModuleName, 6,
		"no such asset")
	ErrNotOwner = sdkerrors.Register(ModuleName, 7,
		"this account does not hold title to that asset")
	// Supply is fixed at fractionalisation. An owner who could issue more
	// against an asset whose interests they had already sold is the whole fraud
	// in one message handler.
	ErrAlreadyFractionalised = sdkerrors.Register(ModuleName, 8,
		"that asset already has shareholders")
	ErrNoShareholders = sdkerrors.Register(ModuleName, 9,
		"that asset has no shareholders to credit")
	ErrInvalidShare = sdkerrors.Register(ModuleName, 10,
		"holder share must be between 1 and 10000 basis points")
	ErrAmountTooSmall = sdkerrors.Register(ModuleName, 11,
		"amount is too small to divide across the shareholding")
	ErrWrongStatus = sdkerrors.Register(ModuleName, 12,
		"the asset is not in a state that allows this")
	// Trading stops at realisation: each token is then a claim on a known fixed
	// pot, and a pool still quoting from its reserves is a free lunch.
	ErrTradingHalted = sdkerrors.Register(ModuleName, 13,
		"this asset has been realised; the only remaining path is redemption")
	ErrNotVerified = sdkerrors.Register(ModuleName, 14,
		"the reported sale price has not met its verification requirement")
	ErrStillInWindow = sdkerrors.Register(ModuleName, 15,
		"the reported sale price is still inside its challenge window")
	ErrAlreadyDisputed = sdkerrors.Register(ModuleName, 16,
		"that sale is already under dispute")
	ErrNotAttestor = sdkerrors.Register(ModuleName, 17,
		"this account is not an appointed attestor for that collection")
	ErrAlreadyAttested = sdkerrors.Register(ModuleName, 18,
		"this attestor has already signed that figure")
	// One attestor is not a threshold, it is a single point of unlimited theft.
	ErrInvalidThreshold = sdkerrors.Register(ModuleName, 19,
		"attestation threshold must be at least two")
	ErrInvalidParams = sdkerrors.Register(ModuleName, 20,
		"invalid parameters")
	ErrInvalidSigner = sdkerrors.Register(ModuleName, 21,
		"expected the governance account as the only signer")
	ErrInvalidAmount = sdkerrors.Register(ModuleName, 22,
		"amount must be positive")
	ErrNothingOwed = sdkerrors.Register(ModuleName, 23,
		"nothing is owed to this account")
	ErrNoSaleReported = sdkerrors.Register(ModuleName, 24,
		"no sale has been reported for that asset")
	ErrWrongDenom = sdkerrors.Register(ModuleName, 25,
		"that is not the denomination this vault pays income in")
)

// Errors from the land registry bridge. They only ever reach an asset that
// names a parcel; an asset with parcel_id 0 is not land and never meets them.
var (
	ErrNoParcel = sdkerrors.Register(ModuleName, 26,
		"the land registry has no parcel with that id")
	// Title lives in x/land. A sponsor who is not the holder is selling rights
	// over ground somebody else owns, and the shares would be worthless the
	// moment anybody checked the registry.
	ErrNotParcelHolder = sdkerrors.Register(ModuleName, 27,
		"the asset's owner is not the current holder of that parcel")
	ErrNoLandAuthorisation = sdkerrors.Register(ModuleName, 28,
		"the land registry has not authorised fractionalising that parcel")
	ErrAuthorisationWithdrawn = sdkerrors.Register(ModuleName, 29,
		"the land registry has withdrawn its authorisation to fractionalise that parcel")
	ErrAuthorisationExpired = sdkerrors.Register(ModuleName, 30,
		"the land registry's authorisation to fractionalise that parcel has expired")
	// The ceiling is on what is sold, not on what is kept, and it bounds the
	// total across every vehicle over the parcel. Comparing the sponsor's
	// retained share against it, or checking one vehicle at a time, permits
	// exactly the issuance the registry set out to forbid.
	ErrShareCeilingExceeded = sdkerrors.Register(ModuleName, 31,
		"the shares issued over that parcel would exceed the ceiling the land registry set")
	ErrLandFractionalisationForbidden = sdkerrors.Register(ModuleName, 32,
		"a restriction on that parcel forbids fractionalisation")
	// Fails closed. A chain built without the registry wired in must refuse
	// land assets outright rather than treat every parcel as unrestricted.
	ErrNoLandRegistry = sdkerrors.Register(ModuleName, 33,
		"this chain has no land registry, so an asset cannot name a parcel")
	// DisputeSale returned ErrStillInWindow for the opposite condition — the
	// guard fires when the block time is PAST claimable_at — which would send
	// whoever debugs a refused dispute looking for a window that has already
	// closed. Two conditions, two errors.
	ErrWindowClosed = sdkerrors.Register(ModuleName, 34,
		"the challenge window on that sale has closed, so it can no longer be disputed")

	// One module account holds every vehicle's money, so a payout the bank
	// keeper can settle is not necessarily a payout this vehicle can afford.
	// These three are what stands between a vehicle and its neighbour's vault.
	ErrVaultUnfunded = sdkerrors.Register(ModuleName, 35,
		"this vehicle does not hold enough to pay that, and the money of another vehicle is not available to it")
	ErrProceedsUnpaid = sdkerrors.Register(ModuleName, 36,
		"the holders' share of the reported price has not been paid into the vault, so redemption cannot open")
	ErrOverpayment = sdkerrors.Register(ModuleName, 37,
		"that is more than the sale still owes, and the surplus would have no owner")
)
