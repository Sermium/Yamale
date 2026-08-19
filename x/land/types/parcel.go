package types

// ForbidsFractionalisation reports whether a standing restriction bars selling
// this parcel in shares.
//
// A lifted restriction does not count: restrictions are marked rather than
// removed, so the record still shows the land was once constrained and who
// released it, and reading the list without that test would enforce a limit an
// office has lawfully lifted.
//
// It lives on the record rather than in the two keepers that ask, because the
// registry and x/tokenisation disagreeing about what a restriction forbids is
// the failure the restriction exists to prevent.
func (p Parcel) ForbidsFractionalisation() bool {
	for _, r := range p.Restrictions {
		if !r.Lifted && r.Kind == RestrictionNoFractionalisation {
			return true
		}
	}
	return false
}

// LiveFreeze returns the index of the freeze currently in force, or -1.
//
// Searched from the end because freezes accumulate and only the last one can be
// live: the keeper refuses a second freeze on an already-frozen parcel, so a
// parcel frozen, released and frozen again carries two entries of which exactly
// one is unlifted.
//
// -1 is a real answer rather than an error, and the callers must handle it. A
// registry imported from a genesis written before freezes were recorded has
// parcels that are FROZEN with no entry to lift, and the alternative —
// manufacturing an entry at import to make the state look tidy — would put
// derived data in the export that InitGenesis never received. That is the
// mismatch that breaks an import/export round trip.
func (p Parcel) LiveFreeze() int {
	for i := len(p.Freezes) - 1; i >= 0; i-- {
		if !p.Freezes[i].Lifted {
			return i
		}
	}
	return -1
}

// Live reports whether an authorisation still permits issuance at unix second
// now.
//
// Withdrawal and expiry are tested together and in one place: a wallet that
// computes "live" differently from the keeper will eventually show somebody a
// permission the chain would refuse, and the moment it does is the moment they
// pay for shares that cannot be issued. The parcel's restrictions are checked
// separately by the caller, which holds the parcel.
func (a FractionalisationAuthority) Live(now int64) bool {
	return !a.Withdrawn && a.ExpiresAt > now
}
