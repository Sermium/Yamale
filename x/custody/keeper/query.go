package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/custody/types"
)

type queryServer struct{ Keeper }

// NewQueryServerImpl returns the Query service implementation.
func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{Keeper: k} }

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	params, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

func (q queryServer) Assets(ctx context.Context, _ *types.QueryAssetsRequest) (*types.QueryAssetsResponse, error) {
	out := []types.Asset{}
	if err := q.Keeper.Assets.Walk(ctx, nil, func(_ string, a types.Asset) (bool, error) {
		out = append(out, a)
		return false, nil
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAssetsResponse{Assets: out}, nil
}

// Solvency answers the only question that matters, per asset, computed from
// what the chain knows rather than reported by whoever runs it.
//
// Deliberately public and unauthenticated. If the number cannot be checked by
// anyone, the arrangement rests on trusting the custodian — and this is a chain
// whose purpose is not having to.
func (q queryServer) Solvency(ctx context.Context, _ *types.QuerySolvencyRequest) (*types.QuerySolvencyResponse, error) {
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()

	out := []types.Solvency{}
	if err := q.Keeper.Assets.Walk(ctx, nil, func(denom string, _ types.Asset) (bool, error) {
		s, err := q.solvencyOf(ctx, denom, height)
		if err != nil {
			return true, err
		}
		out = append(out, s)
		return false, nil
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QuerySolvencyResponse{Solvency: out}, nil
}

func (q queryServer) Deposit(ctx context.Context, req *types.QueryDepositRequest) (*types.QueryDepositResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	d, err := q.Deposits.Get(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, types.ErrNotFound.Error())
	}
	return &types.QueryDepositResponse{Deposit: d}, nil
}

func (q queryServer) Redemption(ctx context.Context, req *types.QueryRedemptionRequest) (*types.QueryRedemptionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	r, err := q.Redemptions.Get(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, types.ErrNotFound.Error())
	}
	return &types.QueryRedemptionResponse{Redemption: r}, nil
}
