package keeper

import (
	"context"
	"errors"

	"yamale/blockchain/x/builderfee/types"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) ListApprovedBuilder(ctx context.Context, req *types.QueryAllApprovedBuilderRequest) (*types.QueryAllApprovedBuilderResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	approvedBuilders, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.ApprovedBuilder,
		req.Pagination,
		func(_ string, value types.ApprovedBuilder) (types.ApprovedBuilder, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllApprovedBuilderResponse{ApprovedBuilder: approvedBuilders, Pagination: pageRes}, nil
}

func (q queryServer) GetApprovedBuilder(ctx context.Context, req *types.QueryGetApprovedBuilderRequest) (*types.QueryGetApprovedBuilderResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.ApprovedBuilder.Get(ctx, req.MsgTypeUrl)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetApprovedBuilderResponse{ApprovedBuilder: val}, nil
}
