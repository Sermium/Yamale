package keeper_test

import (
	"context"
	"errors"
	"testing"

	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/x/group"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/alias/keeper"
	module "yamale/blockchain/x/alias/module"
	"yamale/blockchain/x/alias/types"
)

// The tests for piece two and piece three: the grant registry, and the one check
// every authority action routes through.
//
// Read the AssertScope tests as a group. Individually they look like a list of
// refusals; together they are the statement that there is no input on which a
// missing grant, an unknown role or an unplaceable target produces permission.
// That is the only property that matters here, because the failure mode of a
// perimeter is never that it is too strict.

// stubGroups answers x/group's one question for a fixed set of addresses.
//
// A stub rather than a real x/group keeper, and this is the one place in these
// files where that is the right call: the question is a plain fact about another
// module's state ("is this address a group policy"), and what is under test is
// that *this* module asks it and refuses on a no. Standing up x/group would
// exercise x/group.
type stubGroups struct{ policies map[string]bool }

func (s stubGroups) GroupPolicyInfo(
	_ context.Context, req *group.QueryGroupPolicyInfoRequest,
) (*group.QueryGroupPolicyInfoResponse, error) {
	if s.policies[req.Address] {
		return &group.QueryGroupPolicyInfoResponse{}, nil
	}
	// The shape of a real miss: x/group's query returns an error for an address
	// that has no policy, and that error is all this module looks at.
	return nil, errors.New("no group policy for " + req.Address)
}

type roleFixture struct {
	env    *integration.Env
	k      keeper.Keeper
	ms     types.MsgServer
	qs     types.QueryServer
	groups stubGroups

	// admin holds the foundation exemption: no jurisdiction, and the reserved
	// code as its country.
	admin string
}

func roleSetup(t *testing.T) *roleFixture {
	t.Helper()

	env := integration.New(t, types.ModuleName, module.AppModule{})
	groups := stubGroups{policies: map[string]bool{}}

	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, groups)

	f := &roleFixture{
		env:    env,
		k:      k,
		ms:     keeper.NewMsgServerImpl(k),
		qs:     keeper.NewQueryServerImpl(k),
		groups: groups,
	}

	_, f.admin = env.Addr(t)
	gs := types.DefaultGenesis()
	gs.Params = types.NewParams(types.PayloadLength, f.admin)
	require.NoError(t, k.InitGenesis(env.Ctx, *gs))
	return f
}

// office returns a fresh address that the group keeper will vouch for, so it can
// legitimately hold a role.
func (f *roleFixture) office(t *testing.T) string {
	t.Helper()
	_, addr := f.env.Addr(t)
	f.groups.policies[addr] = true
	return addr
}

// placed returns a fresh account recorded in a country, by governance.
func (f *roleFixture) placed(t *testing.T, cc string) string {
	t.Helper()
	_, addr := f.env.Addr(t)
	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.env.AuthorityString(t), Account: addr, Country: cc,
	})
	require.NoError(t, err)
	return addr
}

func (f *roleFixture) grant(t *testing.T, holder string, role types.Role, scope string) {
	t.Helper()
	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: holder, Role: role, Jurisdiction: scope,
	})
	require.NoError(t, err)
}

// The headline: a grant naming one country does not reach another, and the same
// actor is allowed at home.
func TestAnAuthorityActsAtHomeAndNowhereElse(t *testing.T) {
	f := roleSetup(t)
	lands := f.office(t)
	f.grant(t, lands, types.ROLE_REGISTRY_AUTHORITY, "GH")

	atHome := f.placed(t, "GH")
	abroad := f.placed(t, "KE")

	require.NoError(t, f.k.AssertScope(f.env.Ctx, lands, types.ROLE_REGISTRY_AUTHORITY, atHome))

	err := f.k.AssertScope(f.env.Ctx, lands, types.ROLE_REGISTRY_AUTHORITY, abroad)
	require.ErrorIs(t, err, types.ErrOutOfScope)
	require.ErrorContains(t, err, "ROLE_REGISTRY_AUTHORITY")
	require.ErrorContains(t, err, "KE",
		"the refusal must name the perimeter that was refused, or nobody can tell why")

	// And the same holder is refused a role it was never granted, in the very
	// country it does hold one for. A grant is a triple, not a pair.
	require.ErrorIs(t,
		f.k.AssertScope(f.env.Ctx, lands, types.ROLE_ENFORCEMENT_AUTHORITY, atHome),
		types.ErrOutOfScope)
}

