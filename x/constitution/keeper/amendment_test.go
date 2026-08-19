package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/constitution/types"
)

// The three things an amendment path has to get right, in order of how badly
// each one fails: it must not be able to shorten its own delay, it must not
// enact without the supermajority, and it must actually enact when both are
// satisfied — a mechanism that only ever refuses is a mechanism nobody can
// distinguish from a constitution with no amendment path, which is the design
// this one was chosen over.

func proposeLowerThreshold(t *testing.T, f *fixture, height int64) uint64 {
	t.Helper()

	proposed := types.DefaultInvariants()
	proposed.EnforcementRecoveryDestination = testRecoveryDestination
	proposed.EnforcementThresholdBps = 5_100

	res, err := f.ms.ProposeAmendment(f.at(height), &types.MsgProposeAmendment{
		Authority:  f.env.AuthorityString(t),
		Invariants: proposed,
		Reason:     "the set has grown and two thirds is now unreachable in a day",
	})
	require.NoError(t, err)
	return res.AmendmentId
}

func TestAmendmentEnactsOnlyWithBothDelayAndSupermajority(t *testing.T) {
	f := initFixture(t)
	accounts := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		accounts = append(accounts, f.addValidator(t, 1))
	}

	id := proposeLowerThreshold(t, f, 100)

	amendment, err := f.keeper.Amendment.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.AMENDMENT_STATUS_PENDING, amendment.Status)
	require.Equal(t, int64(100+types.DefaultAmendmentDelayBlocks), amendment.EffectiveAtHeight)
	require.Equal(t, int64(5), amendment.SnapshotPower)

	// Four fifths of five is four. Three is not enough, and the point of the
	// test is that three ratifications plus the whole delay still is not.
	for _, account := range accounts[:3] {
		_, err := f.ms.RatifyAmendment(f.at(200), &types.MsgRatifyAmendment{
			Validator: account, AmendmentId: id,
		})
		require.NoError(t, err)
	}

	// One block before it is due, nothing has happened.
	require.NoError(t, f.keeper.EndBlocker(f.at(amendment.EffectiveAtHeight-1)))
	amendment, err = f.keeper.Amendment.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.AMENDMENT_STATUS_PENDING, amendment.Status)

	// It comes due with three of five behind it and lapses.
	require.NoError(t, f.keeper.EndBlocker(f.at(amendment.EffectiveAtHeight)))
	amendment, err = f.keeper.Amendment.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.AMENDMENT_STATUS_LAPSED, amendment.Status)

	inv, err := f.keeper.GetInvariants(f.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(types.DefaultEnforcementThresholdBps), inv.EnforcementThresholdBps,
		"an amendment that did not reach its threshold must not have changed anything")
}

func TestAmendmentEnactsWhenRatifiedAndDue(t *testing.T) {
	f := initFixture(t)
	accounts := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		accounts = append(accounts, f.addValidator(t, 1))
	}

	id := proposeLowerThreshold(t, f, 100)
	for _, account := range accounts[:4] {
		_, err := f.ms.RatifyAmendment(f.at(200), &types.MsgRatifyAmendment{
			Validator: account, AmendmentId: id,
		})
		require.NoError(t, err)
	}

	amendment, err := f.keeper.Amendment.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, int64(4), amendment.RatifiedPower)

	require.NoError(t, f.keeper.EndBlocker(f.at(amendment.EffectiveAtHeight)))

	amendment, err = f.keeper.Amendment.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.AMENDMENT_STATUS_ENACTED, amendment.Status)

	inv, err := f.keeper.GetInvariants(f.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(5_100), inv.EnforcementThresholdBps)
}

// An amendment that shortened the delay would otherwise shorten its own, and
// the first amendment anybody passed would be the one that made every
// subsequent change instant.
func TestAmendmentCannotShortenItsOwnDelay(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 1)

	proposed := types.DefaultInvariants()
	proposed.EnforcementRecoveryDestination = testRecoveryDestination
	proposed.AmendmentDelayBlocks = types.MinAmendmentDelayBlocks

	res, err := f.ms.ProposeAmendment(f.at(100), &types.MsgProposeAmendment{
		Authority:  f.env.AuthorityString(t),
		Invariants: proposed,
		Reason:     "three weeks is too long for an operational parameter",
	})
	require.NoError(t, err)

	amendment, err := f.keeper.Amendment.Get(f.env.Ctx, res.AmendmentId)
	require.NoError(t, err)
	require.Equal(t, int64(100+types.DefaultAmendmentDelayBlocks), amendment.EffectiveAtHeight,
		"the effective height must come from the delay in force, not the one being proposed")
}

