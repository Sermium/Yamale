package types

import "fmt"

// DefaultParams are a starting position, not a finding. Every one of them
// decides how hard it is to launder a stolen animal or invent a pedigree, and
// they should be argued about by whoever adopts this.
func DefaultParams() Params {
	return Params{
		// Seven days, not land's fourteen. Livestock changes hands at a market
		// on a Tuesday and a fortnight's wait would simply be ignored — and a
		// window everybody routes around protects nobody.
		ChallengeWindow: 7 * 24 * 60 * 60,
		// Two independent attestors. Lower than land's three because the pool
		// of accredited veterinarians in a pastoral zone is small, and a quorum
		// that cannot be met in practice is a transfer that never completes
		// lawfully — which pushes the trade off the registry entirely.
		AttestationQuorum: 2,
		// An attestor from the proposing office is not independent.
		SameAuthorityAttestation: false,
		// 140 days. Below a ewe's roughly 147-day gestation, so a real interval
		// is never refused, and far above the fortnight that would betray a
		// fabricated line.
		MinBirthIntervalDays: 140,
		// A pedigree without a laboratory is the declaring breeder's word for
		// who the sire was, which is exactly what the paper certificates
		// already are.
		RequireDnaForCertified: true,
	}
}

// Validate refuses parameters that would quietly disable the protections.
func (p Params) Validate() error {
	// A quorum of zero lets a transfer complete with nobody having looked at
	// the animal, which is the one claim the chain cannot make for itself.
	if p.AttestationQuorum == 0 {
		return fmt.Errorf(
			"attestation_quorum must be at least 1; at zero an animal changes keeper " +
				"with nobody attesting that it is the animal on the record")
	}
	if p.ChallengeWindow <= 0 {
		return fmt.Errorf(
			"challenge_window must be positive; without one a transfer completes the "+
				"moment it is attested and nobody can object in time, got %d",
			p.ChallengeWindow)
	}
	// Zero would refuse every second birth as impossible, which is not a
	// stricter registry but an unusable one.
	if p.MinBirthIntervalDays == 0 {
		return fmt.Errorf(
			"min_birth_interval_days must be positive; at zero the registry cannot " +
				"tell an impossible declaration from an ordinary one")
	}
	// A year and a half is longer than any gestation plus recovery, so a value
	// above it refuses real births rather than false ones.
	if p.MinBirthIntervalDays > 540 {
		return fmt.Errorf(
			"min_birth_interval_days is %d, which is longer than any gestation this "+
				"registry covers and would refuse ordinary births",
			p.MinBirthIntervalDays)
	}
	return nil
}
