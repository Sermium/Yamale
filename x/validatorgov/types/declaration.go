package types

import (
	"fmt"
	"strings"

	aliastypes "yamale/blockchain/x/alias/types"
)

// MaxDeclarationFieldLength bounds the identifiers.
//
// They are stored forever, read at every epoch and shown to whoever is checking
// the register, and nothing legitimate needs more: an LEI is twenty characters
// and the longest national register number in use is well under this. The bound
// is here so that a validator cannot make the concentration query expensive for
// everyone by declaring a novel.
const MaxDeclarationFieldLength = 128

// Validate checks a declaration is one the ceilings can actually be computed
// over.
//
// All three fields are required. A blank one is not a permissive default: a
// validator with no declared owner belongs to no group, so it would sit outside
// every ceiling on the chain — the one outcome a beneficial-ownership register
// exists to prevent. Refusing it at the message is the only place that can be
// stopped, because by the epoch check it is already state.
func (d Declaration) Validate() error {
	fields := []struct {
		name  string
		value string
	}{
		{"legal_entity_id", d.LegalEntityId},
		{"beneficial_owner_id", d.BeneficialOwnerId},
	}
	for _, f := range fields {
		trimmed := strings.TrimSpace(f.value)
		if trimmed == "" {
			return fmt.Errorf("%s is required; a validator with none belongs to no group and would sit outside every concentration ceiling", f.name)
		}
		if len(trimmed) > MaxDeclarationFieldLength {
			return fmt.Errorf("%s is %d characters, the maximum is %d", f.name, len(trimmed), MaxDeclarationFieldLength)
		}
	}

	// Checked against the assigned list x/alias owns rather than against a
	// shape. NX, QK and ZX are all two letters and none of them is a country,
	// so a mistyped code would create a jurisdiction ceiling that no authority
	// holds and that exactly one validator is ever measured against.
	//
	// The foundation's reserved code is refused along with everything else that
	// is not a country: it is not a national perimeter, and a validator
	// declaring it would be declaring exemption from the one ceiling that
	// exists to keep any single state below a blocking minority.
	if !aliastypes.AssignedCountry(aliastypes.NormaliseCountry(d.Jurisdiction)) {
		return fmt.Errorf("jurisdiction %q is not an assigned ISO 3166-1 alpha-2 country code", d.Jurisdiction)
	}

	return nil
}

// NormaliseDeclaration trims the identifiers and uppercases the country.
//
// Applied before anything is stored, because the ceilings group by exact string
// equality: " ACME" and "ACME" would be two entities holding a fifth each
// rather than one entity holding two fifths, and that is a cap defeated by a
// space bar.
func NormaliseDeclaration(legalEntityID, beneficialOwnerID, jurisdiction string) Declaration {
	return Declaration{
		LegalEntityId:     strings.TrimSpace(legalEntityID),
		BeneficialOwnerId: strings.TrimSpace(beneficialOwnerID),
		Jurisdiction:      aliastypes.NormaliseCountry(strings.TrimSpace(jurisdiction)),
	}
}

// IsStale reports whether a declaration is older than the interval.
//
// A declaration that has never been attested — attested_at_height of zero, a
// record written before this field existed — is stale from the first epoch
// after the import. That is the intended reading: "nobody has ever signed for
// this" is a stronger version of "nobody has signed for this recently", and
// treating it as fresh would exempt exactly the records with the least behind
// them.
func (d Declaration) IsStale(height int64, intervalBlocks uint64) bool {
	if intervalBlocks == 0 {
		// Guarded here as well as in Params.AttestationInterval, because a zero
		// reaching this from an upgraded store would report every validator on
		// the chain as stale in the same block.
		return false
	}
	return height > d.AttestedAtHeight+int64(intervalBlocks)
}
