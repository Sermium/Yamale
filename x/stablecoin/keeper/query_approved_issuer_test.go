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

func createNApprovedIssuer(keeper keeper.Keeper, ctx context.Context, n int) []types.ApprovedIssuer {
	items := make([]types.ApprovedIssuer, n)
	for i := range items {
		items[i].Denom = strconv.Itoa(i)
		items[i].Issuer = strconv.Itoa(i)
		_ = keeper.ApprovedIssuer.Set(ctx, items[i].Denom, items[i])
	}
	return items
}

func TestApprovedIssuerQuerySingle(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNApprovedIssuer(f.keeper, f.ctx, 2)
	tests := []struct {
		desc     string
		request  *types.QueryGetApprovedIssuerRequest
		response *types.QueryGetApprovedIssuerResponse
		err      error
	}{
		{
			desc: "First",
			request: &types.QueryGetApprovedIssuerRequest{
				Denom: msgs[0].Denom,
			},
			response: &types.QueryGetApprovedIssuerResponse{ApprovedIssuer: msgs[0]},
		},
		{
			desc: "Second",
			request: &types.QueryGetApprovedIssuerRequest{
				Denom: msgs[1].Denom,
			},
			response: &types.QueryGetApprovedIssuerResponse{ApprovedIssuer: msgs[1]},
		},
		{
			desc: "KeyNotFound",
			request: &types.QueryGetApprovedIssuerRequest{
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
			response, err := qs.GetApprovedIssuer(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.response, response)
			}
		})
	}
}

func TestApprovedIssuerQueryPaginated(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNApprovedIssuer(f.keeper, f.ctx, 5)

	request := func(next []byte, offset, limit uint64, total bool) *types.QueryAllApprovedIssuerRequest {
		return &types.QueryAllApprovedIssuerRequest{
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
			resp, err := qs.ListApprovedIssuer(f.ctx, request(nil, uint64(i), uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ApprovedIssuer), step)
			require.Subset(t, msgs, resp.ApprovedIssuer)
		}
	})
	t.Run("ByKey", func(t *testing.T) {
		step := 2
		var next []byte
		for i := 0; i < len(msgs); i += step {
			resp, err := qs.ListApprovedIssuer(f.ctx, request(next, 0, uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ApprovedIssuer), step)
			require.Subset(t, msgs, resp.ApprovedIssuer)
			next = resp.Pagination.NextKey
		}
	})
	t.Run("Total", func(t *testing.T) {
		resp, err := qs.ListApprovedIssuer(f.ctx, request(nil, 0, 0, true))
		require.NoError(t, err)
		require.Equal(t, len(msgs), int(resp.Pagination.Total))
		require.EqualExportedValues(t, msgs, resp.ApprovedIssuer)
	})
	t.Run("InvalidRequest", func(t *testing.T) {
		_, err := qs.ListApprovedIssuer(f.ctx, nil)
		require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "invalid request"))
	})
}
