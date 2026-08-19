package keeper

import (
	"context"
	"sort"

	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	constitutiontypes "yamale/blockchain/x/constitution/types"
	"yamale/blockchain/x/validatorgov/types"
)

// Concentration reports what every declared entity, owner and jurisdiction
// holds against its ceiling.
//
// This is the supervisor's query, and it answers with the whole picture rather
// than one group at a time on purpose. Under equal seats a ceiling is a count
// out of a count, so the point of publishing it this way is that it can be
// checked against a list of admitted validators by somebody who is not
// recomputing anything — which is the entire argument for putting beneficial
// ownership on the chain instead of in a filing cabinet.
//
// It computes over the same function the epoch check uses, not a second
// implementation of the same arithmetic. A monitoring view that could disagree
// with the enforcement it is monitoring would be worse than none.
func (q queryServer) Concentration(ctx context.Context, req *types.QueryConcentrationRequest) (*types.QueryConcentrationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	inv, err := q.k.constitutionKeeper.GetInvariants(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	caps := types.CapsFrom(inv)

	holders, err := q.k.activeSeatHolders(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	total := types.TotalPower(holders)

	groups := make([]types.ConcentrationGroup, 0)
	for _, holding := range types.Holdings(holders) {
		capBps := caps.CapBps(holding.Cap)
		groups = append(groups, types.ConcentrationGroup{
			Cap:      holding.Cap,
			Group:    holding.Group,
			Power:    holding.Power,
			PowerBps: constitutiontypes.PowerBps(holding.Power, total),
			CapBps:   capBps,
			Over:     capBps > 0 && holding.Power > constitutiontypes.AllowedPower(total, capBps),
		})
	}

	return &types.QueryConcentrationResponse{
		Groups:              groups,
		TotalPower:          total,
		ActiveValidators:    uint32(len(holders)),
		MinActiveValidators: caps.MinActive,
	}, nil
}

// ListDemotion returns the demotions currently in force.
func (q queryServer) ListDemotion(ctx context.Context, req *types.QueryAllDemotionRequest) (*types.QueryAllDemotionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	demotions, pageRes, err := query.CollectionPaginate(
		ctx, q.k.Demotion, req.Pagination,
		func(_ string, value types.Demotion) (types.Demotion, error) { return value, nil },
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	sort.Slice(demotions, func(i, j int) bool { return demotions[i].Operator < demotions[j].Operator })

	return &types.QueryAllDemotionResponse{Demotion: demotions, Pagination: pageRes}, nil
}
