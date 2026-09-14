package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/syndicate/types"
)

type queryServer struct{ Keeper }

// NewQueryServerImpl returns the read side of the syndication layer.
func NewQueryServerImpl(keeper Keeper) types.QueryServer { return queryServer{Keeper: keeper} }

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

func (q queryServer) Operators(
	ctx context.Context, _ *types.QueryOperatorsRequest,
) (*types.QueryOperatorsResponse, error) {
	out := &types.QueryOperatorsResponse{}
	err := q.Keeper.Operator.Walk(ctx, nil, func(_ string, o types.Operator) (bool, error) {
		out.Operators = append(out.Operators, o)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Investor reports whether an address has been onboarded.
//
// It answers rather than erroring when there is no record, because "this
// address is not onboarded" is a fact a secondary buyer needs, and a NotFound
// is indistinguishable at a call site from the query having failed. Those are
// opposite conclusions: one means do not offer them the tokens, the other means
// try again before deciding anything.
func (q queryServer) Investor(
	ctx context.Context, req *types.QueryInvestorRequest,
) (*types.QueryInvestorResponse, error) {
	inv, err := q.Keeper.Investor.Get(ctx, req.Address)
	if err != nil {
		if isNotFound(err) {
			return &types.QueryInvestorResponse{Onboarded: false}, nil
		}
		return nil, err
	}
	return &types.QueryInvestorResponse{Onboarded: inv.Active, Investor: inv}, nil
}

func (q queryServer) Deal(
	ctx context.Context, req *types.QueryDealRequest,
) (*types.QueryDealResponse, error) {
	deal, err := q.Keeper.Deal.Get(ctx, req.Id)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "no deal %d", req.Id)
		}
		return nil, err
	}
	return &types.QueryDealResponse{Deal: deal}, nil
}

func (q queryServer) DealByRef(
	ctx context.Context, req *types.QueryDealByRefRequest,
) (*types.QueryDealResponse, error) {
	id, err := q.ByDealRef.Get(ctx, req.Reference)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "no deal with reference %q", req.Reference)
		}
		return nil, err
	}
	return q.Deal(ctx, &types.QueryDealRequest{Id: id})
}

func (q queryServer) Deals(
	ctx context.Context, _ *types.QueryDealsRequest,
) (*types.QueryDealsResponse, error) {
	out := &types.QueryDealsResponse{}
	err := q.Keeper.Deal.Walk(ctx, nil, func(_ uint64, d types.Deal) (bool, error) {
		out.Deals = append(out.Deals, d)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Commitments lists what has been committed to one deal.
//
// Public, and that is a deliberate call rather than an oversight. An investor
// deciding whether to join a syndicate wants to know how full the book is —
// that is the single most useful number to them, and a platform that hid it
// would be asking for trust it has not earned. What is public is a list of
// commitments to ONE deal; what does not exist anywhere in this module is a
// query that goes the other way, from a person to everything they hold.
func (q queryServer) Commitments(
	ctx context.Context, req *types.QueryCommitmentsRequest,
) (*types.QueryCommitmentsResponse, error) {
	out := &types.QueryCommitmentsResponse{}
	rng := collections.NewPrefixedPairRange[uint64, string](req.DealId)
	err := q.Keeper.Commitment.Walk(ctx, rng,
		func(_ collections.Pair[uint64, string], c types.Commitment) (bool, error) {
			out.Commitments = append(out.Commitments, c)
			return false, nil
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Entitlement is what a holder could claim right now.
//
// Settled and unsettled together, which is what a statement would show. A
// response reporting only the settled part would show zero for a holder who has
// never transferred — which is most of them, most of the time, and precisely
// the people who would conclude the platform had lost their money.
func (q queryServer) Entitlement(
	ctx context.Context, req *types.QueryEntitlementRequest,
) (*types.QueryEntitlementResponse, error) {
	deal, err := q.Keeper.Deal.Get(ctx, req.DealId)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "no deal %d", req.DealId)
		}
		return nil, err
	}
	holder, err := q.addressCodec.StringToBytes(req.Holder)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%q is not an address", req.Holder)
	}

	balance, accrued, err := q.Keeper.Entitlement(ctx, req.DealId, holder)
	if err != nil {
		return nil, err
	}

	shareBps := uint32(0)
	if deal.Supply.IsPositive() {
		shareBps = uint32(balance.Mul(math.NewInt(types.MaxBps)).Quo(deal.Supply).Int64())
	}

	return &types.QueryEntitlementResponse{
		Balance:   balance,
		Claimable: sdk.NewCoin(deal.SettlementDenom, accrued),
		ShareBps:  shareBps,
	}, nil
}

func (q queryServer) Listings(
	ctx context.Context, req *types.QueryListingsRequest,
) (*types.QueryListingsResponse, error) {
	out := &types.QueryListingsResponse{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](req.DealId)
	err := q.ListingsByDeal.Walk(ctx, rng, func(key collections.Pair[uint64, uint64]) (bool, error) {
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

// Documents are one deal's papers: their digests, never their contents.
//
// Public like everything else on this query service, and for the same reason —
// an investor who cannot see which agreements govern what they own is back in
// the spreadsheet somebody else holds, which is the arrangement this platform
// exists to replace. Nothing private is disclosed by it: a hash reveals nothing
// about a document to anybody who does not already have the document.
func (q queryServer) Documents(
	ctx context.Context, req *types.QueryDocumentsRequest,
) (*types.QueryDocumentsResponse, error) {
	docs, err := q.Keeper.DealDocuments(ctx, req.DealId, req.CurrentOnly)
	if err != nil {
		return nil, err
	}
	return &types.QueryDocumentsResponse{Documents: docs}, nil
}
