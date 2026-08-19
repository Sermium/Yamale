package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/validatorgov/keeper"
	"yamale/blockchain/x/validatorgov/types"
)

// approveOperator runs the admission flow and returns the operator's account
// address in both forms, because rotation state is keyed by the bech32 account
// address while the staking record behind it is keyed by the operator address
// over the same bytes.
func (f *fixture) approveOperator(t *testing.T) (sdk.AccAddress, string) {
	t.Helper()

	ms := keeper.NewMsgServerImpl(f.keeper)
	addr, addrStr := f.env.Addr(t)

	_, err := ms.ApplyValidator(f.env.Ctx, &types.MsgApplyValidator{Creator: addrStr, LegalEntityId: "LEI-TEST", BeneficialOwnerId: "OWNER-TEST", Jurisdiction: "CH"})
	require.NoError(t, err)
	_, err = ms.ApproveValidator(f.env.Ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: addrStr, Approve: true,
	})
	require.NoError(t, err)

	return addr, addrStr
}

// at moves the fixture to a block height and returns the context there. The
// fixture's own context moves too, so a test reads consistently afterwards.
func (f *fixture) at(height int64) sdk.Context {
	f.env.Ctx = f.env.Ctx.WithBlockHeight(height)
	f.ctx = f.env.Ctx
	return f.env.Ctx
}

func (f *fixture) rotation(t *testing.T, id uint64) types.OperatorRotation {
	t.Helper()
	rotation, err := f.keeper.Rotation.Get(f.env.Ctx, id)
	require.NoError(t, err)
	return rotation
}

func TestPlannedRotationTakesEffectOnlyAfterTheDelay(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	_, operator := f.approveOperator(t)
	_, incoming := f.env.Addr(t)

	resp, err := ms.RotateOperator(f.env.Ctx, &types.MsgRotateOperator{
		Creator: operator, NewOperator: incoming,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), resp.RotationId, "rotation ids are numbered from one")

	params, err := f.keeper.Params.Get(f.env.Ctx)
	require.NoError(t, err)
	completesAt := int64(1) + int64(params.PlannedRotationDelayBlocks)
	require.Equal(t, completesAt, f.rotation(t, resp.RotationId).CompletesAtHeight)

	// One block before the delay is up, nothing has moved.
	require.NoError(t, f.keeper.EndBlocker(f.at(completesAt-1)))
	require.Equal(t, types.ROTATION_STATUS_PENDING, f.rotation(t, resp.RotationId).Status)

	stillApproved, err := f.keeper.ApprovedValidator.Has(f.env.Ctx, operator)
	require.NoError(t, err)
	require.True(t, stillApproved, "the outgoing operator keeps its approval until the delay is up")
	notYet, err := f.keeper.ApprovedValidator.Has(f.env.Ctx, incoming)
	require.NoError(t, err)
	require.False(t, notYet, "the incoming operator is not approved before the delay is up")

	// At the completion height it takes effect.
	require.NoError(t, f.keeper.EndBlocker(f.at(completesAt)))
	require.Equal(t, types.ROTATION_STATUS_COMPLETED, f.rotation(t, resp.RotationId).Status)

	gone, err := f.keeper.ApprovedValidator.Has(f.env.Ctx, operator)
	require.NoError(t, err)
	require.False(t, gone, "the allowlist entry moves rather than being duplicated")
	nowApproved, err := f.keeper.ApprovedValidator.Has(f.env.Ctx, incoming)
	require.NoError(t, err)
	require.True(t, nowApproved)

	// The application moves with it, so there is still exactly one record.
	_, err = f.keeper.ValidatorApplication.Get(f.env.Ctx, incoming)
	require.NoError(t, err)
	oldApplication, err := f.keeper.ValidatorApplication.Has(f.env.Ctx, operator)
	require.NoError(t, err)
	require.False(t, oldApplication)

	// The incoming operator can now sign for the validator without any
	// delegation having moved.
	require.ElementsMatch(t, types.RotationGrantedMsgs, f.authz.GrantedTypes(incoming, operator))

	_, open, err := f.keeper.PendingRotationFor(f.env.Ctx, operator)
	require.NoError(t, err)
	require.False(t, open)
}

