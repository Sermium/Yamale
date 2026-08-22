package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/group"

	"yamale/blockchain/x/alias/types"
)

// GrantRole grants a role inside one jurisdiction.
//
// Governance and nobody else, and note that this is stricter than the
// appointments beside it: a foundation administrator may name a country's
// regulator but may not grant a role. The difference is between using a power
// and deciding who holds one. An administrator who could grant roles could grant
// themselves the chain-wide scope, and then grant it to anybody — at which point
// the perimeter is whatever the administrators say it is, which is a perimeter in
// name only.
//
// Every widening of who may act therefore costs a governance cycle, in public,
// one grant at a time. That is the intended friction.
func (k msgServer) GrantRole(ctx context.Context, msg *types.MsgGrantRole) (*types.MsgGrantRoleResponse, error) {
	if err := k.assertGovernance(msg.Authority); err != nil {
		return nil, err
	}
	if _, err := k.addressCodec.StringToBytes(msg.Holder); err != nil {
		return nil, errorsmod.Wrap(err, "invalid holder address")
	}
	if !types.ValidRole(msg.Role) {
		return nil, errorsmod.Wrapf(types.ErrInvalidRole, "%s", types.RoleName(msg.Role))
	}

	scope := types.NormaliseScope(msg.Jurisdiction)
	if !types.ValidGrantScope(scope) {
		return nil, errorsmod.Wrapf(types.ErrInvalidScope, "%q", msg.Jurisdiction)
	}

	// An office that is one key is one bribe, whatever the quorum downstream of
	// it. Checked here because this is the only moment at which the chain has any
	// say over who a role goes to.
	if err := k.assertGroupAccount(ctx, msg.Holder); err != nil {
		return nil, err
	}

	grant := types.RoleGrant{
		Holder:          msg.Holder,
		Role:            msg.Role,
		Jurisdiction:    scope,
		GrantedBy:       msg.Authority,
		GrantedAtHeight: sdk.UnwrapSDKContext(ctx).BlockHeight(),
	}
	// Validate again over the assembled record rather than trusting the checks
	// above. It is the same function genesis uses, so a grant made by proposal
	// and a grant seeded at height zero are held to one rule with one
	// implementation.
	if err := grant.Validate(); err != nil {
		return nil, errorsmod.Wrap(types.ErrInvalidRole, err.Error())
	}

	// Granting the same triple again rewrites the attribution and the height and
	// changes nothing else, which is what a proposal resubmitted after a timeout
	// should do. It is deliberately not an error: failing the second one would
	// leave governance unsure whether the first had landed, and the state either
	// way is identical.
	if err := k.grant(ctx, grant); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
		"role_granted",
		sdk.NewAttribute("holder", grant.Holder),
		sdk.NewAttribute("role", types.RoleName(grant.Role)),
		sdk.NewAttribute("jurisdiction", grant.Jurisdiction),
		sdk.NewAttribute("granted_by", grant.GrantedBy),
	))
	return &types.MsgGrantRoleResponse{}, nil
}

// RevokeRole removes one grant, named exactly.
//
// Removed rather than marked, unlike the freezes and restrictions elsewhere on
// this chain, and the difference is worth being explicit about. Those are
// findings about somebody, and erasing a finding erases the evidence. This is a
// capability: it either exists or it does not, and a revoked grant left in the
// store as a flag is a row that one wrong read treats as live. The record of
// what was granted and when lives in the events and in the proposal that made
// it, which is where a history belongs.
func (k msgServer) RevokeRole(ctx context.Context, msg *types.MsgRevokeRole) (*types.MsgRevokeRoleResponse, error) {
	if err := k.assertGovernance(msg.Authority); err != nil {
		return nil, err
	}
	if !types.ValidRole(msg.Role) {
		return nil, errorsmod.Wrapf(types.ErrInvalidRole, "%s", types.RoleName(msg.Role))
	}

	scope := types.NormaliseScope(msg.Jurisdiction)
	if !types.ValidGrantScope(scope) {
		return nil, errorsmod.Wrapf(types.ErrInvalidScope, "%q", msg.Jurisdiction)
	}

	found, err := k.revoke(ctx, msg.Holder, msg.Role, scope)
	if err != nil {
		return nil, err
	}
	if !found {
		// Not idempotent, on purpose, and this is the one place in the module
		// where that is the right call. "Nothing to revoke" is how a proposal that
		// named the wrong jurisdiction succeeds while leaving the authority it
		// meant to remove in place, and governance has to see that.
		return nil, errorsmod.Wrapf(types.ErrGrantNotFound,
			"%s does not hold %s in %s", msg.Holder, types.RoleName(msg.Role), scope)
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
		"role_revoked",
		sdk.NewAttribute("holder", msg.Holder),
		sdk.NewAttribute("role", types.RoleName(msg.Role)),
		sdk.NewAttribute("jurisdiction", scope),
		sdk.NewAttribute("revoked_by", msg.Authority),
	))
	return &types.MsgRevokeRoleResponse{}, nil
}

// assertGovernance refuses everyone but the governance account.
//
// Deliberately not assertChainAuthority, which also accepts a foundation
// administrator. See GrantRole for why the two messages differ.
func (k Keeper) assertGovernance(signer string) error {
	if signer != k.GetAuthority() {
		return errorsmod.Wrapf(types.ErrInvalidSigner,
			"only governance may grant or revoke a role; expected %s, got %s", k.GetAuthority(), signer)
	}
	return nil
}

// assertGroupAccount refuses a holder that is a plain key.
//
// Skipped when no group keeper is supplied, which is only the case in unit tests
// exercising other rules; the app always wires one. That is a bypass, and it is
// tolerable only here: it can make a grant that should have been refused, and it
// cannot make an action that should have been refused be permitted. Every path
// that actually checks a perimeter fails closed instead — see AssertScope, and
// the consumers, which refuse outright when the registry is missing rather than
// waving the action through.
func (k Keeper) assertGroupAccount(ctx context.Context, addr string) error {
	if k.groups == nil {
		return nil
	}
	if _, err := k.groups.GroupPolicyInfo(ctx, &group.QueryGroupPolicyInfoRequest{
		Address: addr,
	}); err != nil {
		return errorsmod.Wrapf(types.ErrHolderNotGroup, "%s", addr)
	}
	return nil
}
