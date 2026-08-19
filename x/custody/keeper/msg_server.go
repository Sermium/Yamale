package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/custody/types"
)

type msgServer struct{ Keeper }

// NewMsgServerImpl returns the Msg service implementation.
func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{Keeper: k} }

func (k msgServer) RegisterAsset(ctx context.Context, msg *types.MsgRegisterAsset) (*types.MsgRegisterAssetResponse, error) {
	if msg.Authority != k.GetAuthority() {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "expected %s, got %s", k.GetAuthority(), msg.Authority)
	}
	if exists, err := k.Assets.Has(ctx, msg.Denom); err != nil {
		return nil, err
	} else if exists {
		return nil, types.ErrAssetExists
	}
	return &types.MsgRegisterAssetResponse{}, k.Assets.Set(ctx, msg.Denom, types.Asset{
		Denom:       msg.Denom,
		SourceChain: msg.SourceChain,
		Symbol:      msg.Symbol,
		Exponent:    msg.Exponent,
	})
}

func (k msgServer) SetAttestor(ctx context.Context, msg *types.MsgSetAttestor) (*types.MsgSetAttestorResponse, error) {
	if msg.Authority != k.GetAuthority() {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "expected %s, got %s", k.GetAuthority(), msg.Authority)
	}
	if _, err := k.addressCodec.StringToBytes(msg.Attestor); err != nil {
		return nil, errorsmod.Wrap(err, "invalid attestor address")
	}
	if msg.Active {
		return &types.MsgSetAttestorResponse{}, k.Attestors.Set(ctx, msg.Attestor)
	}
	return &types.MsgSetAttestorResponse{}, k.Attestors.Remove(ctx, msg.Attestor)
}

