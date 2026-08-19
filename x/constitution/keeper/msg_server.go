package keeper

import (
	"bytes"
	"context"
	"strings"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/constitution/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// ProposeAmendment opens a change to the invariants and starts its public
// delay.
func (k msgServer) ProposeAmendment(ctx context.Context, msg *types.MsgProposeAmendment) (*types.MsgProposeAmendmentResponse, error) {
	if err := k.assertAuthority(msg.Authority); err != nil {
		return nil, err
	}
	if strings.TrimSpace(msg.Reason) == "" {
		return nil, errorsmod.Wrap(types.ErrInvariantViolation, "an amendment must state its grounds")
	}
	// Validated before it is stored, not at enactment. An amendment that turned
	// out to be invalid three weeks later would leave the chain with a pending
	// change nobody could act on and a queue entry that fails every block it is
	// due.
	if err := msg.Invariants.Validate(); err != nil {
		return nil, errorsmod.Wrap(err, "proposed invariants are not a settlement this chain can enforce")
	}

	current, err := k.GetInvariants(ctx)
	if err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()

	total, err := k.stakingKeeper.GetLastTotalPower(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := total.Int64()

	id, err := k.AmendmentSeq.Next(ctx)
	if err != nil {
		return nil, err
	}

	amendment := types.Amendment{
		Id:       id,
		Proposed: msg.Invariants,
		Reason:   msg.Reason,

		ProposedAtHeight: height,
		// Computed from the delay in force now, not from the delay being
		// proposed. An amendment that shortens the delay must not shorten its
		// own, or the first amendment anybody passes is the one that makes
		// every subsequent amendment instant.
		EffectiveAtHeight: height + int64(current.AmendmentDelayBlocks),

		SnapshotPower: snapshot,
		Status:        types.AMENDMENT_STATUS_PENDING,
	}

	if err := k.Amendment.Set(ctx, id, amendment); err != nil {
		return nil, err
	}
	if err := k.AmendmentQueue.Set(ctx, collections.Join(amendment.EffectiveAtHeight, id)); err != nil {
		return nil, err
	}

	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventAmendmentProposed{
		AmendmentId:       id,
		Reason:            msg.Reason,
		EffectiveAtHeight: amendment.EffectiveAtHeight,
		SnapshotPower:     snapshot,
		RequiredPower:     current.RequiredPower(snapshot),
	}); err != nil {
		return nil, err
	}

	return &types.MsgProposeAmendmentResponse{AmendmentId: id}, nil
}

// RatifyAmendment records one validator agreeing to a pending amendment.
func (k msgServer) RatifyAmendment(ctx context.Context, msg *types.MsgRatifyAmendment) (*types.MsgRatifyAmendmentResponse, error) {
	amendment, err := k.Amendment.Get(ctx, msg.AmendmentId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrAmendmentNotFound.Wrapf("amendment %d", msg.AmendmentId)
		}
		return nil, err
	}
	if amendment.Status != types.AMENDMENT_STATUS_PENDING {
		return nil, types.ErrAmendmentClosed.Wrapf("amendment %d is %s", msg.AmendmentId, amendment.Status)
	}

	operator, power, err := k.bondedValidatorOf(ctx, msg.Validator)
	if err != nil {
		return nil, err
	}

	key := collections.Join(msg.AmendmentId, operator)
	already, err := k.Ratification.Has(ctx, key)
	if err != nil {
		return nil, err
	}
	if already {
		return nil, types.ErrAlreadyRatified.Wrapf("%s on amendment %d", operator, msg.AmendmentId)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()

	if err := k.Ratification.Set(ctx, key, types.Ratification{
		AmendmentId: msg.AmendmentId,
		Validator:   operator,
		Power:       power,
		Height:      height,
	}); err != nil {
		return nil, err
	}

	amendment.RatifiedPower += power
	if err := k.Amendment.Set(ctx, amendment.Id, amendment); err != nil {
		return nil, err
	}

	current, err := k.GetInvariants(ctx)
	if err != nil {
		return nil, err
	}

	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventAmendmentRatified{
		AmendmentId:   amendment.Id,
		Validator:     operator,
		Power:         power,
		RatifiedPower: amendment.RatifiedPower,
		RequiredPower: current.RequiredPower(amendment.SnapshotPower),
	}); err != nil {
		return nil, err
	}

	return &types.MsgRatifyAmendmentResponse{}, nil
}

