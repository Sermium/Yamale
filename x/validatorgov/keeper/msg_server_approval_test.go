package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/validatorgov/keeper"
	"yamale/blockchain/x/validatorgov/types"
)

func TestApplyValidatorRecordsPendingApplication(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, candidateStr := f.env.Addr(t)

	_, err := ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{
		Creator: candidateStr, Moniker: "node-1", Description: "a candidate",
	})
	require.NoError(t, err)

	app, err := f.keeper.ValidatorApplication.Get(f.ctx, candidateStr)
	require.NoError(t, err)
	require.Equal(t, types.StatusPending, app.Status)
	require.Equal(t, candidateStr, app.Candidate)

	// Applying does not by itself put the candidate on the allowlist.
	has, err := f.keeper.ApprovedValidator.Has(f.ctx, candidateStr)
	require.NoError(t, err)
	require.False(t, has)
}

func TestApplyValidatorRejectsInvalidCreator(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, err := ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{Creator: "not-an-address"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid creator address")
}

// A rejected candidate may re-apply: ApplyValidator overwrites the previous
// application, putting it back into the pending state for a fresh vote.
func TestApplyValidatorAfterRejectionResetsToPending(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, candidateStr := f.env.Addr(t)
	_, err := ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{Creator: candidateStr})
	require.NoError(t, err)

	_, err = ms.ApproveValidator(f.ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: candidateStr, Approve: false,
	})
	require.NoError(t, err)

	_, err = ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{Creator: candidateStr})
	require.NoError(t, err)

	app, err := f.keeper.ValidatorApplication.Get(f.ctx, candidateStr)
	require.NoError(t, err)
	require.Equal(t, types.StatusPending, app.Status)
}

// Re-applying does not remove an already-approved candidate from the
// allowlist; approval is revoked only by the staking module's own mechanisms.
func TestApplyValidatorDoesNotRevokeExistingApproval(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, candidateStr := f.env.Addr(t)
	_, err := ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{Creator: candidateStr})
	require.NoError(t, err)
	_, err = ms.ApproveValidator(f.ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: candidateStr, Approve: true,
	})
	require.NoError(t, err)

	_, err = ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{Creator: candidateStr})
	require.NoError(t, err)

	has, err := f.keeper.ApprovedValidator.Has(f.ctx, candidateStr)
	require.NoError(t, err)
	require.True(t, has)
}

func TestApproveValidatorRequiresGovAuthority(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, candidateStr := f.env.Addr(t)
	_, err := ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{Creator: candidateStr})
	require.NoError(t, err)

	// A candidate cannot approve themself onto the validator set.
	_, err = ms.ApproveValidator(f.ctx, &types.MsgApproveValidator{
		Authority: candidateStr, Candidate: candidateStr, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	has, err := f.keeper.ApprovedValidator.Has(f.ctx, candidateStr)
	require.NoError(t, err)
	require.False(t, has)
}

func TestApproveValidatorRejectsInvalidAuthority(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, err := ms.ApproveValidator(f.ctx, &types.MsgApproveValidator{
		Authority: "garbage", Candidate: "irrelevant", Approve: true,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid authority address")
}

func TestApproveValidatorAddsToAllowlist(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, candidateStr := f.env.Addr(t)
	_, err := ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{Creator: candidateStr})
	require.NoError(t, err)

	_, err = ms.ApproveValidator(f.ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: candidateStr, Approve: true,
	})
	require.NoError(t, err)

	approved, err := f.keeper.ApprovedValidator.Get(f.ctx, candidateStr)
	require.NoError(t, err)
	require.Equal(t, candidateStr, approved.Candidate)
	require.Equal(t, "true", approved.Approved)

	app, err := f.keeper.ValidatorApplication.Get(f.ctx, candidateStr)
	require.NoError(t, err)
	require.Equal(t, types.StatusApproved, app.Status)
}

func TestApproveValidatorRejection(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, candidateStr := f.env.Addr(t)
	_, err := ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{Creator: candidateStr})
	require.NoError(t, err)

	_, err = ms.ApproveValidator(f.ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: candidateStr, Approve: false,
	})
	require.NoError(t, err)

	app, err := f.keeper.ValidatorApplication.Get(f.ctx, candidateStr)
	require.NoError(t, err)
	require.Equal(t, types.StatusRejected, app.Status)

	has, err := f.keeper.ApprovedValidator.Has(f.ctx, candidateStr)
	require.NoError(t, err)
	require.False(t, has)
}

func TestApproveValidatorRejectsNonPendingApplication(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, candidateStr := f.env.Addr(t)
	_, err := ms.ApplyValidator(f.ctx, &types.MsgApplyValidator{Creator: candidateStr})
	require.NoError(t, err)
	_, err = ms.ApproveValidator(f.ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: candidateStr, Approve: true,
	})
	require.NoError(t, err)

	// Approving twice must not go through.
	_, err = ms.ApproveValidator(f.ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: candidateStr, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrApplicationNotPending)
}

func TestApproveValidatorUnknownApplication(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, strangerStr := f.env.Addr(t)
	_, err := ms.ApproveValidator(f.ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: strangerStr, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrApplicationNotFound)
}
