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

func (q queryServer) ListApprovedParticipant(ctx context.Context, req *types.QueryAllApprovedParticipantRequest) (*types.QueryAllApprovedParticipantResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	approvedParticipants, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.ApprovedParticipant,
		req.Pagination,
		func(_ string, value types.ApprovedParticipant) (types.ApprovedParticipant, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllApprovedParticipantResponse{ApprovedParticipant: approvedParticipants, Pagination: pageRes}, nil
}

func (q queryServer) GetApprovedParticipant(ctx context.Context, req *types.QueryGetApprovedParticipantRequest) (*types.QueryGetApprovedParticipantResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.ApprovedParticipant.Get(ctx, req.Participant)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetApprovedParticipantResponse{ApprovedParticipant: val}, nil
}