// The failure this whole file exists to rule out: an actor with no grants at all
// being waved through because a lookup returned a zero value and no error.
func TestNoGrantRefusesEveryRoleEverywhere(t *testing.T) {
	f := roleSetup(t)
	_, stranger := f.env.Addr(t)
	target := f.placed(t, "GH")

	for _, role := range []types.Role{
		types.ROLE_REGISTRY_AUTHORITY,
		types.ROLE_MONETARY_AUTHORITY,
		types.ROLE_PAYMENTS_AUTHORITY,
		types.ROLE_ENFORCEMENT_AUTHORITY,
		types.ROLE_SUPERVISOR,
	} {
		require.ErrorIsf(t, f.k.AssertScope(f.env.Ctx, stranger, role, target),
			types.ErrOutOfScope, "%s was permitted to an account holding no grants", types.RoleName(role))
		require.ErrorIsf(t, f.k.AssertScopeIn(f.env.Ctx, stranger, role, "GH"),
			types.ErrOutOfScope, "%s was permitted to an account holding no grants", types.RoleName(role))
	}

	// Not even governance, which can grant every role, may act as though it held
	// one. Being able to decide who holds a power is not holding it.
	require.ErrorIs(t,
		f.k.AssertScope(f.env.Ctx, f.env.AuthorityString(t), types.ROLE_REGISTRY_AUTHORITY, target),
		types.ErrOutOfScope)
}

// The other half of the same failure: a target the chain cannot place must be
// refused, not matched.
//
// Checked before any grant is consulted, so it holds for the chain-wide scope
// too. A holder of "*" acting on an account nobody has recorded is acting
// outside every perimeter there is, including its own.
func TestAnUnplacedTargetIsRefusedToEverybody(t *testing.T) {
	f := roleSetup(t)
	_, nowhere := f.env.Addr(t)

	national := f.office(t)
	f.grant(t, national, types.ROLE_ENFORCEMENT_AUTHORITY, "GH")
	chainWide := f.office(t)
	f.grant(t, chainWide, types.ROLE_ENFORCEMENT_AUTHORITY, types.ChainWide)

	for _, actor := range []string{national, chainWide} {
		err := f.k.AssertScope(f.env.Ctx, actor, types.ROLE_ENFORCEMENT_AUTHORITY, nowhere)
		require.ErrorIsf(t, err, types.ErrNoJurisdiction,
			"%s was allowed to act on an account the chain cannot place", actor)
	}

	// And the moment it is placed, the perimeter decides — which is the proof
	// that the refusal above was about the missing record and not about the
	// grants.
	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.env.AuthorityString(t), Account: nowhere, Country: "GH",
	})
	require.NoError(t, err)
	require.NoError(t, f.k.AssertScope(f.env.Ctx, national, types.ROLE_ENFORCEMENT_AUTHORITY, nowhere))
}

// "No jurisdiction recorded" and "the foundation" must never be the same state:
// one is an error and the other is the highest privilege on the chain.
func TestTheFoundationIsReachableOnlyByTheChainWideScope(t *testing.T) {
	f := roleSetup(t)

	// The administrator resolves to the reserved code rather than to an error,
	// which is the distinction under test.
	country, err := f.k.CountryOf(f.env.Ctx, f.admin)
	require.NoError(t, err)
	require.Equal(t, types.FoundationCountry, country)

	national := f.office(t)
	f.grant(t, national, types.ROLE_ENFORCEMENT_AUTHORITY, "GH")
	require.ErrorIs(t,
		f.k.AssertScope(f.env.Ctx, national, types.ROLE_ENFORCEMENT_AUTHORITY, f.admin),
		types.ErrOutOfScope,
		"a national authority reached a foundation administrator")

	chainWide := f.office(t)
	f.grant(t, chainWide, types.ROLE_ENFORCEMENT_AUTHORITY, types.ChainWide)
	require.NoError(t,
		f.k.AssertScope(f.env.Ctx, chainWide, types.ROLE_ENFORCEMENT_AUTHORITY, f.admin))

	// The reserved code cannot be granted, so there is no way to manufacture a
	// grant that covers the foundation without covering everything — which is
	// what makes the chain-wide list the complete list of accounts that can.
	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: f.office(t),
		Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: types.FoundationCountry,
	})
	require.ErrorIs(t, err, types.ErrInvalidScope)
}

