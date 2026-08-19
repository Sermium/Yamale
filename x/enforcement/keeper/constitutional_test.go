package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	constitutiontypes "yamale/blockchain/x/constitution/types"
	"yamale/blockchain/x/enforcement/types"
)

// A governance proposal that passed is not a licence to move the numbers this
// chain fixed at genesis.
//
// The four here are the ones a set that wanted to act would have an interest in
// moving in the moment it wanted to act: the threshold that decides whether
// assets are taken, the address they go to, and the two delays that make the
// decision answerable. Every one of them reads as housekeeping in a proposal.
func TestUpdateParamsRefusesToChangeAnInvariant(t *testing.T) {
	cases := map[string]struct {
		mutate func(*types.Params)
		expect string
	}{
		"the seizure threshold": {
			func(p *types.Params) { p.ThresholdBps = 5_100 },
			"threshold_bps is fixed at",
		},
		"where seized assets go": {
			func(p *types.Params) { p.RecoveryDestination = otherDestination },
			"recovery_destination is fixed at",
		},
		"how long validators have to vote": {
			func(p *types.Params) { p.VotingPeriodBlocks = 10 },
			"voting_period_blocks is fixed at",
		},
		"how long a provisional freeze lasts": {
			func(p *types.Params) { p.ProvisionalFreezeBlocks = types.DefaultVotingPeriodBlocks },
			"provisional_freeze_blocks is fixed at",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := initFixture(t)

			params, err := f.keeper.Params.Get(f.ctx)
			require.NoError(t, err)
			tc.mutate(&params)

			_, err = f.ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
				Authority: f.env.AuthorityString(t),
				Params:    params,
			})
			require.ErrorIs(t, err, constitutiontypes.ErrInvariantViolation)
			require.ErrorContains(t, err, tc.expect)

			stored, err := f.keeper.Params.Get(f.ctx)
			require.NoError(t, err)
			require.Equal(t, uint64(types.DefaultThresholdBps), stored.ThresholdBps)
			require.Equal(t, testRecoveryDestination, stored.RecoveryDestination)
		})
	}
}

// The parameters that are not constitutional still move by proposal. A module
// where every parameter was frozen would not be a constitution, it would be a
// binary.
func TestUpdateParamsStillChangesWhatIsNotFixed(t *testing.T) {
	f := initFixture(t)

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	params.MaxReasonLength = 1_024
	params.SeizeRequiresEvidence = false

	_, err = f.ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.env.AuthorityString(t),
		Params:    params,
	})
	require.NoError(t, err)

	stored, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(1_024), stored.MaxReasonLength)
	require.False(t, stored.SeizeRequiresEvidence)
}

// A chain whose genesis says one thing about the seizure threshold and whose
// constitution says another has two answers to the only question that matters
// about this module. It is better for it not to start.
func TestInitGenesisRefusesParamsThatDisagreeWithTheConstitution(t *testing.T) {
	f := initFixture(t)

	genesis := types.DefaultGenesis()
	genesis.Params.RecoveryDestination = testRecoveryDestination
	genesis.Params.ThresholdBps = 9_000

	require.ErrorContains(t, f.keeper.InitGenesis(f.ctx, *genesis),
		"disagrees with this chain's constitution")
}
