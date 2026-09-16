package types

import sdk "github.com/cosmos/cosmos-sdk/types"

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
	// Every amount on a vault, checked before the chain is running.
	//
	// A Coin whose amount field is simply absent from the file decodes with a
	// nil math.Int, and math.Int's zero value panics on arithmetic rather than
	// erroring. So `{"denom": "uusd"}` in a hand-edited genesis is not a bad
	// number, it is a validator that halts on the first transfer touching that
	// vehicle — and it halts in InitGenesis's successors rather than here,
	// where nobody is looking. Three fields rather than one because
	// dividend_reserve arrived last and would otherwise be the only one
	// unguarded, which is how the next person learns this the hard way.
	for _, v := range gs.Vaults {
		for _, set := range [][]sdk.Coin{v.Held, v.Funded, v.DividendReserve} {
			for _, c := range set {
				if c.Amount.IsNil() {
					return ErrInvalidAmount.Wrapf(
						"vault for asset %d carries %q with no amount", v.AssetId, c.Denom)
				}
			}
		}
	}
	return nil
}
