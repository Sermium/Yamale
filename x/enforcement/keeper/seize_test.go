package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/enforcement/types"
)

// someValidatorAddress is an operator address to delegate to. It is deliberately
// not added to the voting set: a validator registered here would raise the total
// bonded power, and with it the number of yes votes the case needs.
func (f *fixture) someValidatorAddress(t *testing.T) string {
	t.Helper()
	addr, _ := f.addr(t)
	return sdk.ValAddress(addr).String()
}

// openAndHoldSeizure walks a seizure case through to the validators agreeing
// it, and stops there — with the case held and its delay still running.
//
// Every registered validator votes, including any the test added itself: the
// threshold is measured against the whole bonded set, so a helper that voted
// with a fixed number would quietly stop passing the moment a test needed one
// more validator for its own reasons.
func (f *fixture) openAndHoldSeizure(t *testing.T, target string) uint64 {
	t.Helper()

	for len(f.staking.validators) < 4 {
		f.addValidator(t, 10)
	}
	validators := make([]string, 0, len(f.staking.validators))
	for _, v := range f.staking.validators {
		validators = append(validators, v.account)
	}

	resp, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener:          validators[0],
		Target:          target,
		Action:          types.CASE_ACTION_SEIZE,
		Reason:          "drained a pool and sent the proceeds here",
		EvidenceUri:     "https://example.org/cases/1",
		EvidenceHash:    "9f2c0e1b",
		LegalInstrument: instrument(),
	})
	require.NoError(t, err)

	for _, validator := range validators {
		if enforcementCase, err := f.keeper.Case.Get(f.ctx, resp.Id); err == nil &&
			enforcementCase.Status != types.CASE_STATUS_VOTING {
			break
		}
		_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{
			Voter: validator, CaseId: resp.Id, Option: types.VOTE_OPTION_YES,
		})
		require.NoError(t, err)
	}

	held, err := f.keeper.Case.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_HELD, held.Status,
		"an agreed seizure waits out its delay; it is not carried out in the block it is decided")

	return resp.Id
}

// openAndPassSeizure agrees a seizure and then runs the chain forward to the
// height it may be carried out at, so the funds actually move.
//
// The two halves are separate helpers because they are separate facts. Passing
// the vote no longer takes anything — that is the delay — and a test that wants
// to observe the money moving has to say that it waited.
func (f *fixture) openAndPassSeizure(t *testing.T, target string) uint64 {
	t.Helper()

	id := f.openAndHoldSeizure(t, target)
	f.executeHeld(t, id)
	return id
}

// executeHeld runs the chain to a held case's execute height and lets the end
// blocker carry it out.
func (f *fixture) executeHeld(t *testing.T, id uint64) {
	t.Helper()

	held, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_HELD, held.Status)
	f.runTo(t, held.ExecuteAtHeight)
}

// The whole point of the module, in one test: the funds end up with the
// foundation and the scammer keeps nothing.
func TestAPassedSeizureMovesTheFundsToTheDestination(t *testing.T) {
	f := initFixture(t)
	scammer, scammerStr := f.fundedAddr(t, coins(1_000_000))

	id := f.openAndPassSeizure(t, scammerStr)

	require.Equal(t, math.ZeroInt(), f.env.Balance(scammer, "uyml"))
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(f.destination, "uyml"))

	seized, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_PASSED, seized.Status)
	require.Equal(t, coins(1_000_000), seized.Recovered)
	require.True(t, seized.SweepComplete, "nothing staked, nothing unbonding, nothing left")

	// And the chain-wide total, which is the honest answer to how much this
	// power has actually taken.
	total, err := f.keeper.TotalRecovered(f.ctx)
	require.NoError(t, err)
	require.Equal(t, coins(1_000_000), total)
}

// Delegating is the obvious way to put money out of reach of a seizure. It buys
// the unbonding period and nothing else.
func TestASeizureReachesStakedFundsWhenTheyUnbond(t *testing.T) {
	f := initFixture(t)
	scammer, scammerStr := f.fundedAddr(t, coins(200_000))
	f.staking.delegate(scammerStr, f.someValidatorAddress(t), 800_000)

	id := f.openAndPassSeizure(t, scammerStr)

	// The liquid part is taken at once, and the stake has been unbonded rather
	// than left where it was.
	require.Equal(t, math.NewInt(200_000), f.env.Balance(f.destination, "uyml"))
	require.Equal(t, math.LegacyNewDec(800_000), f.staking.undelegated[scammerStr])

	seized, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)
	require.False(t, seized.SweepComplete, "the unbonding stake has not arrived yet")

	// Sweeping now collects nothing, and says so rather than claiming success.
	sweeper, sweeperStr := f.fundedAddr(t, coins(1))
	_ = sweeper
	empty, err := f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: sweeperStr, CaseId: id})
	require.NoError(t, err)
	require.True(t, empty.Collected.IsZero())
	require.False(t, empty.Complete)

	// The unbonding period ends: the coins are back in the account, and the
	// next sweep takes them. Anyone may send it — the destination is fixed by
	// the parameters, so the sender gains nothing by being the one who does.
	f.staking.matureUnbonding(scammerStr)
	f.env.Fund(t, scammer, coins(800_000))

	collected, err := f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: sweeperStr, CaseId: id})
	require.NoError(t, err)
	require.Equal(t, coins(800_000), collected.Collected)
	require.True(t, collected.Complete)

	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(f.destination, "uyml"))
	require.Equal(t, math.ZeroInt(), f.env.Balance(scammer, "uyml"))
}

