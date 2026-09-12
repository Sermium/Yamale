package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/selfcustody/types"
)

type queryServer struct{ Keeper }

// NewQueryServerImpl returns the read side of the door out.
func NewQueryServerImpl(keeper Keeper) types.QueryServer { return queryServer{Keeper: keeper} }

var _ types.QueryServer = queryServer{}

func (q queryServer) Params(
	ctx context.Context, _ *types.QueryParamsRequest,
) (*types.QueryParamsResponse, error) {
	params, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

// Disclosure reports whether one account's key has been taken.
//
// Answers rather than erroring when there is no record, for the same reason
// x/syndicate's investor lookup does: "this account is still in custody" and
// "the query failed" are opposite conclusions, and a NotFound makes them look
// identical at the call site. A counterparty reading this is deciding whether
// the platform's safeguards still stand behind an account, and they must not
// reach that decision through a failed request.
func (q queryServer) Disclosure(
	ctx context.Context, req *types.QueryDisclosureRequest,
) (*types.QueryDisclosureResponse, error) {
	if _, err := q.addressCodec.StringToBytes(req.Account); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%q is not an address", req.Account)
	}

	d, err := q.Keeper.Disclosure.Get(ctx, req.Account)
	if err != nil {
		if isNotFound(err) {
			return &types.QueryDisclosureResponse{Disclosed: false}, nil
		}
		return nil, err
	}
	return &types.QueryDisclosureResponse{Disclosed: true, Disclosure: d}, nil
}

// Request reports a pending request and when it becomes confirmable.
//
// The two deadlines are computed from the parameters in force now rather than
// read from the stored request. If governance lengthens the waiting period
// while somebody is inside it, their confirmation really will be refused until
// the longer period has run — so this must report the longer date, or the
// holder would be shown a time at which the chain will turn them away.
func (q queryServer) Request(
	ctx context.Context, req *types.QueryRequestRequest,
) (*types.QueryRequestResponse, error) {
	if _, err := q.addressCodec.StringToBytes(req.Account); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%q is not an address", req.Account)
	}

	r, err := q.Keeper.Request.Get(ctx, req.Account)
	if err != nil {
		if isNotFound(err) {
			return &types.QueryRequestResponse{Pending: false}, nil
		}
		return nil, err
	}

	params, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	return &types.QueryRequestResponse{
		Pending:    true,
		Request:    r,
		EligibleAt: r.RequestedAt + params.DisclosureDelay,
		ExpiresAt:  r.RequestedAt + params.RequestExpiry,
	}, nil
}

// Count is how many accounts have taken their keys, and how many are inside the
// waiting period.
//
// Published because the RATE is a public-interest fact — an operator quietly
// pushing customers out of custody, or a sudden spike that means an attack
// campaign is under way, should both be visible to anybody. Published as a
// count and not a list because the list would be a target roster, ordered by
// how easy each account is to rob.
func (q queryServer) Count(
	ctx context.Context, _ *types.QueryCountRequest,
) (*types.QueryCountResponse, error) {
	out := &types.QueryCountResponse{}

	if err := q.Keeper.Disclosure.Walk(ctx, nil, func(string, types.Disclosure) (bool, error) {
		out.Disclosed++
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := q.Keeper.Request.Walk(ctx, nil, func(string, types.Request) (bool, error) {
		out.Pending++
		return false, nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}
