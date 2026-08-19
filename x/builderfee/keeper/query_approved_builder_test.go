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

func createNApprovedBuilder(keeper keeper.Keeper, ctx context.Context, n int) []types.ApprovedBuilder {
	items := make([]types.ApprovedBuilder, n)
	for i := range items {
		items[i].MsgTypeUrl = strconv.Itoa(i)
		items[i].PayoutAddress = strconv.Itoa(i)
		_ = keeper.ApprovedBuilder.Set(ctx, items[i].MsgTypeUrl, items[i])
	}
	return items
}

func TestApprovedBuilderQuerySingle(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNApprovedBuilder(f.keeper, f.ctx, 2)
	tests := []struct {
		desc     string
		request  *types.QueryGetApprovedBuilderRequest
		response *types.QueryGetApprovedBuilderResponse
		err      error
	}{
		{
			desc: "First",
			request: &types.QueryGetApprovedBuilderRequest{
				MsgTypeUrl: msgs[0].MsgTypeUrl,
			},
			response: &types.QueryGetApprovedBuilderResponse{ApprovedBuilder: msgs[0]},
		},
		{
			desc: "Second",
			request: &types.QueryGetApprovedBuilderRequest{
				MsgTypeUrl: msgs[1].MsgTypeUrl,
			},
			response: &types.QueryGetApprovedBuilderResponse{ApprovedBuilder: msgs[1]},
		},
		{
			desc: "KeyNotFound",
			request: &types.QueryGetApprovedBuilderRequest{
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
			response, err := qs.GetApprovedBuilder(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.response, response)
			}
		})
	}
}

func TestApprovedBuilderQueryPaginated(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNApprovedBuilder(f.keeper, f.ctx, 5)

	request := func(next []byte, offset, limit uint64, total bool) *types.QueryAllApprovedBuilderRequest {
		return &types.QueryAllApprovedBuilderRequest{
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
			resp, err := qs.ListApprovedBuilder(f.ctx, request(nil, uint64(i), uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ApprovedBuilder), step)
			require.Subset(t, msgs, resp.ApprovedBuilder)
		}
	})
	t.Run("ByKey", func(t *testing.T) {
		step := 2
		var next []byte
		for i := 0; i < len(msgs); i += step {
			resp, err := qs.ListApprovedBuilder(f.ctx, request(next, 0, uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ApprovedBuilder), step)
			require.Subset(t, msgs, resp.ApprovedBuilder)
			next = resp.Pagination.NextKey
		}
	})
	t.Run("Total", func(t *testing.T) {
		resp, err := qs.ListApprovedBuilder(f.ctx, request(nil, 0, 0, true))
		require.NoError(t, err)
		require.Equal(t, len(msgs), int(resp.Pagination.Total))
		require.EqualExportedValues(t, msgs, resp.ApprovedBuilder)
	})
	t.Run("InvalidRequest", func(t *testing.T) {
		_, err := qs.ListApprovedBuilder(f.ctx, nil)
		require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "invalid request"))
	})
}
