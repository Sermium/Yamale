package keeper_test

import (
	"testing"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/alias/keeper"
	module "yamale/blockchain/x/alias/module"
	"yamale/blockchain/x/alias/types"
)

// The tests for the office's shape: the M-of-N a grant records, and the rule that
// every check holds the holder to it.
//
// Read them as one statement. Individually they are a list of refusals; together
// they say that an office cannot keep an authority it has stopped being able to
// exercise M-of-N, and that it cannot get one it never could. The failure they
// exist to rule out is the one the module shipped with: a check that asked whether
// the holder *was* a group, answered yes for a one-of-one, and was never asked
// again.

// The headline, and the sequence a live devnet reproduces: an office acts, votes
// itself smaller, is refused, restores itself and acts again.
func TestAnOfficeThatVotesItselfSmallerLosesItsAuthority(t *testing.T) {
	f := roleSetup(t)
	enforcement := f.officeShaped(t, 3, 5)
	f.grantRequiring(t, enforcement, types.ROLE_ENFORCEMENT_AUTHORITY, "GH", shape(3, 5))
	target := f.placed(t, "GH")

	require.NoError(t,
		f.k.AssertScope(f.env.Ctx, enforcement, types.ROLE_ENFORCEMENT_AUTHORITY, target),
		"a three-of-five office holding a three-of-five grant must be able to act")

	// The office votes on itself. Nothing about this is exotic: an office is its
	// own admin, which is what makes its membership changeable by its members and
	// by nobody else.
	f.groups.reshape(t, enforcement, 1, 5)

	err := f.k.AssertScope(f.env.Ctx, enforcement, types.ROLE_ENFORCEMENT_AUTHORITY, target)
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "3-of-5", "the refusal must name the shape that was required")
	require.ErrorContains(t, err, "1-of-5", "and the shape the office actually is")
	// The office can fix this itself, so the refusal says how. A message that
	// only refused would send somebody to the foundation for a proposal the
	// office can pass on its own.
	require.ErrorContains(t, err, "voted by the office itself")

	// Nothing was revoked. The grant is still in the store, still correct in every
	// query, and permits nothing — which is the point: the authority went away
	// without anybody having to notice.
	grants, err := f.k.GrantsOf(f.env.Ctx, enforcement)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, "3-of-5", grants[0].RequiredShape.Rule())

	// And it comes back on its own when the office is restored, with no re-grant.
	f.groups.reshape(t, enforcement, 3, 5)
	require.NoError(t,
		f.k.AssertScope(f.env.Ctx, enforcement, types.ROLE_ENFORCEMENT_AUTHORITY, target))
}

// Growth is fine and shrinkage is not, which is the asymmetry the whole design
// turns on.
func TestAnOfficeMayGrowAndMayNotShrink(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)
	f.grantRequiring(t, office, types.ROLE_PAYMENTS_AUTHORITY, "SN", shape(3, 5))
	target := f.placed(t, "SN")

	for _, tc := range []struct {
		name              string
		threshold, member int
		permitted         bool
	}{
		{"three of six: a member joined", 3, 6, true},
		{"four of six: and the office tightened", 4, 6, true},
		{"five of five: unanimity is self-harm, not capture", 5, 5, true},
		{"three of four: a member left and was not replaced", 3, 4, false},
		{"two of five: the office lowered its own bar", 2, 5, false},
		{"one of one: the single key this exists to abolish", 1, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.groups.reshape(t, office, tc.threshold, tc.member)
			err := f.k.AssertScope(f.env.Ctx, office, types.ROLE_PAYMENTS_AUTHORITY, target)
			if tc.permitted {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, types.ErrOfficeShape)
		})
	}
}

