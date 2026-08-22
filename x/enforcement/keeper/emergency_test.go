package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/enforcement/types"
)

// withEmergencyAuthority names a founders' group and returns its address.
func (f *fixture) withEmergencyAuthority(t *testing.T) string {
	t.Helper()

	_, founders := f.addr(t)
	// Named in the parameters *and* granted the enforcement role. The emergency
	// path is not an exception to the perimeter: acting on a single signature is
	// exactly the power that must still stop at a border.
	f.grantEnforcement(t, founders)
	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	params.EmergencyAuthority = founders
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	return founders
}

// The reason this path exists: a theft at three in the morning, and no
// validator awake to open a case.
func TestTheFoundersCanFreezeWithoutWaitingForAValidator(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	founders := f.withEmergencyAuthority(t)
	scammer, scammerStr := f.fundedAddr(t, coins(1_000_000))
	elsewhere, _ := f.addr(t)

	resp, err := f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: founders,
		Target:    scammerStr,
		Reason:    "exchange reported the deposit as stolen minutes ago",
	})
	require.NoError(t, err)

	require.ErrorIs(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(1_000_000)), types.ErrFrozen)
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(scammer, "uyml"))

	opened, err := f.keeper.Case.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.True(t, opened.Emergency, "the record must say the founders did this, not a validator")
	require.Equal(t, founders, opened.Opener)
	require.Equal(t, types.CASE_ACTION_FREEZE, opened.Action)
	require.Equal(t, types.CASE_STATUS_VOTING, opened.Status)
}

// The founders are faster than the validator set. They are not above it: a
// freeze they impose still lapses unless the set confirms it.
func TestAFoundersFreezeStillLapsesIfNobodyConfirmsIt(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	founders := f.withEmergencyAuthority(t)
	scammer, scammerStr := f.fundedAddr(t, coins(500_000))
	elsewhere, _ := f.addr(t)

	resp, err := f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: founders, Target: scammerStr, Reason: "reported stolen",
	})
	require.NoError(t, err)

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	f.atHeight(int64(params.VotingPeriodBlocks) + 1)
	require.NoError(t, f.keeper.EndBlocker(f.ctx))

	lapsed, err := f.keeper.Case.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_EXPIRED, lapsed.Status)
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(500_000)))
}

// And the validators can refuse it outright, which releases the account in the
// same block — the founders cannot hold an account the set has cleared.
func TestValidatorsCanRefuseAFoundersFreeze(t *testing.T) {
	f := initFixture(t)
	one := f.addValidator(t, 10)
	two := f.addValidator(t, 10)
	f.addValidator(t, 10)
	f.addValidator(t, 10)
	founders := f.withEmergencyAuthority(t)
	scammer, scammerStr := f.fundedAddr(t, coins(500_000))
	elsewhere, _ := f.addr(t)

	resp, err := f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: founders, Target: scammerStr, Reason: "reported stolen",
	})
	require.NoError(t, err)

	for _, validator := range []string{one, two} {
		_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{
			Voter: validator, CaseId: resp.Id, Option: types.VOTE_OPTION_NO,
		})
		require.NoError(t, err)
	}

	refused, err := f.keeper.Case.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_REJECTED, refused.Status)
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(500_000)))
}

// The half that makes this authority worth having: a wrong freeze does not have
// to sit on somebody's payroll for a day.
func TestTheFoundersCanReleaseAValidatorsFreezeImmediately(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	founders := f.withEmergencyAuthority(t)
	business, businessStr := f.fundedAddr(t, coins(1_000_000))
	supplier, _ := f.addr(t)

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: businessStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "misread a transaction",
	})
	require.NoError(t, err)
	require.ErrorIs(t, f.env.BankKeeper.SendCoins(f.ctx, business, supplier, coins(10)), types.ErrFrozen)

	_, err = f.ms.EmergencyRelease(f.ctx, &types.MsgEmergencyRelease{
		Authority: founders,
		CaseId:    opened.Id,
		Reason:    "the counterparty was the exchange's own settlement account",
	})
	require.NoError(t, err)

	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, business, supplier, coins(1_000_000)))
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(supplier, "uyml"))

	released, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_REVERSED, released.Status)
	require.Contains(t, released.Reason, "settlement account",
		"the grounds for the release belong beside the accusation, permanently")

	// The case must also be out of the voting queue: left there, the end blocker
	// would find a resolved case waiting to be resolved.
	f.atHeight(released.VotingEndsAtHeight + 1)
	require.NoError(t, f.keeper.EndBlocker(f.ctx))
	after, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_REVERSED, after.Status)
	require.False(t, f.keeper.IsFrozen(f.ctx, businessStr))
}