// The chain-wide scope covers every country, and is the only thing that does.
func TestTheChainWideScopeCoversEveryCountry(t *testing.T) {
	f := roleSetup(t)
	auditor := f.office(t)
	f.grant(t, auditor, types.ROLE_SUPERVISOR, types.ChainWide)

	for _, cc := range []string{"GH", "NG", "KE", "CI", "ZA"} {
		require.NoError(t, f.k.AssertScope(f.env.Ctx, auditor,
			types.ROLE_SUPERVISOR, f.placed(t, cc)))
		require.NoError(t, f.k.AssertScopeIn(f.env.Ctx, auditor, types.ROLE_SUPERVISOR, cc))
	}
	// One role, not all of them.
	require.ErrorIs(t,
		f.k.AssertScopeIn(f.env.Ctx, auditor, types.ROLE_MONETARY_AUTHORITY, "GH"),
		types.ErrOutOfScope)
}

// The zero value is reserved and refused on every path that touches a role.
//
// Four call sites, because this is the mistake the repository has made most
// often: proto3 cannot tell an unset field from a zero, so a role numbered zero
// would make "grant the first role" and "grant whatever the default is" the same
// message.
func TestTheUnsetRoleIsNeverADefault(t *testing.T) {
	f := roleSetup(t)
	holder := f.office(t)
	target := f.placed(t, "GH")

	require.False(t, types.ValidRole(types.ROLE_UNSPECIFIED))

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: holder,
		Role: types.ROLE_UNSPECIFIED, Jurisdiction: "GH",
	})
	require.ErrorIs(t, err, types.ErrInvalidRole)

	_, err = f.ms.RevokeRole(f.env.Ctx, &types.MsgRevokeRole{
		Authority: f.env.AuthorityString(t), Holder: holder,
		Role: types.ROLE_UNSPECIFIED, Jurisdiction: "GH",
	})
	require.ErrorIs(t, err, types.ErrInvalidRole)

	require.ErrorIs(t, f.k.AssertScope(f.env.Ctx, holder, types.ROLE_UNSPECIFIED, target),
		types.ErrInvalidRole)
	require.ErrorIs(t, f.k.AssertScopeIn(f.env.Ctx, holder, types.ROLE_UNSPECIFIED, "GH"),
		types.ErrInvalidRole)

	// And a number that is not a role at all, which is what a client built
	// against a newer proto sends to an older chain.
	require.ErrorIs(t, f.k.AssertScopeIn(f.env.Ctx, holder, types.Role(99), "GH"),
		types.ErrInvalidRole)
}

// A message may name a country. It may not name the chain-wide scope, and it may
// not name the foundation's reserved code.
func TestAMessageCannotDeclareTheWidestPerimeterForItself(t *testing.T) {
	f := roleSetup(t)
	holder := f.office(t)
	f.grant(t, holder, types.ROLE_PAYMENTS_AUTHORITY, types.ChainWide)

	for _, named := range []string{types.ChainWide, types.FoundationCountry, "NX", "", "GHA", "gh "} {
		err := f.k.AssertScopeIn(f.env.Ctx, holder, types.ROLE_PAYMENTS_AUTHORITY, named)
		require.ErrorIsf(t, err, types.ErrInvalidCountry,
			"%q was accepted as a jurisdiction an action can be attributed to", named)
	}

	// Lower case is a typing convenience, not a different perimeter.
	require.NoError(t, f.k.AssertScopeIn(f.env.Ctx, holder, types.ROLE_PAYMENTS_AUTHORITY, "gh"))
}

// Grants are governance's to make. Not the holder's, not another holder's, and
// not a foundation administrator's.
func TestOnlyGovernanceMayGrantOrRevokeARole(t *testing.T) {
	f := roleSetup(t)
	holder := f.office(t)
	f.grant(t, holder, types.ROLE_REGISTRY_AUTHORITY, "GH")

	// The holder cannot widen its own grant. A role its own holder can grant is
	// not a perimeter.
	for _, signer := range []string{holder, f.admin} {
		_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
			Authority: signer, Holder: holder,
			Role: types.ROLE_REGISTRY_AUTHORITY, Jurisdiction: types.ChainWide,
		})
		require.ErrorIsf(t, err, types.ErrInvalidSigner, "%s was allowed to grant a role", signer)

		_, err = f.ms.RevokeRole(f.env.Ctx, &types.MsgRevokeRole{
			Authority: signer, Holder: holder,
			Role: types.ROLE_REGISTRY_AUTHORITY, Jurisdiction: "GH",
		})
		require.ErrorIsf(t, err, types.ErrInvalidSigner, "%s was allowed to revoke a role", signer)
	}

	// The grant is exactly as it was.
	require.NoError(t, f.k.AssertScopeIn(f.env.Ctx, holder, types.ROLE_REGISTRY_AUTHORITY, "GH"))
	require.ErrorIs(t, f.k.AssertScopeIn(f.env.Ctx, holder, types.ROLE_REGISTRY_AUTHORITY, "KE"),
		types.ErrOutOfScope)
}

