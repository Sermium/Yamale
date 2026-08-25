package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/enforcement/types"
)

// A country's enforcement office may accuse. The validator set still decides.
//
// ROLE_ENFORCEMENT_AUTHORITY could be granted before this and no message let it
// be used: OpenCase required a bonded validator and the emergency path required
// being a named parameter, so a national office held a grant that did nothing.
// These tests are the two halves of what changed — an office can now stop money,
// and it still cannot take any.

// office returns a fresh account that holds the enforcement role over the
// fixture's country and is not a validator.
//
// Not a validator is the whole point of it: every other opener in this package
// is one, and a helper that happened to bond its offices would make every test
// below pass through the path that already existed.
func (f *fixture) office(t *testing.T) string {
	t.Helper()
	_, addr := f.addr(t)
	f.grantEnforcement(t, addr)
	return addr
}

func TestAnEnforcementOfficeCanOpenACaseWithoutBeingAValidator(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	authority := f.office(t)
	scammer, scammerStr := f.fundedAddr(t, coins(1_000_000))
	elsewhere, _ := f.addr(t)

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: authority, Target: scammerStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "the national financial intelligence unit reported these deposits",
	})
	require.NoError(t, err)

	// The freeze is real, in the same block, exactly as a validator's is.
	require.ErrorIs(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(1)), types.ErrFrozen)

	stored, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	// Recorded under the office's own address, not under an operator address
	// derived from it. A group policy has no operator address, and the derived
	// form would be an account nobody holds a key for — which is exactly what a
	// role-holders query could not be read against.
	require.Equal(t, authority, stored.Opener)
	require.Equal(t, types.CASE_STATUS_VOTING, stored.Status)
	require.False(t, stored.Emergency,
		"an ordinary case opened by an office is an ordinary case")
}

// The office accuses; it does not decide. It has no vote at all, and the
// two-thirds threshold is unchanged.
func TestAnOfficeThatOpenedACaseStillCannotVoteOnIt(t *testing.T) {
	f := initFixture(t)
	one := f.addValidator(t, 10)
	f.addValidator(t, 10)
	f.addValidator(t, 10)
	authority := f.office(t)
	scammer, scammerStr := f.fundedAddr(t, coins(1_000_000))
	elsewhere, _ := f.addr(t)

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: authority, Target: scammerStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "reported by the financial intelligence unit",
	})
	require.NoError(t, err)

	// The opener's own vote was never assumed from opening, and an office has
	// none to cast: it is not a validator, so the staking lookup refuses it.
	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{
		Voter: authority, CaseId: opened.Id, Option: types.VOTE_OPTION_YES,
	})
	require.ErrorIs(t, err, types.ErrUnknownValidator)

	// And the tally contains no vote nobody sent.
	stored, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	require.Zero(t, stored.YesPower)

	// The set can refuse what the office asked for, which is the whole point of
	// separating accusation from decision. One validator out of three is enough
	// here: at a two-thirds threshold over thirty power, ten voting no already
	// puts the threshold out of reach, and the case resolves the moment the
	// answer can no longer change rather than waiting out the period.
	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{
		Voter: one, CaseId: opened.Id, Option: types.VOTE_OPTION_NO,
	})
	require.NoError(t, err)
	refused, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_REJECTED, refused.Status)
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(1_000_000)))
}

// An office may accuse a seizure. Whether anything is taken is still the set's.
func TestAnOfficeMayAccuseASeizureAndTheSetStillDecidesIt(t *testing.T) {
	f := initFixture(t)
	one := f.addValidator(t, 10)
	two := f.addValidator(t, 10)
	three := f.addValidator(t, 10)
	authority := f.office(t)
	_, scammerStr := f.fundedAddr(t, coins(400_000))

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: authority, Target: scammerStr, Action: types.CASE_ACTION_SEIZE,
		Reason:      "proceeds of a court-confirmed fraud",
		EvidenceUri: "https://example.org/report.pdf", EvidenceHash: "9f2c0e1b",
		LegalInstrument: instrument(),
	})
	require.NoError(t, err)

	// Every guard a validator's seizure carries applies to the office's. The
	// legal instrument especially: an authority that could take assets without
	// naming what ordered it would be a validator set with extra steps.
	_, err = f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: authority, Target: scammerStr, Action: types.CASE_ACTION_SEIZE,
		Reason: "no instrument", EvidenceUri: "https://example.org/x", EvidenceHash: "ab",
	})
	require.Error(t, err)

	// All three: two thirds of thirty rounds UP to twenty-one, so twenty is not a
	// supermajority and the arithmetic is not generous about it.
	for _, validator := range []string{one, two, three} {
		_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{
			Voter: validator, CaseId: opened.Id, Option: types.VOTE_OPTION_YES,
		})
		require.NoError(t, err)
	}
	passed, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_HELD, passed.Status,
		"a seizure the set agreed to still waits out its delay")
}

