package keeper

import (
	"bytes"
	"context"

	"github.com/cosmos/cosmos-sdk/x/group"

	sdk "github.com/cosmos/cosmos-sdk/types"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/land/types"
)

// UpdateParams changes the quorum and the challenge window. Governance only:
// an official who can lower the quorum has no quorum.
func (m msgServer) UpdateParams(
	ctx context.Context, msg *types.MsgUpdateParams,
) (*types.MsgUpdateParamsResponse, error) {
	if err := m.assertGovernance(msg.Authority); err != nil {
		return nil, err
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}
	if err := m.Params.Set(ctx, msg.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}

// RegisterAuthority admits a registry office.
//
// Governance-gated for a specific reason: if an office could admit another
// office, buying one office would buy the power to manufacture the independent
// attestors the whole quorum depends on.
func (m msgServer) RegisterAuthority(
	ctx context.Context, msg *types.MsgRegisterAuthority,
) (*types.MsgRegisterAuthorityResponse, error) {
	if err := m.assertGovernance(msg.Authority); err != nil {
		return nil, err
	}
	if _, err := m.addressCodec.StringToBytes(msg.Office); err != nil {
		return nil, types.ErrInvalidHolder
	}
	// An office must be a group account, so that every decision it later makes
	// — registering a parcel, validating a transfer, lifting a restriction,
	// freezing land — already requires its own M-of-N inside the office. A
	// plain key here would make each of those a single bribe, and no amount of
	// cross-office quorum on transfers fixes registration or freezing.
	if err := m.assertGroupAccount(ctx, msg.Office); err != nil {
		return nil, err
	}
	// The jurisdiction on this record is what the perimeter check is made
	// against, so it has to be a country the chain recognises. Free text here
	// would admit an office to a perimeter no grant can ever cover: it would look
	// admitted and be unable to register a single parcel, and the failure would
	// surface to a registrar rather than to whoever wrote the proposal.
	//
	// Normalised as well as checked, so "gh" and "GH" are the same office rather
	// than one that works and one that does not.
	jurisdiction := aliastypes.NormaliseCountry(msg.Jurisdiction)
	if !aliastypes.AssignedCountry(jurisdiction) {
		return nil, types.ErrInvalidJurisdiction.Wrapf("%q", msg.Jurisdiction)
	}
	if err := m.Authority.Set(ctx, msg.Office, types.Authority{
		Address:      msg.Office,
		Name:         msg.Name,
		Jurisdiction: jurisdiction,
		Active:       msg.Active,
	}); err != nil {
		return nil, err
	}
	return &types.MsgRegisterAuthorityResponse{}, nil
}

// AttachDeed adds a document to the chain of title.
func (m msgServer) AttachDeed(
	ctx context.Context, msg *types.MsgAttachDeed,
) (*types.MsgAttachDeedResponse, error) {
	parcel, err := m.ownParcel(ctx, msg.Creator, msg.ParcelId)
	if err != nil {
		return nil, err
	}
	if msg.DocumentHash == "" {
		return nil, types.ErrNoDocument
	}

	parcel.Deeds = append(parcel.Deeds, types.Deed{
		Kind:         msg.Kind,
		DocumentHash: msg.DocumentHash,
		Uri:          msg.Uri,
		Reference:    msg.Reference,
		IssuedOn:     msg.IssuedOn,
		RecordedAt:   sdk.UnwrapSDKContext(ctx).BlockHeight(),
	})
	if err := m.Parcel.Set(ctx, parcel.Id, parcel); err != nil {
		return nil, err
	}
	return &types.MsgAttachDeedResponse{}, nil
}

// SetRestriction imposes or lifts a limit on what may be done with a parcel.
func (m msgServer) SetRestriction(
	ctx context.Context, msg *types.MsgSetRestriction,
) (*types.MsgSetRestrictionResponse, error) {
	parcel, err := m.ownParcel(ctx, msg.Creator, msg.ParcelId)
	if err != nil {
		return nil, err
	}

	if msg.Lift {
		if int(msg.Index) >= len(parcel.Restrictions) {
			return nil, types.ErrNoRestriction
		}
		// Marked, not removed. A restriction that vanishes takes with it the
		// evidence that the land was ever constrained, and who released it.
		parcel.Restrictions[msg.Index].Lifted = true
	} else {
		if msg.Kind == "" {
			return nil, types.ErrNoRestrictionKind
		}
		parcel.Restrictions = append(parcel.Restrictions, types.Restriction{
			Kind:      msg.Kind,
			Value:     msg.Value,
			Detail:    msg.Detail,
			ImposedBy: msg.Creator,
			ImposedAt: sdk.UnwrapSDKContext(ctx).BlockHeight(),
		})
	}

	if err := m.Parcel.Set(ctx, parcel.Id, parcel); err != nil {
		return nil, err
	}
	return &types.MsgSetRestrictionResponse{}, nil
}

// AuthoriseFractionalisation is the supervised bridge to x/tokenisation.
//
// Selling an exploitation right in shares is legitimate financing — often the
// only credit available to somebody whose sole asset is land. What must not
// happen is that it becomes a way around the restrictions, or a way to move
// control of the land without the registry ever seeing it. So the registry
// grants permission with a ceiling and an expiry, and x/tokenisation refuses to
// open a vehicle without one.
func (m msgServer) AuthoriseFractionalisation(
	ctx context.Context, msg *types.MsgAuthoriseFractionalisation,
) (*types.MsgAuthoriseFractionalisationResponse, error) {
	parcel, err := m.ownParcel(ctx, msg.Creator, msg.ParcelId)
	if err != nil {
		return nil, err
	}
	if parcel.Status != types.STATUS_REGISTERED {
		return nil, types.ErrParcelNotTransferable
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if msg.Withdraw {
		auth, err := m.FractionalisationAuthority.Get(ctx, msg.ParcelId)
		if err != nil {
			return nil, types.ErrNoAuthorisation
		}
		// Marked, not removed, for the same reason a lifted restriction is: a
		// permission that vanishes takes with it the evidence that the registry
		// ever gave it. Stops new issuance. Existing holders are not
		// expropriated here — that is a taking, and it belongs to a court.
		auth.Withdrawn = true
		auth.WithdrawnAt = sdkCtx.BlockHeight()
		if err := m.FractionalisationAuthority.Set(ctx, msg.ParcelId, auth); err != nil {
			return nil, err
		}
		parcel.VehicleId = 0
		if err := m.Parcel.Set(ctx, parcel.Id, parcel); err != nil {
			return nil, err
		}
		return &types.MsgAuthoriseFractionalisationResponse{}, nil
	}

	if msg.MaxShareBps == 0 || msg.MaxShareBps > 10_000 {
		return nil, types.ErrBadShareCeiling
	}
	// Required and in the future. An unset expiry is zero, and zero read as
	// "no expiry" would make the field a permission that never lapses — which
	// is precisely the standing authorisation the expiry exists to prevent.
	// Tested against zero as well as against the clock, because a chain whose
	// block time has not been set yet would accept zero as "in the future".
	if msg.ExpiresAt <= 0 || msg.ExpiresAt <= sdkCtx.BlockTime().Unix() {
		return nil, types.ErrBadExpiry
	}
	// A restriction forbidding it outranks the office's permission — that is
	// what makes restrictions worth recording at all.
	if parcel.ForbidsFractionalisation() {
		return nil, types.ErrFractionalisationForbidden
	}

	// One record per parcel: granting again replaces the terms. Accumulating
	// permissions would leave a parcel carrying two ceilings, and a parcel with
	// two ceilings has the higher one.
	if err := m.FractionalisationAuthority.Set(ctx, msg.ParcelId,
		types.FractionalisationAuthority{
			ParcelId:    msg.ParcelId,
			Right:       msg.Right,
			MaxShareBps: msg.MaxShareBps,
			ExpiresAt:   msg.ExpiresAt,
			GrantedBy:   msg.Creator,
			GrantedAt:   sdkCtx.BlockHeight(),
		}); err != nil {
		return nil, err
	}
	return &types.MsgAuthoriseFractionalisationResponse{}, nil
}

// FreezeParcel stops all movement, or lifts a freeze.
//
// The grounds are written onto the parcel, not merely required and dropped.
// The status alone tells a holder that their land has been stopped and nothing
// about why, which leaves an inquiry and an extortion looking identical from
// the register — and a stop nobody can read the grounds of is a stop nobody can
// argue with.
func (m msgServer) FreezeParcel(
	ctx context.Context, msg *types.MsgFreezeParcel,
) (*types.MsgFreezeParcelResponse, error) {
	parcel, err := m.ownParcel(ctx, msg.Creator, msg.ParcelId)
	if err != nil {
		return nil, err
	}
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()

	if msg.Unfreeze {
		if parcel.Status != types.STATUS_FROZEN {
			return nil, types.ErrNotFrozen
		}
		parcel.Status = types.STATUS_REGISTERED
		// Marked, not removed, like a released encumbrance: the record must
		// still show that this land was once stopped and by whom, or a freeze
		// becomes a thing that can be done and then made never to have
		// happened.
		if i := parcel.LiveFreeze(); i >= 0 {
			parcel.Freezes[i].Lifted = true
			parcel.Freezes[i].LiftedBy = msg.Creator
			parcel.Freezes[i].LiftReason = msg.Reason
			parcel.Freezes[i].LiftedAt = height
		}
	} else {
		if msg.Reason == "" {
			return nil, types.ErrNoReason
		}
		parcel.Status = types.STATUS_FROZEN
		parcel.Freezes = append(parcel.Freezes, types.Freeze{
			Reason:    msg.Reason,
			ImposedBy: msg.Creator,
			ImposedAt: height,
		})
	}

	if err := m.Parcel.Set(ctx, parcel.Id, parcel); err != nil {
		return nil, err
	}
	return &types.MsgFreezeParcelResponse{}, nil
}

// RecordEncumbrance adds or releases a claim against a parcel.
func (m msgServer) RecordEncumbrance(
	ctx context.Context, msg *types.MsgRecordEncumbrance,
) (*types.MsgRecordEncumbranceResponse, error) {
	parcel, err := m.ownParcel(ctx, msg.Creator, msg.ParcelId)
	if err != nil {
		return nil, err
	}

	if msg.Release {
		if int(msg.Index) >= len(parcel.Encumbrances) {
			return nil, types.ErrNoEncumbrance
		}
		parcel.Encumbrances[msg.Index].Released = true
	} else {
		parcel.Encumbrances = append(parcel.Encumbrances, types.Encumbrance{
			Kind:       msg.Kind,
			Holder:     msg.Holder,
			Detail:     msg.Detail,
			RecordedAt: sdk.UnwrapSDKContext(ctx).BlockHeight(),
		})
	}

	if err := m.Parcel.Set(ctx, parcel.Id, parcel); err != nil {
		return nil, err
	}
	return &types.MsgRecordEncumbranceResponse{}, nil
}

// ownParcel loads a parcel and checks the signer is the office in charge of it.
func (m msgServer) ownParcel(
	ctx context.Context, signer string, id uint64,
) (types.Parcel, error) {
	parcel, err := m.Parcel.Get(ctx, id)
	if err != nil {
		return types.Parcel{}, types.ErrNoParcel
	}
	if _, err := m.activeAuthority(ctx, signer); err != nil {
		return types.Parcel{}, err
	}
	if signer != parcel.Authority {
		return types.Parcel{}, types.ErrWrongJurisdiction
	}
	return parcel, nil
}

func (m msgServer) assertGovernance(addr string) error {
	got, err := m.addressCodec.StringToBytes(addr)
	if err != nil {
		return types.ErrInvalidHolder
	}
	if !bytes.Equal(got, m.GetAuthority()) {
		return types.ErrNotGovernance
	}
	return nil
}

// assertGroupAccount refuses an office that is a plain key.
//
// Skipped when no group keeper is supplied, which is only the case in unit
// tests that are exercising other rules — the app always wires one.
func (m msgServer) assertGroupAccount(ctx context.Context, addr string) error {
	if m.groupKeeper == nil {
		return nil
	}
	_, err := m.groupKeeper.GroupPolicyInfo(ctx, &group.QueryGroupPolicyInfoRequest{
		Address: addr,
	})
	if err != nil {
		return types.ErrOfficeNotGroup
	}
	return nil
}
