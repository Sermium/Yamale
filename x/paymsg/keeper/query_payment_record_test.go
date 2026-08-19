package keeper_test

import (
	"context"

	"cosmossdk.io/collections"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

func createNPaymentRecord(keeper keeper.Keeper, ctx context.Context, n int) []types.PaymentRecord {
	items := make([]types.PaymentRecord, n)
	for i := range items {
		items[i].EndToEndId = strconv.Itoa(i)
		items[i].InstructingParticipant = strconv.Itoa(i)
		items[i].InstructedParticipant = strconv.Itoa(i)
		items[i].Debtor = strconv.Itoa(i)
		items[i].Creditor = strconv.Itoa(i)
		items[i].Denom = strconv.Itoa(i)
		items[i].Amount = strconv.Itoa(i)
		items[i].PurposeCode = strconv.Itoa(i)
		items[i].RemittanceInformation = strconv.Itoa(i)
		items[i].BlockHeight = uint64(i)
		_ = keeper.PaymentRecord.Set(ctx, collections.Join(items[i].InstructingParticipant, items[i].EndToEndId), items[i])
	}
	return items
}

func TestPaymentRecordQuerySingle(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNPaymentRecord(f.keeper, f.ctx, 2)
	tests := []struct {
		desc     string
		request  *types.QueryGetPaymentRecordRequest
		response *types.QueryGetPaymentRecordResponse
		err      error
	}{
		{
			desc: "First",
			request: &types.QueryGetPaymentRecordRequest{
				EndToEndId: msgs[0].EndToEndId, InstructingParticipant: msgs[0].InstructingParticipant,
			},
			response: &types.QueryGetPaymentRecordResponse{PaymentRecord: msgs[0]},
		},
		{
			desc: "Second",
			request: &types.QueryGetPaymentRecordRequest{
				EndToEndId: msgs[1].EndToEndId, InstructingParticipant: msgs[1].InstructingParticipant,
			},
			response: &types.QueryGetPaymentRecordResponse{PaymentRecord: msgs[1]},
		},
		{
			desc: "KeyNotFound",
			request: &types.QueryGetPaymentRecordRequest{
				EndToEndId: strconv.Itoa(100000),
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
			response, err := qs.GetPaymentRecord(f.ctx, tc.request)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.response, response)
			}
		})
	}
}

func TestPaymentRecordQueryPaginated(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)
	msgs := createNPaymentRecord(f.keeper, f.ctx, 5)

	request := func(next []byte, offset, limit uint64, total bool) *types.QueryAllPaymentRecordRequest {
		return &types.QueryAllPaymentRecordRequest{
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
			resp, err := qs.ListPaymentRecord(f.ctx, request(nil, uint64(i), uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.PaymentRecord), step)
			require.Subset(t, msgs, resp.PaymentRecord)
		}
	})
	t.Run("ByKey", func(t *testing.T) {
		step := 2
		var next []byte
		for i := 0; i < len(msgs); i += step {
			resp, err := qs.ListPaymentRecord(f.ctx, request(next, 0, uint64(step), false))
			require.NoError(t, err)
			require.LessOrEqual(t, len(resp.PaymentRecord), step)
			require.Subset(t, msgs, resp.PaymentRecord)
			next = resp.Pagination.NextKey
		}
	})
	t.Run("Total", func(t *testing.T) {
		resp, err := qs.ListPaymentRecord(f.ctx, request(nil, 0, 0, true))
		require.NoError(t, err)
		require.Equal(t, len(msgs), int(resp.Pagination.Total))
		require.EqualExportedValues(t, msgs, resp.PaymentRecord)
	})
	t.Run("InvalidRequest", func(t *testing.T) {
		_, err := qs.ListPaymentRecord(f.ctx, nil)
		require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "invalid request"))
	})
}
