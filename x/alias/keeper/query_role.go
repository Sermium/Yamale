package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/alias/types"
)

// RoleGrants lists every role one account holds, and where.
//
// Empty is a real answer rather than a not-found error: "this key may act
// nowhere" is exactly what an operator checking a key before trusting it needs
// to be told, and a 404 reads as "the chain does not know about this account",
// which is a different and misleading claim.
func (q queryServer) RoleGrants(ctx context.Context, req *types.QueryRoleGrantsRequest) (*types.QueryRoleGrantsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	grants, err := q.GrantsOf(ctx, req.Holder)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryRoleGrantsResponse{Grants: grants}, nil
}

// RoleHolders lists who holds roles inside one jurisdiction.
//
// Served from the derived (jurisdiction, role, holder) index and then reading
// each grant back from the registry, so what comes out is the record itself
// rather than a second copy assembled from the key — a copy that could disagree
// with the thing AssertScope actually consults.
//
// Chain-wide grants are not folded in. A country's list shows what that country
// granted; the accounts that no border bounds are listed by ChainWideGrants, on
// their own, because an exception mixed into every country's ordinary entries is
// an exception nobody notices.
func (q queryServer) RoleHolders(ctx context.Context, req *types.QueryRoleHoldersRequest) (*types.QueryRoleHoldersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	scope := types.NormaliseScope(req.Jurisdiction)
	if !types.ValidGrantScope(scope) {
		return nil, status.Error(codes.InvalidArgument, types.ErrInvalidScope.Error())
	}
	// Unspecified means "every role" here, and this is the only place the zero
	// value means anything at all. It is a filter that has not been applied, not
	// a role: nothing is granted, nothing is checked, and no action is permitted
	// by it.
	if req.Role != types.ROLE_UNSPECIFIED && !types.ValidRole(req.Role) {
		return nil, status.Error(codes.InvalidArgument, types.ErrInvalidRole.Error())
	}

	prefix := collections.TriplePrefix[string, int32, string](scope)
	if req.Role != types.ROLE_UNSPECIFIED {
		prefix = collections.TripleSuperPrefix[string, int32, string](scope, int32(req.Role))
	}

	grants, page, err := query.CollectionPaginate(
		ctx,
		q.GrantsByScope,
		req.Pagination,
		func(key collections.Triple[string, int32, string], _ collections.NoValue) (types.RoleGrant, error) {
			return q.Keeper.RoleGrants.Get(ctx, collections.Join3(key.K3(), key.K2(), key.K1()))
		},
		func(o *query.CollectionsPaginateOptions[collections.Triple[string, int32, string]]) {
			o.Prefix = &prefix
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryRoleHoldersResponse{Grants: grants, Pagination: page}, nil
}

// ChainWideGrants lists every grant that no border bounds.
//
// Off the same index as RoleHolders, so the exception list cannot drift from the
// grants it summarises — a second walk with its own filter would be a second
// implementation, and the day the two disagree the console shows the shorter
// answer.
//
// Unpaginated. If this response ever stops fitting in one page, the deployment
// has stopped treating chain-wide authority as an exception, and the right
// response is fewer grants rather than another page of them.
func (q queryServer) ChainWideGrants(ctx context.Context, _ *types.QueryChainWideGrantsRequest) (*types.QueryChainWideGrantsResponse, error) {
	grants := []types.RoleGrant{}
	rng := collections.NewPrefixedTripleRange[string, int32, string](types.ChainWide)
	err := q.GrantsByScope.Walk(ctx, rng, func(key collections.Triple[string, int32, string]) (bool, error) {
		g, err := q.Keeper.RoleGrants.Get(ctx, collections.Join3(key.K3(), key.K2(), key.K1()))
		if err != nil {
			return true, err
		}
		grants = append(grants, g)
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryChainWideGrantsResponse{Grants: grants}, nil
}
