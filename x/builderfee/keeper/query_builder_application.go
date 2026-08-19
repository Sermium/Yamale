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

func (q queryServer) ListBuilderApplication(ctx context.Context, req *types.QueryAllBuilderApplicationRequest) (*types.QueryAllBuilderApplicationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	builderApplications, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.BuilderApplication,
		req.Pagination,
		func(_ string, value types.BuilderApplication) (types.BuilderApplication, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllBuilderApplicationResponse{BuilderApplication: builderApplications, Pagination: pageRes}, nil
}

func (q queryServer) GetBuilderApplication(ctx context.Context, req *types.QueryGetBuilderApplicationRequest) (*types.QueryGetBuilderApplicationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.BuilderApplication.Get(ctx, req.MsgTypeUrl)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetBuilderApplicationResponse{BuilderApplication: val}, nil
}
