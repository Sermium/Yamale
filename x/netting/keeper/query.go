package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/netting/types"
)

// What this service does and does not do about read authorisation is a design
// decision rather than an omission, and it is worth stating where the code is.
//
// A gRPC query carries no signature. The chain does not know who is asking, so
// there is nobody for it to authorise, and a handler that refused some callers
// would be refusing on the strength of a claim in the request — which is not a
// check, it is a suggestion. Worse, anybody who can run a node reads the state
// store directly and never reaches this code at all; on a permissioned network
// every participant runs one.
//
// So the confidentiality this module provides comes from somewhere else
// entirely: the customer payments it exists to keep off the chain are never
// written, and a record that does not exist cannot be read by a competitor
// bank, a node operator, or a future compromise of either. What is on-chain is
// the interbank layer, and that is public to the participants by design —
// they are the ones settling with each other.
//
// The shaping here is still worth having, and it is worth being precise about
// what it buys. Every endpoint that returns obligations demands a participant,
// and none of them enumerates the bilateral matrix. That does not hide the
// graph from a determined node operator. It does keep it out of the indexers
// and explorers that consume whatever the REST gateway offers — the difference
// between a graph that is technically public and one that is published.
//
// Per-caller scoping happens at the authenticated proxy in front of the REST
// gateway, which is the only layer that knows who the caller is. That is a
// deployment convention and is described as one in docs/guides/settlement.md,
// never as a chain-enforced guarantee.

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

// CurrentCycle returns the open window and the height it closes at.
func (q queryServer) CurrentCycle(ctx context.Context, req *types.QueryCurrentCycleRequest) (*types.QueryCurrentCycleResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	id, err := q.k.CurrentCycle.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	cycle, err := q.k.Cycle.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	params, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Zero when netting is off, because then no block ever closes it. Reported
	// rather than computed anyway: the same guard that keeps the end blocker
	// from dividing by zero has to keep this query from doing it, and a query
	// process that panics takes the node's RPC down with it.
	var closesAt int64
	if params.NettingEnabled() {
		blocks := int64(params.CycleBlocks) //nolint:gosec // NettingEnabled bounds CycleBlocks well inside int64
		closesAt = ((cycle.OpenedAtHeight / blocks) + 1) * blocks
	}

	return &types.QueryCurrentCycleResponse{Cycle: cycle, ClosesAtHeight: closesAt}, nil
}

// Cycle returns one window and the compression it achieved, currency by
// currency.
func (q queryServer) Cycle(ctx context.Context, req *types.QueryCycleRequest) (*types.QueryCycleResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	cycle, err := q.k.Cycle.Get(ctx, req.Id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "cycle %d not found", req.Id)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	compression := make([]types.DenomCompression, 0, len(cycle.Outcomes))
	for _, outcome := range cycle.Outcomes {
		compression = append(compression, types.DenomCompression{
			Denom:          outcome.Denom,
			CompressionBps: types.CompressionBps(outcome.GrossAmount, outcome.NetAmount),
		})
	}

	return &types.QueryCycleResponse{Cycle: cycle, Compression: compression}, nil
}

// Position returns one participant's reserve, its committed part and its
// running position in the open window, per currency.
func (q queryServer) Position(ctx context.Context, req *types.QueryPositionRequest) (*types.QueryPositionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	if req.Participant == "" {
		return nil, status.Error(codes.InvalidArgument, "participant is required")
	}

	currentCycle, err := q.k.CurrentCycle.Get(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Walked in store order, which is denom order within the participant's
	// prefix, so the response is stable between calls rather than reshuffling
	// on every read.
	entries := make([]types.PositionEntry, 0)
	rng := collections.NewPrefixedPairRange[string, string](req.Participant)
	err = q.k.Reserve.Walk(ctx, rng, func(key collections.Pair[string, string], reserve math.Int) (bool, error) {
		denom := key.K2()
		locked, err := q.k.GetLocked(ctx, req.Participant, denom)
		if err != nil {
			return false, err
		}
		available, err := q.k.Available(ctx, req.Participant, denom)
		if err != nil {
			return false, err
		}
		position, err := q.k.GetPosition(ctx, currentCycle, denom, req.Participant)
		if err != nil {
			return false, err
		}
		entries = append(entries, types.PositionEntry{
			Denom:       denom,
			Reserve:     reserve,
			Locked:      locked,
			Available:   available,
			NetPosition: position,
		})
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryPositionResponse{Entries: entries}, nil
}

// ParticipantObligations returns the obligations one participant is a party to
// in one cycle, on either side.
func (q queryServer) ParticipantObligations(ctx context.Context, req *types.QueryParticipantObligationsRequest) (*types.QueryParticipantObligationsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	if req.Participant == "" {
		return nil, status.Error(codes.InvalidArgument, "participant is required")
	}

	// Paginated over the index rather than over the obligations, so the cost of
	// one participant's page does not depend on how much traffic everybody else
	// put through the same window.
	prefix := collections.TripleSuperPrefix[string, uint64, uint64](req.Participant, req.CycleId)
	keys, pageRes, err := query.CollectionPaginate(
		ctx, q.k.ObligationByParticipant, req.Pagination,
		func(key collections.Triple[string, uint64, uint64], _ collections.NoValue) (collections.Pair[uint64, uint64], error) {
			return collections.Join(key.K2(), key.K3()), nil
		},
		func(opt *query.CollectionsPaginateOptions[collections.Triple[string, uint64, uint64]]) {
			opt.Prefix = &prefix
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	obligations := make([]types.Obligation, 0, len(keys))
	for _, key := range keys {
		obligation, err := q.k.Obligation.Get(ctx, key)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		obligations = append(obligations, obligation)
	}

	return &types.QueryParticipantObligationsResponse{Obligations: obligations, Pagination: pageRes}, nil
}

// HeldSlices returns every currency slice that failed to settle.
//
// It is the operational alarm, and it is a chain query rather than a metric
// because the thing being reported is money participants believe is settled
// and is not. Anybody may read it, including the institutions carrying the
// exposure — especially them.
func (q queryServer) HeldSlices(ctx context.Context, req *types.QueryHeldSlicesRequest) (*types.QueryHeldSlicesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	held := make([]types.HeldSlice, 0)
	err := q.k.HeldSlice.Walk(ctx, nil, func(key collections.Pair[uint64, string]) (bool, error) {
		cycle, err := q.k.Cycle.Get(ctx, key.K1())
		if err != nil {
			return false, err
		}
		reason := ""
		for _, outcome := range cycle.Outcomes {
			if outcome.Denom == key.K2() {
				reason = outcome.HoldReason
				break
			}
		}
		held = append(held, types.HeldSlice{
			CycleId: key.K1(),
			Denom:   key.K2(),
			Reason:  reason,
			// The block the cycle closed in is the block the slice has been
			// stuck since; there is no separate moment for it to have started.
			HeldSinceHeight: cycle.ClosedAtHeight,
		})
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryHeldSlicesResponse{Held: held}, nil
}
