package keeper

import (
	"context"
	"errors"
	"strconv"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/alias/types"
)

// RegisterViewingKey publishes the sender's X25519 public key, or rotates it.
//
// Self-signed, unlike the jurisdiction beside it, and the difference is not an
// oversight. A jurisdiction is a claim about somebody that they must not be
// able to make for themselves; a viewing key is a claim only about which key
// opens payloads addressed to the sender, and an account that publishes a key
// it does not hold has locked itself out of its own payment detail and nobody
// else out of anything.
//
// Rotation is forward-only. The previous version is left exactly where it is,
// so every envelope already sealed to it stays openable by whoever holds the
// matching private half; new envelopes name the new version. Nothing re-wraps
// history, because re-wrapping means somebody decrypted every stored payload
// and encrypted it again — an act with a real cost and a real risk, which
// belongs to the key holder deciding to do it and never to a registry doing it
// silently.
func (k msgServer) RegisterViewingKey(ctx context.Context, msg *types.MsgRegisterViewingKey) (*types.MsgRegisterViewingKeyResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Account); err != nil {
		return nil, errorsmod.Wrap(err, "invalid account address")
	}
	if err := types.ValidateViewingKey(msg.PublicKey); err != nil {
		return nil, errorsmod.Wrap(types.ErrInvalidViewingKey, err.Error())
	}

	latest, found, err := k.LatestViewingKey(ctx, msg.Account)
	if err != nil {
		return nil, err
	}
	// A version that climbs from the highest ever issued, not from the count of
	// live ones. Deriving it from a count would reuse a number as soon as a key
	// was removed, and an envelope naming a reused version resolves to the
	// wrong key — which the reader sees as an authentication failure
	// indistinguishable from a corrupted payload.
	version := uint64(1)
	if found {
		version = latest.Version + 1
	}

	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	key := types.ViewingKey{
		Address:            msg.Account,
		Version:            version,
		PublicKey:          msg.PublicKey,
		RegisteredAtHeight: height,
	}
	if err := k.ViewingKeys.Set(ctx, collections.Join(msg.Account, version), key); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
		"viewing_key_registered",
		sdk.NewAttribute("address", msg.Account),
		sdk.NewAttribute("version", strconv.FormatUint(version, 10)),
	))
	return &types.MsgRegisterViewingKeyResponse{Version: version}, nil
}

// RevokeViewingKey marks one of the sender's key versions compromised.
//
// It does not delete the key and does not claim the payloads sealed to it
// became unreadable. Ciphertext that has been distributed cannot be recalled,
// and a message implying otherwise would be worse than none: an operator would
// believe the exposure was closed by a transaction. What it does is stop
// senders sealing to it, and let a reader see that a payload they hold was
// addressed to a key somebody else may also hold. Destroying the payload itself
// is the store's job — see tools/payloadstore.
func (k msgServer) RevokeViewingKey(ctx context.Context, msg *types.MsgRevokeViewingKey) (*types.MsgRevokeViewingKeyResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Account); err != nil {
		return nil, errorsmod.Wrap(err, "invalid account address")
	}

	key, err := k.ViewingKeys.Get(ctx, collections.Join(msg.Account, msg.Version))
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrViewingKeyNotFound,
			"%s has no viewing key at version %d", msg.Account, msg.Version)
	}
	// Idempotent rather than an error. A revocation resubmitted after a timeout
	// is the ordinary way this message arrives twice, and failing the second
	// one would leave an operator dealing with a compromised key unsure whether
	// the first had landed.
	if !key.Live() {
		return &types.MsgRevokeViewingKeyResponse{}, nil
	}

	// Both together, always. The boolean is what Live() reads; the height is the
	// record of when the exposure began, which is what decides how many stored
	// payloads are affected.
	key.Revoked = true
	key.RevokedAtHeight = sdk.UnwrapSDKContext(ctx).BlockHeight()
	if err := k.ViewingKeys.Set(ctx, collections.Join(msg.Account, msg.Version), key); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
		"viewing_key_revoked",
		sdk.NewAttribute("address", msg.Account),
		sdk.NewAttribute("version", strconv.FormatUint(msg.Version, 10)),
	))
	return &types.MsgRevokeViewingKeyResponse{}, nil
}

// AppointRegulator names the authority holding the third viewing key over
// payments settling in one country.
//
// Authority-gated, because this is the most powerful grant the confidentiality
// design makes: the appointee can read the ISO 20022 detail of every payment
// declaring their country from the moment they are appointed. An account able
// to appoint itself regulator of Ghana would have granted itself that by
// sending one message.
//
// The country is checked against the assigned list for the same reason a
// jurisdiction is. A mistyped NX appoints a regulator of nowhere, and every
// payment declaring NG goes on being sealed without a regulator on it while the
// appointment sits in state looking done.
func (k msgServer) AppointRegulator(ctx context.Context, msg *types.MsgAppointRegulator) (*types.MsgAppointRegulatorResponse, error) {
	if err := k.assertChainAuthority(ctx, msg.Authority); err != nil {
		return nil, err
	}
	if _, err := k.addressCodec.StringToBytes(msg.Address); err != nil {
		return nil, errorsmod.Wrap(err, "invalid regulator address")
	}

	country := types.NormaliseCountry(msg.Country)
	// AssignedCountry, not IssuableCountry: the foundation's reserved code is
	// refused. It marks the absence of a national perimeter, and a regulator of
	// nowhere is an account that can open payloads no authority is accountable
	// for.
	if !types.AssignedCountry(country) {
		return nil, errorsmod.Wrapf(types.ErrInvalidCountry, "%q", msg.Country)
	}

	if err := k.Regulators.Set(ctx, country, types.RegulatorAppointment{
		Country:           country,
		Address:           msg.Address,
		AppointedBy:       msg.Authority,
		AppointedAtHeight: sdk.UnwrapSDKContext(ctx).BlockHeight(),
	}); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
		"regulator_appointed",
		sdk.NewAttribute("country", country),
		sdk.NewAttribute("address", msg.Address),
		sdk.NewAttribute("appointed_by", msg.Authority),
	))
	return &types.MsgAppointRegulatorResponse{}, nil
}

