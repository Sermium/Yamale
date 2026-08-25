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

// PayloadReaders lists every account entitled to be sealed into the encrypted
// payload of a payment settling in one country.
//
// This is what makes ROLE_SUPERVISOR a role rather than a name in a registry.
// The chain cannot gate a READ by role — a gRPC query carries no signer, so
// there is nobody to check — and pretending otherwise would be worse than an
// empty role. What it can do is publish, authoritatively and from the grant
// registry itself, the set a sender has to wrap the content key to. The
// entitlement is then realised by a write: the sender's, when it seals.
//
// Two sources, one answer, and the order is deliberate. The appointed regulator
// first, because it is the one entitlement that also carries standing to act on
// the payment; then the supervisors, chain-wide grants included, because a
// chain-wide supervisor watches every country and a sender that left it out
// would produce a payload it cannot open.
//
// Deduplicated on the address, because one account can legitimately be both —
// the regulator of NG holds ROLE_SUPERVISOR in NG by the rule AppointRegulator
// now enforces — and an envelope with two recipient blocks for the same key is
// not wrong so much as a claim that the reader is two people. The first basis
// wins, so an account that is both reads as the regulator, which is the stronger
// of the two statements.
//
// Not paginated, and an office that has fallen below its recorded shape is
// still listed. Both are decisions rather than omissions:
//
//   - a sender that received one page of an entitlement set would build an
//     envelope that looks complete and leaves an entitled reader out, with
//     nothing detecting it until that reader tried to open a payload months
//     later. The set is offices, not population.
//   - the shape governs ACTING. A supervisor that lost a member has not lost
//     the private key it reads with, and cutting it out of the recipient set
//     would remove a regulator's visibility exactly while its governance is in
//     disarray — which is the moment oversight is most wanted. Refusing an
//     office an action it cannot properly authorise is a different thing from
//     refusing it sight of what it supervises.
//
// An empty response is a real answer: a country with no appointed regulator and
// no supervisor has nobody entitled, and the payment is readable by its two
// parties and any live auditor. It is not an error and a sender must not retry.
func (q queryServer) PayloadReaders(ctx context.Context, req *types.QueryPayloadReadersRequest) (*types.QueryPayloadReadersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	// AssignedCountry, not IssuableCountry, and not NormaliseScope: the
	// chain-wide marker and the foundation's reserved code are both refused, for
	// the same reason AssertScopeIn refuses them. No payment settles chain-wide,
	// and one declaring the absence of a national perimeter would be one no
	// authority is accountable for.
	country := types.NormaliseCountry(req.Country)
	if !types.AssignedCountry(country) {
		return nil, status.Error(codes.InvalidArgument, types.ErrInvalidCountry.Error())
	}

	readers := []types.PayloadReader{}
	seen := map[string]struct{}{}
	add := func(address string, basis types.PayloadReaderBasis, scope string) error {
		if _, dup := seen[address]; dup {
			return nil
		}
		seen[address] = struct{}{}
		key, found, err := q.LatestViewingKey(ctx, address)
		if err != nil {
			return err
		}
		// An entitled account that has published no key comes back with an empty
		// public_key rather than being dropped from the list. Dropping it would
		// tell a sender there is nobody to seal to, which is a different fact
		// from "there is somebody and they cannot be sealed to yet" — and only
		// the second one has an operator who can fix it.
		if !found {
			key = types.ViewingKey{Address: address}
		}
		readers = append(readers, types.PayloadReader{
			Address: address,
			Basis:   basis,
			Scope:   scope,
			Key:     key,
		})
		return nil
	}

	appointment, err := q.Regulators.Get(ctx, country)
	switch {
	case err == nil:
		if err := add(appointment.Address, types.PAYLOAD_READER_REGULATOR, ""); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	case !errors.Is(err, collections.ErrNotFound):
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Two exact prefix scans rather than one walk of every grant on the chain,
	// and they are the same two lookups assertGranted makes for the same
	// question — the country's own supervisors, and the ones no border bounds.
	// Reading them off GrantsByScope keeps this query's cost proportional to what
	// has been granted over this country rather than to what the chain has ever
	// granted anybody.
	for _, scope := range [...]string{country, types.ChainWide} {
		rng := collections.NewSuperPrefixedTripleRange[string, int32, string](scope, int32(types.ROLE_SUPERVISOR))
		err := q.GrantsByScope.Walk(ctx, rng, func(key collections.Triple[string, int32, string]) (bool, error) {
			return false, add(key.K3(), types.PAYLOAD_READER_SUPERVISOR, scope)
		})
		if err != nil && !errors.Is(err, collections.ErrNotFound) {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &types.QueryPayloadReadersResponse{Readers: readers}, nil
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
