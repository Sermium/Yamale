package keeper

import (
	"context"
	"errors"

	"yamale/blockchain/x/validatorgov/types"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) ListApprovedValidator(ctx context.Context, req *types.QueryAllApprovedValidatorRequest) (*types.QueryAllApprovedValidatorResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	approvedValidators, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.ApprovedValidator,
		req.Pagination,
		func(_ string, value types.ApprovedValidator) (types.ApprovedValidator, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllApprovedValidatorResponse{ApprovedValidator: approvedValidators, Pagination: pageRes}, nil
}

func (q queryServer) GetApprovedValidator(ctx context.Context, req *types.QueryGetApprovedValidatorRequest) (*types.QueryGetApprovedValidatorResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.ApprovedValidator.Get(ctx, req.Candidate)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetApprovedValidatorResponse{ApprovedValidator: val}, nil
}
