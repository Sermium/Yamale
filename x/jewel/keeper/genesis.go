package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/jewel/types"
)

// requireCanonicalAddress refuses an address that the handlers would not have
// stored: the handlers bring every address to its canonical spelling first.
func (k Keeper) requireCanonicalAddress(addr string) error {
	c, err := k.canonical(addr)
	if err != nil {
		return err
	}
	if c != addr {
		return fmt.Errorf("address %s is not in canonical form %s", addr, c)
	}
	return nil
}

// checkCanonicalAddresses holds every address in the genesis to the canonical
// form. Genesis writes addresses without a handler, so this is the only place
// they are checked.
func (k Keeper) checkCanonicalAddresses(gs types.GenesisState) error {
	if gs.Params.Office != "" {
		if err := k.requireCanonicalAddress(gs.Params.Office); err != nil {
			return fmt.Errorf("params.office: %w", err)
		}
	}
	for _, j := range gs.Jewelers {
		if err := k.requireCanonicalAddress(j.Business); err != nil {
			return fmt.Errorf("jeweler %d business: %w", j.Id, err)
		}
		for _, s := range j.Staff {
			if err := k.requireCanonicalAddress(s); err != nil {
				return fmt.Errorf("jeweler %d staff: %w", j.Id, err)
			}
		}
	}
	for _, o := range gs.Offers {
		if err := k.requireCanonicalAddress(o.From); err != nil {
			return fmt.Errorf("offer on jewel %d from: %w", o.JewelId, err)
		}
		if err := k.requireCanonicalAddress(o.Maker); err != nil {
			return fmt.Errorf("offer on jewel %d maker: %w", o.JewelId, err)
		}
		if o.To != "" {
			if err := k.requireCanonicalAddress(o.To); err != nil {
				return fmt.Errorf("offer on jewel %d to: %w", o.JewelId, err)
			}
		}
	}
	for _, r := range gs.Reassignments {
		if err := k.requireCanonicalAddress(r.From); err != nil {
			return fmt.Errorf("reassignment of jewel %d from: %w", r.JewelId, err)
		}
		if err := k.requireCanonicalAddress(r.To); err != nil {
			return fmt.Errorf("reassignment of jewel %d to: %w", r.JewelId, err)
		}
	}
	for _, c := range gs.Cards {
		if err := k.requireCanonicalAddress(c.Card.IssuedBy); err != nil {
			return fmt.Errorf("card of jewel %d issued_by: %w", c.JewelId, err)
		}
	}
	for _, c := range gs.Claims {
		if err := k.requireCanonicalAddress(c.Claim.Claimer); err != nil {
			return fmt.Errorf("claim on jewel %d claimer: %w", c.JewelId, err)
		}
	}
	for _, l := range gs.Listings {
		if err := k.requireCanonicalAddress(l.Listing.Seller); err != nil {
			return fmt.Errorf("listing of jewel %d seller: %w", l.JewelId, err)
		}
	}
	return nil
}

