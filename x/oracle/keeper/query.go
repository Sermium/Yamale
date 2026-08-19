package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"yamale/blockchain/x/oracle/types"
)

// Every response that carries a value also carries how old it is and whether
// the module still considers it usable. A caller that had to work out staleness
// for itself would eventually get it wrong, and the failure mode — acting on a
// price nobody stands behind any more — is the one this module exists to
// prevent.
//
// Every list is paginated. Queries are gas-free and reachable by anyone with a
// node's address, and the collections here — rates, valuers, valuation history —
// all grow over time.

type queryServer struct {
	k Keeper
}

// NewQueryServerImpl returns an implementation of the QueryServer interface.
func NewQueryServerImpl(k Keeper) types.QueryServer {
	return queryServer{k: k}
}

var _ types.QueryServer = queryServer{}

func (q queryServer) Params(ctx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	params, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

func (q queryServer) ExchangeRate(ctx context.Context, req *types.QueryExchangeRateRequest) (*types.QueryExchangeRateResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	params, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	rate, err := q.k.ExchangeRate.Get(ctx, req.Denom)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "no rate has ever been agreed for %s", req.Denom)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return &types.QueryExchangeRateResponse{
		Rate:       rate,
		Stale:      types.IsStale(rate.UpdatedAt, now, params.MaxRateAgeSeconds),
		AgeSeconds: types.AgeSeconds(rate.UpdatedAt, now),
	}, nil
}

func (q queryServer) ExchangeRates(ctx context.Context, req *types.QueryExchangeRatesRequest) (*types.QueryExchangeRatesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	out, page, err := query.CollectionPaginate(
		ctx, q.k.ExchangeRate, req.Pagination,
		func(_ string, v types.ExchangeRate) (types.ExchangeRate, error) { return v, nil },
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryExchangeRatesResponse{Rates: out, Pagination: page}, nil
}

func (q queryServer) Appraisal(ctx context.Context, req *types.QueryAppraisalRequest) (*types.QueryAppraisalResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	params, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	appraisal, err := q.k.Appraisal.Get(ctx, collections.Join(req.ClassId, req.NftId))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "%s/%s has never been valued", req.ClassId, req.NftId)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Whether the signer is still approved is reported alongside the number.
	// The valuation stays valid history either way — it was properly signed when
	// it was made — but a consumer deciding whether to lend against it should
	// know that the chain has since withdrawn that valuer's authority.
	stillApproved := false
	if appraiser, err := q.k.Appraiser.Get(ctx, appraisal.Appraiser); err == nil {
		stillApproved = appraiser.Status == types.AppraiserStatus_APPRAISER_STATUS_APPROVED
	} else if !errors.Is(err, collections.ErrNotFound) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return &types.QueryAppraisalResponse{
		Appraisal:              appraisal,
		Stale:                  types.IsStale(appraisal.ValuedAt, now, params.MaxAppraisalAgeSeconds),
		AgeSeconds:             types.AgeSeconds(appraisal.ValuedAt, now),
		AppraiserStillApproved: stillApproved,
	}, nil
}

// AppraisalHistory returns every superseded valuation of one asset.
//
// Ordered oldest first, which is the order the sequence numbers were assigned
// in; set pagination.reverse to read newest first. The current valuation is not
// included — it is not history yet, and returning it here would make a client
// that concatenates the two count it twice.
func (q queryServer) AppraisalHistory(ctx context.Context, req *types.QueryAppraisalHistoryRequest) (*types.QueryAppraisalHistoryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	prefix := collections.TripleSuperPrefix[string, string, uint64](req.ClassId, req.NftId)
	out, page, err := query.CollectionPaginate(
		ctx, q.k.AppraisalHistory, req.Pagination,
		func(_ collections.Triple[string, string, uint64], v types.Appraisal) (types.Appraisal, error) {
			return v, nil
		},
		func(o *query.CollectionsPaginateOptions[collections.Triple[string, string, uint64]]) {
			o.Prefix = &prefix
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAppraisalHistoryResponse{Appraisals: out, Pagination: page}, nil
}

func (q queryServer) GetAppraiser(ctx context.Context, req *types.QueryGetAppraiserRequest) (*types.QueryGetAppraiserResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	appraiser, err := q.k.Appraiser.Get(ctx, req.Address)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "%s is not a known appraiser", req.Address)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryGetAppraiserResponse{Appraiser: appraiser}, nil
}

func (q queryServer) ListAppraiser(ctx context.Context, req *types.QueryListAppraiserRequest) (*types.QueryListAppraiserResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	out, page, err := query.CollectionPaginate(
		ctx, q.k.Appraiser, req.Pagination,
		func(_ string, v types.Appraiser) (types.Appraiser, error) { return v, nil },
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryListAppraiserResponse{Appraisers: out, Pagination: page}, nil
}

func (q queryServer) MissCounters(ctx context.Context, req *types.QueryMissCountersRequest) (*types.QueryMissCountersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	out, page, err := query.CollectionPaginate(
		ctx, q.k.MissCounter, req.Pagination,
		func(_ string, v types.MissCounter) (types.MissCounter, error) { return v, nil },
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryMissCountersResponse{Counters: out, Pagination: page}, nil
}

// FeederDelegation reports which account votes for a validator, answering with
// the validator's own account when nothing has been delegated.
//
// Returning the effective answer rather than "not found" is what lets a feeder
// operator check its configuration with one call: the common case — no hot key
// — is a valid configuration, not a missing one.
func (q queryServer) FeederDelegation(ctx context.Context, req *types.QueryFeederDelegationRequest) (*types.QueryFeederDelegationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	feeder, err := q.k.FeederOf(ctx, req.Validator)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &types.QueryFeederDelegationResponse{Feeder: feeder}, nil
}
