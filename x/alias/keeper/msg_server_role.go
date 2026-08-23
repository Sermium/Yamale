package keeper

import (
	"context"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/group"

	"yamale/blockchain/x/alias/types"
)

// GrantRole grants a role inside one jurisdiction.
//
// Governance, or the foundation — and the split between what each of them may do
// is the whole of the rule, so read it before the history:
//
//   - A grant naming a **country** may be made by governance or by the
//     foundation, meaning the account x/constitution pins as
//     enforcement_recovery_destination.
//   - A grant naming the **chain-wide** scope is governance and nobody else.
//
// # What this used to be, and why it changed
//
// This message was governance-only, and the argument for that was good enough to
// be worth keeping rather than deleting. It ran: a foundation administrator may
// name a country's regulator but may not grant a role, because the difference is
// between *using* a power and *deciding who holds one*. An administrator who
// could grant roles could grant themselves the chain-wide scope and then grant it
// to anybody, at which point the perimeter is whatever the administrators say it
// is — a perimeter in name only. Every widening of who may act therefore cost a
// governance cycle, in public, one grant at a time, and that friction was the
// intended product rather than a side effect.
//
// What that argument did not survive was contact with enrolling a country.
// Bringing one country onto the rail is not one grant: it is an M-of-N group per
// office, two to five role grants across those offices, and the jurisdiction
// records for the offices' own accounts. Under governance-only, admitting a
// single country was a handful of separate proposals that had to land in a
// particular order, each of which could pass, fail or time out independently — so
// the friction was not being paid once per widening of authority, it was being
// paid several times over for one decision that a room had already taken. The
// predictable outcome of that is not more scrutiny; it is a bundle proposal
// nobody reads, or a deployment that seeds its grants in genesis and never
// revisits them.
//
// So the decision is that admitting a country is the foundation's act. The
// foundation is a 3-of-5 whose members were generated in a ceremony, sit in five
// different organisations and are named on a signed record — see
// docs/guides/key-ceremony.md — and a grant it makes is attributable to the three
// custodians who voted for it.
//
// # What was given up
//
// Publicity and delay. A country's authorities can now be appointed without a
// governance vote, so the validator set no longer has a veto over who administers
// a perimeter, and the decision no longer sits in public for a voting period
// before it takes effect. What replaces those is narrower and worth naming
// honestly: three of five custodians, on chain, with the grant recorded against
// their group address in granted_by and the height in granted_at_height.
//
// The residual risk this leaves is the foundation granting itself, or one office,
// the same role in every country one grant at a time — which reaches the same
// place as a chain-wide grant by a longer road. It is bounded by the assigned
// country list rather than closed, it is enumerable through the RoleGrants query,
// and it is the reason the next paragraph exists rather than being folded into
// this one.
//
// # Why chain-wide stays governance-only
//
// The foundation admitting countries and the foundation manufacturing chain-wide
// authority for itself are different acts, and only the first one was decided.
// The original argument still holds in full for the second: an account that could
// grant itself the scope no border bounds could then grant it to anybody, and the
// perimeter would be advisory. So "*" is refused here for every signer but
// governance, before the constitution is even consulted.
//
// # Why the constitution names the foundation, and not a parameter
//
// The obvious implementation is a list on this module's Params, beside
// foundation_administrators. It was not used, and the reason is that a params
// list is editable by one ordinary governance proposal — so "who may admit a
// country" would be a set that a single vote could append to. Reading the address
// out of x/constitution instead means it is the same account the constitution
// protects: changing it is a constitutional amendment, with the delay and the
// four-fifths ratification that carries. See types.ConstitutionKeeper.
func (k msgServer) GrantRole(ctx context.Context, msg *types.MsgGrantRole) (*types.MsgGrantRoleResponse, error) {
	scope := types.NormaliseScope(msg.Jurisdiction)
	if !types.ValidGrantScope(scope) {
		return nil, errorsmod.Wrapf(types.ErrInvalidScope, "%q", msg.Jurisdiction)
	}
	// The scope decides who may sign, so it is normalised and validated first.
	// Checking the signer against an un-normalised scope would let "*" reach the
	// foundation branch under some other spelling — there is none today, and the
	// ordering is what keeps that true rather than a fact about NormaliseScope.
	if err := k.assertMayGrant(ctx, msg.Authority, scope); err != nil {
		return nil, err
	}
	if _, err := k.addressCodec.StringToBytes(msg.Holder); err != nil {
		return nil, errorsmod.Wrap(err, "invalid holder address")
	}
	if !types.ValidRole(msg.Role) {
		return nil, errorsmod.Wrapf(types.ErrInvalidRole, "%s", types.RoleName(msg.Role))
	}

	// An office that is one key is one bribe, whatever the quorum downstream of
	// it. Checked here because this is the only moment at which the chain has any
	// say over who a role goes to.
	if err := k.assertGroupAccount(ctx, msg.Holder); err != nil {
		return nil, err
	}

	// The M-of-N the office must keep, if this grant records one. Two separate
	// checks and they answer different questions:
	//
	//   - Validate: is the requirement itself coherent? Refuses a present
	//     requirement of zero signatures, and one asking for more signatures than
	//     members. It runs whether or not x/group is wired, because it is a fact
	//     about the message.
	//   - assertShape: does the holder meet it TODAY? Refuses a grant requiring
	//     three-of-five to a one-of-one office. Without this the grant would be
	//     written, read correct in every query, and permit nothing — an office
	//     that believes it holds an authority and does not is worse than one that
	//     was told no.
	//
	// assertShape refuses when no group keeper is wired, so a requirement can only
	// be recorded on a chain that can check it. That is the closed direction and it
	// is the same rule the perimeter path applies: a requirement nobody could
	// verify is not a requirement.
	if err := msg.RequiredShape.Validate(); err != nil {
		return nil, errorsmod.Wrap(types.ErrInvalidRole, err.Error())
	}
	if err := k.assertShape(ctx, msg.Holder, msg.RequiredShape); err != nil {
		return nil, err
	}
	// And a re-grant may not quietly lower the bar a previous one set. See
	// assertShapeNotReduced: the idempotence of re-granting a triple is worth
	// keeping, and an omitted field is not an amendment.
	if err := k.assertShapeNotReduced(ctx, msg.Holder, msg.Role, scope, msg.RequiredShape); err != nil {
		return nil, err
	}

	grant := types.RoleGrant{
		Holder:          msg.Holder,
		Role:            msg.Role,
		Jurisdiction:    scope,
		GrantedBy:       msg.Authority,
		GrantedAtHeight: sdk.UnwrapSDKContext(ctx).BlockHeight(),
		RequiredShape:   msg.RequiredShape,
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
// The same signers as GrantRole, and deliberately not a narrower set. Whoever may
// appoint a country's authority may also remove it; only governance may touch a
// chain-wide grant, in either direction.
//
// Keeping revocation governance-only while granting was widened was considered
// and rejected, because it puts the slow path on the wrong action. The reason to
// revoke in a hurry is that an office's keys are compromised or its authority is
// being abused, and a rule under which the foundation can appoint a national
// enforcement authority with one 3-of-5 vote but needs a three-week governance
// cycle to remove one is a rule that makes the emergency the expensive case.
// Granting authority is the act that needs friction; taking it away is the act
// that needs to be possible on a Sunday.
//
// Removed rather than marked, unlike the freezes and restrictions elsewhere on
// this chain, and the difference is worth being explicit about. Those are
// findings about somebody, and erasing a finding erases the evidence. This is a
// capability: it either exists or it does not, and a revoked grant left in the
// store as a flag is a row that one wrong read treats as live. The record of
// what was granted and when lives in the events and in the proposal that made
// it, which is where a history belongs.
func (k msgServer) RevokeRole(ctx context.Context, msg *types.MsgRevokeRole) (*types.MsgRevokeRoleResponse, error) {
	scope := types.NormaliseScope(msg.Jurisdiction)
	if !types.ValidGrantScope(scope) {
		return nil, errorsmod.Wrapf(types.ErrInvalidScope, "%q", msg.Jurisdiction)
	}
	if err := k.assertMayGrant(ctx, msg.Authority, scope); err != nil {
		return nil, err
	}
	if !types.ValidRole(msg.Role) {
		return nil, errorsmod.Wrapf(types.ErrInvalidRole, "%s", types.RoleName(msg.Role))
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

// assertMayGrant refuses everyone but governance and, for a country scope, the
// foundation.
//
// The order of the three branches is the design, and each one is a refusal rather
// than a fallthrough:
//
//  1. Governance is accepted for any scope. It is the body that decides the
//     perimeter, so requiring it to hold a grant would be circular — the same
//     reasoning that lets governance approve an issuer without holding
//     MONETARY_AUTHORITY.
//
//  2. The chain-wide scope is refused for everybody else, here, before the
//     constitution is read. Not after: a store failure while resolving the
//     foundation must not be the thing that decides whether "*" was allowed, and
//     an implementation that consulted the constitution first would have that
//     ordering hazard even while behaving correctly today.
//
//  3. Otherwise the signer has to be the foundation, resolved from the
//     constitution every time rather than cached. A cached authority is an
//     authority that outlives the amendment that changed it.
//
// Note what is not here: any path on which a missing lookup, an unwritten
// constitution or an absent keeper produces an acceptance. Every one of those is
// an error, and the error leaves governance as the only signer — which is exactly
// the rule this message had before the widening, so the closed state is a state
// the chain has already run in.
func (k Keeper) assertMayGrant(ctx context.Context, signer, scope string) error {
	if signer == k.GetAuthority() {
		return nil
	}

	if scope == types.ChainWide {
		return errorsmod.Wrapf(types.ErrInvalidSigner,
			"only governance may grant or revoke the chain-wide scope %q; expected %s, got %s. "+
				"The foundation may admit a country and may not manufacture authority over every country",
			types.ChainWide, k.GetAuthority(), signer)
	}

	foundation, err := k.foundationAddress(ctx)
	if err != nil {
		return err
	}
	if signer != foundation {
		return errorsmod.Wrapf(types.ErrInvalidSigner,
			"only governance or the foundation may grant or revoke a role in %s; expected %s or %s, got %s",
			scope, k.GetAuthority(), foundation, signer)
	}
	return nil
}

// foundationAddress is the account the constitution pins, and the only place this
// module decides who the foundation is.
//
// Two refusals rather than a zero value, because both of the states they cover
// would otherwise compare equal to an unset signer:
//
//   - no constitution keeper wired. Possible only in a unit test; the app always
//     supplies one. Refused rather than skipped, unlike assertGroupAccount's nil
//     check, and the difference is which way the bypass runs. A missing group
//     keeper can only cause a grant that should have been refused; a missing
//     constitution keeper that returned "" would make every signer the foundation
//     on a chain where the destination is also unset.
//
//   - an unwritten or empty destination. x/constitution's InitGenesis refuses to
//     start a chain whose recovery destination is unset, so on a correct chain
//     this does not happen — which is precisely why it has to be an error here
//     rather than a comparison. This repository has been bitten four times by
//     proto3's inability to tell 0 from unset, and "" == "" is the same bug
//     wearing a string.
func (k Keeper) foundationAddress(ctx context.Context) (string, error) {
	if k.constitution == nil {
		return "", errorsmod.Wrap(types.ErrInvalidSigner,
			"the foundation cannot be identified because x/constitution is not wired in, "+
				"so only governance may grant or revoke a role")
	}
	invariants, err := k.constitution.GetInvariants(ctx)
	if err != nil {
		return "", errorsmod.Wrap(err,
			"the foundation cannot be identified because the constitution could not be read")
	}
	// An unset destination is refused here AND would be refused by the address
	// decode below, and the redundancy is deliberate rather than left over. A
	// mutation pass found that deleting this branch changes no outcome — the empty
	// string is not a decodable address either — so what it is actually for is the
	// message: an operator told "the constitution names no
	// enforcement_recovery_destination" knows to go and look at the constitution,
	// where one told `""` is not an address this chain can read goes looking for a
	// typo that does not exist. The test asserts the text for that reason.
	pinned := strings.TrimSpace(invariants.EnforcementRecoveryDestination)
	if pinned == "" {
		return "", errorsmod.Wrap(types.ErrInvalidSigner,
			"the constitution names no enforcement_recovery_destination, so there is no foundation to accept; "+
				"only governance may grant or revoke a role")
	}
	if _, err := k.addressCodec.StringToBytes(pinned); err != nil {
		return "", errorsmod.Wrapf(types.ErrInvalidSigner,
			"the constitution's enforcement_recovery_destination %q is not an address this chain can read", pinned)
	}
	return pinned, nil
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
//
// Note the deliberate asymmetry with assertShape, which refuses on the same nil.
// The question is what a missing keeper can cause. Here it can only cause a grant
// to a plain key, which the perimeter path will then refuse to act on. There it
// would cause an ACTION by an office whose shape nobody read, which is the thing
// itself.
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
