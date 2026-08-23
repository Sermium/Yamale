package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	"yamale/blockchain/x/alias/types"
)

// This file is the whole of piece three: one check, in one place, that every
// authority action on the chain routes through.
//
// One function rather than eleven, because a perimeter enforced in eleven places
// is a perimeter with eleven ways to be wrong — and the eleventh is always the
// one nobody wrote a test for. Two entry points, because there are genuinely two
// shapes of question and forcing either into the other loses something:
//
//   - AssertScope, where the thing being acted on is an *account* and the chain
//     already knows which country it is in. Freezing an account, admitting an
//     issuer, approving a participant.
//   - AssertScopeIn, where the jurisdiction is *named in the message* and there
//     is no account to look it up from. A payment declares its settlement
//     jurisdiction; a registry office declares the jurisdiction it administers.
//
// Collapsing the second into the first would mean inventing an account to stand
// for a country. Collapsing the first into the second would mean letting the
// caller name the target's country, which is precisely the claim the caller must
// not be able to make: an actor who could tell the check which perimeter its
// target is in would be able to act on anybody by naming their own.
//
// Both are on the keeper rather than in an ante decorator, and that is not a
// stylistic preference. An ante gate sees only messages that arrive as
// transactions; interchain accounts and x/authz reach the message router by
// another road entirely and would bypass it, silently, for exactly the actions
// that matter most. The check has to be where the state changes.

// AssertScope refuses unless the actor's grant of role covers the country the
// target account is recorded in.
//
// Three refusals, in this order, and the order is the design:
//
//  1. an unset or unknown role. Never a default, never "any role".
//  2. a target whose jurisdiction the chain does not know. Not "matches
//     everything" and not "matches nothing" — an error, raised before any grant
//     is consulted, because an actor holding the chain-wide scope must not be
//     able to act on an account the chain cannot place either. On a correct
//     chain this state does not exist: every account has a jurisdiction and the
//     foundation administrators have the reserved code. Reaching it means the
//     account was never placed, and the fix is to place it, not to act on it.
//  3. no grant covering that country. Which includes, and is mostly, the actor
//     holding no grants at all.
//
// Note what is *not* here: any path on which a missing lookup produces a
// permissive answer. Every store error is returned, and every "not found" is a
// refusal. The failure this function exists to prevent is not being too strict;
// it is a zero value read as an authorisation.
func (k Keeper) AssertScope(ctx context.Context, actor string, role types.Role, target string) error {
	if !types.ValidRole(role) {
		return errorsmod.Wrapf(types.ErrInvalidRole, "%s", types.RoleName(role))
	}

	// The target's country, from the registry, never from the caller. CountryOf
	// is the one place the rule lives: a recorded jurisdiction wins, a named
	// foundation administrator with none gets the reserved code, and anybody else
	// with none is refused.
	country, err := k.CountryOf(ctx, target)
	if err != nil {
		if errors.Is(err, types.ErrNoJurisdiction) {
			return errorsmod.Wrapf(types.ErrNoJurisdiction,
				"%s has no recorded jurisdiction, so no authority's perimeter contains it", target)
		}
		return err
	}

	return k.assertGranted(ctx, actor, role, country)
}

// AssertScopeIn refuses unless the actor's grant of role covers a jurisdiction
// the message itself names.
//
// The country is validated against the assigned list before anything else, and
// both of the values that are legal elsewhere in this module are refused here:
//
//   - the chain-wide marker, because no payment settles chain-wide and no
//     registry office administers everywhere. A message that could name it would
//     be a message that chooses the widest perimeter for itself.
//   - the foundation's reserved code, because it marks the absence of a national
//     perimeter. A message declaring it would be declaring that no authority is
//     accountable for the act, which is the opposite of what declaring a
//     settlement jurisdiction is for.
//
// A chain-wide grant still covers whatever country is named, which is what makes
// the marker unnecessary as an input.
func (k Keeper) AssertScopeIn(ctx context.Context, actor string, role types.Role, jurisdiction string) error {
	if !types.ValidRole(role) {
		return errorsmod.Wrapf(types.ErrInvalidRole, "%s", types.RoleName(role))
	}

	country := types.NormaliseCountry(jurisdiction)
	if !types.AssignedCountry(country) {
		return errorsmod.Wrapf(types.ErrInvalidCountry,
			"%q is not a jurisdiction an action can be attributed to", jurisdiction)
	}

	return k.assertGranted(ctx, actor, role, country)
}

// There is deliberately no boolean form of AssertScope on this keeper — no
// HasScope that returns true or false and swallows the error.
//
// It would be the obvious convenience for a handler choosing between two
// legitimate signers, and it is the wrong shape for exactly that job: an invalid
// role, an unplaceable target and a store failure are not "unauthorised", they
// are questions that could not be asked, and a boolean collapses all three into
// the same "no" that a genuine out-of-scope refusal produces. A caller that then
// treats false as "try the other signer" has turned a store failure into a
// fallback. The handlers that need to choose ask HoldsRole about the *actor*
// first, which is a question with an honest boolean answer, and then assert.

