package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/land/types"
)

// RegisterParcel records a parcel for the first time.
//
// This is where "a piece of land cannot be owned twice" is actually enforced.
// Everything else in the module — the quorum, the challenge window — protects a
// title once it exists. This protects the act of creating one, which is the
// other half of the fraud: a second deed issued over ground somebody already
// holds.
func (m msgServer) RegisterParcel(
	ctx context.Context, msg *types.MsgRegisterParcel,
) (*types.MsgRegisterParcelResponse, error) {
	office, err := m.activeAuthority(ctx, msg.Creator)
	if err != nil {
		return nil, err
	}

	if msg.GeometryHash == "" {
		return nil, types.ErrNoGeometry
	}
	if msg.CadastralRef == "" {
		return nil, types.ErrNoCadastralRef
	}
	if _, err := m.addressCodec.StringToBytes(msg.Holder); err != nil {
		return nil, types.ErrInvalidHolder
	}

	// The uniqueness check, against an index rather than a scan. A check whose
	// cost grows with the registry is a check that eventually gets removed.
	if existing, err := m.ByGeometry.Get(ctx, msg.GeometryHash); err == nil {
		return nil, types.ErrGeometryTitled.Wrapf("parcel %d", existing)
	} else if !isNotFound(err) {
		return nil, err
	}

	// The registry's own reference must also be unique, or two records claim to
	// be the same file and reconciliation with the paper world becomes guesswork.
	if existing, err := m.ByRef.Get(ctx, msg.CadastralRef); err == nil {
		return nil, types.ErrRefTaken.Wrapf("parcel %d", existing)
	} else if !isNotFound(err) {
		return nil, err
	}

	id, err := m.NextParcelID.Next(ctx)
	if err != nil {
		return nil, err
	}
	if id == 0 {
		// Parcel 0 is never issued. In proto3 a zero id is indistinguishable
		// from an unset field, and x/tokenisation marks a vehicle as being over
		// land precisely by carrying a parcel id — so a parcel 0 would make
		// every warehouse receipt and bond on the chain look like a vehicle
		// over the first piece of ground this registry ever recorded, and put
		// it under the registry's ceiling and restrictions.
		//
		// Skipped rather than offset, because the counter and the id it hands
		// out have to remain the same number: genesis refuses an import whose
		// counter sits behind an existing id, and an offset id would trip that
		// on the very first parcel.
		if id, err = m.NextParcelID.Next(ctx); err != nil {
			return nil, err
		}
	}

	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	parcel := types.Parcel{
		Id:           id,
		GeometryHash: msg.GeometryHash,
		CadastralRef: msg.CadastralRef,
		Holder:       msg.Holder,
		Authority:    office.Address,
		Status:       types.STATUS_REGISTERED,
		RegisteredAt: height,
	}

	if err := m.Parcel.Set(ctx, id, parcel); err != nil {
		return nil, err
	}
	if err := m.ByGeometry.Set(ctx, msg.GeometryHash, id); err != nil {
		return nil, err
	}
	if err := m.ByRef.Set(ctx, msg.CadastralRef, id); err != nil {
		return nil, err
	}

	return &types.MsgRegisterParcelResponse{Id: id}, nil
}

// activeAuthority resolves a signer to a registry office, or refuses.
//
// This is the single place the jurisdictional perimeter is enforced in this
// module, and it is here rather than at each handler on purpose: every authority
// action in x/land — registering a parcel, validating a transfer, attesting one,
// attaching a deed, imposing a restriction, authorising fractionalisation,
// freezing land — resolves its signer through this function, so a perimeter
// check here is a perimeter check on all of them. Spread across the handlers it
// would be seven checks with seven chances of being the one that was forgotten,
// and the forgotten one is always found by somebody who was looking for it.
//
// It is in the message server rather than in an ante decorator for a reason this
// repository has already paid for once: an ante gate only sees messages that
// arrive as transactions, and an interchain account or an x/authz grant reaches
// the message router by another road entirely.
func (m msgServer) activeAuthority(
	ctx context.Context, addr string,
) (types.Authority, error) {
	office, err := m.Authority.Get(ctx, addr)
	if err != nil {
		if isNotFound(err) {
			return types.Authority{}, types.ErrNotAuthority
		}
		return types.Authority{}, err
	}
	if !office.Active {
		return types.Authority{}, types.ErrAuthorityInactive
	}
	if err := m.assertScope(ctx, addr, office.Jurisdiction); err != nil {
		return types.Authority{}, err
	}
	return office, nil
}

// assertScope refuses an office acting outside the perimeter it was granted.
//
// Fails closed when no registry is wired in. That is the opposite of what the
// group-account check beside it does, and the asymmetry is deliberate: skipping
// the group check can only ever admit an office that should have been refused,
// while skipping this one would permit an *action* that should have been
// refused — which is the failure the whole perimeter exists to prevent. A
// missing dependency must never be an authorisation.
func (m msgServer) assertScope(ctx context.Context, actor, jurisdiction string) error {
	if m.scopeKeeper == nil {
		return aliastypes.ErrNoScopeKeeper
	}
	return m.scopeKeeper.AssertScopeIn(ctx, actor, aliastypes.ROLE_REGISTRY_AUTHORITY, jurisdiction)
}

func isNotFound(err error) bool {
	return err != nil && errorsIs(err, collections.ErrNotFound)
}