// InitGenesis writes the register in and rebuilds every index from the
// records. The indexes are not exported: they are functions of the records,
// and two copies of one fact eventually disagree.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.checkCanonicalAddresses(gs); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, j := range gs.Jewelers {
		if err := k.Jeweler.Set(ctx, j.Id, j); err != nil {
			return err
		}
		if err := k.ByBusiness.Set(ctx, j.Business, j.Id); err != nil {
			return err
		}
		for _, s := range j.Staff {
			if err := k.ByStaff.Set(ctx, s, j.Id); err != nil {
				return err
			}
		}
	}
	for _, j := range gs.Jewels {
		if err := k.Jewel.Set(ctx, j.Id, j); err != nil {
			return err
		}
		if err := k.ByInscription.Set(ctx, collections.Join(j.JewelerId, j.Inscription), j.Id); err != nil {
			return err
		}
		if err := k.Search.Set(ctx, collections.Join(j.Inscription, j.Id)); err != nil {
			return err
		}
	}
	for _, o := range gs.Offers {
		if err := k.Offer.Set(ctx, o.JewelId, o); err != nil {
			return err
		}
	}
	for _, b := range gs.Benefits {
		if err := k.Benefit.Set(ctx, b.Id, b); err != nil {
			return err
		}
		if err := k.BenefitsByJewel.Set(ctx, collections.Join(b.JewelId, b.Id)); err != nil {
			return err
		}
	}
	for _, n := range gs.UsedNonces {
		if err := k.UsedNonce.Set(ctx, collections.Join(n.BenefitId, n.Nonce)); err != nil {
			return err
		}
	}
	for _, r := range gs.Reassignments {
		if err := k.Reassignment.Set(ctx, r.JewelId, r); err != nil {
			return err
		}
	}
	for _, c := range gs.Cards {
		if err := k.Card.Set(ctx, c.JewelId, c.Card); err != nil {
			return err
		}
	}
	for _, l := range gs.Listings {
		if err := k.Listing.Set(ctx, l.JewelId, l.Listing); err != nil {
			return err
		}
	}
	// ClaimsByJeweler is rebuilt here from each claim's jewel; Validate has
	// already checked that every claimed jewel is in this genesis.
	jewelerOf := make(map[uint64]uint64, len(gs.Jewels))
	for _, j := range gs.Jewels {
		jewelerOf[j.Id] = j.JewelerId
	}
	for _, c := range gs.Claims {
		j := types.Jewel{Id: c.JewelId, JewelerId: jewelerOf[c.JewelId]}
		if err := k.setClaim(ctx, j, c.Claim); err != nil {
			return err
		}
	}
	// A zero counter is not written: a fresh chain has no key there, and
	// writing one would make the imported store differ from the exported one.
	for _, s := range []struct {
		seq collections.Sequence
		v   uint64
	}{{k.NextJewelerID, gs.NextJewelerId}, {k.NextJewelID, gs.NextJewelId}, {k.NextBenefitID, gs.NextBenefitId}} {
		if s.v != 0 {
			if err := s.seq.Set(ctx, s.v); err != nil {
				return err
			}
		}
	}
	return nil
}

// ExportGenesis writes the register out, records in key order.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	gs := &types.GenesisState{Params: p}

	if err := k.Jeweler.Walk(ctx, nil, func(_ uint64, v types.Jeweler) (bool, error) {
		gs.Jewelers = append(gs.Jewelers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Jewel.Walk(ctx, nil, func(_ uint64, v types.Jewel) (bool, error) {
		gs.Jewels = append(gs.Jewels, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Offer.Walk(ctx, nil, func(_ uint64, v types.Offer) (bool, error) {
		gs.Offers = append(gs.Offers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Benefit.Walk(ctx, nil, func(_ uint64, v types.Benefit) (bool, error) {
		gs.Benefits = append(gs.Benefits, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.UsedNonce.Walk(ctx, nil, func(key collections.Pair[uint64, string]) (bool, error) {
		gs.UsedNonces = append(gs.UsedNonces, types.UsedNonce{BenefitId: key.K1(), Nonce: key.K2()})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Reassignment.Walk(ctx, nil, func(_ uint64, v types.Reassignment) (bool, error) {
		gs.Reassignments = append(gs.Reassignments, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Card.Walk(ctx, nil, func(id uint64, v types.Card) (bool, error) {
		gs.Cards = append(gs.Cards, types.GenesisCard{JewelId: id, Card: v})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Claim.Walk(ctx, nil, func(id uint64, v types.Claim) (bool, error) {
		gs.Claims = append(gs.Claims, types.GenesisClaim{JewelId: id, Claim: v})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Listing.Walk(ctx, nil, func(id uint64, v types.Listing) (bool, error) {
		gs.Listings = append(gs.Listings, types.GenesisListing{JewelId: id, Listing: v})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if gs.NextJewelerId, err = k.NextJewelerID.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.NextJewelId, err = k.NextJewelID.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.NextBenefitId, err = k.NextBenefitID.Peek(ctx); err != nil {
		return nil, err
	}
	return gs, nil
}
