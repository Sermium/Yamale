package types

import "fmt"

// DefaultGenesis returns the default genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:      DefaultParams(),
		Assets:      []Asset{},
		Attestors:   []string{},
		Deposits:    []Deposit{},
		Redemptions: []Redemption{},
		Reserves:    []Reserve{},
	}
}

// Validate checks the invariants the keeper relies on, not merely the shapes.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	assets := make(map[string]struct{}, len(gs.Assets))
	for _, a := range gs.Assets {
		if a.Denom == "" {
			return fmt.Errorf("asset with an empty denom")
		}
		if _, dup := assets[a.Denom]; dup {
			return fmt.Errorf("asset %q registered twice", a.Denom)
		}
		assets[a.Denom] = struct{}{}
	}

	// An attestor list shorter than the threshold cannot ever credit a deposit;
	// that is a chain that silently accepts money and never issues the claim.
	if len(gs.Attestors) > 0 && uint32(len(gs.Attestors)) < gs.Params.AttestationThreshold {
		return fmt.Errorf("%d attestors cannot reach a threshold of %d",
			len(gs.Attestors), gs.Params.AttestationThreshold)
	}

	refs := make(map[string]struct{}, len(gs.Deposits))
	for _, d := range gs.Deposits {
		if _, ok := assets[d.Denom]; !ok {
			return fmt.Errorf("deposit %s is for unregistered asset %q", d.Id, d.Denom)
		}
		if d.Amount.IsNil() || !d.Amount.IsPositive() {
			return fmt.Errorf("deposit %s has a non-positive amount", d.Id)
		}
		// Credited deposits must have unique external references, or the same
		// external payment was turned into a claim more than once.
		if d.Status == DepositStatus_DEPOSIT_STATUS_CREDITED {
			key := d.Denom + "|" + d.ExternalRef
			if _, dup := refs[key]; dup {
				return fmt.Errorf("external reference %q credited twice for %s", d.ExternalRef, d.Denom)
			}
			refs[key] = struct{}{}
		}
	}

	for _, r := range gs.Redemptions {
		if _, ok := assets[r.Denom]; !ok {
			return fmt.Errorf("redemption %s is for unregistered asset %q", r.Id, r.Denom)
		}
		if r.Amount.IsNil() || !r.Amount.IsPositive() {
			return fmt.Errorf("redemption %s has a non-positive amount", r.Id)
		}
	}

	for _, r := range gs.Reserves {
		if _, ok := assets[r.Denom]; !ok {
			return fmt.Errorf("reserve reported for unregistered asset %q", r.Denom)
		}
		if r.Held.IsNil() || r.Held.IsNegative() {
			return fmt.Errorf("reserve for %q is negative", r.Denom)
		}
	}
	return nil
}
