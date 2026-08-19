package keeper_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

func createNApprovedParticipant(keeper keeper.Keeper, ctx context.Context, n int) []types.ApprovedParticipant {
	items := make([]types.ApprovedParticipant, n)
	for i := range items {
		items[i].Participant = strconv.Itoa(i)
		items[i].Code = strconv.Itoa(i)
		items[i].Name = strconv.Itoa(i)
		_ = keeper.ApprovedParticipant.Set(ctx, items[i].Participant, items[i])
	}
	return items
}

func TestApprovedParticipantQuerySingle(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNApprovedParticipant(f.keeper, f.ctx, 2)
	tests := []struct {
		desc     string
		request  *types.QueryGetApprovedParticipantRequest
		response *types.QueryGetApprovedParticipantResponse
		err      error
	}{
		{
			desc: "First",
			request: &types.QueryGetApprovedParticipantRequest{
				Participant: msgs[0].Participant,
			},
			response: &types.QueryGetApprovedParticipantResponse{ApprovedParticipant: msgs[0]},
		},
		{
			desc: "Second",
			request: &types.QueryGetApprovedParticipantRequest{
				Participant: msgs[1].Participant,
			},
			response: &types.QueryGetApprovedParticipantResponse{ApprovedParticipant: msgs[1]},
		},
		{
			desc: "KeyNotFound",
			request: &types.QueryGetApprovedParticipantRequest{
				Participant: strconv.Itoa(100000),
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
			response, err := qs.GetApprovedParticipant(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.response, response)
			}
		})
	}
}

func TestApprovedParticipantQueryPaginated(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNApprovedParticipant(f.keeper, f.ctx, 5)

	request := func(next []byte, offset, limit uint64, total bool) *types.QueryAllApprovedParticipantRequest {
		return &types.QueryAllApprovedParticipantRequest{
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
			resp, err := qs.ListApprovedParticipant(f.ctx, request(nil, uint64(i), uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ApprovedParticipant), step)
			require.Subset(t, msgs, resp.ApprovedParticipant)
		}
	})
	t.Run("ByKey", func(t *testing.T) {
		step := 2
		var next []byte
		for i := 0; i < len(msgs); i += step {
			resp, err := qs.ListApprovedParticipant(f.ctx, request(next, 0, uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.ApprovedParticipant), step)
			require.Subset(t, msgs, resp.ApprovedParticipant)
			next = resp.Pagination.NextKey
		}
	})
	t.Run("Total", func(t *testing.T) {
		resp, err := qs.ListApprovedParticipant(f.ctx, request(nil, 0, 0, true))
		require.NoError(t, err)
		require.Equal(t, len(msgs), int(resp.Pagination.Total))
		require.EqualExportedValues(t, msgs, resp.ApprovedParticipant)
	})
	t.Run("InvalidRequest", func(t *testing.T) {
		_, err := qs.ListApprovedParticipant(f.ctx, nil)
		require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "invalid request"))
	})
}
