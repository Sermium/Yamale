package keeper_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/validatorgov/keeper"
	"yamale/blockchain/x/validatorgov/types"
)

func createNValidatorApplication(keeper keeper.Keeper, ctx context.Context, n int) []types.ValidatorApplication {
	items := make([]types.ValidatorApplication, n)
	for i := range items {
		items[i].Candidate = strconv.Itoa(i)
		items[i].Status = strconv.Itoa(i)
		_ = keeper.ValidatorApplication.Set(ctx, items[i].Candidate, items[i])
	}
	return items
}

func TestValidatorApplicationQuerySingle(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNValidatorApplication(f.keeper, f.ctx, 2)
	tests := []struct {
		desc     string
		request  *types.QueryGetValidatorApplicationRequest
		response *types.QueryGetValidatorApplicationResponse
		err      error
	}{
		{
			desc: "First",
			request: &types.QueryGetValidatorApplicationRequest{
				Candidate: msgs[0].Candidate,
			},
			response: &types.QueryGetValidatorApplicationResponse{ValidatorApplication: msgs[0]},
		},
		{
			desc: "Second",
			request: &types.QueryGetValidatorApplicationRequest{
				Candidate: msgs[1].Candidate,
			},
			response: &types.QueryGetValidatorApplicationResponse{ValidatorApplication: msgs[1]},
		},
		{
			desc: "KeyNotFound",
			request: &types.QueryGetValidatorApplicationRequest{
				Candidate: strconv.Itoa(100000),
			},
			err: status.Error(codes.NotFound, "not found"),
		},
		{
			desc: "InvalidRequest",
			err:  status.Error(codes.InvalidArgument, "invalid request"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			response, err := qs.GetValidatorApplication(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.response, response)
			}
		})
	}
}

func TestValidatorApplicationQueryPaginated(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNValidatorApplication(f.keeper, f.ctx, 5)

	request := func(next []byte, offset, limit uint64, total bool) *types.QueryAllValidatorApplicationRequest {
		return &types.QueryAllValidatorApplicationRequest{
			Pagination: &query.PageRequest{
				Key:        next,
				Offset:     offset,
				Limit:      limit,
				CountTotal: total,
			},
		}
	}
	t.Run("ByOffset", func(t *testing.T) {
		step := 2
		for i := 0; i < len(msgs); i += step {
			resp, err := qs.ListValidatorApplication(f.ctx, request(nil, uint64(i), uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ValidatorApplication), step)
			require.Subset(t, msgs, resp.ValidatorApplication)
		}
	})
	t.Run("ByKey", func(t *testing.T) {
		step := 2
		var next []byte
		for i := 0; i < len(msgs); i += step {
			resp, err := qs.ListValidatorApplication(f.ctx, request(next, 0, uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ValidatorApplication), step)
			require.Subset(t, msgs, resp.ValidatorApplication)
			next = resp.Pagination.NextKey
		}
	})
	t.Run("Total", func(t *testing.T) {
		resp, err := qs.ListValidatorApplication(f.ctx, request(nil, 0, 0, true))
		require.NoError(t, err)
		require.Equal(t, len(msgs), int(resp.Pagination.Total))
		require.EqualExportedValues(t, msgs, resp.ValidatorApplication)
	})
	t.Run("InvalidRequest", func(t *testing.T) {
		_, err := qs.ListValidatorApplication(f.ctx, nil)
		require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "invalid request"))
	})
}
