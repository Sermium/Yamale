package types

import (
	errorsmod "cosmossdk.io/errors"
)

// Bounds on the free text this module stores.
//
// Applying to be a valuer is permissionless, so every string in an application
// is an attacker-chosen blob that the chain keeps forever. Gas prices the
// transaction, not the permanent cost of carrying those bytes on every node's
// disk and through every state sync, so a chain that only charges gas for them
// is under-pricing state by a wide margin.
//
// These are limits on abuse, not on legitimate use: a valuer's name, the licence
// they were admitted on, and a link to a signed report all fit comfortably. They
// are deliberately not parameters — a limit governance can raise is a limit an
// attacker only has to wait for.
const (
	// MaxNameLength bounds a valuer's name.
	MaxNameLength = 128

	// MaxCredentialsLength bounds the licence or registration governance relied
	// on. Longer than a name because it may carry a registry and a number.
	MaxCredentialsLength = 256

	// MaxIdentifierLength bounds an NFT class or token id. These become part of
	// a store key, so they are kept short.
	MaxIdentifierLength = 64

	// MaxMethodLength bounds the description of how a valuation was reached,
	// e.g. "RICS Red Book" or "administrator NAV".
	MaxMethodLength = 128

	// MaxURILength bounds a link to the signed valuation document.
	MaxURILength = 512

	// MaxHashLength bounds the digest pinning that document. Generous enough for
	// any hex or multibase encoding in use.
	MaxHashLength = 128

	// MaxRateLength bounds a reported price. A decimal with an 18-digit
	// fraction and room for any plausible integer part fits; thousands of
	// digits are parsing work every validator has to do, every round.
	MaxRateLength = 64
)

// checkLength rejects a field longer than its bound.
func checkLength(field, value string, max int) error {
	if len(value) > max {
		return errorsmod.Wrapf(ErrLimitReached, "%s must be at most %d bytes, got %d", field, max, len(value))
	}
	return nil
}

// ValidateAppraiserText bounds the free text on an application.
func ValidateAppraiserText(name, credentials string) error {
	if err := checkLength("name", name, MaxNameLength); err != nil {
		return err
	}
	return checkLength("credentials", credentials, MaxCredentialsLength)
}

// ValidateAppraisalText bounds the free text on a valuation.
func ValidateAppraisalText(classID, nftID, method, reportURI, reportHash string) error {
	if err := checkLength("class_id", classID, MaxIdentifierLength); err != nil {
		return err
	}
	if err := checkLength("nft_id", nftID, MaxIdentifierLength); err != nil {
		return err
	}
	if err := checkLength("method", method, MaxMethodLength); err != nil {
		return err
	}
	if err := checkLength("report_uri", reportURI, MaxURILength); err != nil {
		return err
	}
	return checkLength("report_hash", reportHash, MaxHashLength)
}
