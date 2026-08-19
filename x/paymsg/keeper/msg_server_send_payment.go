package keeper

import (
	"context"

	"yamale/blockchain/x/paymsg/types"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// SendPayment executes an ISO 20022 pacs.008-style instant credit transfer:
// it wraps a real x/bank transfer from the debtor to the creditor, signed by
// the debtor themself, while requiring both the instructing and instructed
// participants (the debtor's and creditor's payment service providers) to be
// DAO-approved. A queryable PaymentRecord is kept per (instructing
// participant, EndToEndId), acting as the camt.053-style statement entry for
// this payment.
//
// The debtor must actually bank with the instructing participant it names.
// Without that check the participant fields were an unverified claim: any
// account could file an instruction attributing it to two institutions that
// had never seen it, and the statement record — the thing this module exists to
// produce — could not be trusted to say who handled a payment.
func (k msgServer) SendPayment(ctx context.Context, msg *types.MsgSendPayment) (*types.MsgSendPaymentResponse, error) {
	// Field limits first: rejecting an oversized message should cost a length
	// comparison, not a series of store reads.
	if err := types.ValidatePaymentFields(msg.EndToEndId, msg.PurposeCode, msg.RemittanceInformation, msg.Denom); err != nil {
		return nil, err
	}
	if err := types.ValidateConfidentiality(
		msg.AmountCommitment, msg.AmountRangeProof, msg.MetadataHash,
		msg.PurposeCode, msg.RemittanceInformation,
	); err != nil {
		return nil, err
	}

	// Whether the jurisdiction is required is read from params rather than
	// decided here, so a block replays under the rule that was in force when it
	// was made. A requirement compiled into the binary would refuse payments
	// that were valid when they were included, and every node syncing from
	// block 0 re-executes those.
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateSettlementJurisdiction(msg.SettlementJurisdiction, params.RequireSettlementJurisdiction); err != nil {
		return nil, err
	}

	debtorBz, err := k.addressCodec.StringToBytes(msg.Debtor)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid debtor address")
	}
	creditorBz, err := k.addressCodec.StringToBytes(msg.Creditor)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid creditor address")
	}

	if has, err := k.ApprovedParticipant.Has(ctx, msg.InstructingParticipant); err != nil {
		return nil, err
	} else if !has {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedParticipant, "instructing participant %s is not approved", msg.InstructingParticipant)
	}
	if has, err := k.ApprovedParticipant.Has(ctx, msg.InstructedParticipant); err != nil {
		return nil, err
	} else if !has {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedParticipant, "instructed participant %s is not approved", msg.InstructedParticipant)
	}

	if err := k.assertInstructedBy(ctx, msg.Debtor, msg.InstructingParticipant); err != nil {
		return nil, err
	}

	key := collections.Join(msg.InstructingParticipant, msg.EndToEndId)
	if has, err := k.PaymentRecord.Has(ctx, key); err != nil {
		return nil, err
	} else if has {
		return nil, errorsmod.Wrapf(types.ErrPaymentExists,
			"%s has already sent a payment with end-to-end id %s", msg.InstructingParticipant, msg.EndToEndId)
	}

	amount, ok := math.NewIntFromString(msg.Amount)
	if !ok || !amount.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid amount %s", msg.Amount)
	}
	coins := sdk.NewCoins(sdk.NewCoin(msg.Denom, amount))

	if err := k.bankKeeper.SendCoins(ctx, sdk.AccAddress(debtorBz), sdk.AccAddress(creditorBz), coins); err != nil {
		return nil, err
	}

	if err := k.PaymentRecord.Set(ctx, key, types.PaymentRecord{
		EndToEndId:             msg.EndToEndId,
		InstructingParticipant: msg.InstructingParticipant,
		InstructedParticipant:  msg.InstructedParticipant,
		Debtor:                 msg.Debtor,
		Creditor:               msg.Creditor,
		Denom:                  msg.Denom,
		Amount:                 msg.Amount,
		PurposeCode:            msg.PurposeCode,
		RemittanceInformation:  msg.RemittanceInformation,
		BlockHeight:            uint64(sdk.UnwrapSDKContext(ctx).BlockHeight()),
		MetadataHash:           msg.MetadataHash,
		SettlementJurisdiction: msg.SettlementJurisdiction,
	}); err != nil {
		return nil, err
	}

	return &types.MsgSendPaymentResponse{}, nil
}

// assertInstructedBy checks that the participant named on a payment is entitled
// to be named on it.
//
// A participant paying out of its own balance is its own instructing agent and
// needs no registration; anybody else has to have been registered by that
// participant as a customer.
func (k Keeper) assertInstructedBy(ctx context.Context, debtor, participant string) error {
	if debtor == participant {
		return nil
	}

	customer, err := k.Customer.Get(ctx, debtor)
	if err != nil || customer.Participant != participant {
		return errorsmod.Wrapf(types.ErrNotACustomer,
			"%s does not bank with %s, so that participant may not be named as instructing this payment",
			debtor, participant)
	}
	return nil
}
