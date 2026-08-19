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

func (q queryServer) ListApprovedIssuer(ctx context.Context, req *types.QueryAllApprovedIssuerRequest) (*types.QueryAllApprovedIssuerResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	approvedIssuers, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.ApprovedIssuer,
		req.Pagination,
		func(_ string, value types.ApprovedIssuer) (types.ApprovedIssuer, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllApprovedIssuerResponse{ApprovedIssuer: approvedIssuers, Pagination: pageRes}, nil
}

func (q queryServer) GetApprovedIssuer(ctx context.Context, req *types.QueryGetApprovedIssuerRequest) (*types.QueryGetApprovedIssuerResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.ApprovedIssuer.Get(ctx, req.Denom)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetApprovedIssuerResponse{ApprovedIssuer: val}, nil
}
