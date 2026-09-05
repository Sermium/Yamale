package keeper_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	aliastypes "yamale/blockchain/x/alias/types"
	constitutiontestutil "yamale/blockchain/x/constitution/testutil"
	"yamale/blockchain/x/enforcement/keeper"
	"yamale/blockchain/x/enforcement/types"
)

// A module wired without the perimeter registry refuses everything it would
// otherwise have to guess about.
//
// This is the state a wiring mistake produces, and it has a test of its own
// because it is the one state where a check does not fail — it is simply not
// there. Every other refusal in this package is a rule saying no; this one is a
// dependency that was never supplied, and the module has to notice.
//
// A mutation pass found the half of it that was untested. assertScope has failed
// closed on a nil registry since the perimeter landed, and holdsEnforcementRole
// is new: OpenCase asks it whether a signer that is not a bonded validator is an
// enforcement office, and a version that answered "yes, and I could not check"
// would let any account open a case on a chain whose registry was not wired.
// Making it answer true survived every other test in the package.
func TestWithNoPerimeterRegistryEveryAuthorityPathRefuses(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	_, targetStr := f.fundedAddr(t, coins(100_000))
	_, stranger := f.addr(t)

	// The same keeper the fixture built, over the same store, with the registry
	// taken away. Built rather than mutated because the field is unexported,
	// which is itself the point: nothing inside the module can put the perimeter
	// back, and nothing inside it can take the perimeter away either.
	unwired := keeper.NewKeeper(
		f.env.StoreService,
		f.env.Codec,
		f.env.AddressCodec,
		f.env.Authority,
		f.env.AuthKeeper,
		f.env.BankKeeper,
		f.staking,
		nil, // no x/distribution in this fixture: no rewards to reclaim
		constitutiontestutil.Init(t, f.env, f.staking,
			constitutiontestutil.Invariants(f.destinationStr)),
		nil,
	)
	// The concentration check IS wired here, so this test keeps testing the one
	// thing it is named for. Without it the caps guard would refuse first and
	// every assertion below would pass for the wrong reason — which is how a
	// test quietly stops covering what it claims to.
	unwired.SetConcentrationKeeper(withinCaps{})
	ms := keeper.NewMsgServerImpl(unwired)

	// A bonded validator, which needs no grant to be recognised as one, is
	// refused because the perimeter cannot be consulted about the target.
	_, err := ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: targetStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "a validator on a chain whose registry is missing",
	})
	require.ErrorIs(t, err, aliastypes.ErrNoScopeKeeper)
	require.False(t, unwired.IsFrozen(f.ctx, targetStr))

	// An account that is neither a validator nor an office is refused BEFORE the
	// perimeter is reached, by the question "are you an enforcement authority" —
	// which is the one that must not be answered permissively when it cannot be
	// answered at all.
	_, err = ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: stranger, Target: targetStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "a stranger on a chain whose registry is missing",
	})
	require.ErrorIs(t, err, aliastypes.ErrNoScopeKeeper)
	require.False(t, unwired.IsFrozen(f.ctx, targetStr))

	// The emergency path too, which is the one that acts on a single signature.
	_, err = ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: stranger, Target: targetStr, Reason: "the fast path is not an exception",
	})
	require.ErrorIs(t, err, aliastypes.ErrNoScopeKeeper)
	require.False(t, unwired.IsFrozen(f.ctx, targetStr))

	// And appointing an ombudsman, which has to be able to ask whether the
	// proposed office already holds the role that would let it open a case.
	params, err := unwired.Params.Get(f.ctx)
	require.NoError(t, err)
	params.Ombudsman = stranger
	_, err = ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.env.AuthorityString(t), Params: params,
	})
	require.ErrorIs(t, err, aliastypes.ErrNoScopeKeeper)
}

// withinCaps is a concentration registry that permits everybody, for the tests
// whose subject is something else.
type withinCaps struct{}

