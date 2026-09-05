package keeper_test

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/validatorgov/types"
)

func seatCoins(seats int64) sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin("uyml", sdk.DefaultPowerReduction.MulRaw(seats)))
}

// The test the owner asked for by name, and the one that decides whether the
// constitutional layer is real or decorative.
//
// Governance may set a validator's power. If a proposal could put a validator
// above a ceiling and have it stay there, the ceiling would be a ceiling on
// growth and mergers only — which is to say, a ceiling on the two ways power
// concentrates that nobody controls, and no ceiling at all on the one somebody
// does. So the epoch check reads the power a validator actually carries and
// does not care how it got there.
func TestGovernanceSetPowerAboveTheCapIsDemotedAnyway(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, 2_500, noCeiling, 1)
	f.env.FundModule(t, types.ModuleName, seatCoins(100))

	favoured, favouredStr := f.admit(t, "ENTITY-BIG", "OWNER-BIG", "CH", 1)
	for i := 0; i < 4; i++ {
		f.admit(t, fmt.Sprintf("ENTITY-%d", i), fmt.Sprintf("OWNER-%d", i), "CH", 1)
	}

	f.epoch(t, 1)
	require.False(t, f.demoted(t, favouredStr))

	// A proposal passes, granting three seats where the ceiling allows one. It
	// is accepted: an admission-time refusal here would make the ceiling look
	// enforced while leaving growth and mergers unguarded, and would make this
	// test impossible to write.
	res, err := keeperMsgServer(f).SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t),
		Validator: favouredStr,
		Seats:     3,
	})
	require.NoError(t, err, "governance sets power freely; the ceiling is enforced at the epoch")
	require.Equal(t, uint64(3), res.Seats)

	// Three seats GRANTED, on top of the one this validator already held. A
	// seat count is what the module's reserve has staked, not the validator's
	// total power: the reserve can take back what it put in and nothing else,
	// and measuring the target against the total meant a stranger's delegation
	// could make a power reduction impossible.
	require.Equal(t, int64(4), f.staking.Seats(favoured))

	f.epoch(t, 2)

	require.True(t, f.demoted(t, favouredStr),
		"a ceiling a proposal can vote itself above is not a ceiling")
	require.True(t, f.staking.IsJailed(favoured))
}

// Below the ceiling, governance moves power and nothing objects. Without this
// the test above would pass against a module that demoted everything.
func TestGovernanceMayRaisePowerBelowTheCap(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 4_000, noCeiling, noCeiling, 1)
	f.env.FundModule(t, types.ModuleName, seatCoins(100))

	favoured, favouredStr := f.admit(t, "ENTITY-BIG", "OWNER-BIG", "CH", 1)
	for i := 0; i < 5; i++ {
		f.admit(t, fmt.Sprintf("ENTITY-%d", i), fmt.Sprintf("OWNER-%d", i), "CH", 1)
	}

	_, err := keeperMsgServer(f).SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t), Validator: favouredStr, Seats: 2,
	})
	require.NoError(t, err)

	// Two granted on top of one held is three, of eight in the set: 3750 basis
	// points, inside a 4000 ceiling.
	f.epoch(t, 1)
	require.False(t, f.demoted(t, favouredStr))
	require.Equal(t, int64(3), f.staking.Seats(favoured))
}

// Lowering a power gives the seats back to the reserve.
func TestGovernanceMayLowerPower(t *testing.T) {
	f := initFixture(t)
	f.caps(t, noCeiling, noCeiling, noCeiling, 1)
	f.env.FundModule(t, types.ModuleName, seatCoins(100))

	validator, validatorStr := f.admit(t, "ENTITY-A", "OWNER-A", "CH", 1)

	ms := keeperMsgServer(f)
	_, err := ms.SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t), Validator: validatorStr, Seats: 4,
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), f.staking.Seats(validator))

	_, err = ms.SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t), Validator: validatorStr, Seats: 2,
	})
	require.NoError(t, err)

	// Back to the one seat this validator held plus the two now granted. The
	// seat it held was never the module's to take.
	require.Equal(t, int64(3), f.staking.Seats(validator))
}

