package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/tokenisation/types"
)

type queryServer struct{ k Keeper }

func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{k: k} }

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Collections(ctx context.Context, req *types.QueryCollectionsRequest) (*types.QueryCollectionsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	cs, page, err := query.CollectionPaginate(ctx, q.k.Collections, req.Pagination,
		func(_ string, c types.Collection) (types.Collection, error) { return c, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryCollectionsResponse{Collections: cs, Pagination: page}, nil
}

func (q queryServer) Assets(ctx context.Context, req *types.QueryAssetsRequest) (*types.QueryAssetsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	as, page, err := query.CollectionFilteredPaginate(ctx, q.k.Assets, req.Pagination,
		func(_ uint64, a types.Asset) (bool, error) {
			return req.CollectionId == "" || a.CollectionId == req.CollectionId, nil
		},
		func(_ uint64, a types.Asset) (types.Asset, error) { return a, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryAssetsResponse{Assets: as, Pagination: page}, nil
}

func (q queryServer) Asset(ctx context.Context, req *types.QueryAssetRequest) (*types.QueryAssetResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	a, err := q.k.Assets.Get(ctx, req.AssetId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "no such asset")
	}
	resp := &types.QueryAssetResponse{Asset: a}
	if v, err := q.k.Vaults.Get(ctx, req.AssetId); err == nil {
		resp.Vault = v
	}
	if s, err := q.k.Sales.Get(ctx, req.AssetId); err == nil {
		resp.Sale = &s
	}
	return resp, nil
}

// Entitlement answers what an account could withdraw right now, including
// income that has accrued since its balance last moved.
//
// It reads through the same arithmetic the handlers use rather than reporting
// the stored `accrued` figure alone: a holder who has not transacted since the
// last distribution has earned income that no write has recorded yet, and a
// balance screen showing them zero would be telling them the truth about the
// database and a lie about their money.
func (q queryServer) Entitlement(ctx context.Context, req *types.QueryEntitlementRequest) (*types.QueryEntitlementResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	holder, err := q.k.addressCodec.StringToBytes(req.Holder)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid address")
	}
	vault, err := q.k.Vaults.Get(ctx, req.AssetId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "no vault for that asset")
	}
	owed, err := q.k.Entitlement(ctx, req.AssetId, holder)
	if err != nil {
		return nil, err
	}
	return &types.QueryEntitlementResponse{Owed: sdk.NewCoin(vault.Denom, owed)}, nil
}