func TestRotationByANonOperatorIsRefused(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	_, operator := f.approveOperator(t)
	_, stranger := f.env.Addr(t)
	_, incoming := f.env.Addr(t)

	// The planned path is signed by the operator being replaced, so somebody
	// who is not an approved operator has nothing to rotate.
	_, err := ms.RotateOperator(f.env.Ctx, &types.MsgRotateOperator{
		Creator: stranger, NewOperator: incoming,
	})
	require.ErrorIs(t, err, types.ErrNotApprovedValidator)

	// And a stranger cannot reach the operator's own rotation through the
	// cancel path either.
	resp, err := ms.RotateOperator(f.env.Ctx, &types.MsgRotateOperator{
		Creator: operator, NewOperator: incoming,
	})
	require.NoError(t, err)

	_, err = ms.CancelOperatorRotation(f.env.Ctx, &types.MsgCancelOperatorRotation{
		Creator: stranger, RotationId: resp.RotationId,
	})
	require.ErrorIs(t, err, types.ErrNotCurrentOperator)
	require.Equal(t, types.ROTATION_STATUS_PENDING, f.rotation(t, resp.RotationId).Status)
}

func TestRecoveryWithoutQuorumNeverCompletes(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	_, operator := f.approveOperator(t)
	f.staking.AddValidator(mustAddr(t, f, operator))
	_, incoming := f.env.Addr(t)

	resp, err := ms.ProposeOperatorRecovery(f.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: incoming, CurrentOperator: operator, NewOperator: incoming, Reason: "the phone went into the sea",
	})
	require.NoError(t, err)

	proposed := f.rotation(t, resp.RotationId)
	require.False(t, proposed.Approved)
	require.Zero(t, proposed.CompletesAtHeight, "an unapproved recovery runs no clock")
	require.False(t, f.staking.IsJailed(mustAddr(t, f, operator)),
		"a proposal nobody has agreed to must not pause a validator, or one transaction would stop any validator on the chain")

	// Anybody other than the admission authority is refused.
	_, err = ms.ApproveOperatorRecovery(f.env.Ctx, &types.MsgApproveOperatorRecovery{
		Authority: incoming, RotationId: resp.RotationId, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	// Long past any window, an unapproved recovery has still not taken effect.
	params, err := f.keeper.Params.Get(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, f.keeper.EndBlocker(f.at(int64(params.RecoveryChallengeWindowBlocks)*2)))

	require.Equal(t, types.ROTATION_STATUS_PENDING, f.rotation(t, resp.RotationId).Status)
	stillTheirs, err := f.keeper.ApprovedValidator.Has(f.env.Ctx, operator)
	require.NoError(t, err)
	require.True(t, stillTheirs)
	notTaken, err := f.keeper.ApprovedValidator.Has(f.env.Ctx, incoming)
	require.NoError(t, err)
	require.False(t, notTaken)
}

func TestRecoveryWithQuorumCompletesAfterTheWindow(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	operatorAddr, operator := f.approveOperator(t)
	f.staking.AddValidator(operatorAddr)
	_, incoming := f.env.Addr(t)

	resp, err := ms.ProposeOperatorRecovery(f.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: incoming, CurrentOperator: operator, NewOperator: incoming, Reason: "the operator died",
	})
	require.NoError(t, err)

	approvedAt := int64(10)
	f.at(approvedAt)
	_, err = ms.ApproveOperatorRecovery(f.env.Ctx, &types.MsgApproveOperatorRecovery{
		Authority: f.env.AuthorityString(t), RotationId: resp.RotationId, Approve: true,
	})
	require.NoError(t, err)

	params, err := f.keeper.Params.Get(f.env.Ctx)
	require.NoError(t, err)
	completesAt := approvedAt + int64(params.RecoveryChallengeWindowBlocks)
	require.Equal(t, completesAt, f.rotation(t, resp.RotationId).CompletesAtHeight)

	// One block short of the window, the validator is still the outgoing
	// operator's.
	require.NoError(t, f.keeper.EndBlocker(f.at(completesAt-1)))
	require.Equal(t, types.ROTATION_STATUS_PENDING, f.rotation(t, resp.RotationId).Status)

	require.NoError(t, f.keeper.EndBlocker(f.at(completesAt)))
	require.Equal(t, types.ROTATION_STATUS_COMPLETED, f.rotation(t, resp.RotationId).Status)

	nowApproved, err := f.keeper.ApprovedValidator.Has(f.env.Ctx, incoming)
	require.NoError(t, err)
	require.True(t, nowApproved)
	require.ElementsMatch(t, types.RotationGrantedMsgs, f.authz.GrantedTypes(incoming, operator))
}

