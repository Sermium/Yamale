package types

import "fmt"

// DefaultGenesis returns a platform where nobody has left yet.
func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

// Validate refuses a genesis the module could not have produced.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	disclosed := map[string]bool{}
	for _, d := range gs.Disclosures {
		if d.Account == "" {
			return fmt.Errorf("a disclosure names no account")
		}
		if disclosed[d.Account] {
			return fmt.Errorf("account %s is disclosed twice; a key is only taken once", d.Account)
		}
		disclosed[d.Account] = true

		// The gap between asking and confirming is the evidence that the
		// waiting period was served. A confirmation before the request is not
		// a record this module could have written.
		if d.DisclosedAt < d.RequestedAt {
			return fmt.Errorf(
				"account %s was disclosed at %d, before it was requested at %d",
				d.Account, d.DisclosedAt, d.RequestedAt)
		}
		if d.AcknowledgementHash == "" {
			return fmt.Errorf(
				"the disclosure for %s records no acknowledgement; it cannot be shown what they agreed to",
				d.Account)
		}
	}

	for _, r := range gs.Requests {
		if r.Account == "" {
			return fmt.Errorf("a request names no account")
		}
		// A pending request against an account whose key is already out would
		// be a second disclosure of a key that cannot be disclosed twice.
		if disclosed[r.Account] {
			return fmt.Errorf(
				"account %s has a pending request and an existing disclosure", r.Account)
		}
	}

	return nil
}