// A seizure does not end the freeze, so funds that arrive afterwards cannot be
// moved on before the next sweep collects them.
func TestFundsArrivingAfterASeizureAreStillTrapped(t *testing.T) {
	f := initFixture(t)
	scammer, scammerStr := f.fundedAddr(t, coins(100_000))

	id := f.openAndPassSeizure(t, scammerStr)
	require.Equal(t, math.NewInt(100_000), f.env.Balance(f.destination, "uyml"))

	// A late payment from an accomplice.
	accomplice, _ := f.fundedAddr(t, coins(50_000))
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, accomplice, scammer, coins(50_000)))

	elsewhere, _ := f.addr(t)
	require.ErrorIs(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(50_000)), types.ErrFrozen)

	_, err := f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: scammerStr, CaseId: id})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(150_000), f.env.Balance(f.destination, "uyml"))
}

// The exception that lets a seizure move funds out of a frozen account must be
// exactly one address wide. If it were not, a frozen account could send
// anywhere by claiming to be paying the foundation.
func TestAFrozenAccountCanOnlySendToTheRecoveryDestination(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	scammer, scammerStr := f.fundedAddr(t, coins(100_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "suspected theft",
	})
	require.NoError(t, err)

	elsewhere, _ := f.addr(t)
	require.ErrorIs(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(1)), types.ErrFrozen)

	// Paying the destination is allowed, and there is nothing to gain by it:
	// the funds are the foundation's either way.
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, f.destination, coins(1)))
}

func TestOnlyAPassedSeizureCanBeSwept(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(100_000))
	_, senderStr := f.fundedAddr(t, coins(1))

	// A freeze case takes nothing, ever.
	freezeCase, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "suspicion",
	})
	require.NoError(t, err)

	_, err = f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: senderStr, CaseId: freezeCase.Id})
	require.ErrorIs(t, err, types.ErrNotSeizure)

	_, err = f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: senderStr, CaseId: 999})
	require.ErrorIs(t, err, types.ErrCaseNotFound)
}

// Overturning a case gives the account back and says on the record that the
// chain was wrong. It does not pretend to return what was already taken.
func TestGovernanceCanReverseAPassedCase(t *testing.T) {
	f := initFixture(t)
	scammer, scammerStr := f.fundedAddr(t, coins(500_000))

	id := f.openAndPassSeizure(t, scammerStr)
	require.Equal(t, math.NewInt(500_000), f.env.Balance(f.destination, "uyml"))

	_, err := f.ms.ReverseCase(f.ctx, &types.MsgReverseCase{
		Authority: f.env.AuthorityString(t),
		CaseId:    id,
		Reason:    "the address belonged to the exchange, not to the thief",
	})
	require.NoError(t, err)

	reversed, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_REVERSED, reversed.Status)
	require.Equal(t, coins(500_000), reversed.Recovered,
		"the record still says what was taken; reversing it does not unsay that")

	// The account can send again.
	f.env.Fund(t, scammer, coins(10))
	elsewhere, _ := f.addr(t)
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(10)))

	// And nobody but governance can do it.
	_, stranger := f.addr(t)
	_, err = f.ms.ReverseCase(f.ctx, &types.MsgReverseCase{Authority: stranger, CaseId: id})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}

// A delegation that cannot be unbonded must not take the whole seizure down
// with it: the liquid funds are still collected, and the stake is retried on
// the next sweep.
func TestAFailedUndelegationDoesNotStopTheSeizure(t *testing.T) {
	f := initFixture(t)
	scammer, scammerStr := f.fundedAddr(t, coins(300_000))
	f.staking.delegate(scammerStr, f.someValidatorAddress(t), 700_000)
	f.staking.undelegateFails = true

	id := f.openAndPassSeizure(t, scammerStr)

	require.Equal(t, math.NewInt(300_000), f.env.Balance(f.destination, "uyml"))
	require.Equal(t, math.ZeroInt(), f.env.Balance(scammer, "uyml"))

	seized, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)
	require.False(t, seized.SweepComplete, "the stake is still there, so the case is not finished")

	// When the chain will accept the undelegation, the next sweep starts it.
	f.staking.undelegateFails = false
	_, err = f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: scammerStr, CaseId: id})
	require.NoError(t, err)
	require.Equal(t, math.LegacyNewDec(700_000), f.staking.undelegated[scammerStr])
}
