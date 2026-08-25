package keeper_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/x/group"
	groupmodule "github.com/cosmos/cosmos-sdk/x/group/module"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/alias/keeper"
	module "yamale/blockchain/x/alias/module"
	"yamale/blockchain/x/alias/types"
	constitutiontypes "yamale/blockchain/x/constitution/types"
)

// The tests for piece two and piece three: the grant registry, and the one check
// every authority action routes through.
//
// Read the AssertScope tests as a group. Individually they look like a list of
// refusals; together they are the statement that there is no input on which a
// missing grant, an unknown role or an unplaceable target produces permission.
// That is the only property that matters here, because the failure mode of a
// perimeter is never that it is too strict.

// stubGroups answers x/group's questions for a fixed set of addresses.
//
// A stub rather than a real x/group keeper, and this is the one place in these
// files where that is the right call: the questions are plain facts about another
// module's state ("is this address a group policy", "what is its threshold", "who
// are its members"), and what is under test is that *this* module asks them and
// refuses on the wrong answer. Standing up x/group would exercise x/group.
//
// What the stub does NOT fake is the encoding. The decision policy is a real
// ThresholdDecisionPolicy inside a real Any, because reading the threshold out of
// that Any is a step the module can get wrong and a stub that handed over the
// number directly would test nothing. groupmodule.AppModuleBasic is registered on
// the test codec for the same reason: the unpack is the chain's unpack.
//
// A pointer, so a test can reshape an office mid-run — which is the whole failure
// under test. An office that could only be created and never changed would be an
// office whose threshold nobody can vote down, and that is precisely the state
// this module used to assume.
type stubGroups struct {
	policies map[string]*stubOffice
	byID     map[uint64]*stubOffice
	next     uint64

	// policyCalls and memberCalls count what the module asked for. They are here
	// so a test can assert the COST of the shape check rather than describing it
	// in a comment — in particular that a grant carrying no requirement asks
	// x/group nothing at all, which is the claim that keeps this change from being
	// a tax on every authority action on the chain.
	policyCalls int
	memberCalls int
}

// stubOffice is one group and its policy: a threshold, and one weight per member.
//
// Weights are strings because x/group's are, and a test that passed integers
// would not be able to construct the case that matters most — the group whose
// first member weighs three, which reads as a three-of-five and acts as a
// one-of-five.
type stubOffice struct {
	id        uint64
	threshold string
	weights   []string

	// percentage installs a PercentageDecisionPolicy instead of a threshold one,
	// which is the policy this module refuses to hold a recorded shape against.
	percentage string
}

func newStubGroups() *stubGroups {
	return &stubGroups{policies: map[string]*stubOffice{}, byID: map[uint64]*stubOffice{}}
}

// add registers an address as a group policy of m-of-n with equal weights, which
// is what every ceremony in this repository produces.
func (s *stubGroups) add(addr string, threshold, members int) *stubOffice {
	weights := make([]string, members)
	for i := range weights {
		weights[i] = "1"
	}
	return s.addWeighted(addr, fmt.Sprintf("%d", threshold), weights)
}

func (s *stubGroups) addWeighted(addr, threshold string, weights []string) *stubOffice {
	s.next++
	office := &stubOffice{id: s.next, threshold: threshold, weights: weights}
	s.policies[addr] = office
	s.byID[office.id] = office
	return office
}

// reshape is the office voting on itself: a new threshold, a new membership, the
// same policy address. Nothing about it is unusual — an office administers itself,
// which is what makes this possible and what makes the check necessary.
func (s *stubGroups) reshape(t *testing.T, addr string, threshold, members int) {
	t.Helper()
	office, ok := s.policies[addr]
	require.True(t, ok, "no such office")
	weights := make([]string, members)
	for i := range weights {
		weights[i] = "1"
	}
	office.threshold = fmt.Sprintf("%d", threshold)
	office.weights = weights
}

