package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"yamale/blockchain/x/jewel/types"
)

type queryServer struct{ k Keeper }

// NewQueryServerImpl returns the register's Query service.
func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{k: k} }

var _ types.QueryServer = queryServer{}

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Jeweler(ctx context.Context, req *types.QueryJewelerRequest) (*types.QueryJewelerResponse, error) {
	j, err := q.k.getJeweler(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &types.QueryJewelerResponse{Jeweler: j}, nil
}

func (q queryServer) Jewelers(ctx context.Context, req *types.QueryJewelersRequest) (*types.QueryJewelersResponse, error) {
	jewelers, page, err := query.CollectionPaginate(ctx, q.k.Jeweler, req.Pagination,
		func(_ uint64, j types.Jeweler) (types.Jeweler, error) { return j, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryJewelersResponse{Jewelers: jewelers, Pagination: page}, nil
}

func (q queryServer) Jewel(ctx context.Context, req *types.QueryJewelRequest) (*types.QueryJewelResponse, error) {
	j, err := q.k.getJewel(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	owner, err := q.k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	res := &types.QueryJewelResponse{Jewel: j, Owner: owner}
	if o, err := q.k.Offer.Get(ctx, j.Id); err == nil {
		res.Offer = &o
	} else if !isNotFound(err) {
		return nil, err
	}
	if r, err := q.k.Reassignment.Get(ctx, j.Id); err == nil {
		res.Reassignment = &r
	} else if !isNotFound(err) {
		return nil, err
	}
	if res.HasCard, err = q.k.Card.Has(ctx, j.Id); err != nil {
		return nil, err
	}
	if c, err := q.k.Claim.Get(ctx, j.Id); err == nil {
		res.Claim = &c
	} else if !isNotFound(err) {
		return nil, err
	}
	if l, err := q.k.Listing.Get(ctx, j.Id); err == nil {
		res.Listing = &l
	} else if !isNotFound(err) {
		return nil, err
	}
	err = q.k.BenefitsByJewel.Walk(ctx, collections.NewPrefixedPairRange[uint64, uint64](j.Id),
		func(key collections.Pair[uint64, uint64]) (bool, error) {
			b, err := q.k.Benefit.Get(ctx, key.K2())
			if err != nil {
				return true, err
			}
			res.Benefits = append(res.Benefits, b)
			return false, nil
		})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (q queryServer) JewelsByInscription(ctx context.Context, req *types.QueryJewelsByInscriptionRequest) (*types.QueryJewelsByInscriptionResponse, error) {
	code, err := types.NormaliseInscription(req.Inscription)
	if err != nil {
		return nil, err
	}
	res := &types.QueryJewelsByInscriptionResponse{}
	err = q.k.Search.Walk(ctx, collections.NewPrefixedPairRange[string, uint64](code),
		func(key collections.Pair[string, uint64]) (bool, error) {
			j, err := q.k.Jewel.Get(ctx, key.K2())
			if err != nil {
				return true, err
			}
			res.Jewels = append(res.Jewels, j)
			return len(res.Jewels) >= types.MaxSearchResults, nil
		})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (q queryServer) ClaimsByJeweler(ctx context.Context, req *types.QueryClaimsByJewelerRequest) (*types.QueryClaimsByJewelerResponse, error) {
	if _, err := q.k.getJeweler(ctx, req.JewelerId); err != nil {
		return nil, err
	}
	claims, page, err := query.CollectionPaginate(ctx, q.k.ClaimsByJeweler, req.Pagination,
		func(key collections.Pair[uint64, uint64], _ collections.NoValue) (types.ClaimEntry, error) {
			j, err := q.k.Jewel.Get(ctx, key.K2())
			if err != nil {
				return types.ClaimEntry{}, err
			}
			c, err := q.k.Claim.Get(ctx, key.K2())
			if err != nil {
				return types.ClaimEntry{}, err
			}
			return types.ClaimEntry{Jewel: j, Claim: c}, nil
		},
		query.WithCollectionPaginationPairPrefix[uint64, uint64](req.JewelerId))
	if err != nil {
		return nil, err
	}
	return &types.QueryClaimsByJewelerResponse{Claims: claims, Pagination: page}, nil
}

// Listings pages through the marketplace.
func (q queryServer) Listings(ctx context.Context, req *types.QueryListingsRequest) (*types.QueryListingsResponse, error) {
	list, page, err := query.CollectionPaginate(ctx, q.k.Listing, req.Pagination,
		func(id uint64, l types.Listing) (types.ListingEntry, error) {
			j, err := q.k.Jewel.Get(ctx, id)
			if err != nil {
				return types.ListingEntry{}, err
			}
			return types.ListingEntry{Jewel: j, Listing: l}, nil
		})
	if err != nil {
		return nil, err
	}
	return &types.QueryListingsResponse{Listings: list, Pagination: page}, nil
}
