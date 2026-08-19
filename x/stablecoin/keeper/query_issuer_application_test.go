package keeper_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/stablecoin/keeper"
	"yamale/blockchain/x/stablecoin/types"
)

func createNIssuerApplication(keeper keeper.Keeper, ctx context.Context, n int) []types.IssuerApplication {
	items := make([]types.IssuerApplication, n)
	for i := range items {
		items[i].Denom = strconv.Itoa(i)
		items[i].Status = strconv.Itoa(i)
		_ = keeper.IssuerApplication.Set(ctx, items[i].Denom, items[i])
	}
	return items
}

func TestIssuerApplicationQuerySingle(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNIssuerApplication(f.keeper, f.ctx, 2)
	tests := []struct {
		desc     string
		request  *types.QueryGetIssuerApplicationRequest
		response *types.QueryGetIssuerApplicationResponse
		err      error
	}{
		{
			desc: "First",
			request: &types.QueryGetIssuerApplicationRequest{
				Denom: msgs[0].Denom,
			},
			response: &types.QueryGetIssuerApplicationResponse{IssuerApplication: msgs[0]},
		},
		{
			desc: "Second",
			request: &types.QueryGetIssuerApplicationRequest{
				Denom: msgs[1].Denom,
			},
			response: &types.QueryGetIssuerApplicationResponse{IssuerApplication: msgs[1]},
		},
		{
			desc: "KeyNotFound",
			request: &types.QueryGetIssuerApplicationRequest{
				Denom: strconv.Itoa(100000),
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
			response, err := qs.GetIssuerApplication(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.response, response)
			}
		})
	}
}

func TestIssuerApplicationQueryPaginated(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNIssuerApplication(f.keeper, f.ctx, 5)

	request := func(next []byte, offset, limit uint64, total bool) *types.QueryAllIssuerApplicationRequest {
		return &types.QueryAllIssuerApplicationRequest{
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
			resp, err := qs.ListIssuerApplication(f.ctx, request(nil, uint64(i), uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.IssuerApplication), step)
			require.Subset(t, msgs, resp.IssuerApplication)
		}
	})
	t.Run("ByKey", func(t *testing.T) {
		step := 2
		var next []byte
		for i := 0; i < len(msgs); i += step {
			resp, err := qs.ListIssuerApplication(f.ctx, request(next, 0, uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.IssuerApplication), step)
			require.Subset(t, msgs, resp.IssuerApplication)
			next = resp.Pagination.NextKey
		}
	})
	t.Run("Total", func(t *testing.T) {
		resp, err := qs.ListIssuerApplication(f.ctx, request(nil, 0, 0, true))
		require.NoError(t, err)
		require.Equal(t, len(msgs), int(resp.Pagination.Total))
		require.EqualExportedValues(t, msgs, resp.IssuerApplication)
	})
	t.Run("InvalidRequest", func(t *testing.T) {
		_, err := qs.ListIssuerApplication(f.ctx, nil)
		require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "invalid request"))
	})
}
