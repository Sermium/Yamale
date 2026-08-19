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

func createNApprovedValidator(keeper keeper.Keeper, ctx context.Context, n int) []types.ApprovedValidator {
	items := make([]types.ApprovedValidator, n)
	for i := range items {
		items[i].Candidate = strconv.Itoa(i)
		items[i].Approved = strconv.Itoa(i)
		_ = keeper.ApprovedValidator.Set(ctx, items[i].Candidate, items[i])
	}
	return items
}

func TestApprovedValidatorQuerySingle(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNApprovedValidator(f.keeper, f.ctx, 2)
	tests := []struct {
		desc     string
		request  *types.QueryGetApprovedValidatorRequest
		response *types.QueryGetApprovedValidatorResponse
		err      error
	}{
		{
			desc: "First",
			request: &types.QueryGetApprovedValidatorRequest{
				Candidate: msgs[0].Candidate,
			},
			response: &types.QueryGetApprovedValidatorResponse{ApprovedValidator: msgs[0]},
		},
		{
			desc: "Second",
			request: &types.QueryGetApprovedValidatorRequest{
				Candidate: msgs[1].Candidate,
			},
			response: &types.QueryGetApprovedValidatorResponse{ApprovedValidator: msgs[1]},
		},
		{
			desc: "KeyNotFound",
			request: &types.QueryGetApprovedValidatorRequest{
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
			response, err := qs.GetApprovedValidator(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.response, response)
			}
		})
	}
}

func TestApprovedValidatorQueryPaginated(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNApprovedValidator(f.keeper, f.ctx, 5)

	request := func(next []byte, offset, limit uint64, total bool) *types.QueryAllApprovedValidatorRequest {
		return &types.QueryAllApprovedValidatorRequest{
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
			resp, err := qs.ListApprovedValidator(f.ctx, request(nil, uint64(i), uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ApprovedValidator), step)
			require.Subset(t, msgs, resp.ApprovedValidator)
		}
	})
	t.Run("ByKey", func(t *testing.T) {
		step := 2
		var next []byte
		for i := 0; i < len(msgs); i += step {
			resp, err := qs.ListApprovedValidator(f.ctx, request(next, 0, uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ApprovedValidator), step)
			require.Subset(t, msgs, resp.ApprovedValidator)
			next = resp.Pagination.NextKey
		}
	})
	t.Run("Total", func(t *testing.T) {
		resp, err := qs.ListApprovedValidator(f.ctx, request(nil, 0, 0, true))
		require.NoError(t, err)
		require.Equal(t, len(msgs), int(resp.Pagination.Total))
		require.EqualExportedValues(t, msgs, resp.ApprovedValidator)
	})
	t.Run("InvalidRequest", func(t *testing.T) {
		_, err := qs.ListApprovedValidator(f.ctx, nil)
		require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "invalid request"))
	})
}
