package keeper

import (
	"context"
	"errors"

	"yamale/blockchain/x/paymsg/types"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) ListParticipantApplication(ctx context.Context, req *types.QueryAllParticipantApplicationRequest) (*types.QueryAllParticipantApplicationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	participantApplications, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.ParticipantApplication,
		req.Pagination,
		func(_ string, value types.ParticipantApplication) (types.ParticipantApplication, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllParticipantApplicationResponse{ParticipantApplication: participantApplications, Pagination: pageRes}, nil
}

func (q queryServer) GetParticipantApplication(ctx context.Context, req *types.QueryGetParticipantApplicationRequest) (*types.QueryGetParticipantApplicationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.ParticipantApplication.Get(ctx, req.Creator)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetParticipantApplicationResponse{ParticipantApplication: val}, nil
}