// A role goes to an office, and an office that is one key is one bribe.
func TestARoleCannotBeGrantedToAPlainKey(t *testing.T) {
	f := roleSetup(t)
	_, plainKey := f.env.Addr(t)

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: plainKey,
		Role: types.ROLE_MONETARY_AUTHORITY, Jurisdiction: "NG",
	})
	require.ErrorIs(t, err, types.ErrHolderNotGroup)

	// Nothing was written on the way out — a refused grant that left a row behind
	// would be a grant.
	require.ErrorIs(t,
		f.k.AssertScopeIn(f.env.Ctx, plainKey, types.ROLE_MONETARY_AUTHORITY, "NG"),
		types.ErrOutOfScope)
}

// Revocation takes the power away, and revoking something that was never granted
// is an error rather than a quiet success.
func TestRevocationEndsTheGrantAndSaysSoWhenThereIsNothingToEnd(t *testing.T) {
	f := roleSetup(t)
	holder := f.office(t)
	f.grant(t, holder, types.ROLE_ENFORCEMENT_AUTHORITY, "GH")
	f.grant(t, holder, types.ROLE_ENFORCEMENT_AUTHORITY, "KE")

	_, err := f.ms.RevokeRole(f.env.Ctx, &types.MsgRevokeRole{
		Authority: f.env.AuthorityString(t), Holder: holder,
		Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "GH",
	})
	require.NoError(t, err)

	require.ErrorIs(t, f.k.AssertScopeIn(f.env.Ctx, holder, types.ROLE_ENFORCEMENT_AUTHORITY, "GH"),
		types.ErrOutOfScope)
	// The other perimeter is untouched: revoking one country must not revoke the
	// rest, which is why the jurisdiction is part of what is revoked.
	require.NoError(t, f.k.AssertScopeIn(f.env.Ctx, holder, types.ROLE_ENFORCEMENT_AUTHORITY, "KE"))

	// A second revocation of the same grant is refused. "Nothing to revoke" is
	// how a proposal that named the wrong jurisdiction succeeds while leaving the
	// authority it meant to remove in place.
	_, err = f.ms.RevokeRole(f.env.Ctx, &types.MsgRevokeRole{
		Authority: f.env.AuthorityString(t), Holder: holder,
		Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "GH",
	})
	require.ErrorIs(t, err, types.ErrGrantNotFound)

	// And the derived view followed the revocation rather than keeping a stale
	// entry. An index that still lists a revoked authority is worse than none: it
	// is what a governance console shows somebody deciding whether the perimeter
	// is what they think it is.
	held, err := f.qs.RoleHolders(f.env.Ctx, &types.QueryRoleHoldersRequest{Jurisdiction: "GH"})
	require.NoError(t, err)
	require.Empty(t, held.Grants)
}

// HoldsRole answers a different question from AssertScope, and permits nothing.
func TestHoldsRoleIsAboutTheActorAndNotTheTarget(t *testing.T) {
	f := roleSetup(t)
	holder := f.office(t)
	f.grant(t, holder, types.ROLE_MONETARY_AUTHORITY, "NG")

	holds, err := f.k.HoldsRole(f.env.Ctx, holder, types.ROLE_MONETARY_AUTHORITY)
	require.NoError(t, err)
	require.True(t, holds)

	holds, err = f.k.HoldsRole(f.env.Ctx, holder, types.ROLE_REGISTRY_AUTHORITY)
	require.NoError(t, err)
	require.False(t, holds)

	_, stranger := f.env.Addr(t)
	holds, err = f.k.HoldsRole(f.env.Ctx, stranger, types.ROLE_MONETARY_AUTHORITY)
	require.NoError(t, err)
	require.False(t, holds)

	// Holding the role somewhere is not permission to act anywhere: this is the
	// distinction the two functions exist to keep apart.
	require.ErrorIs(t,
		f.k.AssertScope(f.env.Ctx, holder, types.ROLE_MONETARY_AUTHORITY, f.placed(t, "GH")),
		types.ErrOutOfScope)

	_, err = f.k.HoldsRole(f.env.Ctx, holder, types.ROLE_UNSPECIFIED)
	require.ErrorIs(t, err, types.ErrInvalidRole)
}