// AttestDeposit records one attestor's statement, and mints once the threshold
// is reached.
//
// The deposit is keyed by (denom, external_ref) rather than given a fresh id
// per attestation, so two attestors describing the same source-chain
// transaction converge on one record. They must agree on recipient and amount
// as well: a differing amount produces a refusal rather than a vote, which is
// what stops one attestor nudging a figure upward and having it counted as
// agreement.
func (k msgServer) AttestDeposit(ctx context.Context, msg *types.MsgAttestDeposit) (*types.MsgAttestDepositResponse, error) {
	if ok, err := k.Attestors.Has(ctx, msg.Attestor); err != nil {
		return nil, err
	} else if !ok {
		return nil, types.ErrNotAttestor
	}
	asset, err := k.Assets.Get(ctx, msg.Denom)
	if err != nil {
		return nil, types.ErrUnknownAsset
	}
	if asset.Paused {
		return nil, types.ErrIssuancePaused
	}
	if msg.Amount.IsNil() || !msg.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	if _, err := k.addressCodec.StringToBytes(msg.Recipient); err != nil {
		return nil, errorsmod.Wrap(err, "invalid recipient address")
	}

	// The replay guard, checked before anything is written. Without it the same
	// source transaction can be attested again after crediting and mint a
	// second claim against one deposit.
	refKey := collections.Join(msg.Denom, msg.ExternalRef)
	if used, err := k.ExternalRefs.Has(ctx, refKey); err != nil {
		return nil, err
	} else if used {
		return nil, types.ErrDuplicateRef
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()
	id := depositID(msg.Denom, msg.ExternalRef)

	deposit, err := k.Deposits.Get(ctx, id)
	if err != nil {
		deposit = types.Deposit{
			Id:              id,
			Denom:           msg.Denom,
			Recipient:       msg.Recipient,
			Amount:          msg.Amount,
			ExternalRef:     msg.ExternalRef,
			Status:          types.DepositStatus_DEPOSIT_STATUS_PENDING,
			CreatedAtHeight: height,
		}
	} else {
		// Disagreement is not agreement.
		if !deposit.Amount.Equal(msg.Amount) || deposit.Recipient != msg.Recipient {
			return nil, errorsmod.Wrapf(types.ErrInvalidAmount,
				"attestors disagree on deposit %s: recorded %s to %s", id, deposit.Amount, deposit.Recipient)
		}
		if deposit.Status != types.DepositStatus_DEPOSIT_STATUS_PENDING {
			return nil, types.ErrDuplicateRef
		}
	}

	attKey := collections.Join(id, msg.Attestor)
	if done, err := k.Attestations.Has(ctx, attKey); err != nil {
		return nil, err
	} else if done {
		return nil, types.ErrAlreadyAttested
	}
	if err := k.Attestations.Set(ctx, attKey); err != nil {
		return nil, err
	}

	count, err := k.countAttestations(ctx, id)
	if err != nil {
		return nil, err
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	credited := false
	if count >= params.AttestationThreshold {
		if err := k.credit(ctx, &deposit); err != nil {
			return nil, err
		}
		if err := k.ExternalRefs.Set(ctx, refKey); err != nil {
			return nil, err
		}
		credited = true
	}
	if err := k.Deposits.Set(ctx, id, deposit); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent("custody_attested",
		sdk.NewAttribute("deposit_id", id),
		sdk.NewAttribute("attestor", msg.Attestor),
		sdk.NewAttribute("credited", fmt.Sprintf("%t", credited)),
	))
	return &types.MsgAttestDepositResponse{DepositId: id, Attestations: count, Credited: credited}, nil
}

// credit mints the claim, net of fee, to the recipient.
func (k Keeper) credit(ctx context.Context, d *types.Deposit) error {
	fee, err := k.fee(ctx, d.Amount)
	if err != nil {
		return err
	}
	net := d.Amount.Sub(fee)

	// Minted in full and the fee retained, rather than minting only the net.
	// The claim outstanding must equal the asset held; minting only the net
	// would make the reserve exceed the supply by the fee and quietly break the
	// solvency comparison.
	full := sdk.NewCoins(sdk.NewCoin(d.Denom, d.Amount))
	if err := k.bank.MintCoins(ctx, types.ModuleName, full); err != nil {
		return err
	}
	recipient, err := k.addressCodec.StringToBytes(d.Recipient)
	if err != nil {
		return err
	}
	if net.IsPositive() {
		out := sdk.NewCoins(sdk.NewCoin(d.Denom, net))
		if err := k.bank.SendCoinsFromModuleToAccount(ctx, types.ModuleName, recipient, out); err != nil {
			return err
		}
	}
	d.Status = types.DepositStatus_DEPOSIT_STATUS_CREDITED
	return nil
}

func (k Keeper) countAttestations(ctx context.Context, id string) (uint32, error) {
	var n uint32
	rng := collections.NewPrefixedPairRange[string, string](id)
	err := k.Attestations.Walk(ctx, rng, func(collections.Pair[string, string]) (bool, error) {
		n++
		return false, nil
	})
	return n, err
}

func (k msgServer) ReportReserve(ctx context.Context, msg *types.MsgReportReserve) (*types.MsgReportReserveResponse, error) {
	if ok, err := k.Attestors.Has(ctx, msg.Attestor); err != nil {
		return nil, err
	} else if !ok {
		return nil, types.ErrNotAttestor
	}
	if _, err := k.Assets.Get(ctx, msg.Denom); err != nil {
		return nil, types.ErrUnknownAsset
	}
	if msg.Held.IsNil() || msg.Held.IsNegative() {
		return nil, types.ErrInvalidAmount
	}
	return &types.MsgReportReserveResponse{}, k.Reserves.Set(ctx, msg.Denom, types.Reserve{
		Denom:      msg.Denom,
		Held:       msg.Held,
		AsOfHeight: sdk.UnwrapSDKContext(ctx).BlockHeight(),
		Attestor:   msg.Attestor,
	})
}

// RequestRedemption burns the claim now and queues the payout.
//
// Burning at request rather than at settlement is the point: leaving the claim
// in circulation while the asset is being sent would let it be spent again and
// redeemed a second time.
func (k msgServer) RequestRedemption(ctx context.Context, msg *types.MsgRequestRedemption) (*types.MsgRequestRedemptionResponse, error) {
	if _, err := k.Assets.Get(ctx, msg.Denom); err != nil {
		return nil, types.ErrUnknownAsset
	}
	if msg.Amount.IsNil() || !msg.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	redeemer, err := k.addressCodec.StringToBytes(msg.Redeemer)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid redeemer address")
	}

	coins := sdk.NewCoins(sdk.NewCoin(msg.Denom, msg.Amount))
	if err := k.bank.SendCoinsFromAccountToModule(ctx, redeemer, types.ModuleName, coins); err != nil {
		return nil, err
	}
	if err := k.bank.BurnCoins(ctx, types.ModuleName, coins); err != nil {
		return nil, err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()

	seq, err := k.RedeemSeq.Next(ctx)
	if err != nil {
		return nil, err
	}
	// Ids start at 1: in proto3 an id of 0 is indistinguishable from unset.
	id := fmt.Sprintf("r%d", seq+1)

	fee, err := k.fee(ctx, msg.Amount)
	if err != nil {
		return nil, err
	}

	r := types.Redemption{
		Id:       id,
		Denom:    msg.Denom,
		Redeemer: msg.Redeemer,
		// What is owed off-chain is net of fee. The full claim was burned, so
		// the fee stays in the reserve rather than being minted anywhere.
		Amount:            msg.Amount.Sub(fee),
		Destination:       msg.Destination,
		Status:            types.RedemptionStatus_REDEMPTION_STATUS_PENDING,
		RequestedAtHeight: height,
		// Stored, not computed at payout: a later parameter change must not
		// retroactively move an existing redemption's window.
		PayableAtHeight: height + int64(params.RedemptionDelayBlocks),
	}
	if err := k.Redemptions.Set(ctx, id, r); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent("custody_redemption_requested",
		sdk.NewAttribute("id", id),
		sdk.NewAttribute("denom", msg.Denom),
		sdk.NewAttribute("payable_at_height", fmt.Sprint(r.PayableAtHeight)),
	))
	return &types.MsgRequestRedemptionResponse{RedemptionId: id, PayableAtHeight: r.PayableAtHeight}, nil
}

