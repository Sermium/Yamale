package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/validatorgov/types"
)

// RotateOperator is the planned path: the operator being replaced signs for
// their own replacement.
//
// Nothing beyond the signature is required of it. Anyone who can submit this
// already controls the validator, the commission and everything else the
// operator address protects, so a vote would protect nothing — while making
// deliberate rotation expensive enough that operators would put it off until
// the key was actually lost, which is the slow path this one exists to avoid.
func (k msgServer) RotateOperator(ctx context.Context, msg *types.MsgRotateOperator) (*types.MsgRotateOperatorResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}
	if err := k.validateNewOperator(ctx, msg.Creator, msg.NewOperator); err != nil {
		return nil, err
	}

	approved, err := k.ApprovedValidator.Has(ctx, msg.Creator)
	if err != nil {
		return nil, err
	}
	if !approved {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedValidator,
			"%s is not an approved validator operator, so there is nothing to rotate", msg.Creator)
	}

	if err := k.requireNoOpenRotation(ctx, msg.Creator); err != nil {
		return nil, err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()

	id, err := k.openRotation(ctx, types.OperatorRotation{
		CurrentOperator:   msg.Creator,
		NewOperator:       msg.NewOperator,
		Kind:              types.ROTATION_KIND_PLANNED,
		Status:            types.ROTATION_STATUS_PENDING,
		Proposer:          msg.Creator,
		OpenedAtHeight:    height,
		CompletesAtHeight: height + int64(params.PlannedDelay()),
	})
	if err != nil {
		return nil, err
	}

	return &types.MsgRotateOperatorResponse{RotationId: id}, nil
}

// CancelOperatorRotation withdraws a rotation before it takes effect. Only the
// operator being replaced may do it: for a planned rotation they are the one
// who asked, and for a recovery a signature from them ends it anyway.
func (k msgServer) CancelOperatorRotation(ctx context.Context, msg *types.MsgCancelOperatorRotation) (*types.MsgCancelOperatorRotationResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	rotation, err := k.Rotation.Get(ctx, msg.RotationId)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrRotationNotFound, "no rotation with id %d", msg.RotationId)
	}
	if rotation.Status != types.ROTATION_STATUS_PENDING {
		return nil, errorsmod.Wrapf(types.ErrRotationNotPending, "rotation %d is %s", rotation.Id, rotation.Status)
	}
	if rotation.CurrentOperator != msg.Creator {
		return nil, errorsmod.Wrapf(types.ErrNotCurrentOperator,
			"rotation %d replaces %s, not %s", rotation.Id, rotation.CurrentOperator, msg.Creator)
	}

	if err := k.closeRotation(ctx, &rotation, types.ROTATION_STATUS_CANCELLED); err != nil {
		return nil, err
	}

	return &types.MsgCancelOperatorRotationResponse{}, nil
}

// validateNewOperator rejects the two ways a rotation can name a destination
// that would break the module's one-record-per-operator invariant.
func (k Keeper) validateNewOperator(ctx context.Context, currentOperator, newOperator string) error {
	if _, err := k.addressCodec.StringToBytes(newOperator); err != nil {
		return errorsmod.Wrap(err, "invalid new operator address")
	}
	if newOperator == currentOperator {
		return errorsmod.Wrapf(types.ErrOperatorUnchanged, "%s", newOperator)
	}

	// Rotating onto an address that already operates a validator would collapse
	// two validators onto one allowlist entry, and completing it would delete
	// the destination's own approval on the way past.
	taken, err := k.ApprovedValidator.Has(ctx, newOperator)
	if err != nil {
		return err
	}
	if taken {
		return errorsmod.Wrapf(types.ErrOperatorInUse, "%s", newOperator)
	}

	return nil
}

// requireNoOpenRotation refuses a second rotation against an operator that
// already has one open. The pending index holds a single id per operator, so a
// second would overwrite the first — leaving a validator paused by a rotation
// nothing could ever resolve.
func (k Keeper) requireNoOpenRotation(ctx context.Context, operator string) error {
	existing, found, err := k.PendingRotationFor(ctx, operator)
	if err != nil {
		return err
	}
	if found {
		return errorsmod.Wrapf(types.ErrRotationInProgress,
			"rotation %d is already open against %s", existing.Id, operator)
	}
	return nil
}