func (s *stubGroups) GroupPolicyInfo(
	_ context.Context, req *group.QueryGroupPolicyInfoRequest,
) (*group.QueryGroupPolicyInfoResponse, error) {
	s.policyCalls++
	office, ok := s.policies[req.Address]
	if !ok {
		// The shape of a real miss: x/group's query returns an error for an address
		// that has no policy, and that error is all this module looks at.
		return nil, errors.New("no group policy for " + req.Address)
	}
	var decision group.DecisionPolicy = &group.ThresholdDecisionPolicy{
		Threshold: office.threshold,
		Windows:   &group.DecisionPolicyWindows{VotingPeriod: time.Hour},
	}
	if office.percentage != "" {
		decision = &group.PercentageDecisionPolicy{
			Percentage: office.percentage,
			Windows:    &group.DecisionPolicyWindows{VotingPeriod: time.Hour},
		}
	}
	policy, err := codectypes.NewAnyWithValue(decision)
	if err != nil {
		return nil, err
	}
	return &group.QueryGroupPolicyInfoResponse{Info: &group.GroupPolicyInfo{
		Address:        req.Address,
		GroupId:        office.id,
		Admin:          req.Address,
		DecisionPolicy: policy,
	}}, nil
}

func (s *stubGroups) GroupMembers(
	_ context.Context, req *group.QueryGroupMembersRequest,
) (*group.QueryGroupMembersResponse, error) {
	s.memberCalls++
	office, ok := s.byID[req.GroupId]
	if !ok {
		return nil, errors.New("no such group")
	}
	// Refused rather than answered, and this is the assertion that a comment could
	// not make. x/group's member query pages and defaults to a hundred, so a caller
	// that forgets the limit gets a short answer for a large group and no
	// indication that it was short. Failing here means a change that drops the
	// limit fails the shape tests instead of quietly undercounting a big office.
	if req.Pagination == nil || req.Pagination.Limit == 0 {
		return nil, errors.New("asked for a group's members with no page limit")
	}
	members := make([]*group.GroupMember, 0, len(office.weights))
	for i, weight := range office.weights {
		members = append(members, &group.GroupMember{
			GroupId: office.id,
			Member: &group.Member{
				Address: fmt.Sprintf("member-%d-of-group-%d", i, office.id),
				Weight:  weight,
			},
		})
	}
	if uint64(len(members)) > req.Pagination.Limit {
		members = members[:req.Pagination.Limit]
	}
	return &group.QueryGroupMembersResponse{Members: members}, nil
}

// stubConstitution answers the one question x/alias asks x/constitution: which
// address is the foundation.
//
// A pointer receiver over a mutable field, so a test can move the pinned address
// or blank it out mid-run. Blanking it is not a hypothetical shape — it is the
// state a chain would be in if an invariant were somehow unset, and the assertion
// that an unset destination does not compare equal to an unset signer is the one
// this stub exists for.
type stubConstitution struct {
	invariants constitutiontypes.Invariants
	err        error
}

func (s *stubConstitution) GetInvariants(_ context.Context) (constitutiontypes.Invariants, error) {
	if s.err != nil {
		return constitutiontypes.Invariants{}, s.err
	}
	return s.invariants, nil
}

type roleFixture struct {
	env    *integration.Env
	k      keeper.Keeper
	ms     types.MsgServer
	qs     types.QueryServer
	groups *stubGroups
	consti *stubConstitution

	// admin holds the foundation exemption: no jurisdiction, and the reserved
	// code as its country.
	admin string

	// foundation is the address the constitution pins as the recovery
	// destination — the account that may admit a country. A group policy, like
	// the real one, because a role holder has to be one and the foundation is the
	// account granting the role rather than holding it: making it a plain key
	// here would let a test pass that the chain would refuse.
	foundation string
}

func roleSetup(t *testing.T) *roleFixture {
	t.Helper()

	env := integration.New(t, types.ModuleName, module.AppModule{}, groupmodule.AppModuleBasic{})
	groups := newStubGroups()
	consti := &stubConstitution{}

	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, groups, consti)

	f := &roleFixture{
		env:    env,
		k:      k,
		ms:     keeper.NewMsgServerImpl(k),
		qs:     keeper.NewQueryServerImpl(k),
		groups: groups,
		consti: consti,
	}

	_, f.foundation = env.Addr(t)
	// Three of five, like the real one, because the constitution fixes it there.
	groups.add(f.foundation, 3, 5)
	consti.invariants = constitutiontypes.DefaultInvariants()
	consti.invariants.EnforcementRecoveryDestination = f.foundation

	_, f.admin = env.Addr(t)
	gs := types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{administratorGrant(f.admin)}
	require.NoError(t, k.InitGenesis(env.Ctx, *gs))
	return f
}

