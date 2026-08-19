package keeper

import (
	"context"
	"errors"
	"strconv"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"yamale/blockchain/x/validatorgov/types"
)

// PendingRotationFor returns the rotation open against an operator, if any.
func (k Keeper) PendingRotationFor(ctx context.Context, operator string) (types.OperatorRotation, bool, error) {
	id, err := k.PendingRotation.Get(ctx, operator)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.OperatorRotation{}, false, nil
		}
		return types.OperatorRotation{}, false, err
	}

	rotation, err := k.Rotation.Get(ctx, id)
	if err != nil {
		return types.OperatorRotation{}, false, err
	}
	return rotation, true, nil
}

// nextRotationID hands out the next rotation id, and never hands out zero.
//
// InitGenesis seeds the sequence at one, but a collections.Sequence that was
// never seeded starts at zero, and in proto3 an id of zero is indistinguishable
// from an unset field. A rotation numbered zero would be a pending rotation
// that no query could name and no cancel message could reach, against a
// validator that had been paused by it.
func (k Keeper) nextRotationID(ctx context.Context) (uint64, error) {
	id, err := k.RotationSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	if id != 0 {
		return id, nil
	}
	return k.RotationSeq.Next(ctx)
}

// openRotation numbers a rotation, stores it, and indexes it against the
// operator it names. A rotation with a completion height already set — every
// planned one — is queued at the same time.
func (k Keeper) openRotation(ctx context.Context, rotation types.OperatorRotation) (uint64, error) {
	id, err := k.nextRotationID(ctx)
	if err != nil {
		return 0, err
	}
	rotation.Id = id

	if err := k.Rotation.Set(ctx, id, rotation); err != nil {
		return 0, err
	}
	if err := k.PendingRotation.Set(ctx, rotation.CurrentOperator, id); err != nil {
		return 0, err
	}
	if rotation.CompletesAtHeight > 0 {
		if err := k.RotationQueue.Set(ctx, collections.Join(rotation.CompletesAtHeight, id)); err != nil {
			return 0, err
		}
	}

	return id, sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventRotationProposed{
		RotationId:        id,
		CurrentOperator:   rotation.CurrentOperator,
		NewOperator:       rotation.NewOperator,
		Proposer:          rotation.Proposer,
		Kind:              rotation.Kind,
		Reason:            rotation.Reason,
		CompletesAtHeight: rotation.CompletesAtHeight,
	})
}

// closeRotation takes a rotation out of the indexes that make it open and
// records how it ended.
//
// The pause is lifted here rather than in each caller, so that every way a
// rotation can end — vetoed, cancelled, rejected, completed — gives the
// validator back. A path that forgot to would leave a validator jailed by a
// rotation that no longer exists, with nothing on the chain able to say why.
func (k Keeper) closeRotation(ctx context.Context, rotation *types.OperatorRotation, status types.RotationStatus) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if err := k.PendingRotation.Remove(ctx, rotation.CurrentOperator); err != nil {
		return err
	}
	if rotation.CompletesAtHeight > 0 {
		if err := k.RotationQueue.Remove(ctx, collections.Join(rotation.CompletesAtHeight, rotation.Id)); err != nil {
			return err
		}
	}
	if rotation.PausedValidator {
		if err := k.unpauseValidator(ctx, rotation.CurrentOperator); err != nil {
			return err
		}
		rotation.PausedValidator = false
	}

	rotation.Status = status
	rotation.ResolvedAtHeight = sdkCtx.BlockHeight()
	if err := k.Rotation.Set(ctx, rotation.Id, *rotation); err != nil {
		return err
	}

	return sdkCtx.EventManager().EmitTypedEvent(&types.EventRotationResolved{
		RotationId:      rotation.Id,
		CurrentOperator: rotation.CurrentOperator,
		NewOperator:     rotation.NewOperator,
		Kind:            rotation.Kind,
		Status:          status,
	})
}

// completeRotation makes the new address the operator of record.
func (k Keeper) completeRotation(ctx context.Context, rotation *types.OperatorRotation) error {
	// The allowlist entry moves rather than being duplicated. Leaving the old
	// address approved would let a rotated-away key create a second validator,
	// which is the one thing the admission gate exists to stop.
	if err := k.ApprovedValidator.Remove(ctx, rotation.CurrentOperator); err != nil {
		return err
	}
	if err := k.ApprovedValidator.Set(ctx, rotation.NewOperator, types.ApprovedValidator{
		Candidate: rotation.NewOperator,
		Approved:  strconv.FormatBool(true),
	}); err != nil {
		return err
	}

	// The application moves with it, keeping one record per operator. A
	// candidate left behind under the old address would still read as approved
	// to anyone querying applications rather than the allowlist.
	application, err := k.ValidatorApplication.Get(ctx, rotation.CurrentOperator)
	if err == nil {
		if err := k.ValidatorApplication.Remove(ctx, rotation.CurrentOperator); err != nil {
			return err
		}
		application.Candidate = rotation.NewOperator
		if err := k.ValidatorApplication.Set(ctx, rotation.NewOperator, application); err != nil {
			return err
		}
	} else if !errors.Is(err, collections.ErrNotFound) {
		return err
	}

	if err := k.grantOperatorAuthority(ctx, rotation.CurrentOperator, rotation.NewOperator); err != nil {
		return err
	}

	return k.closeRotation(ctx, rotation, types.ROTATION_STATUS_COMPLETED)
}