// A one-of-one is a group policy, which is why the old check passed it.
func TestAOneOfOneOfficeIsRefusedAGrantRequiringMore(t *testing.T) {
	f := roleSetup(t)
	single := f.officeShaped(t, 1, 1)

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: single,
		Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "GH",
		RequiredShape: shape(3, 5),
	})
	require.ErrorIs(t, err, types.ErrOfficeShape,
		"a grant requiring three-of-five must not be written against a one-of-one")
	require.ErrorContains(t, err, "1-of-1")
	require.ErrorContains(t, err, "cannot be made to")
	// And it must not describe a grant that was never made. A live devnet run
	// found the version of this message that said the office "was granted its
	// authority as 3-of-5 and is now 1-of-1", which is a sentence about history
	// that did not happen and sends the reader looking for a grant to repair.
	require.NotContains(t, err.Error(), "was granted its authority",
		"a refusal to MAKE a grant must not read as a report about one that exists")

	// And nothing was written. A refused grant that left a row behind would be an
	// authority nobody granted.
	grants, err := f.k.GrantsOf(f.env.Ctx, single)
	require.NoError(t, err)
	require.Empty(t, grants)

	// The same office can still be granted a role with no requirement, which is
	// the pre-existing behaviour and is unchanged on purpose. Closing that would
	// be a different decision with a different blast radius — see the runbook.
	f.grant(t, single, types.ROLE_SUPERVISOR, "GH")
}

// The case a threshold number alone cannot see: five members, a threshold of
// three, and one member who reaches it alone.
func TestAWeightedGroupIsCountedInPeopleAndNotInWeight(t *testing.T) {
	f := roleSetup(t)
	_, office := f.env.Addr(t)
	// Threshold 3 over weights 3, 1, 1, 1, 1. Reads as a three-of-five in every
	// query x/group offers; the first member signs alone.
	f.groups.addWeighted(office, "3", []string{"3", "1", "1", "1", "1"})

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: office,
		Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "GH",
		RequiredShape: shape(3, 5),
	})
	require.ErrorIs(t, err, types.ErrOfficeShape,
		"a group whose first member reaches the threshold alone is a one-of-five")
	require.ErrorContains(t, err, "1-of-5")

	// Two members weighing two each, threshold three: it takes both, so it is a
	// two-of-two and satisfies a required two-of-two.
	_, pair := f.env.Addr(t)
	f.groups.addWeighted(pair, "3", []string{"2", "2"})
	f.grantRequiring(t, pair, types.ROLE_SUPERVISOR, "GH", shape(2, 2))
}

// A member who cannot vote is not a share of an office.
func TestWeightlessMembersDoNotCountTowardsTheMemberFloor(t *testing.T) {
	f := roleSetup(t)
	_, padded := f.env.Addr(t)
	// Three voting members and two passengers. x/group removes a member whose
	// weight is set to zero, so this is the defensive case rather than the
	// expected one — and it must not satisfy a five-member floor.
	f.groups.addWeighted(padded, "3", []string{"1", "1", "1", "0", "0"})

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: padded,
		Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "GH",
		RequiredShape: shape(3, 5),
	})
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "3-of-3", "the weightless members must not be counted")
}

// A percentage policy is refused rather than converted.
func TestAPercentageDecisionPolicyCannotHoldARecordedShape(t *testing.T) {
	f := roleSetup(t)
	_, office := f.env.Addr(t)
	office60 := f.groups.addWeighted(office, "3", []string{"1", "1", "1", "1", "1"})
	office60.percentage = "0.6"

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: office,
		Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "GH",
		RequiredShape: shape(3, 5),
	})
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "percentage",
		"the refusal must say why a percentage cannot carry a fixed requirement")

	// Without a requirement it is still an ordinary group and may hold a role. The
	// refusal is about holding a *fixed shape* against a policy that moves, not
	// about percentage policies being illegitimate.
	f.grant(t, office, types.ROLE_SUPERVISOR, "GH")
}

// An office whose threshold nobody can reach is frozen, and says so.
func TestAnOfficeThatCannotActAtAllIsRefusedAndNamedAsSuch(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)
	f.grantRequiring(t, office, types.ROLE_ENFORCEMENT_AUTHORITY, "GH", shape(3, 5))
	target := f.placed(t, "GH")

	// Six signatures required from five members. x/group permits this and the
	// office is inert; the refusal has to send an operator to the group rather
	// than to the grant.
	f.groups.reshape(t, office, 6, 5)
	err := f.k.AssertScope(f.env.Ctx, office, types.ROLE_ENFORCEMENT_AUTHORITY, target)
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "no set of members can act at all")
}

