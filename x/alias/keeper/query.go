package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/alias/types"
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

// Alias resolves an identifier.
//
// Accepts whatever form the client holds — hyphenated, lower case, with I and O
// where 1 and 0 belong — and normalises before looking up, so a person reading
// an identifier off paper is not punished for the way they typed it.
func (q queryServer) Alias(ctx context.Context, req *types.QueryAliasRequest) (*types.QueryAliasResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	id := types.Normalise(req.Id)
	if !types.Valid(id) {
		// Refused before the lookup: a malformed identifier is a typo, and
		// saying so is more useful than "not found", which reads as "that
		// account does not exist".
		return nil, status.Error(codes.InvalidArgument, types.ErrMalformedID.Error())
	}

	alias, err := q.Aliases.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.NotFound, types.ErrNotFound.Error())
	}
	return &types.QueryAliasResponse{Alias: alias}, nil
}

func (q queryServer) AliasOf(ctx context.Context, req *types.QueryAliasOfRequest) (*types.QueryAliasOfResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	id, err := q.Owners.Get(ctx, req.Address)
	if err != nil {
		return nil, status.Error(codes.NotFound, types.ErrNotRegistered.Error())
	}
	alias, err := q.Aliases.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.Internal, "reverse index disagrees with the registry")
	}
	return &types.QueryAliasOfResponse{Alias: alias}, nil
}

// Retired distinguishes an identifier that was given up from one that never
// existed. Both fail to resolve; only one of them means somebody used to be
// there, which is what a person about to send money needs to know.
func (q queryServer) Retired(ctx context.Context, req *types.QueryRetiredRequest) (*types.QueryRetiredResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	id := types.Normalise(req.Id)
	if !types.Valid(id) {
		return nil, status.Error(codes.InvalidArgument, types.ErrMalformedID.Error())
	}
	dead, err := q.Keeper.Retired.Has(ctx, id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryRetiredResponse{Retired: dead}, nil
}

// Jurisdiction reports the country recorded against one account.
//
// Not-found is a real answer rather than a gap: outside the foundation
// administrators, an account with no recorded jurisdiction holds no identifier
// and will not be issued one.
func (q queryServer) Jurisdiction(ctx context.Context, req *types.QueryJurisdictionRequest) (*types.QueryJurisdictionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	j, err := q.Jurisdictions.Get(ctx, req.Address)
	if err != nil {
		return nil, status.Error(codes.NotFound, types.ErrNoJurisdiction.Error())
	}
	return &types.QueryJurisdictionResponse{Jurisdiction: j}, nil
}

// Perimeter lists the accounts recorded in one country.
//
// Served from the derived (country, address) index rather than by filtering
// every jurisdiction record, so an authority querying a small country does not
// pay for the size of a large one — a query whose cost is the whole chain is a
// query an operator learns not to run.
//
// It walks the index and reads each record, so what comes back is the record
// itself and not a second copy of it that could disagree.
func (q queryServer) Perimeter(ctx context.Context, req *types.QueryPerimeterRequest) (*types.QueryPerimeterResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	country := types.NormaliseCountry(req.Country)
	if !types.IssuableCountry(country) {
		return nil, status.Error(codes.InvalidArgument, types.ErrInvalidCountry.Error())
	}

	records, page, err := query.CollectionPaginate(
		ctx,
		q.Keeper.Perimeter,
		req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (types.Jurisdiction, error) {
			return q.Jurisdictions.Get(ctx, key.K2())
		},
		query.WithCollectionPaginationPairPrefix[string, string](country),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryPerimeterResponse{Jurisdictions: records, Pagination: page}, nil
}