// office returns a fresh address that the group keeper will vouch for, so it can
// legitimately hold a role.
//
// Three of five, because that is what a ceremony produces and because a fixture
// whose default office was a one-of-one would let a test pass that the chain
// refuses. Tests that care about the shape say so with officeShaped.
func (f *roleFixture) office(t *testing.T) string {
	return f.officeShaped(t, 3, 5)
}

// officeShaped returns a fresh group policy of exactly the given m-of-n.
func (f *roleFixture) officeShaped(t *testing.T, threshold, members int) string {
	t.Helper()
	_, addr := f.env.Addr(t)
	f.groups.add(addr, threshold, members)
	return addr
}

// shape is the requirement a grant records, spelled the way a test reads.
func shape(signatures, members uint32) *types.OfficeShape {
	return &types.OfficeShape{Signatures: signatures, Members: members}
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

// grantRequiring grants with a recorded shape requirement.
func (f *roleFixture) grantRequiring(
	t *testing.T, holder string, role types.Role, scope string, required *types.OfficeShape,
) {
	t.Helper()
	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: holder, Role: role,
		Jurisdiction: scope, RequiredShape: required,
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
// Everybody who is neither governance nor the foundation is refused, and that
// includes the two accounts most likely to look privileged: the holder of the
// role itself, and a foundation *administrator* from this module's own params.
//
// The second is the one worth keeping an eye on. Widening this message to accept
// "the foundation" means one specific account — the one x/constitution pins — and
// not the params list that happens to share the word. If those two ever became
// the same set, the account that may admit a country would be editable by an
// ordinary governance proposal, which is the property the widening was designed
// not to have.
func TestNeitherAHolderNorAnAdministratorMayGrantOrRevokeARole(t *testing.T) {
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

// The foundation may appoint a country's authority, which is the point of the
// widening: enrolling a country is one decision by three of five custodians
// rather than a handful of governance proposals that have to land in order.
func TestTheFoundationMayGrantAndRevokeInsideACountry(t *testing.T) {
	f := roleSetup(t)
	office := f.office(t)
	target := f.placed(t, "SN")

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_PAYMENTS_AUTHORITY, Jurisdiction: "SN",
	})
	require.NoError(t, err)

	// The grant works, and it works only where it was made.
	require.NoError(t, f.k.AssertScope(f.env.Ctx, office, types.ROLE_PAYMENTS_AUTHORITY, target))
	require.ErrorIs(t,
		f.k.AssertScopeIn(f.env.Ctx, office, types.ROLE_PAYMENTS_AUTHORITY, "NG"),
		types.ErrOutOfScope)

	// Attributed to the foundation rather than to governance. "Who says this
	// account may act on Senegalese accounts" is the question asked when the
	// answer turns out to be wrong, and a grant recorded against the wrong
	// authority cannot answer it.
	grants, err := f.k.GrantsOf(f.env.Ctx, office)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, f.foundation, grants[0].GrantedBy)

	// And it can take it back. Symmetric on purpose: an authority that can be
	// appointed with one 3-of-5 vote and needs a governance cycle to remove is an
	// arrangement where the emergency is the expensive case.
	_, err = f.ms.RevokeRole(f.env.Ctx, &types.MsgRevokeRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_PAYMENTS_AUTHORITY, Jurisdiction: "SN",
	})
	require.NoError(t, err)
	require.ErrorIs(t, f.k.AssertScope(f.env.Ctx, office, types.ROLE_PAYMENTS_AUTHORITY, target),
		types.ErrOutOfScope)
}

// The foundation admitting a country and the foundation manufacturing authority
// over every country are different acts, and only the first one was decided.
func TestTheFoundationCannotReachTheChainWideScope(t *testing.T) {
	f := roleSetup(t)
	office := f.office(t)

	for _, spelling := range []string{types.ChainWide} {
		_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
			Authority: f.foundation, Holder: office,
			Role: types.ROLE_SUPERVISOR, Jurisdiction: spelling,
		})
		require.ErrorIs(t, err, types.ErrInvalidSigner)
	}

	// Nor may it revoke one. A chain-wide grant that the foundation could remove
	// would be a chain-wide grant the foundation controls.
	f.grant(t, office, types.ROLE_SUPERVISOR, types.ChainWide)
	_, err := f.ms.RevokeRole(f.env.Ctx, &types.MsgRevokeRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_SUPERVISOR, Jurisdiction: types.ChainWide,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
	require.NoError(t, f.k.AssertScopeIn(f.env.Ctx, office, types.ROLE_SUPERVISOR, "KE"))
}