// A threshold of zero is refused rather than read as "one signature".
//
// x/group's own ValidateBasic refuses it, so this is a state the chain should
// never hold — which is exactly why it is a refusal here and not an arithmetic
// case. A threshold of zero passes the first vote cast, so reading it as a
// one-of-five would hand a five-member office to whichever member votes first,
// and the shape check would report that as healthy. Found as a mutation survivor:
// deleting the branch broke no test.
func TestAThresholdOfZeroIsRefusedRatherThanCountedAsOne(t *testing.T) {
	f := roleSetup(t)
	_, office := f.env.Addr(t)
	f.groups.addWeighted(office, "0", []string{"1", "1", "1", "1", "1"})

	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority: f.env.AuthorityString(t), Holder: office,
		Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "GH",
		RequiredShape: shape(1, 5),
	})
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "any single member acts alone")
}

// The grant event carries the shape, so a history can be reconstructed from the
// events rather than only from the store.
//
// Also found as a mutation survivor: removing the attribute broke no test, which
// meant the audit trail of "when did this office stop being pinned to a
// three-of-five" was unasserted.
func TestTheGrantEventNamesTheRequiredShape(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)
	f.grantRequiring(t, office, types.ROLE_PAYMENTS_AUTHORITY, "SN", shape(3, 5))

	found := ""
	for _, event := range f.env.Ctx.EventManager().Events() {
		if event.Type != "role_granted" {
			continue
		}
		for _, attr := range event.Attributes {
			if attr.Key == "required_shape" {
				found = attr.Value
			}
		}
	}
	require.Equal(t, "3-of-5", found, "the event must record what the office was pinned to")

	// And a grant with no requirement says so in words rather than leaving a blank
	// an indexer has to interpret.
	other := f.officeShaped(t, 3, 5)
	f.grant(t, other, types.ROLE_SUPERVISOR, "SN")
	last := ""
	for _, event := range f.env.Ctx.EventManager().Events() {
		if event.Type != "role_granted" {
			continue
		}
		for _, attr := range event.Attributes {
			if attr.Key == "required_shape" {
				last = attr.Value
			}
		}
	}
	require.Equal(t, "no required shape", last)
}

// A group too large to read a shape from is a refusal that names the cap.
func TestAnOfficeLargerThanTheModuleCanPageIsRefusedRatherThanUndercounted(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)
	f.grantRequiring(t, office, types.ROLE_SUPERVISOR, "GH", shape(3, 5))
	target := f.placed(t, "GH")

	f.groups.reshape(t, office, 3, types.MaxOfficeMembers+1)
	err := f.k.AssertScope(f.env.Ctx, office, types.ROLE_SUPERVISOR, target)
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "refusing rather than counting a page of it")

	// One below the cap is read normally, so the boundary is where it says it is
	// rather than one out — an off-by-one here would refuse every legitimate large
	// commission.
	f.groups.reshape(t, office, 3, types.MaxOfficeMembers)
	require.NoError(t, f.k.AssertScope(f.env.Ctx, office, types.ROLE_SUPERVISOR, target))
}

// A holder that stops being a group policy at all is refused, not defaulted.
func TestAHolderThatIsNoLongerAGroupPolicyIsRefused(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)
	f.grantRequiring(t, office, types.ROLE_ENFORCEMENT_AUTHORITY, "GH", shape(3, 5))
	target := f.placed(t, "GH")

	delete(f.groups.policies, office)
	err := f.k.AssertScope(f.env.Ctx, office, types.ROLE_ENFORCEMENT_AUTHORITY, target)
	require.ErrorIs(t, err, types.ErrHolderNotGroup)
}

// The jurisdiction-in-the-message path holds the shape too.
//
// Both entry points or neither: a rule enforced on one of two doors is a rule with
// a door.
func TestAssertScopeInHoldsTheShapeAsWell(t *testing.T) {
	f := roleSetup(t)
	lands := f.officeShaped(t, 2, 3)
	f.grantRequiring(t, lands, types.ROLE_REGISTRY_AUTHORITY, "SN", shape(2, 3))

	require.NoError(t, f.k.AssertScopeIn(f.env.Ctx, lands, types.ROLE_REGISTRY_AUTHORITY, "SN"))

	f.groups.reshape(t, lands, 1, 3)
	err := f.k.AssertScopeIn(f.env.Ctx, lands, types.ROLE_REGISTRY_AUTHORITY, "SN")
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "2-of-3")
}