func (withinCaps) AssertOperatorWithinCaps(context.Context, string) error { return nil }

// And the mirror of the test above: with no CONCENTRATION registry, every path
// that exercises an enforcement power refuses.
//
// The two unwired cases are separate tests because they are separate
// guarantees, and a single test asserting "something refused" would keep
// passing if one of them silently stopped.
//
// nil must read as refuse and never as permit. The alternative is that
// forgetting one line in app.go restores the epoch-long window in which a group
// over a constitutional ceiling still holds these powers — and nothing would
// fail until somebody read the code.
func TestWithNoConcentrationRegistryEveryEnforcementPowerRefuses(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	_, targetStr := f.fundedAddr(t, coins(100_000))

	// The fixture's own keeper, with the concentration registry never set.
	unwired := keeper.NewKeeper(
		f.env.StoreService,
		f.env.Codec,
		f.env.AddressCodec,
		f.env.Authority,
		f.env.AuthKeeper,
		f.env.BankKeeper,
		f.staking,
		nil, // no x/distribution in this fixture: no rewards to reclaim
		constitutiontestutil.Init(t, f.env, f.staking,
			constitutiontestutil.Invariants(f.destinationStr)),
		f.perimeter.Keeper,
	)
	ms := keeper.NewMsgServerImpl(unwired)

	_, err := ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: targetStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "a case on a chain that cannot check its own ceilings",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "concentration registry is not wired")
	require.False(t, unwired.IsFrozen(f.ctx, targetStr))

	// The emergency path too, which is the one that acts on a single signature
	// and therefore the one where an epoch-long window matters most.
	_, err = ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: validator, Target: targetStr, Reason: "the fast path is not an exception",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "concentration registry is not wired")
	require.False(t, unwired.IsFrozen(f.ctx, targetStr))
}

// overCaps is a concentration registry that refuses everybody, naming a
// ceiling the way the real one does.
type overCaps struct{}

func (overCaps) AssertOperatorWithinCaps(_ context.Context, operator string) error {
	return fmt.Errorf("%s is above the legal entity concentration ceiling", operator)
}

// A validator over a constitutional ceiling may not exercise an enforcement
// power, at the moment it tries.
//
// Audit finding 3.3. The ceilings are swept at an epoch boundary and these
// powers do not wait for one: a freeze lands in a block, a seizure in a single
// vote. So a group that crossed a ceiling held exactly the powers the
// constitution was written to deny it, for up to a whole epoch — and a freeze
// imposed in that window is not undone by the demotion that follows it.
//
// Being within caps is now a precondition of the power rather than a correction
// after its use, and this is what asserts it.
func TestAValidatorOverACeilingCannotExerciseEnforcementPowers(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	_, targetStr := f.fundedAddr(t, coins(100_000))

	over := keeper.NewKeeper(
		f.env.StoreService,
		f.env.Codec,
		f.env.AddressCodec,
		f.env.Authority,
		f.env.AuthKeeper,
		f.env.BankKeeper,
		f.staking,
		nil, // no x/distribution in this fixture: no rewards to reclaim
		constitutiontestutil.Init(t, f.env, f.staking,
			constitutiontestutil.Invariants(f.destinationStr)),
		f.perimeter.Keeper,
	)
	over.SetConcentrationKeeper(overCaps{})
	ms := keeper.NewMsgServerImpl(over)

	_, err := ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: targetStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "opened by a validator whose group is over its ceiling",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "concentration ceiling")
	require.False(t, over.IsFrozen(f.ctx, targetStr))

	// And the fast path, which is where an epoch-long window costs the most:
	// one signature, effective immediately, and not reversed by the demotion.
	_, err = ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: validator, Target: targetStr, Reason: "the fast path is not an exception",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "concentration ceiling")
	require.False(t, over.IsFrozen(f.ctx, targetStr),
		"a validator over a constitutional ceiling froze an account")
}