// The exception has to be listable, or nobody audits it.
func TestTheChainWideGrantsAreListedOnTheirOwn(t *testing.T) {
	f := roleSetup(t)

	national := f.office(t)
	f.grant(t, national, types.ROLE_REGISTRY_AUTHORITY, "GH")
	f.grant(t, national, types.ROLE_REGISTRY_AUTHORITY, "KE")

	wide := f.office(t)
	f.grant(t, wide, types.ROLE_SUPERVISOR, types.ChainWide)
	f.grant(t, wide, types.ROLE_ENFORCEMENT_AUTHORITY, types.ChainWide)

	res, err := f.qs.ChainWideGrants(f.env.Ctx, &types.QueryChainWideGrantsRequest{})
	require.NoError(t, err)
	require.Len(t, res.Grants, 2)
	for _, g := range res.Grants {
		require.Equal(t, types.ChainWide, g.Jurisdiction)
		require.Equal(t, wide, g.Holder)
		require.Equal(t, f.env.AuthorityString(t), g.GrantedBy,
			"a grant nobody is recorded as having made cannot be accounted for")
	}

	// A country's own list shows what that country granted, and does not quietly
	// fold in the exceptions — which would hide them among the ordinary entries
	// of every country at once.
	gh, err := f.qs.RoleHolders(f.env.Ctx, &types.QueryRoleHoldersRequest{Jurisdiction: "GH"})
	require.NoError(t, err)
	require.Len(t, gh.Grants, 1)
	require.Equal(t, national, gh.Grants[0].Holder)

	// The holder's own view does include them, because "what may this account do"
	// with its widest grant left out would be actively misleading.
	mine, err := f.qs.RoleGrants(f.env.Ctx, &types.QueryRoleGrantsRequest{Holder: wide})
	require.NoError(t, err)
	require.Len(t, mine.Grants, 2)

	// And an account with nothing gets an empty answer rather than an error:
	// "may act nowhere" is what an operator checking a key needs to be told.
	_, stranger := f.env.Addr(t)
	none, err := f.qs.RoleGrants(f.env.Ctx, &types.QueryRoleGrantsRequest{Holder: stranger})
	require.NoError(t, err)
	require.Empty(t, none.Grants)
}

// The role filter on the reverse view narrows without widening.
func TestRoleHoldersCanBeNarrowedToOneRole(t *testing.T) {
	f := roleSetup(t)
	lands := f.office(t)
	bank := f.office(t)
	f.grant(t, lands, types.ROLE_REGISTRY_AUTHORITY, "NG")
	f.grant(t, bank, types.ROLE_MONETARY_AUTHORITY, "NG")

	all, err := f.qs.RoleHolders(f.env.Ctx, &types.QueryRoleHoldersRequest{Jurisdiction: "NG"})
	require.NoError(t, err)
	require.Len(t, all.Grants, 2)

	only, err := f.qs.RoleHolders(f.env.Ctx, &types.QueryRoleHoldersRequest{
		Jurisdiction: "ng", Role: types.ROLE_MONETARY_AUTHORITY,
	})
	require.NoError(t, err)
	require.Len(t, only.Grants, 1)
	require.Equal(t, bank, only.Grants[0].Holder)
}

