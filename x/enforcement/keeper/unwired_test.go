package keeper_test

import (
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
		constitutiontestutil.Init(t, f.env, f.staking,
			constitutiontestutil.Invariants(f.destinationStr)),
		nil,
	)
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
