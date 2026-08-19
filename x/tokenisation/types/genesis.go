package types

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params: DefaultParams(),
		// Ids start at 1: zero is indistinguishable from an unset proto field.
		NextAssetId: 1,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	for _, c := range gs.Collections {
		if c.Verification == VERIFY_ATTESTORS && c.AttestationThreshold < 2 {
			return ErrInvalidThreshold
		}
	}
	// Shares already issued over each land parcel. Summed here so an import
	// cannot start out over a ceiling the running chain would have refused —
	// the keeper only ever compares an increment, so a state that arrives
	// already breached would never be noticed.
	overParcel := map[uint64]uint64{}
	for _, a := range gs.Assets {
		if a.HolderShareBps > 10_000 {
			return ErrInvalidShare
		}
		// A fractionalised asset with no share, or a share with no denom, is a
		// vehicle whose income has nowhere to go.
		if (a.FractionDenom == "") != (a.HolderShareBps == 0) {
			return ErrInvalidShare
		}
		if a.ParcelId != 0 && a.FractionDenom != "" {
			overParcel[a.ParcelId] += uint64(a.HolderShareBps)
			if overParcel[a.ParcelId] > 10_000 {
				return ErrShareCeilingExceeded
			}
		}
	}
	return nil
}