// The derived index is rebuilt from the grants rather than carried in genesis, so
// an export has to round-trip byte for byte and the rebuilt index has to agree
// with what it indexes.
func TestGenesisRoundTripsWithRoleGrants(t *testing.T) {
	f := roleSetup(t)

	for _, cc := range []string{"GH", "NG", "KE", types.ChainWide} {
		holder := f.office(t)
		f.grant(t, holder, types.ROLE_REGISTRY_AUTHORITY, cc)
		f.grant(t, holder, types.ROLE_SUPERVISOR, cc)
	}

	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())
	require.Len(t, exported.RoleGrants, 8)

	// A second environment, so the import lands in an empty store.
	//
	// Reusing the first would leave the derived index already populated by the
	// messages above, and every assertion below would pass whether InitGenesis
	// rebuilt it or not — which is exactly what they exist to check.
	other := integration.New(t, types.ModuleName, module.AppModule{})
	fresh := keeper.NewKeeper(other.Codec, other.AddressCodec, other.StoreService,
		log.NewNopLogger(), other.AuthorityString(t), nil, nil)
	require.NoError(t, fresh.InitGenesis(other.Ctx, *exported))

	again, err := fresh.ExportGenesis(other.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, again)

	// Byte for byte as well as field by field: comparing the encodings is the
	// only assertion that catches a field added to RoleGrant and forgotten in
	// InitGenesis or ExportGenesis.
	first, err := exported.Marshal()
	require.NoError(t, err)
	second, err := again.Marshal()
	require.NoError(t, err)
	require.Equal(t, first, second)

	// The rebuilt reverse index must agree with every grant, in both of the
	// directions a caller reads it from — and the check must not be able to pass
	// on an empty set.
	freshQS := keeper.NewQueryServerImpl(fresh)
	wide := 0
	for _, g := range again.RoleGrants {
		if g.Jurisdiction == types.ChainWide {
			wide++
			continue
		}
		listed, err := freshQS.RoleHolders(other.Ctx, &types.QueryRoleHoldersRequest{
			Jurisdiction: g.Jurisdiction, Role: g.Role,
		})
		require.NoError(t, err)
		require.Containsf(t, listed.Grants, g,
			"%s holds %s in %s but the rebuilt index does not list it",
			g.Holder, types.RoleName(g.Role), g.Jurisdiction)
	}
	require.Equal(t, 2, wide, "the fixture stopped exercising the chain-wide scope")

	listed, err := freshQS.ChainWideGrants(other.Ctx, &types.QueryChainWideGrantsRequest{})
	require.NoError(t, err)
	require.Len(t, listed.Grants, wide)

	// And the perimeter still refuses across a border after the round trip, which
	// is the only thing the index is for.
	require.ErrorIs(t,
		fresh.AssertScopeIn(other.Ctx, again.RoleGrants[0].Holder, types.ROLE_MONETARY_AUTHORITY, "GH"),
		types.ErrOutOfScope)
}

// Nothing re-examines a record written at height zero, so the handler's rules
// have to hold against a file too.
func TestGenesisRefusesAGrantTheHandlerWouldRefuse(t *testing.T) {
	const holder = "yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p"

	gs := types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{{Holder: holder, Jurisdiction: "GH"}}
	require.ErrorContains(t, gs.Validate(), "ROLE_UNSPECIFIED")

	gs = types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{
		{Holder: holder, Role: types.ROLE_SUPERVISOR, Jurisdiction: types.FoundationCountry},
	}
	require.ErrorContains(t, gs.Validate(), "neither an assigned country code")

	gs = types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{
		{Holder: holder, Role: types.ROLE_SUPERVISOR, Jurisdiction: "NX"},
	}
	require.ErrorContains(t, gs.Validate(), "neither an assigned country code")

	gs = types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{
		{Holder: "", Role: types.ROLE_SUPERVISOR, Jurisdiction: "GH"},
	}
	require.ErrorContains(t, gs.Validate(), "empty holder")

	// A duplicated triple. The derived index would hold one entry for two
	// records, so the file would stop round-tripping the moment either was
	// revoked.
	gs = types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{
		{Holder: holder, Role: types.ROLE_SUPERVISOR, Jurisdiction: "GH"},
		{Holder: holder, Role: types.ROLE_SUPERVISOR, Jurisdiction: "GH"},
	}
	require.ErrorContains(t, gs.Validate(), "twice")

	// The same file with one of each loads, including the chain-wide form.
	gs = types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{
		{Holder: holder, Role: types.ROLE_SUPERVISOR, Jurisdiction: "GH"},
		{Holder: holder, Role: types.ROLE_SUPERVISOR, Jurisdiction: types.ChainWide},
	}
	require.NoError(t, gs.Validate())
}

// A grant seeded at genesis is a grant, and the perimeter reads it the same way
// it reads one made by proposal.
func TestAGrantSeededAtGenesisWorks(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, nil)

	_, holder := env.Addr(t)
	_, target := env.Addr(t)

	gs := types.DefaultGenesis()
	gs.Jurisdictions = []types.Jurisdiction{{Address: target, Country: "NG"}}
	gs.RoleGrants = []types.RoleGrant{
		{Holder: holder, Role: types.ROLE_PAYMENTS_AUTHORITY, Jurisdiction: "NG"},
	}
	require.NoError(t, gs.Validate())
	require.NoError(t, k.InitGenesis(env.Ctx, *gs))

	require.NoError(t, k.AssertScope(env.Ctx, holder, types.ROLE_PAYMENTS_AUTHORITY, target))
	require.ErrorIs(t, k.AssertScopeIn(env.Ctx, holder, types.ROLE_PAYMENTS_AUTHORITY, "GH"),
		types.ErrOutOfScope)
}
