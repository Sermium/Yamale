package types

func DefaultParams() Params {
	return Params{
		// A day is the floor because a window shorter than the time it takes a
		// dispersed holder to notice is not a protection, it is a formality.
		MinChallengeWindowSeconds: 86_400,
		MaxChallengeWindowSeconds: 86_400 * 90,
		MaxDisputeBondBps:         500,
	}
}

func (p Params) Validate() error {
	if p.MinChallengeWindowSeconds <= 0 || p.MaxChallengeWindowSeconds < p.MinChallengeWindowSeconds {
		return ErrInvalidParams
	}
	if p.MaxDisputeBondBps > 10_000 {
		return ErrInvalidParams
	}
	return nil
}
