package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/land/types"
)

type queryServer struct {
	Keeper
}

// NewQueryServerImpl returns the module's Query service implementation.
func NewQueryServerImpl(k Keeper) types.QueryServer {
	return queryServer{Keeper: k}
}

func (q queryServer) Params(
	ctx context.Context, _ *types.QueryParamsRequest,
) (*types.QueryParamsResponse, error) {
	params, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

func (q queryServer) Parcel(
	ctx context.Context, req *types.QueryParcelRequest,
) (*types.QueryParcelResponse, error) {
	parcel, err := q.Keeper.Parcel.Get(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "no such parcel")
	}
	return &types.QueryParcelResponse{Parcel: parcel}, nil
}

// ParcelByRef is how a citizen actually looks: by the number on their paper.
func (q queryServer) ParcelByRef(
	ctx context.Context, req *types.QueryParcelByRefRequest,
) (*types.QueryParcelByRefResponse, error) {
	id, err := q.ByRef.Get(ctx, req.CadastralRef)
	if err != nil {
		return nil, status.Error(codes.NotFound, "no parcel with that reference")
	}
	parcel, err := q.Keeper.Parcel.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryParcelByRefResponse{Parcel: parcel}, nil
}

// ParcelByGeometry lets a surveyor ask whether ground is already titled before
// a second title over it is ever proposed.
func (q queryServer) ParcelByGeometry(
	ctx context.Context, req *types.QueryParcelByGeometryRequest,
) (*types.QueryParcelByGeometryResponse, error) {
	id, err := q.ByGeometry.Get(ctx, req.GeometryHash)
	if err != nil {
		return nil, status.Error(codes.NotFound, "this ground is not titled")
	}
	parcel, err := q.Keeper.Parcel.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryParcelByGeometryResponse{Parcel: parcel}, nil
}

func (q queryServer) Transfer(
	ctx context.Context, req *types.QueryTransferRequest,
) (*types.QueryTransferResponse, error) {
	transfer, err := q.Keeper.Transfer.Get(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "no such transfer")
	}
	return &types.QueryTransferResponse{Transfer: transfer}, nil
}

func (q queryServer) TransfersByParcel(
	ctx context.Context, req *types.QueryTransfersByParcelRequest,
) (*types.QueryTransfersByParcelResponse, error) {
	var out []types.Transfer
	if err := q.Keeper.Transfer.Walk(ctx, nil, func(_ uint64, v types.Transfer) (bool, error) {
		if v.ParcelId == req.ParcelId {
			out = append(out, v)
		}
		return false, nil
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryTransfersByParcelResponse{Transfers: out}, nil
}

// PendingTransfers is what makes the challenge window real: nobody can object
// to a transfer they cannot see.
func (q queryServer) PendingTransfers(
	ctx context.Context, _ *types.QueryPendingTransfersRequest,
) (*types.QueryPendingTransfersResponse, error) {
	var out []types.Transfer
	if err := q.Keeper.Transfer.Walk(ctx, nil, func(_ uint64, v types.Transfer) (bool, error) {
		if v.CompletedAt == 0 && v.ObjectedBy == "" {
			out = append(out, v)
		}
		return false, nil
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryPendingTransfersResponse{Transfers: out}, nil
}

func (q queryServer) Authorities(
	ctx context.Context, _ *types.QueryAuthoritiesRequest,
) (*types.QueryAuthoritiesResponse, error) {
	var out []types.Authority
	if err := q.Authority.Walk(ctx, nil, func(_ string, v types.Authority) (bool, error) {
		out = append(out, v)
		return false, nil
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAuthoritiesResponse{Authorities: out}, nil
}

func (q queryServer) ParcelsByHolder(
	ctx context.Context, req *types.QueryParcelsByHolderRequest,
) (*types.QueryParcelsByHolderResponse, error) {
	var out []types.Parcel
	if err := q.Keeper.Parcel.Walk(ctx, nil, func(_ uint64, v types.Parcel) (bool, error) {
		if v.Holder == req.Holder {
			out = append(out, v)
		}
		return false, nil
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryParcelsByHolderResponse{Parcels: out}, nil
}

// FractionalisationAuthority answers, in one call, the question a buyer of
// shares in a piece of land needs answered before they pay: did the registry
// permit this, and does the permission still stand.
//
// The `live` flag is computed here rather than left to the caller so that a
// wallet and the keeper cannot disagree about what "live" means — a disagreement
// only ever discovered by somebody whose money has already moved.
func (q queryServer) FractionalisationAuthority(
	ctx context.Context, req *types.QueryFractionalisationAuthorityRequest,
) (*types.QueryFractionalisationAuthorityResponse, error) {
	auth, err := q.Keeper.FractionalisationAuthority.Get(ctx, req.ParcelId)
	if err != nil {
		return nil, status.Error(codes.NotFound,
			"this parcel has no fractionalisation authorisation")
	}
	parcel, err := q.Keeper.Parcel.Get(ctx, req.ParcelId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return &types.QueryFractionalisationAuthorityResponse{
		Authorisation: auth,
		Live:          auth.Live(now) && !parcel.ForbidsFractionalisation(),
	}, nil
}