// M-5: delegation is permissionless, and a seat target measured against the
// validator's total tokens let anybody block a governance decision.
//
// One MsgDelegate of any size made current exceed what the reserve held, and
// releaseSeats then failed at ValidateUnbondAmount — the reserve does not hold
// a stranger's stake and cannot unbond it. Governance could not lower that
// validator's power until the stranger chose to unbond. The mirror was worse:
// after a power was raised against an inflated current, the stranger
// undelegating dropped the validator silently below what it was granted.
func TestAStrangersDelegationCannotBlockAPowerReduction(t *testing.T) {
	f := initFixture(t)
	f.caps(t, noCeiling, noCeiling, noCeiling, 1)
	f.env.FundModule(t, types.ModuleName, seatCoins(100))

	validator, validatorStr := f.admit(t, "ENTITY-A", "OWNER-A", "CH", 1)

	ms := keeperMsgServer(f)
	_, err := ms.SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t), Validator: validatorStr, Seats: 4,
	})
	require.NoError(t, err)

	// A stranger delegates, permissionlessly, more than the reserve has staked.
	stranger, _ := f.env.Addr(t)
	stored, err := f.staking.GetValidator(f.env.Ctx, sdk.ValAddress(validator))
	require.NoError(t, err)
	_, err = f.staking.Delegate(f.env.Ctx, stranger,
		sdk.DefaultPowerReduction.MulRaw(50), 0, stored, true)
	require.NoError(t, err)

	// Governance lowers the power anyway, because it is lowering its own grant.
	_, err = ms.SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t), Validator: validatorStr, Seats: 1,
	})
	require.NoError(t, err)

	// One seat held, one granted, fifty from the stranger. The module took back
	// exactly the three it had put in and touched nothing else.
	require.Equal(t, int64(52), f.staking.Seats(validator))
}

// The reserve is funded, not minted. A module that could both decide who
// validates and create the token deciding how much they weigh could hand itself
// a validator set, so an empty reserve fails the message rather than conjuring
// the difference.
func TestSetValidatorPowerFailsWhenTheReserveCannotCoverIt(t *testing.T) {
	f := initFixture(t)
	f.caps(t, noCeiling, noCeiling, noCeiling, 1)
	f.env.FundModule(t, types.ModuleName, seatCoins(1))

	_, validatorStr := f.admit(t, "ENTITY-A", "OWNER-A", "CH", 1)

	_, err := keeperMsgServer(f).SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t), Validator: validatorStr, Seats: 9,
	})
	require.ErrorIs(t, err, types.ErrSeatReserveEmpty)
}

func TestSetValidatorPowerRefusesZeroSeats(t *testing.T) {
	f := initFixture(t)
	_, validatorStr := f.admit(t, "ENTITY-A", "OWNER-A", "CH", 1)

	_, err := keeperMsgServer(f).SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t), Validator: validatorStr, Seats: 0,
	})
	require.ErrorIs(t, err, types.ErrInvalidSeats)
}

func TestSetValidatorPowerIsAuthorityGated(t *testing.T) {
	f := initFixture(t)
	_, validatorStr := f.admit(t, "ENTITY-A", "OWNER-A", "CH", 1)
	_, stranger := f.env.Addr(t)

	_, err := keeperMsgServer(f).SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: stranger, Validator: validatorStr, Seats: 2,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}

func TestSetValidatorPowerRefusesAnUnapprovedValidator(t *testing.T) {
	f := initFixture(t)
	stranger, strangerStr := f.env.Addr(t)
	f.staking.AddValidatorWithSeats(stranger, 1)

	_, err := keeperMsgServer(f).SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t), Validator: strangerStr, Seats: 2,
	})
	require.ErrorIs(t, err, types.ErrNotApprovedValidator)
}

// M-8: the ante decorator is not the only road into MsgCreateValidator.
//
// It descends into MsgExec, which closes the authz route. x/group execution
// takes a different one: a passed proposal dispatches its messages straight
// through the message router, after the ante chain has run — and both
// chain-wide foundation administrators on this chain are x/group accounts, so
// either could have created a validator for a candidate nobody voted on.
// Interchain accounts arrive the same way.
//
// The gate is therefore also a staking hook, which x/staking calls from inside
// CreateValidator and whose error fails the message. This exercises the hook
// directly, because that is the layer the finding is about: a transaction test
// would go through the decorator and prove nothing.
func TestTheValidatorGateHoldsWhereTheAnteChainDoesNotRun(t *testing.T) {
	f := initFixture(t)
	f.caps(t, noCeiling, noCeiling, noCeiling, 1)

	hooks := f.keeper.Hooks()
	ctx := f.env.Ctx.WithBlockHeight(1)

	unapproved, _ := f.env.Addr(t)
	require.Error(t, hooks.AfterValidatorCreated(ctx, sdk.ValAddress(unapproved)),
		"an unapproved candidate reached the staking keeper and was created")

	approved, _ := f.admit(t, "ENTITY-OK", "OWNER-OK", "CH", 1)
	require.NoError(t, hooks.AfterValidatorCreated(ctx, sdk.ValAddress(approved)))

	// Height zero is the gentx ceremony, which is how the founding set is
	// onboarded and is deliberately outside the vote.
	require.NoError(t, hooks.AfterValidatorCreated(f.env.Ctx.WithBlockHeight(0), sdk.ValAddress(unapproved)))
}