func TestOldKeySigningDuringTheWindowVetoesTheRecovery(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	operatorAddr, operator := f.approveOperator(t)
	f.staking.AddValidator(operatorAddr)
	_, incoming := f.env.Addr(t)

	resp, err := ms.ProposeOperatorRecovery(f.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: incoming, CurrentOperator: operator, NewOperator: incoming, Reason: "unreachable for weeks",
	})
	require.NoError(t, err)

	f.at(10)
	_, err = ms.ApproveOperatorRecovery(f.env.Ctx, &types.MsgApproveOperatorRecovery{
		Authority: f.env.AuthorityString(t), RotationId: resp.RotationId, Approve: true,
	})
	require.NoError(t, err)
	require.True(t, f.staking.IsJailed(operatorAddr))

	// The key that was said to be lost signs something. It does not matter
	// what: possession is the whole disproof.
	f.at(20)
	require.NoError(t, f.keeper.VetoBySignature(f.env.Ctx, operator))

	vetoed := f.rotation(t, resp.RotationId)
	require.Equal(t, types.ROTATION_STATUS_VETOED, vetoed.Status)
	require.Equal(t, int64(20), vetoed.ResolvedAtHeight)
	require.False(t, f.staking.IsJailed(operatorAddr), "the pause lifts with the recovery it belonged to")

	// The window running out afterwards must not resurrect it.
	params, err := f.keeper.Params.Get(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, f.keeper.EndBlocker(f.at(10+int64(params.RecoveryChallengeWindowBlocks))))

	require.Equal(t, types.ROTATION_STATUS_VETOED, f.rotation(t, resp.RotationId).Status)
	stillTheirs, err := f.keeper.ApprovedValidator.Has(f.env.Ctx, operator)
	require.NoError(t, err)
	require.True(t, stillTheirs)
	notTaken, err := f.keeper.ApprovedValidator.Has(f.env.Ctx, incoming)
	require.NoError(t, err)
	require.False(t, notTaken)
	require.Empty(t, f.authz.GrantedTypes(incoming, operator))
}

// TestVetoWorksBeforeTheRecoveryIsApproved covers the operator who notices
// early. The veto is live from the moment a recovery is proposed, not from the
// moment it is approved: an operator who has to wait for a quorum to act before
// they are allowed to object is being made to watch.
func TestVetoWorksBeforeTheRecoveryIsApproved(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	operatorAddr, operator := f.approveOperator(t)
	f.staking.AddValidator(operatorAddr)
	_, incoming := f.env.Addr(t)

	resp, err := ms.ProposeOperatorRecovery(f.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: incoming, CurrentOperator: operator, NewOperator: incoming, Reason: "gone",
	})
	require.NoError(t, err)

	require.NoError(t, f.keeper.VetoBySignature(f.at(2), operator))
	require.Equal(t, types.ROTATION_STATUS_VETOED, f.rotation(t, resp.RotationId).Status)
	require.False(t, f.staking.IsJailed(operatorAddr))

	// And the quorum can no longer approve what the key already disproved.
	_, err = ms.ApproveOperatorRecovery(f.env.Ctx, &types.MsgApproveOperatorRecovery{
		Authority: f.env.AuthorityString(t), RotationId: resp.RotationId, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrRotationNotPending)
}

func TestPlannedRotationIsNotVetoedByItsOwnSigner(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	_, operator := f.approveOperator(t)
	_, incoming := f.env.Addr(t)

	resp, err := ms.RotateOperator(f.env.Ctx, &types.MsgRotateOperator{
		Creator: operator, NewOperator: incoming,
	})
	require.NoError(t, err)

	// The operator asked for this one. Treating their next transaction as an
	// objection would make a planned rotation impossible to carry out at all.
	require.NoError(t, f.keeper.VetoBySignature(f.at(2), operator))
	require.Equal(t, types.ROTATION_STATUS_PENDING, f.rotation(t, resp.RotationId).Status)
}

func TestValidatorIsPausedForARecoveryAndRestoredAfterwards(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	operatorAddr, operator := f.approveOperator(t)
	f.staking.AddValidator(operatorAddr)
	_, incoming := f.env.Addr(t)

	require.False(t, f.staking.IsJailed(operatorAddr))

	resp, err := ms.ProposeOperatorRecovery(f.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: incoming, CurrentOperator: operator, NewOperator: incoming, Reason: "lost in a fire",
	})
	require.NoError(t, err)
	require.False(t, f.staking.IsJailed(operatorAddr), "a proposal alone pauses nothing")

	f.at(10)
	_, err = ms.ApproveOperatorRecovery(f.env.Ctx, &types.MsgApproveOperatorRecovery{
		Authority: f.env.AuthorityString(t), RotationId: resp.RotationId, Approve: true,
	})
	require.NoError(t, err)

	require.True(t, f.staking.IsJailed(operatorAddr), "a validator whose ownership is contested does not sign blocks")
	require.True(t, f.rotation(t, resp.RotationId).PausedValidator)

	params, err := f.keeper.Params.Get(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, f.keeper.EndBlocker(f.at(10+int64(params.RecoveryChallengeWindowBlocks))))

	require.Equal(t, types.ROTATION_STATUS_COMPLETED, f.rotation(t, resp.RotationId).Status)
	require.False(t, f.staking.IsJailed(operatorAddr), "the validator goes back to work under its new operator")
	require.False(t, f.rotation(t, resp.RotationId).PausedValidator)
}

