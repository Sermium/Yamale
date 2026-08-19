package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/oracle/types"
)

// Findings from the pre-genesis review of this module. Each test is the exploit
// or the failure, written so it fails against the code as it was.

// A genesis file edited after `validate-genesis` — which is what every launch
// ceremony does — can carry a zero vote period. The EndBlocker takes a modulo
// of it on the first block, and an integer division by zero is a panic, not an
// error: the chain does not start, and no transaction can fix it because there
// is no chain to send one to.
//
// Params.Validate() rejects zero, so MsgUpdateParams and a validated genesis
// cannot produce this. That is exactly why the guard belongs in the EndBlocker:
// the one path that reaches it is the one path with no validation in front.
func TestAZeroVotePeriodMustNotHaltTheChain(t *testing.T) {
	f := initFixture(t)

	params := types.DefaultParams()
	params.VotePeriod = 0
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	f.at(testNow, 1)
	require.NotPanics(t, func() {
		_ = f.keeper.EndBlocker(f.ctx)
	}, "a zero vote period must stop rounds, not stop the chain")
}

// The same shape one level down: a genesis with no accepted denoms is valid
// today, and must simply agree no rates rather than misbehave.
func TestAnEmptyDenomSetAgreesNothing(t *testing.T) {
	f := initFixture(t)

	params := types.DefaultParams()
	params.AcceptedDenoms = nil
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	operator, feeder := f.addValidator(t, 100)
	_, err := f.ms.SubmitExchangeRates(f.ctx, &types.MsgSubmitExchangeRates{
		Feeder: feeder, Validator: operator,
		Rates: []types.RateVote{{Denom: denom, Rate: "1.00"}},
	})
	require.Error(t, err, "no denom is accepted, so no vote can be")

	f.at(testNow, 12)
	require.NoError(t, f.keeper.EndBlocker(f.ctx))
}

// Applying is permissionless, so every string in the application is an
// attacker-chosen blob that the chain stores forever. Without a bound, one
// transaction can commit close to a megabyte of arbitrary bytes to state at the
// cost of its gas, and repeat.
func TestApplicationTextIsBounded(t *testing.T) {
	f := initFixture(t)
	_, addr := f.env.Addr(t)

	huge := make([]byte, 100_000)
	for i := range huge {
		huge[i] = 'a'
	}

	_, err := f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: addr, Name: string(huge), Credentials: "RICS",
	})
	require.Error(t, err, "an unbounded name is permanent state an attacker chooses")

	_, err = f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: addr, Name: "Firm", Credentials: string(huge),
	})
	require.Error(t, err)

	_, err = f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: addr, Name: "Firm", Credentials: "RICS", ClassIds: []string{string(huge)},
	})
	require.Error(t, err)

	// A reasonable application still works.
	_, err = f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: addr, Name: "Alpine Valuation SA", Credentials: "RICS 12345", ClassIds: []string{"realestate"},
	})
	require.NoError(t, err)
}

// The same exposure on the valuation itself. An approved valuer is trusted to
// be honest about numbers, not trusted with unbounded state — and a stolen
// valuer key should not be able to bloat every validator's disk.
func TestValuationTextIsBounded(t *testing.T) {
	f := initFixture(t)
	f.nft.mint(classID, nftID)
	appraiser := f.approvedAppraiser(t)
	now := f.env.Ctx.BlockTime().Unix()

	huge := make([]byte, 100_000)
	for i := range huge {
		huge[i] = 'a'
	}

	_, err := f.ms.SubmitAppraisal(f.ctx, &types.MsgSubmitAppraisal{
		Appraiser: appraiser, ClassId: classID, NftId: nftID,
		Value: "1000", ValueDenom: "uusd", ValuedAt: now,
		Method: string(huge),
	})
	require.Error(t, err)

	_, err = f.ms.SubmitAppraisal(f.ctx, &types.MsgSubmitAppraisal{
		Appraiser: appraiser, ClassId: classID, NftId: nftID,
		Value: "1000", ValueDenom: "uusd", ValuedAt: now,
		Method: "RICS Red Book", ReportUri: string(huge),
	})
	require.Error(t, err)

	require.NoError(t, f.submitAppraisal(t, appraiser, "1000", now))
}

// An identifier the chain never looks up is still a key it stores. Bounding
// them keeps an asset reference to something an NFT class could plausibly be
// called.
func TestAssetIdentifiersAreBounded(t *testing.T) {
	f := initFixture(t)
	appraiser := f.approvedAppraiser(t)
	now := f.env.Ctx.BlockTime().Unix()

	huge := make([]byte, 100_000)
	for i := range huge {
		huge[i] = 'a'
	}

	// Minted, so the asset genuinely exists and the length check is the only
	// thing that can reject this. Without that the test would pass on
	// ErrAssetNotFound and prove nothing.
	f.nft.mint(string(huge), nftID)
	f.nft.mint(classID, string(huge))

	_, err := f.ms.SubmitAppraisal(f.ctx, &types.MsgSubmitAppraisal{
		Appraiser: appraiser, ClassId: string(huge), NftId: nftID,
		Value: "1000", ValueDenom: "uusd", ValuedAt: now, Method: "RICS Red Book",
	})
	require.ErrorIs(t, err, types.ErrLimitReached)

	_, err = f.ms.SubmitAppraisal(f.ctx, &types.MsgSubmitAppraisal{
		Appraiser: appraiser, ClassId: classID, NftId: string(huge),
		Value: "1000", ValueDenom: "uusd", ValuedAt: now, Method: "RICS Red Book",
	})
	require.ErrorIs(t, err, types.ErrLimitReached)
}

// A rate is a decimal string of the sender's choosing. LegacyDec parses
// arbitrarily long inputs, so without a bound a vote can carry a number with
// thousands of digits — parsed by every validator, then stored and re-parsed
// every round.
func TestRateStringsAreBounded(t *testing.T) {
	f := initFixture(t)
	operator, feeder := f.addValidator(t, 100)

	// Long in the integer part, where LegacyDec imposes no limit of its own —
	// its 18-decimal cap only bounds the fractional side.
	long := "1"
	for i := 0; i < 5_000; i++ {
		long += "1"
	}

	_, err := f.ms.SubmitExchangeRates(f.ctx, &types.MsgSubmitExchangeRates{
		Feeder: feeder, Validator: operator,
		Rates: []types.RateVote{{Denom: denom, Rate: long}},
	})
	require.Error(t, err, "an absurdly long decimal is work every validator has to do")

	f.vote(t, feeder, operator, denom, "1.00")
}
