package types

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Field limits, taken from ISO 20022 rather than invented.
//
// These are the lengths every system on the other side of one of these payments
// already enforces, so a value that would not fit here would not survive the
// journey anyway. They also bound what an attacker can commit to state: the
// end-to-end id is a store key, and remittance information is free text the
// chain keeps forever, so unbounded versions of either are state bloat priced
// at one transaction fee.
const (
	// MaxEndToEndIDLength is ISO 20022's Max35Text.
	MaxEndToEndIDLength = 35

	// MaxPurposeCodeLength is an ExternalPurpose1Code, always four characters.
	MaxPurposeCodeLength = 4

	// MaxRemittanceLength is Max140Text, the unstructured remittance field.
	MaxRemittanceLength = 140
)

// ValidatePaymentFields checks everything about a payment instruction that does
// not require reading state.
//
// The denom check matters more than it looks: sdk.NewCoin panics on an invalid
// denom rather than returning an error. That panic is recovered into a failed
// transaction rather than halting the chain, but a handler should refuse its
// own bad input instead of relying on a recover several layers up.
func ValidatePaymentFields(endToEndID, purposeCode, remittance, denom string) error {
	if endToEndID == "" {
		return errorsmod.Wrap(ErrInvalidPaymentField, "end_to_end_id must be set; it is what identifies this payment to both sides")
	}
	if len(endToEndID) > MaxEndToEndIDLength {
		return errorsmod.Wrapf(ErrInvalidPaymentField,
			"end_to_end_id must be at most %d characters, got %d", MaxEndToEndIDLength, len(endToEndID))
	}
	if len(purposeCode) > MaxPurposeCodeLength {
		return errorsmod.Wrapf(ErrInvalidPaymentField,
			"purpose_code must be at most %d characters, got %d", MaxPurposeCodeLength, len(purposeCode))
	}
	if len(remittance) > MaxRemittanceLength {
		return errorsmod.Wrapf(ErrInvalidPaymentField,
			"remittance_information must be at most %d characters, got %d", MaxRemittanceLength, len(remittance))
	}
	if err := sdk.ValidateDenom(denom); err != nil {
		return errorsmod.Wrapf(ErrInvalidPaymentField, "invalid denom %q: %s", denom, err)
	}
	return nil
}

// ValidateSettlementJurisdiction checks the country whose authority settles a
// payment.
//
// The format check is the cheap half. It is here at all because a jurisdiction
// that is merely "some string" is not a perimeter: the field decides which
// authority may act on a cross-border payment and which regulator holds a
// viewing key over its payload, and both of those are answered by looking the
// value up. A lowercase "ng", a three-letter "NGA" or a truncated "N" matches
// no authority, so the payment would be settled by nobody while looking on the
// record as though somebody was named.
//
// Membership of the assigned ISO 3166-1 list is deliberately not checked here.
// x/alias owns the jurisdiction registry, and a second list in this module
// would be a second answer to the same question — the one that goes stale.
//
// required comes from Params rather than from this package, because when the
// field becomes mandatory is a governance decision made at a height, not a
// property of the code. See Params.require_settlement_jurisdiction.
func ValidateSettlementJurisdiction(country string, required bool) error {
	if country == "" {
		if required {
			return errorsmod.Wrap(ErrInvalidSettlementJurisdiction,
				"settlement_jurisdiction must be set; it names the authority that may act on this payment and the regulator that can open its payload")
		}
		return nil
	}

	if len(country) != 2 {
		return errorsmod.Wrapf(ErrInvalidSettlementJurisdiction,
			"settlement_jurisdiction must be a two-letter ISO 3166-1 alpha-2 code, got %q", country)
	}
	for i := 0; i < len(country); i++ {
		if country[i] < 'A' || country[i] > 'Z' {
			return errorsmod.Wrapf(ErrInvalidSettlementJurisdiction,
				"settlement_jurisdiction must be uppercase, got %q", country)
		}
	}
	return nil
}

// ValidateConfidentiality checks the fields MsgSendPayment reserves for the
// confidentiality design.
//
// Two rules, and both are about refusing to look like a feature that is not
// there yet.
//
// A commitment or a range proof is refused outright. The chain does not verify
// either one, and amount is still in the clear beside them, so a field the
// chain silently ignores is one a client can be built on top of — and the
// person using that client would be told their amount was hidden while it sat
// in plaintext in the same message. When commitments ship they arrive with
// verification; until then, setting them is an error rather than a no-op.
//
// A metadata hash beside the plaintext it replaces is refused for the reason
// the hash exists. Sending both puts the customer's name on the ledger and the
// hash next to it, which is the privacy of neither and the storage cost of
// both.
func ValidateConfidentiality(amountCommitment, amountRangeProof, metadataHash []byte, purposeCode, remittance string) error {
	if len(amountCommitment) > 0 || len(amountRangeProof) > 0 {
		return errorsmod.Wrap(ErrConfidentialAmountUnavailable,
			"amount_commitment and amount_range_proof are reserved and not yet verified; a payment that sets them would have its amount in plaintext regardless")
	}

	if len(metadataHash) == 0 {
		return nil
	}
	if len(metadataHash) != MetadataHashLength {
		return errorsmod.Wrapf(ErrInvalidMetadata,
			"metadata_hash must be %d bytes, got %d", MetadataHashLength, len(metadataHash))
	}
	if purposeCode != "" || remittance != "" {
		return errorsmod.Wrap(ErrInvalidMetadata,
			"a payment carries its ISO 20022 detail as metadata_hash or as plaintext, never both; sending both records the detail it was meant to keep off the ledger")
	}
	return nil
}