// A grant that records no requirement behaves exactly as it did before, and costs
// exactly what it did before.
//
// This is the decision about existing grants, written as a test rather than as a
// paragraph: absent means no requirement, absence is a nil pointer and not a zero,
// and the office of an unrequirement-carrying grant is never even looked up.
func TestAGrantWithNoRecordedShapeIsUnchangedAndFree(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)
	f.grant(t, office, types.ROLE_ENFORCEMENT_AUTHORITY, "GH")
	target := f.placed(t, "GH")

	grants, err := f.k.GrantsOf(f.env.Ctx, office)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Nil(t, grants[0].RequiredShape,
		"absent must decode as nil, never as a zero-valued requirement")
	require.Equal(t, "no required shape", grants[0].RequiredShape.Rule())

	before := f.groups.policyCalls + f.groups.memberCalls
	require.NoError(t,
		f.k.AssertScope(f.env.Ctx, office, types.ROLE_ENFORCEMENT_AUTHORITY, target))
	require.Equal(t, before, f.groups.policyCalls+f.groups.memberCalls,
		"a grant with no recorded shape must not ask x/group anything")

	// And the office can shrink to a single key without losing the authority,
	// which is the honest cost of treating absence as "no requirement". The fix
	// for an existing grant is to re-grant it with one.
	f.groups.reshape(t, office, 1, 1)
	require.NoError(t,
		f.k.AssertScope(f.env.Ctx, office, types.ROLE_ENFORCEMENT_AUTHORITY, target))
}

// One shape check is two queries, and that is the whole cost.
func TestTheShapeCheckCostsTwoKeeperQueries(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)
	f.grantRequiring(t, office, types.ROLE_ENFORCEMENT_AUTHORITY, "GH", shape(3, 5))
	target := f.placed(t, "GH")

	policies, members := f.groups.policyCalls, f.groups.memberCalls
	require.NoError(t,
		f.k.AssertScope(f.env.Ctx, office, types.ROLE_ENFORCEMENT_AUTHORITY, target))
	require.Equal(t, 1, f.groups.policyCalls-policies)
	require.Equal(t, 1, f.groups.memberCalls-members)
}

// A requirement that requires nothing is refused, and so is one nothing could
// satisfy.
func TestAnIncoherentRequirementIsRefused(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)

	for _, tc := range []struct {
		name     string
		required *types.OfficeShape
		says     string
	}{
		{"zero signatures", shape(0, 5), "omit required_shape entirely"},
		{"zero of zero", shape(0, 0), "omit required_shape entirely"},
		{"more signatures than members", shape(3, 2), "no office could ever satisfy"},
		{"beyond what can be read", shape(3, types.MaxOfficeMembers+1), "can read a group's shape from"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
				Authority: f.env.AuthorityString(t), Holder: office,
				Role: types.ROLE_SUPERVISOR, Jurisdiction: "GH",
				RequiredShape: tc.required,
			})
			require.ErrorIs(t, err, types.ErrInvalidRole)
			require.ErrorContains(t, err, tc.says)
		})
	}
}

// A re-grant ratchets: it may raise a requirement and may not lower or drop one.
//
// The failure this rules out is not hypothetical. A proposal resubmitted after a
// timeout is composed from the summary rather than from the stored grant, and the
// summary does not mention required_shape — so the obvious resubmission would
// remove the pin while claiming to change nothing.
func TestARegrantMayRaiseTheRequirementAndMayNotDropIt(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)
	f.grantRequiring(t, office, types.ROLE_ENFORCEMENT_AUTHORITY, "GH", shape(3, 5))

	regrant := func(required *types.OfficeShape) error {
		_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
			Authority: f.env.AuthorityString(t), Holder: office,
			Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "GH",
			RequiredShape: required,
		})
		return err
	}

	// The idempotent resubmission, unchanged: same triple, same requirement.
	require.NoError(t, regrant(shape(3, 5)))

	err := regrant(nil)
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "Omitting")
	require.ErrorContains(t, err, "revoke the grant and make a new one")

	err = regrant(shape(2, 5))
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "would reduce it to 2-of-5")

	err = regrant(shape(3, 4))
	require.ErrorIs(t, err, types.ErrOfficeShape,
		"lowering the member floor is a reduction even with the threshold intact")

	// Raising is ordinary, once the office is big enough to meet it.
	require.ErrorIs(t, regrant(shape(4, 6)), types.ErrOfficeShape,
		"and the raised requirement still has to be one the office meets today")
	f.groups.reshape(t, office, 4, 6)
	require.NoError(t, regrant(shape(4, 6)))

	// Revoke-then-grant is the deliberate way down, and it is one proposal on a
	// real chain because x/group executes a proposal's messages together.
	_, err = f.ms.RevokeRole(f.env.Ctx, &types.MsgRevokeRole{
		Authority: f.env.AuthorityString(t), Holder: office,
		Role: types.ROLE_ENFORCEMENT_AUTHORITY, Jurisdiction: "GH",
	})
	require.NoError(t, err)
	require.NoError(t, regrant(shape(2, 3)))
}

