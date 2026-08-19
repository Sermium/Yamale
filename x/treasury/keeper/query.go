package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"yamale/blockchain/x/treasury/types"
)

// Queries are gas-free and reachable by anyone with the node's address, so every
// list here is paginated. A query that walks an entire collection is a way to
// make a node do unbounded work for free, and the collections it would walk —
// treasuries, locks, role assignments — are ones anybody can grow.

type queryServer struct {
	k Keeper
}

// NewQueryServerImpl returns an implementation of the QueryServer interface.
func NewQueryServerImpl(k Keeper) types.QueryServer {
	return queryServer{k: k}
}

var _ types.QueryServer = queryServer{}

func (q queryServer) Params(ctx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	params, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

func (q queryServer) GetTreasury(ctx context.Context, req *types.QueryGetTreasuryRequest) (*types.QueryGetTreasuryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	t, err := q.k.Treasury.Get(ctx, req.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "treasury %d not found", req.Id)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryGetTreasuryResponse{Treasury: t}, nil
}

func (q queryServer) ListTreasury(ctx context.Context, req *types.QueryAllTreasuryRequest) (*types.QueryAllTreasuryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	out, page, err := query.CollectionPaginate(
		ctx, q.k.Treasury, req.Pagination,
		func(_ uint64, v types.Treasury) (types.Treasury, error) { return v, nil },
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAllTreasuryResponse{Treasury: out, Pagination: page}, nil
}

// TreasuryBalances reports the split a client actually needs: what is held,
// what is committed, and what may still be spent.
//
// Unpaginated deliberately: this walks one treasury's denoms, and the number of
// denoms a treasury holds is bounded by how many exist on the chain.
func (q queryServer) TreasuryBalances(ctx context.Context, req *types.QueryTreasuryBalancesRequest) (*types.QueryTreasuryBalancesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	var out []types.DenomBalance
	rng := collections.NewPrefixedPairRange[uint64, string](req.TreasuryId)
	if err := q.k.Balance.Walk(ctx, rng, func(_ collections.Pair[uint64, string], v types.TreasuryBalance) (bool, error) {
		total, locked := balanceAmounts(v)
		available := total.Sub(locked)
		if available.IsNegative() {
			available = math.ZeroInt()
		}
		out = append(out, types.DenomBalance{
			Denom:     v.Denom,
			Total:     total.String(),
			Locked:    locked.String(),
			Available: available.String(),
		})
		return false, nil
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryTreasuryBalancesResponse{Balances: out}, nil
}

func (q queryServer) GetLock(ctx context.Context, req *types.QueryGetLockRequest) (*types.QueryGetLockResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	l, err := q.k.Lock.Get(ctx, req.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "lock %d not found", req.Id)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryGetLockResponse{Lock: l}, nil
}

func (q queryServer) ListLock(ctx context.Context, req *types.QueryAllLockRequest) (*types.QueryAllLockResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	out, page, err := query.CollectionPaginate(
		ctx, q.k.Lock, req.Pagination,
		func(_ uint64, v types.Lock) (types.Lock, error) { return v, nil },
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAllLockResponse{Lock: out, Pagination: page}, nil
}

func (q queryServer) LocksByTreasury(ctx context.Context, req *types.QueryLocksByTreasuryRequest) (*types.QueryLocksByTreasuryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	out, page, err := query.CollectionPaginate(
		ctx, q.k.LockByTreasury, req.Pagination,
		func(key collections.Pair[uint64, uint64], _ collections.NoValue) (types.Lock, error) {
			return q.k.Lock.Get(ctx, key.K2())
		},
		query.WithCollectionPaginationPairPrefix[uint64, uint64](req.TreasuryId),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryLocksByTreasuryResponse{Lock: out, Pagination: page}, nil
}

func (q queryServer) LocksByBeneficiary(ctx context.Context, req *types.QueryLocksByBeneficiaryRequest) (*types.QueryLocksByBeneficiaryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	out, page, err := query.CollectionPaginate(
		ctx, q.k.LockByBeneficiary, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (types.Lock, error) {
			return q.k.Lock.Get(ctx, key.K2())
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.Beneficiary),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryLocksByBeneficiaryResponse{Lock: out, Pagination: page}, nil
}

// ClaimableAmount answers the beneficiary's only real question: what can I take
// right now, and what is still to come?
func (q queryServer) ClaimableAmount(ctx context.Context, req *types.QueryClaimableAmountRequest) (*types.QueryClaimableAmountResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	l, err := q.k.Lock.Get(ctx, req.LockId)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "lock %d not found", req.LockId)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return &types.QueryClaimableAmountResponse{
		Claimable: types.ClaimableAmount(l, now).String(),
		Vested:    types.VestedAmount(l, now).String(),
		Remaining: types.RemainingAmount(l).String(),
	}, nil
}

func (q queryServer) ListRole(ctx context.Context, req *types.QueryListRoleRequest) (*types.QueryListRoleResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	out, page, err := query.CollectionPaginate(
		ctx, q.k.Role, req.Pagination,
		func(_ collections.Pair[uint64, string], v types.RoleAssignment) (types.RoleAssignment, error) {
			return v, nil
		},
		query.WithCollectionPaginationPairPrefix[uint64, string](req.TreasuryId),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryListRoleResponse{Role: out, Pagination: page}, nil
}

func (q queryServer) GetSpendPolicy(ctx context.Context, req *types.QueryGetSpendPolicyRequest) (*types.QueryGetSpendPolicyResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	p, err := q.k.SpendPolicy.Get(ctx, collections.Join(req.TreasuryId, req.Denom))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "treasury %d has no policy for %s", req.TreasuryId, req.Denom)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryGetSpendPolicyResponse{Policy: p}, nil
}

// SpendCapacity reports every bound on the next spend, so a client never shows
// an allowance the chain would refuse. The computation lives on the keeper so
// this query and the enforcement path cannot drift apart.
func (q queryServer) SpendCapacity(ctx context.Context, req *types.QuerySpendCapacityRequest) (*types.QuerySpendCapacityResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	capacity, err := q.k.SpendCapacityAt(ctx, req.TreasuryId, req.Denom, now)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QuerySpendCapacityResponse{
		RemainingThisPeriod: capacity.RemainingThisPeriod.String(),
		Available:           capacity.Available.String(),
		PerTransactionLimit: capacity.PerTransactionLimit,
		PeriodResetsAt:      capacity.PeriodResetsAt,
	}, nil
}

// getLock loads a lock or reports a clean not-found.
func (k Keeper) getLock(ctx context.Context, id uint64) (types.Lock, error) {
	l, err := k.Lock.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Lock{}, types.ErrLockNotFound.Wrapf("lock %d", id)
		}
		return types.Lock{}, err
	}
	return l, nil
}