// The chain-wide refusal does not depend on the constitution being readable.
//
// Ordering, asserted rather than assumed. An implementation that resolved the
// foundation first and checked the scope afterwards would behave identically on a
// healthy chain and would let a store failure decide whether "*" was allowed.
func TestTheChainWideRefusalDoesNotDependOnTheConstitution(t *testing.T) {
	f := roleSetup(t)
	office := f.office(t)
	f.consti.err = errors.New("the constitution could not be read")

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_SUPERVISOR, Jurisdiction: types.ChainWide,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
	require.Contains(t, err.Error(), types.ChainWide,
		"the refusal should name the scope it refused, not the constitution it never read")
}

// An unset recovery destination does not make an unset signer the foundation.
//
// This is the proto3 zero-value trap in a string costume, and this repository has
// been caught by the numeric version of it four times. If foundationAddress
// returned "" for an unwritten invariant, then a message whose authority field
// nobody filled in would compare equal to it and every grant would be permitted
// by the absence of a constitution.
func TestAnUnsetFoundationIsNobodyRatherThanEverybody(t *testing.T) {
	f := roleSetup(t)
	office := f.office(t)
	f.consti.invariants.EnforcementRecoveryDestination = ""

	for _, signer := range []string{"", f.foundation} {
		_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
			Authority: signer, Holder: office,
			Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "SN",
		})
		require.ErrorIsf(t, err, types.ErrInvalidSigner,
			"an unwritten recovery destination admitted %q", signer)
		// The message, not just the error, and a mutation pass is why. An empty
		// destination is caught twice — once as an unset invariant and once as an
		// undecodable address — so removing the first guard changes no outcome and
		// no test noticed. What it changes is what an operator is told: "there is
		// no enforcement_recovery_destination" sends them to the constitution,
		// where "not an address this chain can read" sends them hunting a typo
		// that does not exist.
		require.Containsf(t, err.Error(), "names no enforcement_recovery_destination",
			"the refusal for %q should say the constitution names no destination", signer)
	}

	// Governance is unaffected: the closed state is the rule this message had
	// before the widening, not a chain where nobody can grant anything.
	f.grant(t, office, types.ROLE_ENFORCEMENT_AUTHORITY, "SN")
}

// A constitution that cannot be read is a refusal, never an acceptance.
func TestAnUnreadableConstitutionRefusesTheFoundation(t *testing.T) {
	f := roleSetup(t)
	office := f.office(t)
	f.consti.err = errors.New("store failure")

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_MONETARY_AUTHORITY, Jurisdiction: "SN",
	})
	require.Error(t, err)
	require.ErrorIs(t,
		f.k.AssertScopeIn(f.env.Ctx, office, types.ROLE_MONETARY_AUTHORITY, "SN"),
		types.ErrOutOfScope, "a refused grant left a row behind")
}