// An account with no grant and no bond is refused, and told both roads.
func TestAnAccountThatIsNeitherValidatorNorAuthorityCannotOpenACase(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	_, stranger := f.addr(t)
	_, targetStr := f.fundedAddr(t, coins(100_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: stranger, Target: targetStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "I say so",
	})
	require.ErrorIs(t, err, types.ErrUnknownValidator)
	// The wording matters and is asserted for the reason the seizure tests give:
	// a signer who meant to act as a validator and one who meant to act as an
	// office go looking in different places, and a refusal naming only one of the
	// two roads sends half of them to the wrong module.
	require.ErrorContains(t, err, "is not a bonded validator and holds no grant of ROLE_ENFORCEMENT_AUTHORITY")
	require.False(t, f.keeper.IsFrozen(f.ctx, targetStr))
}

// The perimeter is what permits, and it runs on the office exactly as it runs on
// a validator. A grant somewhere else is not a grant here.
func TestAnOfficeGrantedElsewhereCannotOpenACaseHere(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	_, targetStr := f.fundedAddr(t, coins(100_000))

	// Placed in the fixture's country so it is a resolvable account, but granted
	// the role over a different one.
	_, abroad := f.addr(t)
	f.perimeter.Grant(t, abroad, aliastypes.ROLE_ENFORCEMENT_AUTHORITY, "NG")

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: abroad, Target: targetStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "an account in somebody else's jurisdiction",
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)
	require.ErrorContains(t, err, "covering "+country)
	require.False(t, f.keeper.IsFrozen(f.ctx, targetStr))
}

// The office withdraws its own case, and nobody else's.
func TestAnOfficeCanWithdrawTheCaseItOpened(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	authority := f.office(t)
	other := f.office(t)
	scammer, scammerStr := f.fundedAddr(t, coins(500_000))
	elsewhere, _ := f.addr(t)

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: authority, Target: scammerStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "reported, then explained",
	})
	require.NoError(t, err)

	// Another office holding the same role in the same country is still not the
	// opener. Withdrawal is identity, not authority.
	_, err = f.ms.WithdrawCase(f.ctx, &types.MsgWithdrawCase{Opener: other, CaseId: opened.Id})
	require.ErrorIs(t, err, types.ErrNotTheOpener)

	// Nor is a validator, which is the case that would have been silently
	// possible if the opener were resolved to an operator address on both paths.
	_, err = f.ms.WithdrawCase(f.ctx, &types.MsgWithdrawCase{Opener: validator, CaseId: opened.Id})
	require.ErrorIs(t, err, types.ErrNotTheOpener)

	_, err = f.ms.WithdrawCase(f.ctx, &types.MsgWithdrawCase{Opener: authority, CaseId: opened.Id})
	require.NoError(t, err)
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(500_000)))
}

// Withdrawal survives the grant being revoked, which is deliberate: it lifts a
// freeze, and a rule that made de-escalation conditional on still holding a
// power would leave somebody's account frozen because the office that was wrong
// about them lost its authority afterwards.
func TestAnOfficeWhoseGrantWasRevokedCanStillWithdrawItsOwnCase(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	authority := f.office(t)
	scammer, scammerStr := f.fundedAddr(t, coins(500_000))
	elsewhere, _ := f.addr(t)

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: authority, Target: scammerStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "opened, then the office was stood down",
	})
	require.NoError(t, err)

	f.perimeter.Revoke(t, authority, aliastypes.ROLE_ENFORCEMENT_AUTHORITY, country)

	// It cannot open another one.
	_, otherTarget := f.fundedAddr(t, coins(1_000))
	_, err = f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: authority, Target: otherTarget, Action: types.CASE_ACTION_FREEZE,
		Reason: "still trying",
	})
	require.Error(t, err)

	// And it can still take back the one it opened.
	_, err = f.ms.WithdrawCase(f.ctx, &types.MsgWithdrawCase{Opener: authority, CaseId: opened.Id})
	require.NoError(t, err)
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(500_000)))
}
