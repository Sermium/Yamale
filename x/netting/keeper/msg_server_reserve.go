package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/netting/types"
)

// PostReserve moves coins from the participant's own balance into the module
// account, where they become that participant's settlement reserve.
//
// This is the prefunding that makes deferred settlement safe. Everything a
// participant is permitted to owe across unsettled windows is bounded by what
// it has posted here, so at the moment a window closes the money to discharge
// it is already in this module's custody. That is what a settlement system
// gives up unwinding risk to get: CHIPS runs on the same principle, and it is
// why it has never had to recompute a cycle around a participant that could
// not pay.
func (k msgServer) PostReserve(ctx context.Context, msg *types.MsgPostReserve) (*types.MsgPostReserveResponse, error) {
	if !msg.Amount.IsValid() || !msg.Amount.IsAllPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "amount %s", msg.Amount)
	}

	participantBz, err := k.addressCodec.StringToBytes(msg.Participant)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid participant address")
	}

	if err := k.assertApproved(ctx, msg.Participant); err != nil {
		return nil, err
	}

	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sdk.AccAddress(participantBz), types.ModuleName, msg.Amount); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	for _, coin := range msg.Amount {
		reserve, err := k.GetReserve(ctx, msg.Participant, coin.Denom)
		if err != nil {
			return nil, err
		}
		reserve = reserve.Add(coin.Amount)
		if err := k.setReserve(ctx, msg.Participant, coin.Denom, reserve); err != nil {
			return nil, err
		}
		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventReserveChanged{
			Participant: msg.Participant,
			Denom:       coin.Denom,
			Amount:      reserve,
			Deposited:   true,
		}); err != nil {
			return nil, err
		}
	}

	return &types.MsgPostReserveResponse{}, nil
}

// WithdrawReserve returns the uncommitted part of a reserve to the
// participant's own balance.
//
// The committed part cannot leave, and that check is the difference between a
// prefunded system and a promise. Without it a participant could submit its
// obligations, withdraw the collateral behind them in the very next
// transaction, and leave its counterparties holding claims against an account
// that no longer covers them — which is a default engineered in one block, not
// a credit event anybody could have seen coming.
func (k msgServer) WithdrawReserve(ctx context.Context, msg *types.MsgWithdrawReserve) (*types.MsgWithdrawReserveResponse, error) {
	if !msg.Amount.IsValid() || !msg.Amount.IsAllPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "amount %s", msg.Amount)
	}

	participantBz, err := k.addressCodec.StringToBytes(msg.Participant)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid participant address")
	}

	// Deliberately not gated on current approval. An institution that has been
	// removed from the rail must still be able to take back money that is not
	// committed to anything — stranding a former participant's own collateral
	// would make removal a confiscation, which is a power this module was never
	// meant to hold and one that x/enforcement exists to exercise properly.
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	for _, coin := range msg.Amount {
		available, err := k.Available(ctx, msg.Participant, coin.Denom)
		if err != nil {
			return nil, err
		}
		if available.LT(coin.Amount) {
			locked, err := k.GetLocked(ctx, msg.Participant, coin.Denom)
			if err != nil {
				return nil, err
			}
			return nil, errorsmod.Wrapf(types.ErrReserveCommitted,
				"%s may withdraw %s%s: %s is committed to positions that have not settled",
				msg.Participant, available, coin.Denom, locked)
		}

		reserve, err := k.GetReserve(ctx, msg.Participant, coin.Denom)
		if err != nil {
			return nil, err
		}
		reserve = reserve.Sub(coin.Amount)
		if err := k.setReserve(ctx, msg.Participant, coin.Denom, reserve); err != nil {
			return nil, err
		}
		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventReserveChanged{
			Participant: msg.Participant,
			Denom:       coin.Denom,
			Amount:      reserve,
			Deposited:   false,
		}); err != nil {
			return nil, err
		}
	}

	// The transfer happens after the books are written, so a send the bank
	// refuses — a frozen participant, a blocked destination — fails the whole
	// transaction and leaves the reserve exactly as it was. The alternative
	// order would need a compensating write, and a compensating write is a
	// second chance to get the arithmetic wrong.
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(participantBz), msg.Amount); err != nil {
		return nil, err
	}

	return &types.MsgWithdrawReserveResponse{}, nil
}

// assertApproved refuses an institution the rail does not currently recognise.
func (k Keeper) assertApproved(ctx context.Context, participant string) error {
	approved, err := k.participants.ApprovedParticipantExists(ctx, participant)
	if err != nil {
		return err
	}
	if !approved {
		return errorsmod.Wrapf(types.ErrNotApprovedParticipant, "%s", participant)
	}
	return nil
}
