package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/validatorgov/types"
)

func (q queryServer) ListOperatorRotation(ctx context.Context, req *types.QueryAllOperatorRotationRequest) (*types.QueryAllOperatorRotationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	rotations, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.Rotation,
		req.Pagination,
		func(_ uint64, value types.OperatorRotation) (types.OperatorRotation, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllOperatorRotationResponse{OperatorRotation: rotations, Pagination: pageRes}, nil
}

func (q queryServer) GetOperatorRotation(ctx context.Context, req *types.QueryGetOperatorRotationRequest) (*types.QueryGetOperatorRotationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.Rotation.Get(ctx, req.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetOperatorRotationResponse{OperatorRotation: val}, nil
}

// PendingOperatorRotation answers from the operator address rather than from a
// rotation id, because the person who most needs this answer is a delegator
// deciding whether to undelegate, and the only identifier they have is the
// validator they delegated to.
func (q queryServer) PendingOperatorRotation(ctx context.Context, req *types.QueryPendingOperatorRotationRequest) (*types.QueryPendingOperatorRotationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	rotation, found, err := q.k.PendingRotationFor(ctx, req.CurrentOperator)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryPendingOperatorRotationResponse{Found: found, OperatorRotation: rotation}, nil
}
