package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/alias/types"
)

// ViewingKeys returns every version of one account's viewing key, newest first.
//
// Every version, not just the live one, because a payload sealed last year is
// sealed to the key that was live last year. A client that could only fetch the
// current key would report an old but perfectly readable payment as
// undecryptable — the exact failure the version field exists to prevent.
//
// An account that has published none comes back with an empty list rather than
// NotFound. That is an answer a sender can act on: it cannot be sent an
// encrypted payload, which is a fact about the world, not a failed request to
// retry.
func (q queryServer) ViewingKeys(ctx context.Context, req *types.QueryViewingKeysRequest) (*types.QueryViewingKeysResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if _, err := q.addressCodec.StringToBytes(req.Address); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid address")
	}

	keys := []types.ViewingKey{}
	rng := collections.NewPrefixedPairRange[string, uint64](req.Address).Descending()
	err := q.Keeper.ViewingKeys.Walk(ctx, rng, func(_ collections.Pair[string, uint64], v types.ViewingKey) (bool, error) {
		keys = append(keys, v)
		return false, nil
	})
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryViewingKeysResponse{Keys: keys}, nil
}

// Regulator returns the authority appointed over one country, with its current
// viewing key.
//
// Both in one response because the sender needs both, and asking separately
// invites the half-answer: an appointment resolved, a key not fetched, and an
// envelope built without the regulator on it that looks complete to everybody
// until the regulator tries to open it.
//
// The key comes back with an empty public_key when the regulator has been
// appointed but has published none. A sender must see that rather than be
// handed thirty-two zero bytes, which would seal an envelope that appears
// addressed to the regulator and opens for nobody.
func (q queryServer) Regulator(ctx context.Context, req *types.QueryRegulatorRequest) (*types.QueryRegulatorResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	country := types.NormaliseCountry(req.Country)
	if !types.AssignedCountry(country) {
		return nil, status.Error(codes.InvalidArgument, types.ErrInvalidCountry.Error())
	}

	appointment, err := q.Regulators.Get(ctx, country)
	if err != nil {
		return nil, status.Error(codes.NotFound, types.ErrNoRegulator.Error())
	}

	key, found, err := q.LatestViewingKey(ctx, appointment.Address)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !found {
		key = types.ViewingKey{Address: appointment.Address}
	}
	return &types.QueryRegulatorResponse{Appointment: appointment, Key: key}, nil
}

// Auditors lists the grants that have not expired, with their current keys.
//
// A list endpoint in a module that avoids them, and it is the right call here:
// who may read across accounts is a fact the people being read about are
// entitled to see, and a sender cannot seal a correct envelope without the
// whole set. The list is bounded by MaxLiveAuditorGrants, so it is not the
// unbounded scan the module refuses elsewhere.
//
// Expired grants are filtered out here rather than deleted from state, because
// "who could read this payment in 2026" is asked years afterwards and a store
// that pruned them would answer "nobody" — which is worse than not answering,
// because it looks like an answer.
func (q queryServer) Auditors(ctx context.Context, req *types.QueryAuditorsRequest) (*types.QueryAuditorsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()

	auditors := []types.AuditorEntitlement{}
	err := q.AuditorGrants.Walk(ctx, nil, func(_ string, g types.AuditorGrant) (bool, error) {
		if !g.Live(height) {
			return false, nil
		}
		key, found, err := q.LatestViewingKey(ctx, g.Address)
		if err != nil {
			return true, err
		}
		if !found {
			key = types.ViewingKey{Address: g.Address}
		}
		auditors = append(auditors, types.AuditorEntitlement{Grant: g, Key: key})
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryAuditorsResponse{Auditors: auditors}, nil
}
