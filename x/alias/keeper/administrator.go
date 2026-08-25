package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/alias/types"
)

// The foundation administrator, which is a role grant and used to be a
// parameter.
//
// Three places consult it — CountryOf's ZZ exemption, the correction of an
// account's country, and the two confidentiality grants in
// msg_server_viewing_key.go — and they ask two different questions, which is why
// there are two functions here rather than one.
//
// # Why two, and not one with a boolean argument
//
// Placement is a FACT and authority is an ACT, and an office's shape bears on
// the second and not the first.
//
// A grant may record the M-of-N its holder must keep (see types.OfficeShape),
// and an office that has fallen below it cannot act. But the exemption from
// "every account has a jurisdiction" is not an action anybody takes — it is the
// answer to "where is this account", asked by AssertScope about a TARGET, by the
// identifier issuer, and by genesis validation. If it consulted the shape, an
// office that lost a member would stop having a country at all: its own
// identifier would become unissuable, and every authority action against it
// would fail with "no recorded jurisdiction", which is a sentence about the
// wrong thing entirely. Worse, it would fail closed in the direction that
// removes an account from every perimeter rather than the one that refuses it a
// power.
//
// So hasFoundationGrant answers the placement question and reads only presence.
// actsAsFoundationAdministrator answers the authority question and refuses an
// office below its shape, exactly as assertGranted does for every other role.

// hasFoundationGrant reports whether an account holds the chain-wide grant of
// ROLE_FOUNDATION_ADMINISTRATOR.
//
// Presence only, and one exact read rather than a scan. The scope is not a
// parameter: the role is chain-wide or nothing — ValidGrantScope plus
// types.ChainWideOnly refuse any other form at every write — so looking it up
// under a country would be looking for a record no path can create.
func (k Keeper) hasFoundationGrant(ctx context.Context, addr string) (bool, error) {
	has, err := k.RoleGrants.Has(ctx,
		collections.Join3(addr, int32(types.ROLE_FOUNDATION_ADMINISTRATOR), types.ChainWide))
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return false, err
	}
	return has, nil
}

// actsAsFoundationAdministrator reports whether an account may exercise the
// authority the role carries.
//
// Three outcomes, and the third is the one worth reading the signature for:
//
//   - (true, nil): the grant is there and the office still meets the shape it
//     was granted under.
//   - (false, nil): no grant. Not an error, because every caller has another
//     signer it accepts — governance, or the account's own participant — and a
//     caller that turned this into a refusal would report the wrong reason for
//     a message that was going to be accepted anyway.
//   - (false, err): a store failure, or an office that has fallen below its
//     recorded M-of-N. Returned rather than folded into the false above,
//     because "your office is a 1-of-1 and this grant requires 3-of-5" and "you
//     are not an administrator" send an operator to different places, and the
//     first one is a proposal the office passes by itself.
func (k Keeper) actsAsFoundationAdministrator(ctx context.Context, addr string) (bool, error) {
	grant, err := k.RoleGrants.Get(ctx,
		collections.Join3(addr, int32(types.ROLE_FOUNDATION_ADMINISTRATOR), types.ChainWide))
	if errors.Is(err, collections.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := k.assertShape(ctx, addr, grant.RequiredShape); err != nil {
		return false, err
	}
	return true, nil
}

// countFoundationAdministrators counts the chain-wide grants of the role,
// ignoring one holder.
//
// The holder being granted is excluded so that re-granting an existing
// administrator — which is how a proposal resubmitted after a timeout arrives,
// and how a shape requirement is added to a grant that had none — is not refused
// by a place it already occupies. Without that, the cap would make the eighth
// administrator's grant impossible to amend.
//
// A prefix scan over (ChainWide, role) rather than a walk of every grant on the
// chain, so the cost is the size of the set being capped. The cap pays for its
// own enforcement, which is what makes it safe to check on a write path.
func (k Keeper) countFoundationAdministrators(ctx context.Context, excluding string) (int, error) {
	count := 0
	rng := collections.NewSuperPrefixedTripleRange[string, int32, string](
		types.ChainWide, int32(types.ROLE_FOUNDATION_ADMINISTRATOR))
	err := k.GrantsByScope.Walk(ctx, rng,
		func(key collections.Triple[string, int32, string]) (bool, error) {
			if key.K3() != excluding {
				count++
			}
			return false, nil
		})
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return 0, err
	}
	return count, nil
}
