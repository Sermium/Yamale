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
	require.Equal(t, int64(3), f.staking.Seats(favoured))

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

	// Two of seven is 2857 basis points, inside a 4000 ceiling.
	f.epoch(t, 1)
	require.False(t, f.demoted(t, favouredStr))
	require.Equal(t, int64(2), f.staking.Seats(favoured))
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
	require.Equal(t, int64(4), f.staking.Seats(validator))

	_, err = ms.SetValidatorPower(f.env.Ctx, &types.MsgSetValidatorPower{
		Authority: f.env.AuthorityString(t), Validator: validatorStr, Seats: 2,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), f.staking.Seats(validator))
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