// GrantAuditor grants the time-boxed cross-account reading role.
//
// Bounded three ways at once — it expires by height, the number of live grants
// is capped, and every grant records who made it. All three exist because this
// is the role that reads payment detail belonging to people who have no
// relationship with the holder.
func (k msgServer) GrantAuditor(ctx context.Context, msg *types.MsgGrantAuditor) (*types.MsgGrantAuditorResponse, error) {
	if err := k.assertChainAuthority(ctx, msg.Authority); err != nil {
		return nil, err
	}
	if _, err := k.addressCodec.StringToBytes(msg.Address); err != nil {
		return nil, errorsmod.Wrap(err, "invalid auditor address")
	}

	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	// Strictly in the future, and no zero-means-forever. A role that becomes
	// permanent by leaving a field unset is not time-boxed, it is time-boxed by
	// convention — and the convention is what fails when nobody is looking.
	if msg.ExpiresAtHeight <= height {
		return nil, errorsmod.Wrapf(types.ErrInvalidAuditorGrant,
			"expires_at_height must be above the current height %d, got %d", height, msg.ExpiresAtHeight)
	}

	live, err := k.countLiveAuditorGrants(ctx, height, msg.Address)
	if err != nil {
		return nil, err
	}
	if live >= types.MaxLiveAuditorGrants {
		return nil, errorsmod.Wrapf(types.ErrInvalidAuditorGrant,
			"at most %d auditor grants may be live at once, and %d already are",
			types.MaxLiveAuditorGrants, live)
	}

	if err := k.AuditorGrants.Set(ctx, msg.Address, types.AuditorGrant{
		Address:         msg.Address,
		GrantedBy:       msg.Authority,
		GrantedAtHeight: height,
		ExpiresAtHeight: msg.ExpiresAtHeight,
	}); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
		"auditor_granted",
		sdk.NewAttribute("address", msg.Address),
		sdk.NewAttribute("granted_by", msg.Authority),
		sdk.NewAttribute("expires_at_height", strconv.FormatInt(msg.ExpiresAtHeight, 10)),
	))
	return &types.MsgGrantAuditorResponse{}, nil
}

// assertChainAuthority accepts governance or a named foundation administrator.
//
// The same two signers that may correct a jurisdiction, because these grants
// are the same class of act: they decide who may see inside a perimeter. A
// foundation administrator is allowed so that appointing a country's regulator
// does not require a governance cycle at the moment a deployment is being stood
// up — the list of administrators is itself governance-controlled, capped and
// deduplicated, so this widens who may act without widening who decides.
func (k Keeper) assertChainAuthority(ctx context.Context, signer string) error {
	if signer == k.GetAuthority() {
		return nil
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}
	if params.IsFoundationAdministrator(signer) {
		return nil
	}
	return errorsmod.Wrapf(types.ErrInvalidSigner,
		"expected the governance account or a foundation administrator, got %s", signer)
}

// countLiveAuditorGrants counts grants still in force, ignoring one address.
//
// The address being granted is excluded so that renewing an existing auditor is
// not refused by the cap it already occupies. Without that, an auditor whose
// grant is about to lapse could not be extended once the cap was full, and the
// operator's only route would be to let the grant expire — leaving a gap in
// exactly the supervision the role exists for.
func (k Keeper) countLiveAuditorGrants(ctx context.Context, height int64, excluding string) (int, error) {
	live := 0
	err := k.AuditorGrants.Walk(ctx, nil, func(addr string, g types.AuditorGrant) (bool, error) {
		if addr != excluding && g.Live(height) {
			live++
		}
		return false, nil
	})
	return live, err
}

// LatestViewingKey returns the highest version an account has published.
//
// Read by walking the account's versions in descending order rather than from a
// stored "current" pointer. A pointer is a second copy of a fact that is
// already in the data, and the failure of a second copy here is a sender
// sealing to a key the registry no longer agrees is current.
func (k Keeper) LatestViewingKey(ctx context.Context, addr string) (types.ViewingKey, bool, error) {
	var latest types.ViewingKey
	found := false
	rng := collections.NewPrefixedPairRange[string, uint64](addr).Descending()
	err := k.ViewingKeys.Walk(ctx, rng, func(_ collections.Pair[string, uint64], v types.ViewingKey) (bool, error) {
		latest, found = v, true
		return true, nil
	})
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return types.ViewingKey{}, false, err
	}
	return latest, found, nil
}