// grantOperatorAuthority authorises the incoming operator to send the messages
// that operate the outgoing operator's validator.
//
// This is what actually moves the signing. x/staking keys a validator record,
// and every delegation pointing at it, by the operator address the validator
// was created with, and the SDK has no operation that re-keys one — so the new
// address cannot become the validator's operator address without unwinding
// every delegation behind it, which is precisely the cost rotation exists to
// avoid. An authz grant moves the authority instead, and leaves the stake and
// the delegations exactly where they were.
func (k Keeper) grantOperatorAuthority(ctx context.Context, currentOperator, newOperator string) error {
	granter, err := k.addressCodec.StringToBytes(currentOperator)
	if err != nil {
		return err
	}
	grantee, err := k.addressCodec.StringToBytes(newOperator)
	if err != nil {
		return err
	}

	for _, msgTypeURL := range types.RotationGrantedMsgs {
		// No expiration. A grant that lapsed would silently take the validator
		// back off whoever is running it, at a moment nobody chose.
		if err := k.authzKeeper.SaveGrant(ctx, grantee, granter, authztypes.NewGenericAuthorization(msgTypeURL), nil); err != nil {
			return err
		}
	}

	return nil
}

// pauseValidator jails the validator behind an operator address, reporting
// whether this call is what jailed it.
//
// Jailing is the pause the guide describes: the validator leaves the active
// set, so it stops signing blocks and its power stops counting, while nothing
// is slashed and no delegation unbonds. Two cases deliberately do not pause and
// report false. An operator approved but never used has no validator to pause.
// A validator already jailed — for downtime, say — must not be recorded as
// paused by this rotation, because the rotation would then un-jail it when it
// ended and hand back a validator that consensus had put out for its own
// reasons.
func (k Keeper) pauseValidator(ctx context.Context, operator string) (bool, error) {
	validator, found, err := k.validatorFor(ctx, operator)
	if err != nil || !found || validator.IsJailed() {
		return false, err
	}

	consAddr, err := validator.GetConsAddr()
	if err != nil {
		return false, err
	}
	if err := k.stakingKeeper.Jail(ctx, sdk.ConsAddress(consAddr)); err != nil {
		return false, err
	}
	return true, nil
}

// unpauseValidator un-jails a validator this module jailed.
//
// x/staking panics on un-jailing a validator that is not jailed, so the check
// is not defensive tidiness: between the pause and here, the operator may have
// been vetoed, cancelled and un-jailed through x/slashing in the same window.
func (k Keeper) unpauseValidator(ctx context.Context, operator string) error {
	validator, found, err := k.validatorFor(ctx, operator)
	if err != nil || !found || !validator.IsJailed() {
		return err
	}

	consAddr, err := validator.GetConsAddr()
	if err != nil {
		return err
	}
	return k.stakingKeeper.Unjail(ctx, sdk.ConsAddress(consAddr))
}

// validatorFor resolves the staking record behind an operator address. An
// operator that was approved but never created a validator is an ordinary
// outcome, not an error.
func (k Keeper) validatorFor(ctx context.Context, operator string) (stakingtypes.Validator, bool, error) {
	addr, err := k.addressCodec.StringToBytes(operator)
	if err != nil {
		return stakingtypes.Validator{}, false, err
	}

	validator, err := k.stakingKeeper.GetValidator(ctx, sdk.ValAddress(addr))
	if err != nil {
		if errors.Is(err, stakingtypes.ErrNoValidatorFound) {
			return stakingtypes.Validator{}, false, nil
		}
		return stakingtypes.Validator{}, false, err
	}
	return validator, true, nil
}

// VetoBySignature ends any recovery open against a signer.
//
// This is the rule that makes the recovery path safe to offer at all: a
// recovery says the operator's key is gone, and one signature by that key is
// the complete disproof. It is checked against every signer of every
// transaction rather than against a particular message, because requiring a
// specific message would mean an operator who did not know the rule was being
// applied — the ordinary case, since they lost nothing and were not watching —
// would fail to invoke it while transacting normally all week.
//
// Planned rotations are untouched: the operator signed that one themselves, and
// treating their next transaction as an objection to their own request would
// make a planned rotation impossible to actually carry out.
func (k Keeper) VetoBySignature(ctx context.Context, signer string) error {
	rotation, found, err := k.PendingRotationFor(ctx, signer)
	if err != nil || !found {
		return err
	}
	if rotation.Kind != types.ROTATION_KIND_RECOVERY {
		return nil
	}

	return k.closeRotation(ctx, &rotation, types.ROTATION_STATUS_VETOED)
}