// Releasing after a seizure gives back the account, not the money. The module
// says so rather than implying otherwise.
func TestReleasingAfterASeizureDoesNotReturnTheFunds(t *testing.T) {
	f := initFixture(t)
	scammer, scammerStr := f.fundedAddr(t, coins(400_000))
	founders := f.withEmergencyAuthority(t)

	id := f.openAndPassSeizure(t, scammerStr)
	require.Equal(t, math.NewInt(400_000), f.env.Balance(f.destination, "uyml"))

	_, err := f.ms.EmergencyRelease(f.ctx, &types.MsgEmergencyRelease{
		Authority: founders, CaseId: id, Reason: "identification was wrong",
	})
	require.NoError(t, err)

	require.False(t, f.keeper.IsFrozen(f.ctx, scammerStr))
	require.Equal(t, math.NewInt(400_000), f.env.Balance(f.destination, "uyml"),
		"only the recovery destination can send seized funds back")
	require.Equal(t, math.ZeroInt(), f.env.Balance(scammer, "uyml"))

	released, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, coins(400_000), released.Recovered,
		"the record still says what was taken; releasing the account does not unsay it")
}

// There is no emergency seizure, and no way to reach one. This is the boundary
// the whole design rests on.
func TestTheFoundersCannotSeize(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	founders := f.withEmergencyAuthority(t)
	_, scammerStr := f.fundedAddr(t, coins(1_000_000))

	// The emergency message has no action field at all — a seizure cannot even
	// be expressed. What it can do is freeze, and the case it opens is a freeze
	// case, which Sweep refuses.
	resp, err := f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: founders, Target: scammerStr, Reason: "stolen funds",
	})
	require.NoError(t, err)

	_, err = f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: founders, CaseId: resp.Id})
	require.ErrorIs(t, err, types.ErrNotSeizure)
	require.Equal(t, math.ZeroInt(), f.env.Balance(f.destination, "uyml"))
}

func TestOnlyTheNamedAuthorityMayUseTheEmergencyPath(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	founders := f.withEmergencyAuthority(t)
	_, scammerStr := f.fundedAddr(t, coins(100_000))
	_, stranger := f.addr(t)

	_, err := f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: stranger, Target: scammerStr, Reason: "I say so",
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	// A validator's own account does not inherit it either.
	_, err = f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: validator, Target: scammerStr, Reason: "I am a validator",
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	opened, err := f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: founders, Target: scammerStr, Reason: "reported stolen",
	})
	require.NoError(t, err)

	_, err = f.ms.EmergencyRelease(f.ctx, &types.MsgEmergencyRelease{
		Authority: stranger, CaseId: opened.Id,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}

// An unset authority must mean nobody, never anybody. A signer field that
// happened to be empty must not match an empty parameter.
func TestWithNoAuthoritySetThereIsNoEmergencyPath(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(100_000))

	_, err := f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: "", Target: scammerStr, Reason: "nobody in particular",
	})
	require.ErrorIs(t, err, types.ErrNoEmergencyAuthority)

	_, other := f.addr(t)
	_, err = f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: other, Target: scammerStr, Reason: "still nobody",
	})
	require.ErrorIs(t, err, types.ErrNoEmergencyAuthority)
}

func TestAnEmergencyFreezeStillRefusesModuleAccountsAndNeedsGrounds(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	founders := f.withEmergencyAuthority(t)
	_, scammerStr := f.fundedAddr(t, coins(100_000))

	_, err := f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: founders, Target: scammerStr, Reason: "   ",
	})
	require.ErrorIs(t, err, types.ErrInvalidCase)

	moduleAddr := f.env.AuthKeeper.GetModuleAddress("testfunder")
	require.NotNil(t, moduleAddr)
	moduleAddrStr, err := f.env.AddressCodec.BytesToString(moduleAddr)
	require.NoError(t, err)
	f.env.Block(moduleAddr)

	_, err = f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: founders, Target: moduleAddrStr, Reason: "the chain's own funds",
	})
	require.ErrorIs(t, err, types.ErrProtectedAddress)
}

func TestReleasingACaseThatIsAlreadyClosedIsRefused(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	founders := f.withEmergencyAuthority(t)
	_, scammerStr := f.fundedAddr(t, coins(100_000))

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "a mistake",
	})
	require.NoError(t, err)
	_, err = f.ms.WithdrawCase(f.ctx, &types.MsgWithdrawCase{Opener: validator, CaseId: opened.Id})
	require.NoError(t, err)

	_, err = f.ms.EmergencyRelease(f.ctx, &types.MsgEmergencyRelease{
		Authority: founders, CaseId: opened.Id,
	})
	require.ErrorIs(t, err, types.ErrCaseClosed)

	_, err = f.ms.EmergencyRelease(f.ctx, &types.MsgEmergencyRelease{
		Authority: founders, CaseId: 999,
	})
	require.ErrorIs(t, err, types.ErrCaseNotFound)
}