func (k msgServer) SettleRedemption(ctx context.Context, msg *types.MsgSettleRedemption) (*types.MsgSettleRedemptionResponse, error) {
	if ok, err := k.Attestors.Has(ctx, msg.Attestor); err != nil {
		return nil, err
	} else if !ok {
		return nil, types.ErrNotAttestor
	}
	r, err := k.Redemptions.Get(ctx, msg.RedemptionId)
	if err != nil {
		return nil, types.ErrNotFound
	}
	if r.Status == types.RedemptionStatus_REDEMPTION_STATUS_SETTLED {
		return nil, types.ErrAlreadySettled
	}
	// Enforced here rather than only in a client. A window that can be skipped
	// by calling the chain directly is not a window.
	if sdk.UnwrapSDKContext(ctx).BlockHeight() < r.PayableAtHeight {
		return nil, errorsmod.Wrapf(types.ErrNotPayableYet, "payable at height %d", r.PayableAtHeight)
	}

	r.Status = types.RedemptionStatus_REDEMPTION_STATUS_SETTLED
	r.SettledRef = msg.SettledRef
	return &types.MsgSettleRedemptionResponse{}, k.Redemptions.Set(ctx, msg.RedemptionId, r)
}

func (k msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != k.GetAuthority() {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "expected %s, got %s", k.GetAuthority(), msg.Authority)
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, errorsmod.Wrap(types.ErrInvalidParams, err.Error())
	}
	return &types.MsgUpdateParamsResponse{}, k.Params.Set(ctx, msg.Params)
}

// depositID is derived from the asset and the source transaction, so every
// attestor naming the same external payment lands on the same record.
func depositID(denom, ref string) string { return denom + ":" + ref }
