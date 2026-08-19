package types

import "fmt"

// DefaultGenesis returns the default genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

// Validate performs basic genesis state validation.
//
// The appraiser checks are the ones worth having: an appraisal signed by
// somebody who is not in the genesis appraiser set would be a valuation with no
// accountable author, which is the one property this module exists to
// guarantee.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	seenRate := make(map[string]bool, len(gs.ExchangeRates))
	for _, r := range gs.ExchangeRates {
		if seenRate[r.Denom] {
			return fmt.Errorf("duplicate exchange rate for %s", r.Denom)
		}
		seenRate[r.Denom] = true
	}

	appraisers := make(map[string]bool, len(gs.Appraisers))
	for _, a := range gs.Appraisers {
		if a.Address == "" {
			return fmt.Errorf("appraiser with no address")
		}
		if appraisers[a.Address] {
			return fmt.Errorf("duplicate appraiser %s", a.Address)
		}
		if a.Status == AppraiserStatus_APPRAISER_STATUS_UNSPECIFIED {
			return fmt.Errorf("appraiser %s has an unspecified status", a.Address)
		}
		appraisers[a.Address] = true
	}

	seenAsset := make(map[string]bool, len(gs.Appraisals))
	for _, ap := range gs.Appraisals {
		key := ap.ClassId + "/" + ap.NftId
		if seenAsset[key] {
			return fmt.Errorf("more than one current appraisal for %s", key)
		}
		seenAsset[key] = true
		// A superseded valuation in the current set would be read as what the
		// asset is worth today while being flagged as replaced, which is a
		// contradiction the chain would then act on.
		if ap.Superseded {
			return fmt.Errorf("current appraisal for %s is marked superseded; it belongs in appraisal_history", key)
		}
		if err := validateAppraisal(ap, appraisers); err != nil {
			return err
		}
	}

	for _, ap := range gs.AppraisalHistory {
		if !ap.Superseded {
			return fmt.Errorf("historic appraisal for %s/%s is not marked superseded", ap.ClassId, ap.NftId)
		}
		if err := validateAppraisal(ap, appraisers); err != nil {
			return err
		}
	}

	seenFeeder := make(map[string]bool, len(gs.FeederDelegations))
	for _, d := range gs.FeederDelegations {
		if seenFeeder[d.Validator] {
			return fmt.Errorf("duplicate feeder delegation for %s", d.Validator)
		}
		seenFeeder[d.Validator] = true
	}

	return nil
}

// validateAppraisal applies the checks a valuation must pass wherever it
// appears — a number with no accountable author is the one thing this module
// exists to prevent, so it is checked in the history too rather than only in
// the current set.
func validateAppraisal(ap Appraisal, appraisers map[string]bool) error {
	key := ap.ClassId + "/" + ap.NftId
	if !appraisers[ap.Appraiser] {
		return fmt.Errorf("appraisal for %s was signed by %s, who is not an appraiser", key, ap.Appraiser)
	}
	if ap.ValueDenom == "" {
		return fmt.Errorf("appraisal for %s has no denom", key)
	}
	if ap.ValuedAt <= 0 {
		return fmt.Errorf("appraisal for %s has no valuation date", key)
	}
	return nil
}
