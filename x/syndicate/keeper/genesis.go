package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/syndicate/types"
)

// InitGenesis writes a platform in, rebuilding the indexes from the records.
//
// The reference and denomination indexes are derived rather than exported: they
// are pure functions of the deals, so exporting them would be exporting the
// same fact twice and inviting the two copies to disagree. Positions are NOT
// derived, and the difference is the whole of the matter — a position is a
// running total of what a holder has already been paid, which cannot be
// recomputed from anything else in the file.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}

	for _, o := range gs.Operators {
		if err := k.Operator.Set(ctx, o.Address, o); err != nil {
			return err
		}
	}
	for _, i := range gs.Investors {
		if err := k.Investor.Set(ctx, i.Address, i); err != nil {
			return err
		}
	}
	for _, d := range gs.Deals {
		if err := k.Deal.Set(ctx, d.Id, d); err != nil {
			return err
		}
		if err := k.ByDealRef.Set(ctx, d.Reference, d.Id); err != nil {
			return err
		}
		if d.Denom != "" {
			if err := k.ByDenom.Set(ctx, d.Denom, d.Id); err != nil {
				return err
			}
		}
	}
	for _, c := range gs.Commitments {
		if err := k.Commitment.Set(ctx, collections.Join(c.DealId, c.Investor), c); err != nil {
			return err
		}
	}
	for _, p := range gs.Positions {
		if err := k.Position.Set(ctx, collections.Join(p.DealId, p.Holder), p); err != nil {
			return err
		}
	}
	for _, l := range gs.Listings {
		if err := k.Listing.Set(ctx, l.Id, l); err != nil {
			return err
		}
		if err := k.ListingsByDeal.Set(ctx, collections.Join(l.DealId, l.Id)); err != nil {
			return err
		}
	}

	for _, d := range gs.Documents {
		if err := k.Document.Set(ctx, d.Id, d); err != nil {
			return err
		}
		if err := k.DocumentsByDeal.Set(ctx, collections.Join(d.DealId, d.Id)); err != nil {
			return err
		}
	}

	if err := k.NextDealID.Set(ctx, gs.NextDealId); err != nil {
		return err
	}
	if err := k.NextListingID.Set(ctx, gs.NextListingId); err != nil {
		return err
	}
	return k.NextDocumentID.Set(ctx, gs.NextDocumentId)
}

// ExportGenesis reads the platform out.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	gs := types.GenesisState{Params: params}

	if err := k.Operator.Walk(ctx, nil, func(_ string, o types.Operator) (bool, error) {
		gs.Operators = append(gs.Operators, o)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Investor.Walk(ctx, nil, func(_ string, i types.Investor) (bool, error) {
		gs.Investors = append(gs.Investors, i)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Deal.Walk(ctx, nil, func(_ uint64, d types.Deal) (bool, error) {
		gs.Deals = append(gs.Deals, d)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Commitment.Walk(ctx, nil,
		func(_ collections.Pair[uint64, string], c types.Commitment) (bool, error) {
			gs.Commitments = append(gs.Commitments, c)
			return false, nil
		}); err != nil {
		return nil, err
	}
	if err := k.Position.Walk(ctx, nil,
		func(_ collections.Pair[uint64, string], p types.Position) (bool, error) {
			gs.Positions = append(gs.Positions, p)
			return false, nil
		}); err != nil {
		return nil, err
	}
	if err := k.Listing.Walk(ctx, nil, func(_ uint64, l types.Listing) (bool, error) {
		gs.Listings = append(gs.Listings, l)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Document.Walk(ctx, nil, func(_ uint64, d types.Document) (bool, error) {
		gs.Documents = append(gs.Documents, d)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if gs.NextDealId, err = k.NextDealID.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.NextListingId, err = k.NextListingID.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.NextDocumentId, err = k.NextDocumentID.Peek(ctx); err != nil {
		return nil, err
	}
	return &gs, nil
}
