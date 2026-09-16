package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/market/types"
)

type queryServer struct {
	Keeper
}

// NewQueryServerImpl returns the read side of the market.
func NewQueryServerImpl(k Keeper) types.QueryServer {
	return queryServer{Keeper: k}
}

var _ types.QueryServer = queryServer{}

func (q queryServer) Params(
	ctx context.Context, _ *types.QueryParamsRequest,
) (*types.QueryParamsResponse, error) {
	params, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

func (q queryServer) Listing(
	ctx context.Context, req *types.QueryListingRequest,
) (*types.QueryListingResponse, error) {
	listing, err := q.Keeper.Listing.Get(ctx, req.Id)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "listing %d", req.Id)
		}
		return nil, err
	}
	return &types.QueryListingResponse{Listing: listing}, nil
}

func (q queryServer) Listings(
	ctx context.Context, req *types.QueryListingsRequest,
) (*types.QueryListingsResponse, error) {
	out := &types.QueryListingsResponse{}
	rng := collections.NewPrefixedPairRange[string, uint64](req.Denom)
	err := q.ListingsByDenom.Walk(ctx, rng,
		func(key collections.Pair[string, uint64]) (bool, error) {
			listing, err := q.Keeper.Listing.Get(ctx, key.K2())
			if err != nil {
				if isNotFound(err) {
					return false, nil
				}
				return true, err
			}
			if req.LiveOnly && listing.State != types.LISTING_STATE_LIVE {
				return false, nil
			}
			out.Listings = append(out.Listings, listing)
			return false, nil
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (q queryServer) Auction(
	ctx context.Context, req *types.QueryAuctionRequest,
) (*types.QueryAuctionResponse, error) {
	auction, err := q.Keeper.Auction.Get(ctx, req.Id)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "auction %d", req.Id)
		}
		return nil, err
	}
	return &types.QueryAuctionResponse{Auction: auction}, nil
}

func (q queryServer) Auctions(
	ctx context.Context, req *types.QueryAuctionsRequest,
) (*types.QueryAuctionsResponse, error) {
	out := &types.QueryAuctionsResponse{}
	rng := collections.NewPrefixedPairRange[string, uint64](req.Denom)
	err := q.AuctionsByDenom.Walk(ctx, rng,
		func(key collections.Pair[string, uint64]) (bool, error) {
			auction, err := q.Keeper.Auction.Get(ctx, key.K2())
			if err != nil {
				if isNotFound(err) {
					return false, nil
				}
				return true, err
			}
			if req.OpenOnly && auction.State != types.AUCTION_STATE_OPEN {
				return false, nil
			}
			out.Auctions = append(out.Auctions, auction)
			return false, nil
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (q queryServer) Order(
	ctx context.Context, req *types.QueryOrderRequest,
) (*types.QueryOrderResponse, error) {
	order, err := q.Keeper.Order.Get(ctx, req.Id)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "order %d", req.Id)
		}
		return nil, err
	}
	return &types.QueryOrderResponse{Order: order}, nil
}

// Book returns both sides, best price first, bounded.
//
// The depth limit is a bound on a READ, which is the unusual one in this module.
// The book is the only structure here whose size grows with how well the market
// is doing, so an unbounded book query is a public endpoint that becomes more
// expensive to abuse the more successful the chain becomes. An interface showing
// a ladder wants twenty levels and never asked for the rest.
func (q queryServer) Book(
	ctx context.Context, req *types.QueryBookRequest,
) (*types.QueryBookResponse, error) {
	depth := req.Depth
	if depth == 0 {
		depth = types.DefaultBookDepth
	}
	if depth > types.MaxBookDepth {
		depth = types.MaxBookDepth
	}

	out := &types.QueryBookResponse{}

	// Best-effort: a book over a denomination this chain has no vehicle for is
	// empty rather than an error, because "no such market" and "an empty market"
	// are the same answer to the question a client is asking.
	if _, settlementDenom, _, err := q.tokenisationKeeper.ShareAsset(ctx, req.Denom); err == nil {
		out.SettlementDenom = settlementDenom
	}

	bids, err := q.side(ctx, q.Bids, req.Denom, depth)
	if err != nil {
		return nil, err
	}
	asks, err := q.side(ctx, q.Asks, req.Denom, depth)
	if err != nil {
		return nil, err
	}
	out.Bids, out.Asks = bids, asks
	return out, nil
}

// side reads one side of one book in priority order, stopping at depth.
func (q queryServer) side(
	ctx context.Context,
	index collections.KeySet[collections.Triple[string, uint64, uint64]],
	denom string,
	depth uint32,
) ([]types.Order, error) {
	var out []types.Order
	rng := collections.NewPrefixedTripleRange[string, uint64, uint64](denom)
	err := index.Walk(ctx, rng, func(key collections.Triple[string, uint64, uint64]) (bool, error) {
		order, err := q.Keeper.Order.Get(ctx, key.K3())
		if err != nil {
			if isNotFound(err) {
				return false, nil
			}
			return true, err
		}
		out = append(out, order)
		return uint32(len(out)) >= depth, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