// With no constitution keeper wired at all, governance is the only signer.
//
// Not skipped, which is what the nil group keeper does. The difference is which
// way the bypass runs: a missing group keeper can only cause a grant that should
// have been refused, and a missing constitution keeper that resolved to the empty
// string would make every signer the foundation.
func TestWithoutAConstitutionKeeperOnlyGovernanceMayGrant(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{}, groupmodule.AppModuleBasic{})
	groups := newStubGroups()
	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, groups, nil)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))
	ms := keeper.NewMsgServerImpl(k)

	_, office := env.Addr(t)
	groups.add(office, 3, 5)
	_, stranger := env.Addr(t)

	_, err := ms.GrantRole(env.Ctx, &types.MsgGrantRole{
		Authority: stranger, Holder: office,
		Role: types.ROLE_REGISTRY_AUTHORITY, Jurisdiction: "SN",
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	_, err = ms.GrantRole(env.Ctx, &types.MsgGrantRole{
		Authority: env.AuthorityString(t), Holder: office,
		Role: types.ROLE_REGISTRY_AUTHORITY, Jurisdiction: "SN",
	})
	require.NoError(t, err)
}

// The foundation is resolved on every message, never cached.
//
// A cached authority is an authority that outlives the amendment that changed it.
// Moving the pinned address has to take the power away from the old holder in the
// very next message, because the reason to move it is usually that the old one
// should no longer have it.
func TestMovingThePinnedAddressMovesTheAuthority(t *testing.T) {
	f := roleSetup(t)
	office := f.office(t)
	old := f.foundation

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: old, Holder: office,
		Role: types.ROLE_SUPERVISOR, Jurisdiction: "SN",
	})
	require.NoError(t, err)

	_, replacement := f.env.Addr(t)
	f.consti.invariants.EnforcementRecoveryDestination = replacement

	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: old, Holder: office,
		Role: types.ROLE_SUPERVISOR, Jurisdiction: "NG",
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: replacement, Holder: office,
		Role: types.ROLE_SUPERVISOR, Jurisdiction: "NG",
	})
	require.NoError(t, err)
}

// The foundation may revoke a country grant that GOVERNANCE made, and may re-make
// one, rewriting the attribution to itself.
//
// Both of those follow from the signers being symmetric, both are surprising, and
// an untested surprise is worse than the surprise. So they are written down here
// rather than discovered.
//
// The revocation is the one worth thinking about, because it is a reduction of
// governance's power: an authority the validator set deliberately installed can be
// removed by three custodians. It is kept because the alternative — letting the
// foundation revoke only what it granted — means an office installed by governance
// whose keys are compromised cannot be stopped without a governance cycle, and the
// whole reason revocation is not governance-only is that the emergency must not be
// the expensive case. The foundation cannot reach a state governance could not: it
// can grant the same country grant straight back, and it can never touch "*".
//
// The re-grant rewriting granted_by is a consequence of granting the same triple
// being idempotent, which is deliberate elsewhere (a proposal resubmitted after a
// timeout must not fail). What it costs is that granted_by names the last authority
// to write the grant rather than the first, and the events carry the history.
func TestTheFoundationCanUndoAndRewriteAGovernanceGrant(t *testing.T) {
	f := roleSetup(t)
	office := f.office(t)
	target := f.placed(t, "SN")

	// Governance grants it.
	f.grant(t, office, types.ROLE_REGISTRY_AUTHORITY, "SN")
	grants, err := f.k.GrantsOf(f.env.Ctx, office)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, f.env.AuthorityString(t), grants[0].GrantedBy)

	// The foundation re-grants the same triple. The grant is unchanged except that
	// it is now attributed to the foundation.
	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_REGISTRY_AUTHORITY, Jurisdiction: "SN",
	})
	require.NoError(t, err)
	grants, err = f.k.GrantsOf(f.env.Ctx, office)
	require.NoError(t, err)
	require.Len(t, grants, 1, "re-granting the same triple must not create a second row")
	require.Equal(t, f.foundation, grants[0].GrantedBy)
	require.NoError(t, f.k.AssertScope(f.env.Ctx, office, types.ROLE_REGISTRY_AUTHORITY, target))

	// And the foundation can take it away.
	_, err = f.ms.RevokeRole(f.env.Ctx, &types.MsgRevokeRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_REGISTRY_AUTHORITY, Jurisdiction: "SN",
	})
	require.NoError(t, err)
	require.ErrorIs(t, f.k.AssertScope(f.env.Ctx, office, types.ROLE_REGISTRY_AUTHORITY, target),
		types.ErrOutOfScope)

	// What it cannot do, in either direction, is the chain-wide scope — so there is
	// no state reachable this way that governance could not have reached.
	f.grant(t, office, types.ROLE_SUPERVISOR, types.ChainWide)
	_, err = f.ms.RevokeRole(f.env.Ctx, &types.MsgRevokeRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_SUPERVISOR, Jurisdiction: types.ChainWide,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}

