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

func (q queryServer) ListValidatorApplication(ctx context.Context, req *types.QueryAllValidatorApplicationRequest) (*types.QueryAllValidatorApplicationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	validatorApplications, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.ValidatorApplication,
		req.Pagination,
		func(_ string, value types.ValidatorApplication) (types.ValidatorApplication, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllValidatorApplicationResponse{ValidatorApplication: validatorApplications, Pagination: pageRes}, nil
}

func (q queryServer) GetValidatorApplication(ctx context.Context, req *types.QueryGetValidatorApplicationRequest) (*types.QueryGetValidatorApplicationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.ValidatorApplication.Get(ctx, req.Candidate)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetValidatorApplicationResponse{ValidatorApplication: val}, nil
}