// A chain-wide grant whose office has fallen does not cancel a country grant that
// is still good.
func TestAFallenChainWideGrantDoesNotHideAGoodCountryGrant(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 5, 9)
	f.grantRequiring(t, office, types.ROLE_ENFORCEMENT_AUTHORITY, types.ChainWide, shape(5, 9))
	f.grantRequiring(t, office, types.ROLE_ENFORCEMENT_AUTHORITY, "GH", shape(2, 3))
	target := f.placed(t, "GH")

	require.NoError(t,
		f.k.AssertScope(f.env.Ctx, office, types.ROLE_ENFORCEMENT_AUTHORITY, target))

	// The office drops to a three, which its chain-wide grant does not allow and
	// its Ghanaian grant does. It keeps Ghana and loses everywhere else.
	f.groups.reshape(t, office, 2, 3)
	require.NoError(t,
		f.k.AssertScope(f.env.Ctx, office, types.ROLE_ENFORCEMENT_AUTHORITY, target),
		"the country grant is still satisfied and still permits the action")

	elsewhere := f.placed(t, "KE")
	err := f.k.AssertScope(f.env.Ctx, office, types.ROLE_ENFORCEMENT_AUTHORITY, elsewhere)
	require.ErrorIs(t, err, types.ErrOfficeShape,
		"and the chain-wide grant it has fallen below is refused rather than reported missing")
	require.ErrorContains(t, err, "5-of-9")
}

// HoldsRole answers "is this an authority of this kind", which a fallen office
// still is.
//
// It exists for error legibility and permits nothing, so it stays shape-blind on
// purpose: the alternative costs two x/group queries on a path that only chooses
// an error message, and it would report a fallen office as "not a payments
// authority", which sends the reader looking for the wrong bug.
func TestHoldsRoleIsDeliberatelyBlindToTheShape(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)
	f.grantRequiring(t, office, types.ROLE_PAYMENTS_AUTHORITY, "SN", shape(3, 5))
	f.groups.reshape(t, office, 1, 1)

	held, err := f.k.HoldsRole(f.env.Ctx, office, types.ROLE_PAYMENTS_AUTHORITY)
	require.NoError(t, err)
	require.True(t, held)

	require.ErrorIs(t,
		f.k.AssertScopeIn(f.env.Ctx, office, types.ROLE_PAYMENTS_AUTHORITY, "SN"),
		types.ErrOfficeShape, "and the check that permits things says the true thing")
}

// With no group keeper wired, a requirement can neither be recorded nor honoured.
//
// The opposite of what assertGroupAccount does on the same nil, and the asymmetry
// is the decision: a missing group keeper there can only produce a grant that
// should have been refused, and the perimeter path refuses to act on it. Here it
// would produce an ACTION by an office whose shape nobody read.
func TestWithoutAGroupKeeperAShapeCanBeNeitherRecordedNorHonoured(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, nil, nil)
	ms := keeper.NewMsgServerImpl(k)

	_, office := env.Addr(t)
	_, target := env.Addr(t)

	// A grant seeded at genesis, because that is the only way a requirement can
	// reach the store on a chain that cannot check one: InitGenesis validates the
	// record and writes it, and it deliberately does not query x/group — the
	// module's genesis runs before there is any guarantee x/group's has.
	gs := types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{{
		Holder:        office,
		Role:          types.ROLE_ENFORCEMENT_AUTHORITY,
		Jurisdiction:  "GH",
		GrantedBy:     env.AuthorityString(t),
		RequiredShape: &types.OfficeShape{Signatures: 3, Members: 5},
	}}
	gs.Jurisdictions = append(gs.Jurisdictions, types.Jurisdiction{Address: target, Country: "GH"})
	require.NoError(t, k.InitGenesis(env.Ctx, *gs))

	err := k.AssertScope(env.Ctx, office, types.ROLE_ENFORCEMENT_AUTHORITY, target)
	require.ErrorIs(t, err, types.ErrOfficeShape,
		"a requirement that cannot be checked must not be assumed to hold")
	require.ErrorContains(t, err, "x/group is not wired in")

	// And a fresh grant carrying one is refused rather than written unverified.
	_, err = ms.GrantRole(env.Ctx, &types.MsgGrantRole{
		Authority: env.AuthorityString(t), Holder: office,
		Role: types.ROLE_SUPERVISOR, Jurisdiction: "GH",
		RequiredShape: &types.OfficeShape{Signatures: 2, Members: 3},
	})
	require.ErrorIs(t, err, types.ErrOfficeShape)

	// A grant with no requirement is unaffected, which is what keeps every
	// consuming module's fixture working.
	_, err = ms.GrantRole(env.Ctx, &types.MsgGrantRole{
		Authority: env.AuthorityString(t), Holder: office,
		Role: types.ROLE_SUPERVISOR, Jurisdiction: "GH",
	})
	require.NoError(t, err)
}