// The floor on the delay is a constant in the binary, not a value in the store,
// precisely so that this is impossible.
func TestAmendmentCannotProposeADelayBelowTheFloor(t *testing.T) {
	f := initFixture(t)

	proposed := types.DefaultInvariants()
	proposed.EnforcementRecoveryDestination = testRecoveryDestination
	proposed.AmendmentDelayBlocks = types.MinAmendmentDelayBlocks - 1

	_, err := f.ms.ProposeAmendment(f.at(100), &types.MsgProposeAmendment{
		Authority:  f.env.AuthorityString(t),
		Invariants: proposed,
		Reason:     "faster",
	})
	require.ErrorContains(t, err, "below the floor")
}

// Measuring ratification against the power bonded at enactment rather than at
// proposal would make "jail everyone who would refuse" a way to pass anything.
func TestRatificationIsMeasuredAgainstTheSnapshot(t *testing.T) {
	f := initFixture(t)
	accounts := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		accounts = append(accounts, f.addValidator(t, 1))
	}

	id := proposeLowerThreshold(t, f, 100)

	// Two ratify, then the other three stop being bonded. Against the surviving
	// set two of two would be unanimous; against the snapshot it is two of five.
	for _, account := range accounts[:2] {
		_, err := f.ms.RatifyAmendment(f.at(200), &types.MsgRatifyAmendment{
			Validator: account, AmendmentId: id,
		})
		require.NoError(t, err)
	}
	for _, account := range accounts[2:] {
		addr, err := f.env.AddressCodec.StringToBytes(account)
		require.NoError(t, err)
		f.staking.unbond(addr)
	}

	amendment, err := f.keeper.Amendment.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.NoError(t, f.keeper.EndBlocker(f.at(amendment.EffectiveAtHeight)))

	amendment, err = f.keeper.Amendment.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.AMENDMENT_STATUS_LAPSED, amendment.Status,
		"an amendment must not be passable by shrinking the electorate")
}

func TestRatifyingTwiceIsRefused(t *testing.T) {
	f := initFixture(t)
	account := f.addValidator(t, 1)
	id := proposeLowerThreshold(t, f, 100)

	_, err := f.ms.RatifyAmendment(f.at(200), &types.MsgRatifyAmendment{Validator: account, AmendmentId: id})
	require.NoError(t, err)

	_, err = f.ms.RatifyAmendment(f.at(201), &types.MsgRatifyAmendment{Validator: account, AmendmentId: id})
	require.ErrorIs(t, err, types.ErrAlreadyRatified)
}

func TestOnlyAuthorityMayPropose(t *testing.T) {
	f := initFixture(t)
	_, stranger := f.env.Addr(t)

	proposed := types.DefaultInvariants()
	proposed.EnforcementRecoveryDestination = testRecoveryDestination

	_, err := f.ms.ProposeAmendment(f.at(100), &types.MsgProposeAmendment{
		Authority: stranger, Invariants: proposed, Reason: "mine now",
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}

func TestWithdrawnAmendmentDoesNotEnact(t *testing.T) {
	f := initFixture(t)
	account := f.addValidator(t, 1)

	id := proposeLowerThreshold(t, f, 100)
	_, err := f.ms.RatifyAmendment(f.at(200), &types.MsgRatifyAmendment{Validator: account, AmendmentId: id})
	require.NoError(t, err)

	_, err = f.ms.WithdrawAmendment(f.at(300), &types.MsgWithdrawAmendment{
		Authority: f.env.AuthorityString(t), AmendmentId: id,
	})
	require.NoError(t, err)

	amendment, err := f.keeper.Amendment.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.AMENDMENT_STATUS_WITHDRAWN, amendment.Status)

	require.NoError(t, f.keeper.EndBlocker(f.at(amendment.EffectiveAtHeight)))

	inv, err := f.keeper.GetInvariants(f.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(types.DefaultEnforcementThresholdBps), inv.EnforcementThresholdBps)
}
