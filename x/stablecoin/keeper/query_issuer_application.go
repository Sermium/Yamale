package keeper

import (
	"context"
	"errors"

	"yamale/blockchain/x/stablecoin/types"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) ListIssuerApplication(ctx context.Context, req *types.QueryAllIssuerApplicationRequest) (*types.QueryAllIssuerApplicationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	issuerApplications, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.IssuerApplication,
		req.Pagination,
		func(_ string, value types.IssuerApplication) (types.IssuerApplication, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllIssuerApplicationResponse{IssuerApplication: issuerApplications, Pagination: pageRes}, nil
}

func (q queryServer) GetIssuerApplication(ctx context.Context, req *types.QueryGetIssuerApplicationRequest) (*types.QueryGetIssuerApplicationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.IssuerApplication.Get(ctx, req.Denom)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetIssuerApplicationResponse{IssuerApplication: val}, nil
}
