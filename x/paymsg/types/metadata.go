package types

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"

	errorsmod "cosmossdk.io/errors"
)

const (
	// MetadataHashLength is the length of the commitment the chain records.
	//
	// Checked rather than assumed. A hash of any other length is not a hash —
	// it is whatever the sender felt like putting in the field, stored forever
	// in a statement record where it will read as a commitment to somebody
	// reconciling against it years later.
	MetadataHashLength = sha256.Size

	// MetadataSaltLength is the salt every payload carries.
	//
	// 32 bytes rather than 8 or 16 because the salt is the only thing standing
	// between a hash of a four-character purpose code and a lookup table, and
	// the ledger is public and permanent: an attacker gets unlimited time and
	// unlimited guesses, so the margin has to be one nobody can grind through
	// rather than one that is merely inconvenient today.
	MetadataSaltLength = 32
)

// NewPaymentMetadata builds a payload with a fresh salt.
//
// It belongs to whoever is composing the payment — a wallet, a participant's
// back office — and never to the chain. A node calling this would be
// generating a secret it then has to keep, which is the arrangement the whole
// design exists to avoid; the keeper only ever sees the hash.
func NewPaymentMetadata(purposeCode, remittanceInformation string) (PaymentMetadata, error) {
	salt := make([]byte, MetadataSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return PaymentMetadata{}, errorsmod.Wrap(err, "unable to generate a metadata salt")
	}

	return PaymentMetadata{
		Salt:                  salt,
		PurposeCode:           purposeCode,
		RemittanceInformation: remittanceInformation,
	}, nil
}

// Hash returns the value that goes on-chain in MsgSendPayment.metadata_hash.
//
// The salt is required here rather than defaulted, because a payload that
// reached this function without one has already lost the property the hash was
// added for, and returning a usable hash anyway would hide that until the
// ledger was public.
func (m PaymentMetadata) Hash() ([]byte, error) {
	if len(m.Salt) != MetadataSaltLength {
		return nil, errorsmod.Wrapf(ErrInvalidMetadata,
			"salt must be %d bytes, got %d", MetadataSaltLength, len(m.Salt))
	}
	if err := ValidateMetadataFields(m.PurposeCode, m.RemittanceInformation); err != nil {
		return nil, err
	}

	bz, err := m.Marshal()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(bz)
	return sum[:], nil
}

// VerifyMetadata reports whether a payload held off-chain is the one a payment
// recorded.
//
// This is the only check that gives the off-chain store any authority. Without
// it the payload is a claim by whoever is storing it, and a participant in a
// dispute could produce whichever version of the remittance information suited
// them; with it, either the payload hashes to what the block says or it is not
// the payload.
func VerifyMetadata(payload PaymentMetadata, recorded []byte) error {
	if len(recorded) != MetadataHashLength {
		return errorsmod.Wrapf(ErrInvalidMetadata,
			"recorded hash must be %d bytes, got %d", MetadataHashLength, len(recorded))
	}

	computed, err := payload.Hash()
	if err != nil {
		return err
	}
	if !bytes.Equal(computed, recorded) {
		return errorsmod.Wrap(ErrInvalidMetadata,
			"payload does not hash to the value recorded on-chain, so it is not the payload this payment carried")
	}
	return nil
}

// ValidateMetadataFields applies the ISO 20022 limits to a payload.
//
// The same limits as the plaintext fields they replace. Moving the detail
// off-chain does not make it unbounded: a payload that exceeds what the
// receiving system can hold still cannot be delivered, and the limits are
// worth failing on while the sender is still there to be told.
func ValidateMetadataFields(purposeCode, remittanceInformation string) error {
	if len(purposeCode) > MaxPurposeCodeLength {
		return errorsmod.Wrapf(ErrInvalidMetadata,
			"purpose_code must be at most %d characters, got %d", MaxPurposeCodeLength, len(purposeCode))
	}
	if len(remittanceInformation) > MaxRemittanceLength {
		return errorsmod.Wrapf(ErrInvalidMetadata,
			"remittance_information must be at most %d characters, got %d", MaxRemittanceLength, len(remittanceInformation))
	}
	return nil
}