func TestRecoveryDoesNotUnjailAValidatorItDidNotPause(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	operatorAddr, operator := f.approveOperator(t)
	validator := f.staking.AddValidator(operatorAddr)
	_, incoming := f.env.Addr(t)

	// Jailed for downtime before anybody proposed anything.
	consAddr, err := validator.GetConsAddr()
	require.NoError(t, err)
	require.NoError(t, f.staking.Jail(f.env.Ctx, sdk.ConsAddress(consAddr)))

	resp, err := ms.ProposeOperatorRecovery(f.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: incoming, CurrentOperator: operator, NewOperator: incoming, Reason: "gone",
	})
	require.NoError(t, err)

	f.at(10)
	_, err = ms.ApproveOperatorRecovery(f.env.Ctx, &types.MsgApproveOperatorRecovery{
		Authority: f.env.AuthorityString(t), RotationId: resp.RotationId, Approve: true,
	})
	require.NoError(t, err)
	require.False(t, f.rotation(t, resp.RotationId).PausedValidator,
		"this rotation did not jail it, so this rotation must not be recorded as having")

	params, err := f.keeper.Params.Get(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, f.keeper.EndBlocker(f.at(10+int64(params.RecoveryChallengeWindowBlocks))))

	require.True(t, f.staking.IsJailed(operatorAddr),
		"a rotation must not clear a jailing that consensus imposed for its own reasons")
}

func TestASecondRotationAgainstTheSameOperatorIsRefused(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.at(1)
	_, operator := f.approveOperator(t)
	_, first := f.env.Addr(t)
	_, second := f.env.Addr(t)

	_, err := ms.RotateOperator(f.env.Ctx, &types.MsgRotateOperator{Creator: operator, NewOperator: first})
	require.NoError(t, err)

	_, err = ms.RotateOperator(f.env.Ctx, &types.MsgRotateOperator{Creator: operator, NewOperator: second})
	require.ErrorIs(t, err, types.ErrRotationInProgress)

	_, err = ms.ProposeOperatorRecovery(f.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: second, CurrentOperator: operator, NewOperator: second, Reason: "gone",
	})
	require.ErrorIs(t, err, types.ErrRotationInProgress)
}

// TestZeroParamsStillLeaveADelay writes a Params of zeros straight into the
// store, the way a hand-written genesis or an upgrade that added the fields
// would, and checks that neither path completes in the block it opened in.
//
// Params.Validate() rejects these values, but Validate() only runs on the
// genesis and on messages that pass through it. The delays are read on the
// paths that decide when a validator's key changes hands, and this chain has
// been halted before by a parameter reaching such a path as a zero.
func TestZeroParamsStillLeaveADelay(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	require.NoError(t, f.keeper.Params.Set(f.env.Ctx, types.Params{}))

	f.at(1)
	_, planned := f.approveOperator(t)
	_, plannedTo := f.env.Addr(t)
	rotation, err := ms.RotateOperator(f.env.Ctx, &types.MsgRotateOperator{
		Creator: planned, NewOperator: plannedTo,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1)+int64(types.DefaultPlannedRotationDelayBlocks),
		f.rotation(t, rotation.RotationId).CompletesAtHeight)

	_, contested := f.approveOperator(t)
	_, contestedTo := f.env.Addr(t)
	recovery, err := ms.ProposeOperatorRecovery(f.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: contestedTo, CurrentOperator: contested, NewOperator: contestedTo, Reason: "gone",
	})
	require.NoError(t, err)
	_, err = ms.ApproveOperatorRecovery(f.env.Ctx, &types.MsgApproveOperatorRecovery{
		Authority: f.env.AuthorityString(t), RotationId: recovery.RotationId, Approve: true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1)+int64(types.DefaultRecoveryChallengeWindowBlocks),
		f.rotation(t, recovery.RotationId).CompletesAtHeight)

	// Neither has taken effect in the block it was opened in.
	require.NoError(t, f.keeper.EndBlocker(f.env.Ctx))
	require.Equal(t, types.ROTATION_STATUS_PENDING, f.rotation(t, rotation.RotationId).Status)
	require.Equal(t, types.ROTATION_STATUS_PENDING, f.rotation(t, recovery.RotationId).Status)
}

func mustAddr(t *testing.T, f *fixture, bech32 string) sdk.AccAddress {
	t.Helper()
	bz, err := f.addressCodec.StringToBytes(bech32)
	require.NoError(t, err)
	return bz
}
