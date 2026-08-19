package keeper_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/builderfee/keeper"
	"yamale/blockchain/x/builderfee/types"
)

func createNBuilderApplication(keeper keeper.Keeper, ctx context.Context, n int) []types.BuilderApplication {
	items := make([]types.BuilderApplication, n)
	for i := range items {
		items[i].MsgTypeUrl = strconv.Itoa(i)
		items[i].Status = strconv.Itoa(i)
		items[i].PayoutAddress = strconv.Itoa(i)
		items[i].Creator = strconv.Itoa(i)
		_ = keeper.BuilderApplication.Set(ctx, items[i].MsgTypeUrl, items[i])
	}
	return items
}

func TestBuilderApplicationQuerySingle(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNBuilderApplication(f.keeper, f.ctx, 2)
	tests := []struct {
		desc     string
		request  *types.QueryGetBuilderApplicationRequest
		response *types.QueryGetBuilderApplicationResponse
		err      error
	}{
		{
			desc: "First",
			request: &types.QueryGetBuilderApplicationRequest{
				MsgTypeUrl: msgs[0].MsgTypeUrl,
			},
			response: &types.QueryGetBuilderApplicationResponse{BuilderApplication: msgs[0]},
		},
		{
			desc: "Second",
			request: &types.QueryGetBuilderApplicationRequest{
				MsgTypeUrl: msgs[1].MsgTypeUrl,
			},
			response: &types.QueryGetBuilderApplicationResponse{BuilderApplication: msgs[1]},
		},
		{
			desc: "KeyNotFound",
			request: &types.QueryGetBuilderApplicationRequest{
				MsgTypeUrl: strconv.Itoa(100000),
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
			response, err := qs.GetBuilderApplication(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.response, response)
			}
		})
	}
}

func TestBuilderApplicationQueryPaginated(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNBuilderApplication(f.keeper, f.ctx, 5)

	request := func(next []byte, offset, limit uint64, total bool) *types.QueryAllBuilderApplicationRequest {
		return &types.QueryAllBuilderApplicationRequest{
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
			resp, err := qs.ListBuilderApplication(f.ctx, request(nil, uint64(i), uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.BuilderApplication), step)
			require.Subset(t, msgs, resp.BuilderApplication)
		}
	})
	t.Run("ByKey", func(t *testing.T) {
		step := 2
		var next []byte
		for i := 0; i < len(msgs); i += step {
			resp, err := qs.ListBuilderApplication(f.ctx, request(next, 0, uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.BuilderApplication), step)
			require.Subset(t, msgs, resp.BuilderApplication)
			next = resp.Pagination.NextKey
		}
	})
	t.Run("Total", func(t *testing.T) {
		resp, err := qs.ListBuilderApplication(f.ctx, request(nil, 0, 0, true))
		require.NoError(t, err)
		require.Equal(t, len(msgs), int(resp.Pagination.Total))
		require.EqualExportedValues(t, msgs, resp.BuilderApplication)
	})
	t.Run("InvalidRequest", func(t *testing.T) {
		_, err := qs.ListBuilderApplication(f.ctx, nil)
		require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "invalid request"))
	})
}
