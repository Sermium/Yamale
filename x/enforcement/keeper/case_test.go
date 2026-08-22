package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/enforcement/types"
)

// The freeze is the fast half of this module, so the first thing worth proving
// is that it is actually fast: money stops moving in the block the case was
// opened, not when the vote ends.
func TestOpeningACaseStopsTheMoneyImmediately(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	scammer, scammerStr := f.fundedAddr(t, coins(1_000_000))
	victim, _ := f.addr(t)

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator,
		Target: scammerStr,
		Action: types.CASE_ACTION_FREEZE,
		Reason: "drained a liquidity pool",
	})
	require.NoError(t, err)

	err = f.env.BankKeeper.SendCoins(f.ctx, scammer, victim, coins(500_000))
	require.ErrorIs(t, err, types.ErrFrozen)
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(scammer, "uyml"),
		"the transfer was refused, so nothing should have moved")

	// Being frozen stops sending, not receiving. Refusing incoming funds would
	// bounce payments from people who have done nothing wrong.
	funder, _ := f.fundedAddr(t, coins(10))
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, funder, scammer, coins(10)))
}

// A single validator can freeze an account, so the leash on that power is the
// only thing standing between this module and an arbitrary one.
func TestAnUnvotedFreezeExpiresByItself(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	scammer, scammerStr := f.fundedAddr(t, coins(1_000_000))
	victim, _ := f.addr(t)

	resp, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator,
		Target: scammerStr,
		Action: types.CASE_ACTION_FREEZE,
		Reason: "suspected fraud",
	})
	require.NoError(t, err)

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)

	// The vote runs out first: the case expires, which is not the same as being
	// rejected, and the account is released there and then.
	f.atHeight(int64(params.VotingPeriodBlocks) + 1)
	require.NoError(t, f.keeper.EndBlocker(f.ctx))

	closed, err := f.keeper.Case.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_EXPIRED, closed.Status,
		"nobody voting is not a finding, and the record should not read as one")

	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, victim, coins(500_000)))
	require.Equal(t, math.NewInt(500_000), f.env.Balance(victim, "uyml"))
}

// Even if the end blocker never ran — a chain halted mid-case, a queue entry
// lost — the freeze itself carries its own expiry.
func TestTheProvisionalFreezeOutlivesTheVoteButNotForever(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(1_000_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator,
		Target: scammerStr,
		Action: types.CASE_ACTION_FREEZE,
		Reason: "suspected fraud",
	})
	require.NoError(t, err)

	freeze, found, err := f.keeper.FreezeOf(f.ctx, scammerStr)
	require.NoError(t, err)
	require.True(t, found)

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	require.Greater(t, freeze.ExpiresAtHeight, int64(params.VotingPeriodBlocks),
		"a freeze that lapsed mid-vote would hand the account back before the chain had decided anything")
}

// Four validators of equal power: two thirds of forty is 26.67, and the
// threshold rounds up to 27, so three of the four have to agree. Rounding down
// would let two of four — half the set — pass a case the threshold says needs
// two thirds.
func TestASupermajorityIsNeededToPass(t *testing.T) {
	f := initFixture(t)
	one := f.addValidator(t, 10)
	two := f.addValidator(t, 10)
	three := f.addValidator(t, 10)
	four := f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(1_000_000))

	resp, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: one,
		Target: scammerStr,
		Action: types.CASE_ACTION_FREEZE,
		Reason: "phishing",
	})
	require.NoError(t, err)

	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: one, CaseId: resp.Id, Option: types.VOTE_OPTION_YES})
	require.NoError(t, err)
	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: two, CaseId: resp.Id, Option: types.VOTE_OPTION_YES})
	require.NoError(t, err)

	open, err := f.keeper.Case.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_VOTING, open.Status, "half the set is not a supermajority")

	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: three, CaseId: resp.Id, Option: types.VOTE_OPTION_YES})
	require.NoError(t, err)

	passed, err := f.keeper.Case.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_PASSED, passed.Status)

	// And the freeze it imposed no longer expires: it is the set's decision now,
	// not one validator's suspicion.
	freeze, found, err := f.keeper.FreezeOf(f.ctx, scammerStr)
	require.NoError(t, err)
	require.True(t, found)
	require.Zero(t, freeze.ExpiresAtHeight)

	// A vote on a case that has already resolved is refused rather than lost.
	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: four, CaseId: resp.Id, Option: types.VOTE_OPTION_NO})
	require.ErrorIs(t, err, types.ErrCaseClosed)
}

