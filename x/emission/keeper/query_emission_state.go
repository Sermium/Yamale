package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/emission/types"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) GetEmissionState(ctx context.Context, req *types.QueryGetEmissionStateRequest) (*types.QueryGetEmissionStateResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.EmissionState.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetEmissionStateResponse{EmissionState: val}, nil
}