// The foundation can grant itself a country-scoped role, and the chain permits it
// on purpose rather than by omission.
//
// It looks alarming and it is bounded. The foundation already holds the account
// every seized asset is sent to and can appoint any office it likes, so a
// self-grant confers nothing it could not confer on a group it controls. What it
// buys is a real case: a country admitted before its offices exist, where the
// foundation acts as the interim authority until there is somebody to hand it to.
// Refusing holder == authority would block that, and would not prevent the thing
// it looks like it prevents.
//
// The chain-wide scope is still closed, which is the boundary that matters, and
// `ceremony country` refuses to COMPOSE a self-grant — an office whose members
// include the foundation is refused at init, and an office confirmed at the
// foundation's own address is refused at confirm. So reaching this state takes a
// hand-written proposal rather than a mistake.
func TestTheFoundationMayHoldACountryRoleItself(t *testing.T) {
	f := roleSetup(t)
	target := f.placed(t, "SN")

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: f.foundation,
		Role: types.ROLE_PAYMENTS_AUTHORITY, Jurisdiction: "SN",
	})
	require.NoError(t, err)
	require.NoError(t, f.k.AssertScope(f.env.Ctx, f.foundation, types.ROLE_PAYMENTS_AUTHORITY, target))

	// And still not everywhere.
	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: f.foundation,
		Role: types.ROLE_PAYMENTS_AUTHORITY, Jurisdiction: types.ChainWide,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	ng := f.placed(t, "NG")
	require.ErrorIs(t, f.k.AssertScope(f.env.Ctx, f.foundation, types.ROLE_PAYMENTS_AUTHORITY, ng),
		types.ErrOutOfScope)
}

// Everything else the message refuses, it still refuses for the foundation.
//
// The widening is about who may sign, and nothing else. A separate signer path
// that skipped the group check or the role check would be a second, weaker
// implementation of the same message.
func TestTheFoundationIsBoundByEveryOtherRule(t *testing.T) {
	f := roleSetup(t)
	_, plainKey := f.env.Addr(t)
	office := f.office(t)

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: plainKey,
		Role: types.ROLE_PAYMENTS_AUTHORITY, Jurisdiction: "SN",
	})
	require.ErrorIs(t, err, types.ErrHolderNotGroup)

	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_UNSPECIFIED, Jurisdiction: "SN",
	})
	require.ErrorIs(t, err, types.ErrInvalidRole)

	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: office,
		Role: types.ROLE_PAYMENTS_AUTHORITY, Jurisdiction: "ZZ",
	})
	require.ErrorIs(t, err, types.ErrInvalidScope,
		"the foundation's reserved code is a grant over nowhere that reads as a grant over everywhere")

	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.foundation, Holder: "not-an-address",
		Role: types.ROLE_PAYMENTS_AUTHORITY, Jurisdiction: "SN",
	})
	require.Error(t, err)
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
	// Three, not two: the fixture's foundation administrator is itself a
	// chain-wide grant now, and it belongs on this page for exactly the reason
	// the page exists. An account exempt from every national perimeter that could
	// only be found by reading a parameter was an exception nobody audited.
	require.Len(t, res.Grants, 3)
	for _, g := range res.Grants {
		require.Equal(t, types.ChainWide, g.Jurisdiction)
		if g.Role == types.ROLE_FOUNDATION_ADMINISTRATOR {
			require.Equal(t, f.admin, g.Holder)
			continue
		}
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
	// Nine: eight granted above plus the fixture's foundation administrator,
	// which is a role grant like any other and has to survive an export like one.
	require.Len(t, exported.RoleGrants, 9)

	// A second environment, so the import lands in an empty store.
	//
	// Reusing the first would leave the derived index already populated by the
	// messages above, and every assertion below would pass whether InitGenesis
	// rebuilt it or not — which is exactly what they exist to check.
	other := integration.New(t, types.ModuleName, module.AppModule{})
	fresh := keeper.NewKeeper(other.Codec, other.AddressCodec, other.StoreService,
		log.NewNopLogger(), other.AuthorityString(t), nil, nil, nil)
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
	// Three: the two the loop granted at the chain-wide scope, plus the fixture's
	// foundation administrator, which is a chain-wide grant like any other.
	require.Equal(t, 3, wide, "the fixture stopped exercising the chain-wide scope")

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
		log.NewNopLogger(), env.AuthorityString(t), nil, nil, nil)

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