// Rejection has to be as fast as passing. An account held frozen through a
// voting period whose outcome is already decided is somebody's money stopped
// for no reason.
func TestACaseIsRejectedAsSoonAsItCannotPass(t *testing.T) {
	f := initFixture(t)
	one := f.addValidator(t, 10)
	two := f.addValidator(t, 10)
	three := f.addValidator(t, 10)
	f.addValidator(t, 10)
	scammer, scammerStr := f.fundedAddr(t, coins(1_000_000))
	other, _ := f.addr(t)

	resp, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: one,
		Target: scammerStr,
		Action: types.CASE_ACTION_FREEZE,
		Reason: "mistaken identity",
	})
	require.NoError(t, err)

	// Twenty of forty voting no leaves twenty, and the threshold needs 27: the
	// case cannot pass any more, whatever the two who have not voted do.
	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: two, CaseId: resp.Id, Option: types.VOTE_OPTION_NO})
	require.NoError(t, err)
	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: three, CaseId: resp.Id, Option: types.VOTE_OPTION_NO})
	require.NoError(t, err)

	rejected, err := f.keeper.Case.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_REJECTED, rejected.Status)

	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, other, coins(1_000)),
		"a rejected case must release the account in the same block")
}

func TestOnlyBondedValidatorsMayOpenAndVote(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	// A second validator so that one yes does not resolve the case, which would
	// hide the double-vote check behind an already-closed one.
	f.addValidator(t, 10)
	_, stranger := f.addr(t)
	_, scammerStr := f.fundedAddr(t, coins(1_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: stranger,
		Target: scammerStr,
		Action: types.CASE_ACTION_FREEZE,
		Reason: "anyone could say this",
	})
	require.ErrorIs(t, err, types.ErrUnknownValidator)

	resp, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator,
		Target: scammerStr,
		Action: types.CASE_ACTION_FREEZE,
		Reason: "theft",
	})
	require.NoError(t, err)

	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: stranger, CaseId: resp.Id, Option: types.VOTE_OPTION_YES})
	require.ErrorIs(t, err, types.ErrUnknownValidator)

	// And nobody votes twice.
	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: validator, CaseId: resp.Id, Option: types.VOTE_OPTION_YES})
	require.NoError(t, err)
	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: validator, CaseId: resp.Id, Option: types.VOTE_OPTION_NO})
	require.ErrorIs(t, err, types.ErrAlreadyVoted)
}

// Module accounts hold the chain's own money. Freezing one would stop staking,
// distribution or every payment on the chain for everybody, and there is nobody
// behind such an address to accuse.
func TestModuleAccountsCannotBeFrozen(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)

	moduleAddr := f.env.AuthKeeper.GetModuleAddress(integration.FunderModuleName)
	require.NotNil(t, moduleAddr)
	moduleAddrStr, err := f.env.AddressCodec.BytesToString(moduleAddr)
	require.NoError(t, err)
	f.env.Block(moduleAddr)

	_, err = f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator,
		Target: moduleAddrStr,
		Action: types.CASE_ACTION_FREEZE,
		Reason: "the staked funds are stolen",
	})
	require.ErrorIs(t, err, types.ErrProtectedAddress)
}

func TestOneCaseAtATimePerAddress(t *testing.T) {
	f := initFixture(t)
	one := f.addValidator(t, 10)
	two := f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(1_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: one, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "first",
	})
	require.NoError(t, err)

	// Otherwise withdrawing one case would lift a freeze another still relies
	// on, and the account would be released while an accusation was live.
	_, err = f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: two, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "second",
	})
	require.ErrorIs(t, err, types.ErrAlreadyFrozen)
}

func TestOnlyTheOpenerMayWithdraw(t *testing.T) {
	f := initFixture(t)
	one := f.addValidator(t, 10)
	two := f.addValidator(t, 10)
	scammer, scammerStr := f.fundedAddr(t, coins(1_000))
	other, _ := f.addr(t)

	resp, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: one, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "a mistake",
	})
	require.NoError(t, err)

	_, err = f.ms.WithdrawCase(f.ctx, &types.MsgWithdrawCase{Opener: two, CaseId: resp.Id})
	require.ErrorIs(t, err, types.ErrNotTheOpener)

	_, err = f.ms.WithdrawCase(f.ctx, &types.MsgWithdrawCase{Opener: one, CaseId: resp.Id})
	require.NoError(t, err)

	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, other, coins(1_000)))
}

// A seizure takes assets, so the bar for opening one is higher than for a
// freeze: without evidence there is nothing for anyone to check afterwards.
func TestASeizureNeedsEvidenceAndSomewhereToSend(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(1_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_SEIZE, Reason: "theft",
	})
	require.ErrorIs(t, err, types.ErrEvidenceRequired)

	// And with no destination configured, a seizure has nowhere to send what it
	// takes — so it cannot be opened at all.
	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	params.RecoveryDestination = ""
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	_, err = f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_SEIZE, Reason: "theft",
		EvidenceUri: "https://example.org/case", EvidenceHash: "abc123",
	})
	require.ErrorIs(t, err, types.ErrInvalidCase)
}