// HoldsRole reports whether an account holds a role in any jurisdiction at all.
//
// It exists for error legibility and for nothing else, so read what it is not:
// it is *not* an authorisation check, it does not look at the target, and no
// action anywhere is permitted on the strength of it. It answers "is this
// account an authority of this kind" so that a message from somebody who is not
// one can be refused as an invalid signer, rather than with a complaint about
// the perimeter of a target they were never entitled to touch.
//
// The two handlers that use it — issuer approval and participant approval —
// accept either governance or a scoped authority, and without this they would
// tell a random account that its victim has no recorded jurisdiction, which
// sends the reader looking for the wrong bug. AssertScope still runs afterwards
// and is still the only thing that permits anything.
//
// It deliberately does not consult the holder's shape. An office that has fallen
// below the M-of-N it was granted under is still an authority of that kind — it
// is one that cannot act — and that is the right answer to the question this
// function asks. Reading the shape here would cost two x/group queries on a path
// that only chooses an error message, and it would turn the fallen office's
// refusal back into "you are not a payments authority", which is the wrong bug
// to send somebody looking for. AssertScope says the true thing.
func (k Keeper) HoldsRole(ctx context.Context, actor string, role types.Role) (bool, error) {
	if !types.ValidRole(role) {
		return false, errorsmod.Wrapf(types.ErrInvalidRole, "%s", types.RoleName(role))
	}
	held := false
	rng := collections.NewSuperPrefixedTripleRange[string, int32, string](actor, int32(role))
	err := k.RoleGrants.Walk(ctx, rng,
		func(_ collections.Triple[string, int32, string], _ types.RoleGrant) (bool, error) {
			held = true
			return true, nil
		})
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return false, err
	}
	return held, nil
}

// assertGranted is the lookup, and the only place a grant is read for a
// decision.
//
// Two exact reads, no scan. The chain-wide grant is tried first because it is
// the cheaper answer for the accounts most likely to be acting on an arbitrary
// target, and because trying it second would mean a country lookup ran for every
// foundation action. Neither read can match by accident: the country grant is
// looked up under the country the registry gave, and the chain-wide grant under
// a marker that no country code folds to.
//
// # A grant is not a fact, it is a fact plus a condition
//
// Finding the grant is no longer the end of the question. A grant may record the
// M-of-N its holder must keep — see types.OfficeShape — and an office that has
// fallen below it does not hold the authority any more, whatever the store says.
// So each grant found is checked against the office's shape right now, and only a
// grant that is both present AND still satisfied permits anything.
//
// The loop continues past a grant whose shape has fallen rather than refusing on
// the spot, because the two grants are separate authorities with separate
// requirements: an actor whose chain-wide grant demanded five-of-nine and whose
// country grant demanded two-of-three legitimately still holds the second. What
// is remembered is the FIRST shape failure, so that an actor who holds nothing
// usable is told why — "your office fell below three-of-five" rather than "you
// hold no grant", which would send an operator to the wrong module entirely.
//
// Both reads are for the same holder, so the two shape checks would ask x/group
// the same two questions twice. That is the worst case and it happens only for an
// actor holding both a chain-wide and a country grant of one role, both carrying
// requirements, with the chain-wide one no longer met. The ordinary case is one
// grant, one policy read and one member read; a grant with no recorded
// requirement reads x/group not at all.
func (k Keeper) assertGranted(ctx context.Context, actor string, role types.Role, country string) error {
	var fallen error
	for _, scope := range [...]string{types.ChainWide, country} {
		grant, err := k.RoleGrants.Get(ctx, collections.Join3(actor, int32(role), scope))
		if errors.Is(err, collections.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := k.assertShape(ctx, actor, grant.RequiredShape); err != nil {
			if fallen == nil {
				fallen = err
			}
			continue
		}
		return nil
	}
	if fallen != nil {
		return fallen
	}
	return errorsmod.Wrapf(types.ErrOutOfScope,
		"%s holds no grant of %s covering %s", actor, types.RoleName(role), country)
}

// grant writes both directions of a grant at once.
//
// Only ever written together, like every other pair in this module: a
// half-written grant is a reverse view that disagrees with the registry, and the
// reverse view is what a governance console shows somebody deciding whether the
// perimeter is what they think it is.
func (k Keeper) grant(ctx context.Context, g types.RoleGrant) error {
	if err := k.RoleGrants.Set(ctx, collections.Join3(g.Holder, int32(g.Role), g.Jurisdiction), g); err != nil {
		return err
	}
	return k.GrantsByScope.Set(ctx, collections.Join3(g.Jurisdiction, int32(g.Role), g.Holder))
}

// revoke removes both directions, and reports whether there was anything there.
//
// found is returned rather than swallowed so the handler can tell revoking the
// grant somebody meant from revoking nothing at all — an operator told only
// "done" cannot tell those apart, and the second one leaves an authority in
// place that they believe they have removed.
func (k Keeper) revoke(ctx context.Context, holder string, role types.Role, jurisdiction string) (bool, error) {
	key := collections.Join3(holder, int32(role), jurisdiction)
	has, err := k.RoleGrants.Has(ctx, key)
	if err != nil || !has {
		return false, err
	}
	if err := k.RoleGrants.Remove(ctx, key); err != nil {
		return false, err
	}
	if err := k.GrantsByScope.Remove(ctx, collections.Join3(jurisdiction, int32(role), holder)); err != nil {
		return false, err
	}
	return true, nil
}

// GrantsOf returns every role an account holds, chain-wide grants included.
//
// A prefix scan over the holder, so the cost is what that holder has been
// granted rather than what the chain has ever granted anybody.
func (k Keeper) GrantsOf(ctx context.Context, holder string) ([]types.RoleGrant, error) {
	grants := []types.RoleGrant{}
	rng := collections.NewPrefixedTripleRange[string, int32, string](holder)
	err := k.RoleGrants.Walk(ctx, rng,
		func(_ collections.Triple[string, int32, string], g types.RoleGrant) (bool, error) {
			grants = append(grants, g)
			return false, nil
		})
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return nil, err
	}
	return grants, nil
}
