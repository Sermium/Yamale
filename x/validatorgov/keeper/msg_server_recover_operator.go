package keeper

import (
	"bytes"
	"context"
	"strings"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/validatorgov/types"
)

// ProposeOperatorRecovery opens a claim that an operator's key is gone.
//
// Anybody may open one, because the person best placed to notice is rarely the
// person who lost the key. What stops that being a weapon is that opening one
// does nothing: the validator is not paused, no clock runs, and nothing takes
// effect until the admission quorum agrees. Pausing on the proposal instead
// would let one transaction from any address on the chain stop any validator
// on it.
func (k msgServer) ProposeOperatorRecovery(ctx context.Context, msg *types.MsgProposeOperatorRecovery) (*types.MsgProposeOperatorRecoveryResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}
	if _, err := k.addressCodec.StringToBytes(msg.CurrentOperator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid current operator address")
	}
	if err := k.validateNewOperator(ctx, msg.CurrentOperator, msg.NewOperator); err != nil {
		return nil, err
	}
	if strings.TrimSpace(msg.Reason) == "" {
		return nil, types.ErrMissingReason
	}

	approved, err := k.ApprovedValidator.Has(ctx, msg.CurrentOperator)
	if err != nil {
		return nil, err
	}
	if !approved {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedValidator,
			"%s is not an approved validator operator, so there is nothing to recover", msg.CurrentOperator)
	}

	if err := k.requireNoOpenRotation(ctx, msg.CurrentOperator); err != nil {
		return nil, err
	}

	// CompletesAtHeight is left unset on purpose. An unapproved recovery has no
	// clock, and writing a height here would put it in the end blocker's queue
	// where it would take effect without anybody having agreed to it.
	id, err := k.openRotation(ctx, types.OperatorRotation{
		CurrentOperator: msg.CurrentOperator,
		NewOperator:     msg.NewOperator,
		Kind:            types.ROTATION_KIND_RECOVERY,
		Status:          types.ROTATION_STATUS_PENDING,
		Proposer:        msg.Creator,
		Reason:          msg.Reason,
		OpenedAtHeight:  sdk.UnwrapSDKContext(ctx).BlockHeight(),
	})
	if err != nil {
		return nil, err
	}

	return &types.MsgProposeOperatorRecoveryResponse{RotationId: id}, nil
}

// ApproveOperatorRecovery is the admission quorum's decision, executed by the
// governance module account exactly as MsgApproveValidator is. Recovering a
// validator is gated by the same offices that admitted it, because the two
// decisions are the same decision: who is allowed to hold this validator.
//
// Approval is the moment everything starts — the pause, the challenge window,
// and the delegators' chance to leave — so it is also the moment the module
// makes the most noise.
func (k msgServer) ApproveOperatorRecovery(ctx context.Context, msg *types.MsgApproveOperatorRecovery) (*types.MsgApproveOperatorRecoveryResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authority) {
		expectedAuthorityStr, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expectedAuthorityStr, msg.Authority)
	}

	rotation, err := k.Rotation.Get(ctx, msg.RotationId)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrRotationNotFound, "no rotation with id %d", msg.RotationId)
	}
	if rotation.Kind != types.ROTATION_KIND_RECOVERY {
		return nil, errorsmod.Wrapf(types.ErrRotationNotRecovery,
			"rotation %d was signed by its own operator and needs no approval", rotation.Id)
	}
	if rotation.Status != types.ROTATION_STATUS_PENDING {
		return nil, errorsmod.Wrapf(types.ErrRotationNotPending, "rotation %d is %s", rotation.Id, rotation.Status)
	}
	if rotation.Approved {
		return nil, errorsmod.Wrapf(types.ErrRecoveryAlreadyDecided,
			"rotation %d is already approved and its challenge window ends at height %d", rotation.Id, rotation.CompletesAtHeight)
	}

	if !msg.Approve {
		if err := k.closeRotation(ctx, &rotation, types.ROTATION_STATUS_REJECTED); err != nil {
			return nil, err
		}
		return &types.MsgApproveOperatorRecoveryResponse{}, nil
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	paused, err := k.pauseValidator(ctx, rotation.CurrentOperator)
	if err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	rotation.Approved = true
	rotation.PausedValidator = paused
	rotation.CompletesAtHeight = sdkCtx.BlockHeight() + int64(params.ChallengeWindow())

	if err := k.Rotation.Set(ctx, rotation.Id, rotation); err != nil {
		return nil, err
	}
	if err := k.RotationQueue.Set(ctx, collections.Join(rotation.CompletesAtHeight, rotation.Id)); err != nil {
		return nil, err
	}

	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventRecoveryApproved{
		RotationId:        rotation.Id,
		CurrentOperator:   rotation.CurrentOperator,
		NewOperator:       rotation.NewOperator,
		CompletesAtHeight: rotation.CompletesAtHeight,
		ValidatorPaused:   paused,
	}); err != nil {
		return nil, err
	}

	return &types.MsgApproveOperatorRecoveryResponse{}, nil
}