// A requirement seeded at genesis is checked on use, so a genesis file cannot
// walk around the rule.
func TestAGenesisSeededRequirementIsHeldToOnUse(t *testing.T) {
	f := roleSetup(t)
	single := f.officeShaped(t, 1, 1)
	target := f.placed(t, "GH")

	// Written directly into genesis, which is the one route that does not pass
	// through GrantRole's check that the holder meets the requirement today.
	gs := types.DefaultGenesis()
	gs.Params = types.NewParams(types.PayloadLength, f.admin)
	gs.RoleGrants = []types.RoleGrant{{
		Holder:        single,
		Role:          types.ROLE_ENFORCEMENT_AUTHORITY,
		Jurisdiction:  "GH",
		GrantedBy:     f.env.AuthorityString(t),
		RequiredShape: shape(3, 5),
	}}
	require.NoError(t, f.k.InitGenesis(f.env.Ctx, *gs))

	err := f.k.AssertScope(f.env.Ctx, single, types.ROLE_ENFORCEMENT_AUTHORITY, target)
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "1-of-1")
}

// Genesis validation refuses an incoherent requirement, by the same function the
// handler uses.
func TestGenesisRefusesAnIncoherentRequirement(t *testing.T) {
	f := roleSetup(t)
	office := f.officeShaped(t, 3, 5)

	gs := types.DefaultGenesis()
	gs.Params = types.NewParams(types.PayloadLength, f.admin)
	gs.RoleGrants = []types.RoleGrant{{
		Holder:        office,
		Role:          types.ROLE_SUPERVISOR,
		Jurisdiction:  "GH",
		GrantedBy:     f.env.AuthorityString(t),
		RequiredShape: shape(4, 2),
	}}
	require.Error(t, gs.Validate())
	require.ErrorContains(t, gs.Validate(), "no office could ever satisfy")
}

// The requirement survives an export and an import byte for byte.
//
// A grant with no requirement must export with the field ABSENT rather than
// zeroed, because a derived state that stores something where genesis wrote
// nothing is how an import stops matching an export.
func TestGenesisRoundTripsARequiredShape(t *testing.T) {
	f := roleSetup(t)
	pinned := f.officeShaped(t, 2, 3)
	unpinned := f.officeShaped(t, 2, 3)
	f.grantRequiring(t, pinned, types.ROLE_REGISTRY_AUTHORITY, "SN", shape(2, 3))
	f.grant(t, unpinned, types.ROLE_SUPERVISOR, "SN")

	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())

	first, err := f.env.Codec.Marshal(exported)
	require.NoError(t, err)

	// Import into a fresh store and export again. Equal bytes or the upgrade path
	// is broken, whatever the fields look like when read one at a time.
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, f.groups, f.consti)
	require.NoError(t, k.InitGenesis(env.Ctx, *exported))
	reexported, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	second, err := env.Codec.Marshal(reexported)
	require.NoError(t, err)
	require.Equal(t, first, second)

	for _, g := range exported.RoleGrants {
		switch g.Holder {
		case pinned:
			require.Equal(t, "2-of-3", g.RequiredShape.Rule())
		case unpinned:
			require.Nil(t, g.RequiredShape)
		}
	}
}
