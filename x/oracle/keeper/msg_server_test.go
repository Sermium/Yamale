package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/oracle/types"
)

const denom = "uusd"

func TestOnlyBondedValidatorsMayVote(t *testing.T) {
	f := initFixture(t)
	operator, feeder := f.addValidator(t, 100)

	// A stranger cannot vote for somebody else's validator.
	_, strangerStr := f.env.Addr(t)
	_, err := f.ms.SubmitExchangeRates(f.ctx, &types.MsgSubmitExchangeRates{
		Feeder: strangerStr, Validator: operator,
		Rates: []types.RateVote{{Denom: denom, Rate: "1.00"}},
	})
	require.ErrorIs(t, err, types.ErrNotTheFeeder)

	// An address that is not a validator at all cannot vote for itself.
	unknown, unknownStr := f.env.Addr(t)
	_, err = f.ms.SubmitExchangeRates(f.ctx, &types.MsgSubmitExchangeRates{
		Feeder: unknownStr, Validator: sdk.ValAddress(unknown).String(),
		Rates: []types.RateVote{{Denom: denom, Rate: "1.00"}},
	})
	require.ErrorIs(t, err, types.ErrUnknownValidator)

	// The validator's own account may.
	f.vote(t, feeder, operator, denom, "1.00")
}

// A validator that has stopped being bonded keeps no influence over the rate,
// even if its vote was accepted while it still was.
func TestUnbondedValidatorsAreDroppedAtTally(t *testing.T) {
	f := initFixture(t)
	honest, honestFeeder := f.addValidator(t, 100)
	leaving, leavingFeeder := f.addValidator(t, 900)

	f.vote(t, honestFeeder, honest, denom, "1.00")
	f.vote(t, leavingFeeder, leaving, denom, "50.00")

	f.staking.unbond(leaving)
	f.tally(t)

	// The remaining validator is now the whole of the reporting stake, so its
	// rate stands rather than the departed majority's.
	require.Equal(t, "1.000000000000000000", f.rate(t, denom).Rate)
}

func TestVoteRejectsUnusableRates(t *testing.T) {
	f := initFixture(t)
	operator, feeder := f.addValidator(t, 100)

	for _, rate := range []string{"0", "-1.5", "abc", ""} {
		_, err := f.ms.SubmitExchangeRates(f.ctx, &types.MsgSubmitExchangeRates{
			Feeder: feeder, Validator: operator,
			Rates: []types.RateVote{{Denom: denom, Rate: rate}},
		})
		require.Error(t, err, "rate %q should be rejected", rate)
	}

	// A denom nobody agreed to price is refused rather than stored and ignored.
	_, err := f.ms.SubmitExchangeRates(f.ctx, &types.MsgSubmitExchangeRates{
		Feeder: feeder, Validator: operator,
		Rates: []types.RateVote{{Denom: "ubogus", Rate: "1.00"}},
	})
	require.ErrorIs(t, err, types.ErrDenomNotAccepted)

	// Two prices for one denom in one submission is not a report to act on.
	_, err = f.ms.SubmitExchangeRates(f.ctx, &types.MsgSubmitExchangeRates{
		Feeder: feeder, Validator: operator,
		Rates: []types.RateVote{{Denom: denom, Rate: "1.00"}, {Denom: denom, Rate: "2.00"}},
	})
	require.Error(t, err)
}

// Re-submitting corrects a report rather than counting twice, which is what
// lets a feeder that noticed its own mistake fix it within the round.
func TestResubmittingReplacesTheEarlierVote(t *testing.T) {
	f := initFixture(t)
	operator, feeder := f.addValidator(t, 100)

	f.vote(t, feeder, operator, denom, "1.00")
	f.vote(t, feeder, operator, denom, "2.00")
	f.tally(t)

	require.Equal(t, "2.000000000000000000", f.rate(t, denom).Rate)
}

// Below the threshold the previous rate stands and ages, rather than a minority
// setting the price.
func TestBelowThresholdNoRateIsAgreed(t *testing.T) {
	f := initFixture(t)
	small, smallFeeder := f.addValidator(t, 100)
	f.addValidator(t, 900) // silent

	f.vote(t, smallFeeder, small, denom, "1.00")
	f.tally(t)

	_, err := f.keeper.ExchangeRate.Get(f.ctx, denom)
	require.Error(t, err, "a tenth of the stake must not be able to set the rate")
}

