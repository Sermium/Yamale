package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/constitution/types"
)

// What this module holds is public by construction. A settlement that could not
// be inspected from outside would be indistinguishable from a parameter set
// somebody promised not to change, which is what this chain had before.

type queryServer struct {
	k Keeper
}

// NewQueryServerImpl returns an implementation of the QueryServer interface.
func NewQueryServerImpl(k Keeper) types.QueryServer {
	return queryServer{k: k}
}

var _ types.QueryServer = queryServer{}

// Invariants returns the settlement in force.
func (q queryServer) Invariants(ctx context.Context, req *types.QueryInvariantsRequest) (*types.QueryInvariantsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	inv, err := q.k.GetInvariants(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryInvariantsResponse{Invariants: inv}, nil
}

// Amendment returns one amendment by id.
func (q queryServer) Amendment(ctx context.Context, req *types.QueryAmendmentRequest) (*types.QueryAmendmentResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	amendment, err := q.k.Amendment.Get(ctx, req.Id)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "amendment %d not found", req.Id)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAmendmentResponse{Amendment: amendment}, nil
}

// ListAmendment returns a page of amendments.
func (q queryServer) ListAmendment(ctx context.Context, req *types.QueryAllAmendmentRequest) (*types.QueryAllAmendmentResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	amendments, pageRes, err := query.CollectionPaginate(
		ctx, q.k.Amendment, req.Pagination,
		func(_ uint64, value types.Amendment) (types.Amendment, error) { return value, nil },
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllAmendmentResponse{Amendment: amendments, Pagination: pageRes}, nil
}

// Ratifications lists who has ratified one amendment, and what it still needs.
//
// The required power is returned alongside deliberately. Somebody watching a
// change to the seizure threshold needs to know how close it is, and making
// that two queries means an interface will show them only the first one.
func (q queryServer) Ratifications(ctx context.Context, req *types.QueryRatificationsRequest) (*types.QueryRatificationsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	amendment, err := q.k.Amendment.Get(ctx, req.AmendmentId)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "amendment %d not found", req.AmendmentId)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	inv, err := q.k.GetInvariants(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	ratifications := make([]types.Ratification, 0)
	rng := collections.NewPrefixedPairRange[uint64, string](req.AmendmentId)
	if err := q.k.Ratification.Walk(ctx, rng, func(_ collections.Pair[uint64, string], r types.Ratification) (bool, error) {
		ratifications = append(ratifications, r)
		return false, nil
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryRatificationsResponse{
		Ratifications: ratifications,
		RequiredPower: inv.RequiredPower(amendment.SnapshotPower),
	}, nil
}
