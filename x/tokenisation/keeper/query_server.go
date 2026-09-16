package keeper

import (
	"context"
	"cosmossdk.io/collections"
	"cosmossdk.io/math"

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

// Covenants lists what has been published against one tokenisation.
//
// Public, and unauthenticated like every other read here. A holder who cannot
// see which undertakings govern what they own is back in the arrangement where
// the operator decides what to tell them, which is the arrangement this whole
// module exists to replace. Nothing private is disclosed by it: a digest says
// nothing about a document to anybody who does not already have the document.
func (q queryServer) Covenants(
	ctx context.Context, req *types.QueryCovenantsRequest,
) (*types.QueryCovenantsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	covenants, err := q.k.AssetCovenants(ctx, req.AssetId, req.CurrentOnly)
	if err != nil {
		return nil, err
	}
	return &types.QueryCovenantsResponse{Covenants: covenants}, nil
}

// Position returns one holder's whole record against a tokenisation.
//
// Entitlement answers only "what could they withdraw now". Everything else a
// holder needs about their own position -- what it cost, how much of it that
// cost covers, and the time accruing in the current period -- had no query,
// so a browser was reading it out of the raw store and decoding the key layout
// by hand. That couples an interface to an implementation detail nobody would
// think to announce when changing.
//
// `found` is its own field rather than a nil check. Most accounts hold most
// tokenisations not at all, and that is an ANSWER: a caller that could not tell
// it from a failed read would show "you own nothing" when the chain had not
// spoken.
func (q queryServer) Position(
	ctx context.Context, req *types.QueryPositionRequest,
) (*types.QueryPositionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	holder, err := q.k.addressCodec.StringToBytes(req.Holder)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "that is not an address")
	}

	empty := types.Position{
		AssetId:      req.AssetId,
		Holder:       req.Holder,
		LastIndex:    math.LegacyZeroDec(),
		Accrued:      math.ZeroInt(),
		CostBasis:    math.ZeroInt(),
		CostQty:      math.ZeroInt(),
		TokenSeconds: math.ZeroInt(),
	}

	pos, err := q.k.Positions.Get(ctx, collections.Join(req.AssetId, req.Holder))
	if err != nil {
		return &types.QueryPositionResponse{
			Position: empty, Found: false,
			Owed: math.ZeroInt(), PendingTokenSeconds: math.ZeroInt(),
		}, nil
	}

	// Normalised on the way out. A position written before these fields existed
	// carries nil, and math.Int marshals a nil as an empty string that the
	// client side then fails to parse -- so the query would break for exactly
	// the holders the migration exists to protect.
	pos.Accrued = zeroIfNil(pos.Accrued)
	pos.CostBasis = zeroIfNil(pos.CostBasis)
	pos.CostQty = zeroIfNil(pos.CostQty)
	pos.TokenSeconds = zeroIfNil(pos.TokenSeconds)

	owed, err := q.k.Entitlement(ctx, req.AssetId, holder)
	if err != nil {
		return nil, err
	}

	pending := math.ZeroInt()
	if asset, err := q.k.Assets.Get(ctx, req.AssetId); err == nil && asset.FractionDenom != "" {
		balance := q.k.bankKeeper.GetBalance(
			sdk.UnwrapSDKContext(ctx), holder, asset.FractionDenom).Amount
		pending, err = q.k.PendingTokenSeconds(
			ctx, req.AssetId, holder, balance, blockSeconds(ctx))
		if err != nil {
			return nil, err
		}
	}

	return &types.QueryPositionResponse{
		Position: pos, Found: true, Owed: owed, PendingTokenSeconds: pending,
	}, nil
}

// DividendReserve makes the backing promise measurable from outside.
//
// The two figures it compares come from different modules' state: `outstanding`
// is the bank keeper's supply of the dividend denom, `reserve` is this module's
// per-vehicle ledger. A reader who does not trust this module can read the
// supply from x/bank themselves and compare. Taking both from here would be
// comparing a number with itself, which always agrees and checks nothing.
func (q queryServer) DividendReserve(
	ctx context.Context, req *types.QueryDividendReserveRequest,
) (*types.QueryDividendReserveResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	vault, err := q.k.Vaults.Get(ctx, req.AssetId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "no vault for that asset")
	}
	denom, err := q.k.dividendDenom(ctx, req.AssetId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "that asset has no shareholding, so it has no dividend token")
	}
	outstanding, err := q.k.DividendOutstanding(ctx, req.AssetId)
	if err != nil {
		return nil, err
	}
	return &types.QueryDividendReserveResponse{
		DividendDenom: denom,
		Outstanding:   outstanding,
		Reserve:       sdk.NewCoin(vault.Denom, sdk.Coins(vault.DividendReserve).AmountOf(vault.Denom)),
		Unreserved:    sdk.NewCoin(vault.Denom, sdk.Coins(vault.Held).AmountOf(vault.Denom)),
	}, nil
}
