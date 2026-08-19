package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/enforcement/types"
)

// Everything this module does is public by construction. A power to freeze and
// seize that could not be inspected from outside would be indistinguishable
// from an arbitrary one, so the case, its grounds, its evidence and every vote
// on it are all queryable by anyone, resolved or not.

type queryServer struct {
	k Keeper
}

// NewQueryServerImpl returns an implementation of the QueryServer interface.
func NewQueryServerImpl(k Keeper) types.QueryServer {
	return queryServer{k: k}
}

var _ types.QueryServer = queryServer{}

// Params returns the module parameters.
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

// GetCase returns one case with every vote cast on it.
func (q queryServer) GetCase(ctx context.Context, req *types.QueryGetCaseRequest) (*types.QueryGetCaseResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	enforcementCase, err := q.k.Case.Get(ctx, req.Id)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "case %d not found", req.Id)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	votes, err := q.votesOf(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryGetCaseResponse{Case: enforcementCase, Votes: votes}, nil
}

// ListCase returns a page of cases.
func (q queryServer) ListCase(ctx context.Context, req *types.QueryListCaseRequest) (*types.QueryListCaseResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	cases, pageRes, err := query.CollectionPaginate(
		ctx, q.k.Case, req.Pagination,
		func(_ uint64, value types.Case) (types.Case, error) { return value, nil },
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryListCaseResponse{Case: cases, Pagination: pageRes}, nil
}

// OpenCases returns the cases still being voted on.
//
// This is the query a validator runs to find out what is waiting for them.
// It walks the voting queue rather than every case ever opened, so it stays
// cheap on a chain with a long history.
func (q queryServer) OpenCases(ctx context.Context, req *types.QueryOpenCasesRequest) (*types.QueryOpenCasesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	open := make([]types.Case, 0)
	err := q.k.VotingQueue.Walk(ctx, nil, func(key collections.Pair[int64, uint64]) (bool, error) {
		enforcementCase, err := q.k.Case.Get(ctx, key.K2())
		if err != nil {
			return false, err
		}
		if enforcementCase.Status == types.CASE_STATUS_VOTING {
			open = append(open, enforcementCase)
		}
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryOpenCasesResponse{Case: open}, nil
}

// CaseVotes returns the votes on a case and the tally they add up to.
func (q queryServer) CaseVotes(ctx context.Context, req *types.QueryCaseVotesRequest) (*types.QueryCaseVotesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	enforcementCase, err := q.k.Case.Get(ctx, req.CaseId)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "case %d not found", req.CaseId)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	votes, err := q.votesOf(ctx, req.CaseId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	params, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryCaseVotesResponse{
		Votes:            votes,
		YesPower:         enforcementCase.YesPower,
		NoPower:          enforcementCase.NoPower,
		AbstainPower:     enforcementCase.AbstainPower,
		TotalPowerAtOpen: enforcementCase.TotalPowerAtOpen,
		RequiredPower:    params.RequiredPower(enforcementCase.TotalPowerAtOpen),
	}, nil
}

// FreezeStatus answers whether an address may send, and on what grounds if not.
//
// It returns the case alongside the freeze deliberately. Somebody whose
// transfer was refused needs to know who accused them and why, and making that
// two queries means an interface will show them only the first one.
func (q queryServer) FreezeStatus(ctx context.Context, req *types.QueryFreezeStatusRequest) (*types.QueryFreezeStatusResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	freeze, found, err := q.k.FreezeOf(ctx, req.Address)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !found {
		return &types.QueryFreezeStatusResponse{Frozen: false}, nil
	}

	enforcementCase, err := q.k.Case.Get(ctx, freeze.CaseId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryFreezeStatusResponse{Frozen: true, Freeze: freeze, Case: enforcementCase}, nil
}

// ListFreeze returns a page of frozen addresses.
func (q queryServer) ListFreeze(ctx context.Context, req *types.QueryListFreezeRequest) (*types.QueryListFreezeResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	freezes, pageRes, err := query.CollectionPaginate(
		ctx, q.k.Freeze, req.Pagination,
		func(_ string, value types.Freeze) (types.Freeze, error) { return value, nil },
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryListFreezeResponse{Freeze: freezes, Pagination: pageRes}, nil
}

// Recovered totals what this module has taken, and how often.
func (q queryServer) Recovered(ctx context.Context, req *types.QueryRecoveredRequest) (*types.QueryRecoveredResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	total, err := q.k.TotalRecovered(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	opened, err := q.k.CaseSeq.Peek(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	// The sequence holds the id the next case would take, and ids start at one.
	if opened > 0 {
		opened--
	}

	passed, err := q.k.CasesPassed.Get(ctx)
	if err != nil && !isNotFound(err) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryRecoveredResponse{Total: total, CasesOpened: opened, CasesPassed: passed}, nil
}

// HeldCases returns the seizures that have been agreed and are waiting out
// their delay.
//
// This is the ombudsman's list, and it is the reason the delay is worth having:
// everything still stoppable at no cost to anybody, with the height it stops
// being stoppable at. An office that had to poll every case ever opened to find
// out what it could still veto would find out too late.
//
// It walks the execution queue rather than every case, so it stays cheap, and
// the queue is ordered by execute height — so the answer comes back soonest
// first, which is the order somebody deciding what to look at needs.
func (q queryServer) HeldCases(ctx context.Context, req *types.QueryHeldCasesRequest) (*types.QueryHeldCasesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	// One entry per case, so no de-duplication is needed: a deferral moves the
	// entry and the case's execute_at_height together rather than adding a
	// second one. If that ever stopped being true this query would report the
	// chain as about to seize from the same account twice.
	held := make([]types.Case, 0)
	err := q.k.ExecutionQueue.Walk(ctx, nil, func(key collections.Pair[int64, uint64]) (bool, error) {
		enforcementCase, err := q.k.Case.Get(ctx, key.K2())
		if err != nil {
			return false, err
		}
		if enforcementCase.Status == types.CASE_STATUS_HELD {
			held = append(held, enforcementCase)
		}
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryHeldCasesResponse{Case: held}, nil
}

// SeizureWindow reports how much of the rolling cap is left.
//
// The window's start height is returned alongside the totals rather than left
// for the caller to work out from the parameters, because a caller that
// computed it themselves could compute it differently from the chain — and the
// number that matters is the one the chain will actually enforce.
func (q queryServer) SeizureWindow(ctx context.Context, req *types.QuerySeizureWindowRequest) (*types.QuerySeizureWindowResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	params, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	seized, count, err := q.k.seizureWindow(ctx, params, height)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Only the denominations the cap names, and never below zero. A "remaining"
	// that went negative would read as a debt the chain was owed rather than as
	// a limit that has been reached.
	remaining := sdk.NewCoins()
	for _, capped := range params.SeizureWindowCap {
		left := capped.Amount.Sub(seized.AmountOf(capped.Denom))
		if left.IsPositive() {
			remaining = remaining.Add(sdk.NewCoin(capped.Denom, left))
		}
	}

	return &types.QuerySeizureWindowResponse{
		WindowStartHeight: params.WindowStartHeight(height),
		CurrentHeight:     height,
		Seized:            seized,
		Cap:               params.SeizureWindowCap,
		Remaining:         remaining,
		SeizureCount:      count,
		MaxSeizures:       params.MaxSeizuresPerWindow,
	}, nil
}

func (q queryServer) votesOf(ctx context.Context, caseID uint64) ([]types.Vote, error) {
	votes := make([]types.Vote, 0)
	rng := collections.NewPrefixedPairRange[uint64, string](caseID)
	err := q.k.Vote.Walk(ctx, rng, func(_ collections.Pair[uint64, string], vote types.Vote) (bool, error) {
		votes = append(votes, vote)
		return false, nil
	})
	return votes, err
}
