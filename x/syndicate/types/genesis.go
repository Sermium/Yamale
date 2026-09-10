package types

import (
	"fmt"

	"cosmossdk.io/math"
)

// DefaultGenesis returns an empty platform with the protective ceilings.
func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

// Validate refuses a genesis the module could not have produced.
//
// The counter checks are the ones that matter most. A counter sitting at or
// below an id already present hands that id out again on the next deal, and the
// second record overwrites the first — silently, because a write to an existing
// key is not an error. On a registry that loses a herd, that is bad. Here it is
// two syndicates sharing an identifier, one of which has investors' tokens
// denominated against it.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	seenRef := map[string]uint64{}
	seenDenom := map[string]uint64{}
	deals := map[uint64]Deal{}

	for _, d := range gs.Deals {
		if d.Id == 0 {
			return fmt.Errorf(
				"deal 0 is never issued; a zero id is indistinguishable from an unset field")
		}
		if d.Id >= gs.NextDealId {
			return fmt.Errorf(
				"deal %d exists but next_deal_id is %d, so the next deal would overwrite it",
				d.Id, gs.NextDealId)
		}
		if prev, ok := seenRef[d.Reference]; ok {
			return fmt.Errorf("deals %d and %d share the reference %q", prev, d.Id, d.Reference)
		}
		seenRef[d.Reference] = d.Id

		// A denomination shared by two deals is two different claims that a
		// wallet, an explorer and the bank module all treat as one asset.
		if d.Denom != "" {
			if prev, ok := seenDenom[d.Denom]; ok {
				return fmt.Errorf("deals %d and %d share the denomination %q", prev, d.Id, d.Denom)
			}
			seenDenom[d.Denom] = d.Id
		}

		if err := validateDealShape(d); err != nil {
			return err
		}
		deals[d.Id] = d
	}

	for _, c := range gs.Commitments {
		d, ok := deals[c.DealId]
		if !ok {
			return fmt.Errorf("commitment by %s names deal %d, which is not here",
				c.Investor, c.DealId)
		}
		// Escrowed money against a deal that is no longer taking any is money
		// the module would never refund and never allocate.
		if d.State != DEAL_STATE_OPEN {
			return fmt.Errorf(
				"deal %d is %s but still holds a commitment from %s; that money has no path out",
				d.Id, d.State, c.Investor)
		}
		if !c.Amount.IsPositive() {
			return fmt.Errorf("the commitment by %s to deal %d is not positive", c.Investor, c.DealId)
		}
	}

	for _, p := range gs.Positions {
		d, ok := deals[p.DealId]
		if !ok {
			return fmt.Errorf("position for %s names deal %d, which is not here", p.Holder, p.DealId)
		}
		if p.Index.IsNil() || p.Index.IsNegative() {
			return fmt.Errorf("the position for %s in deal %d has no index", p.Holder, p.DealId)
		}
		// A position ahead of its deal's index would earn nothing for ever, and
		// the arithmetic that produced it is wrong somewhere upstream.
		if p.Index.GT(d.DistributionIndex) {
			return fmt.Errorf(
				"the position for %s in deal %d is at index %s, ahead of the deal's %s",
				p.Holder, p.DealId, p.Index, d.DistributionIndex)
		}
	}

	for _, l := range gs.Listings {
		if l.Id == 0 {
			return fmt.Errorf("listing 0 is never issued")
		}
		if l.Id >= gs.NextListingId {
			return fmt.Errorf(
				"listing %d exists but next_listing_id is %d", l.Id, gs.NextListingId)
		}
		if _, ok := deals[l.DealId]; !ok {
			return fmt.Errorf("listing %d names deal %d, which is not here", l.Id, l.DealId)
		}
	}

	return nil
}

// validateDealShape refuses a deal whose own numbers contradict each other.
func validateDealShape(d Deal) error {
	for name, v := range map[string]math.Int{
		"target":             d.Target,
		"minimum":            d.Minimum,
		"minimum_ticket":     d.MinimumTicket,
		"raised":             d.Raised,
		"supply":             d.Supply,
		"distribution_index": d.DistributionIndex,
		"distributed":        d.Distributed,
	} {
		if v.IsNil() {
			return fmt.Errorf("deal %d has no %s", d.Id, name)
		}
		if v.IsNegative() {
			return fmt.Errorf("deal %d has a negative %s", d.Id, name)
		}
	}

	// A funded deal must have a denomination and a supply; an unfunded one must
	// have neither. The half-state — a denomination with no supply — would be a
	// tradeable claim on nothing.
	funded := d.State == DEAL_STATE_CLOSED || d.State == DEAL_STATE_REALISED
	if funded && (d.Denom == "" || !d.Supply.IsPositive()) {
		return fmt.Errorf("deal %d is %s but has no tokens in issue", d.Id, d.State)
	}
	if !funded && (d.Denom != "" || d.Supply.IsPositive()) {
		return fmt.Errorf(
			"deal %d is %s but carries a denomination; that is a claim on nothing",
			d.Id, d.State)
	}
	return nil
}