func TestVotesAreClearedAfterTheRound(t *testing.T) {
	f := initFixture(t)
	operator, feeder := f.addValidator(t, 100)

	f.vote(t, feeder, operator, denom, "1.00")
	f.tally(t)
	require.Equal(t, "1.000000000000000000", f.rate(t, denom).Rate)

	// The next round starts empty, so silence is silence rather than a repeat
	// of the last price.
	f.at(f.env.Ctx.BlockTime().Unix()+60, f.env.Ctx.BlockHeight()+12)
	require.NoError(t, f.keeper.EndBlocker(f.ctx))

	// The old rate is still readable, and still says when it was agreed.
	require.Equal(t, "1.000000000000000000", f.rate(t, denom).Rate)

	counter, err := f.keeper.MissCounter.Get(f.ctx, operator)
	require.NoError(t, err)
	require.Equal(t, uint64(2), counter.Windows)
	require.Equal(t, uint64(1), counter.Misses, "the silent second round must be counted as a miss")
}

// Delegation is what lets a validator keep its operator key offline. Only the
// operator may change it — a feeder that could nominate its successor would be
// able to keep itself alive after the validator tried to revoke it.
func TestFeederDelegation(t *testing.T) {
	f := initFixture(t)
	operator, ownAccount := f.addValidator(t, 100)
	hot, hotStr := f.env.Addr(t)
	_ = hot

	_, err := f.ms.DelegateFeeder(f.ctx, &types.MsgDelegateFeeder{
		Operator: hotStr, Validator: operator, Feeder: hotStr,
	})
	require.ErrorIs(t, err, types.ErrNotTheFeeder, "a stranger must not be able to nominate a feeder")

	_, err = f.ms.DelegateFeeder(f.ctx, &types.MsgDelegateFeeder{
		Operator: ownAccount, Validator: operator, Feeder: hotStr,
	})
	require.NoError(t, err)

	// The hot key now votes, and the operator key no longer does.
	f.vote(t, hotStr, operator, denom, "1.00")
	_, err = f.ms.SubmitExchangeRates(f.ctx, &types.MsgSubmitExchangeRates{
		Feeder: ownAccount, Validator: operator,
		Rates: []types.RateVote{{Denom: denom, Rate: "1.00"}},
	})
	require.ErrorIs(t, err, types.ErrNotTheFeeder)

	// Delegating back to the operator removes the delegation rather than
	// storing it, so state means exactly one thing: a hot key is in use.
	_, err = f.ms.DelegateFeeder(f.ctx, &types.MsgDelegateFeeder{
		Operator: ownAccount, Validator: operator, Feeder: ownAccount,
	})
	require.NoError(t, err)

	_, err = f.keeper.Feeder.Get(f.ctx, operator)
	require.Error(t, err, "the delegation should be removed, not stored as a self-delegation")

	f.vote(t, ownAccount, operator, denom, "1.00")
}

func TestFeederOfDefaultsToTheValidatorsOwnAccount(t *testing.T) {
	f := initFixture(t)
	operator, ownAccount := f.addValidator(t, 100)

	resp, err := f.qs.FeederDelegation(f.ctx, &types.QueryFeederDelegationRequest{Validator: operator})
	require.NoError(t, err)
	require.Equal(t, ownAccount, resp.Feeder)
}

func TestUpdateParamsRequiresGovernance(t *testing.T) {
	f := initFixture(t)
	_, strangerStr := f.env.Addr(t)

	params := types.DefaultParams()
	params.VotePeriod = 30

	_, err := f.ms.UpdateParams(f.ctx, &types.MsgUpdateParams{Authority: strangerStr, Params: params})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	_, err = f.ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.env.AuthorityString(t), Params: params,
	})
	require.NoError(t, err)

	stored, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(30), stored.VotePeriod)

	// A threshold a minority could clear is refused even from governance: the
	// module's whole guarantee is that setting the rate costs half the stake.
	params.VoteThresholdBps = 1000
	_, err = f.ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.env.AuthorityString(t), Params: params,
	})
	require.Error(t, err)
}
