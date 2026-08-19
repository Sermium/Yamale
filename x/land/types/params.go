package types

import "fmt"

// DefaultParams are a starting position, not a finding. The quorum in
// particular is the number that decides how many people a buyer must corrupt,
// and it should be argued about by whoever adopts this.
func DefaultParams() Params {
	return Params{
		// Below three, two colluding officials suffice.
		AttestationQuorum: 3,
		// Fourteen days: long enough for word to reach a family member in
		// another city, short enough that legitimate sales are not strangled.
		ChallengeWindow: 14 * 24 * 60 * 60,
		// An attestor from the proposing office is not independent. Allowing it
		// collapses the mechanism back to a single bribe.
		SameAuthorityAttestation: false,
	}
}

// Validate refuses parameters that would quietly disable the protections.
func (p Params) Validate() error {
	// A quorum of zero would let a transfer complete with nobody attesting,
	// which is the failure this module exists to prevent — so it is rejected
	// rather than treated as "no attestation required".
	if p.AttestationQuorum == 0 {
		return fmt.Errorf("attestation quorum must be at least 1")
	}
	// A zero window means quorum and completion can land in the same block, and
	// nobody can object to a transfer they never had time to see.
	if p.ChallengeWindow <= 0 {
		return fmt.Errorf("challenge window must be positive")
	}
	return nil
}