// WithdrawAmendment takes a pending amendment back before it takes effect.
func (k msgServer) WithdrawAmendment(ctx context.Context, msg *types.MsgWithdrawAmendment) (*types.MsgWithdrawAmendmentResponse, error) {
	if err := k.assertAuthority(msg.Authority); err != nil {
		return nil, err
	}

	amendment, err := k.Amendment.Get(ctx, msg.AmendmentId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrAmendmentNotFound.Wrapf("amendment %d", msg.AmendmentId)
		}
		return nil, err
	}
	if amendment.Status != types.AMENDMENT_STATUS_PENDING {
		return nil, types.ErrAmendmentClosed.Wrapf("amendment %d is %s", msg.AmendmentId, amendment.Status)
	}

	return &types.MsgWithdrawAmendmentResponse{}, k.resolve(ctx, &amendment, types.AMENDMENT_STATUS_WITHDRAWN)
}

// resolve closes an amendment and takes it out of the queue.
//
// The queue entry is removed here rather than left to expire, so that an
// amendment withdrawn a week before its effective height does not sit there
// being re-read every block until then.
func (k Keeper) resolve(ctx context.Context, amendment *types.Amendment, status types.AmendmentStatus) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	amendment.Status = status
	amendment.ResolvedAtHeight = sdkCtx.BlockHeight()
	if err := k.Amendment.Set(ctx, amendment.Id, *amendment); err != nil {
		return err
	}
	if err := k.AmendmentQueue.Remove(ctx, collections.Join(amendment.EffectiveAtHeight, amendment.Id)); err != nil {
		return err
	}

	current, err := k.GetInvariants(ctx)
	if err != nil {
		return err
	}

	return sdkCtx.EventManager().EmitTypedEvent(&types.EventAmendmentResolved{
		AmendmentId:   amendment.Id,
		Status:        status,
		RatifiedPower: amendment.RatifiedPower,
		RequiredPower: current.RequiredPower(amendment.SnapshotPower),
	})
}

func (k Keeper) assertAuthority(authority string) error {
	bz, err := k.addressCodec.StringToBytes(authority)
	if err != nil {
		return errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), bz) {
		expected, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expected, authority)
	}
	return nil
}

// bondedValidatorOf resolves the account a validator signs with to its operator
// address and the power it currently carries.
//
// Messages here are signed with the validator's account key rather than named
// by operator address, because that is the key a validator actually holds and
// the one every CLI and wallet knows how to use. The operator address is what
// gets recorded: a ratification attributed to an account nobody recognises is a
// signature on a constitutional change with no visible author.
func (k Keeper) bondedValidatorOf(ctx context.Context, account string) (string, int64, error) {
	bz, err := k.addressCodec.StringToBytes(account)
	if err != nil {
		return "", 0, errorsmod.Wrap(err, "invalid address")
	}

	operatorAddr := sdk.ValAddress(bz)
	validator, err := k.stakingKeeper.Validator(ctx, operatorAddr)
	if err != nil || validator == nil {
		return "", 0, types.ErrUnknownValidator.Wrapf("%s is not a validator", account)
	}
	if !validator.IsBonded() {
		return "", 0, types.ErrUnknownValidator.Wrapf("%s is not bonded", operatorAddr.String())
	}

	return operatorAddr.String(), validator.GetConsensusPower(sdk.DefaultPowerReduction), nil
}
