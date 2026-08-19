package keeper

import (
	"context"
	"errors"

	"yamale/blockchain/x/paymsg/types"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (q queryServer) ListPaymentRecord(ctx context.Context, req *types.QueryAllPaymentRecordRequest) (*types.QueryAllPaymentRecordResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	paymentRecords, pageRes, err := query.CollectionPaginate(
		ctx,
		q.k.PaymentRecord,
		req.Pagination,
		func(_ collections.Pair[string, string], value types.PaymentRecord) (types.PaymentRecord, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryAllPaymentRecordResponse{PaymentRecord: paymentRecords, Pagination: pageRes}, nil
}

func (q queryServer) GetPaymentRecord(ctx context.Context, req *types.QueryGetPaymentRecordRequest) (*types.QueryGetPaymentRecordResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	val, err := q.k.PaymentRecord.Get(ctx, collections.Join(req.InstructingParticipant, req.EndToEndId))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not found")
		}

		return nil, status.Error(codes.Internal, "internal error")
	}

	return &types.QueryGetPaymentRecordResponse{PaymentRecord: val}, nil
}
