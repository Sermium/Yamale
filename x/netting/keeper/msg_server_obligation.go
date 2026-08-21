package keeper

import (
	"context"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/netting/types"
)

// SubmitObligation records what one participant owes another.
//
// It is the only way value enters the netting layer, and everything that keeps
// settlement from failing happens here rather than at settlement. Three
// decisions, in this order:
//
//  1. Gross or net. Decided by the chain from the amount and the currency's
//     policy, never by the sender. High value settles now; the rest waits.
//  2. For a netted obligation, whether the sender may owe this much. Its net
//     debit across every unsettled window must stay within the reserve it has
//     already prefunded. This is the check that makes settlement unable to
//     fail, and it is made synchronously, in a transaction, where the answer
//     "no" is a rejected message rather than a stuck cycle.
//  3. Only then is anything written.
//
// The ordering is the whole design. A netting system that admits obligations
// first and discovers at close that somebody cannot cover its position has to
// choose between settling partially and recomputing the cycle without them —
// and both of those change what other participants owe after they were told it
// was settled. That is unwinding risk, and the world's payment systems spent
// the 1990s getting rid of it.
func (k msgServer) SubmitObligation(ctx context.Context, msg *types.MsgSubmitObligation) (*types.MsgSubmitObligationResponse, error) {
	// Field checks first: refusing a malformed message should cost comparisons,
	// not store reads.
	if err := types.ValidateObligationFields(
		msg.FromParticipant, msg.ToParticipant, msg.Denom, msg.Amount, msg.BatchHash,
	); err != nil {
		return nil, err
	}

	fromBz, err := k.addressCodec.StringToBytes(msg.FromParticipant)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid from_participant address")
	}
	toBz, err := k.addressCodec.StringToBytes(msg.ToParticipant)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid to_participant address")
	}

	if err := k.assertApproved(ctx, msg.FromParticipant); err != nil {
		return nil, err
	}
	if err := k.assertApproved(ctx, msg.ToParticipant); err != nil {
		return nil, err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	cycleID, err := k.CurrentCycle.Get(ctx)
	if err != nil {
		return nil, err
	}

	mode := settlementModeFor(params, msg.Denom, msg.Amount)

	id, err := k.ObligationSeq.Next(ctx)
	if err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	obligation := types.Obligation{
		CycleId:           cycleID,
		Id:                id,
		FromParticipant:   msg.FromParticipant,
		ToParticipant:     msg.ToParticipant,
		Denom:             msg.Denom,
		Amount:            msg.Amount,
		BatchHash:         msg.BatchHash,
		Mode:              mode,
		SubmittedAtHeight: sdkCtx.BlockHeight(),
	}

	switch mode {
	case types.SETTLEMENT_MODE_GROSS:
		// Straight out of the sender's own balance, in this block. No reserve
		// is involved and no exposure is created: the money is with the
		// creditor before the transaction returns, which is the property that
		// makes gross settlement the right answer for the amounts that matter
		// most.
		coins := sdk.NewCoins(sdk.NewCoin(msg.Denom, msg.Amount))
		if err := k.bankKeeper.SendCoins(ctx, sdk.AccAddress(fromBz), sdk.AccAddress(toBz), coins); err != nil {
			return nil, err
		}
	case types.SETTLEMENT_MODE_NET:
		if err := k.applyToPositions(ctx, cycleID, msg.Denom, msg.FromParticipant, msg.ToParticipant, msg.Amount); err != nil {
			return nil, err
		}
		if err := k.recordSubmission(ctx, cycleID, msg.Denom, msg.Amount); err != nil {
			return nil, err
		}
	default:
		// settlementModeFor returns one of the two above and nothing else. The
		// branch exists so that a mode added later without a handler fails
		// loudly here rather than writing an obligation that settles in no way
		// at all.
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "unhandled settlement mode %s", mode)
	}

	if err := k.Obligation.Set(ctx, collections.Join(cycleID, id), obligation); err != nil {
		return nil, err
	}
	// Indexed from both sides, so either party can page through what it is
	// party to without reading anybody else's.
	if err := k.ObligationByParticipant.Set(ctx, collections.Join3(msg.FromParticipant, cycleID, id)); err != nil {
		return nil, err
	}
	if err := k.ObligationByParticipant.Set(ctx, collections.Join3(msg.ToParticipant, cycleID, id)); err != nil {
		return nil, err
	}

	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventObligationSubmitted{
		Id:              id,
		CycleId:         cycleID,
		Mode:            mode,
		FromParticipant: msg.FromParticipant,
		ToParticipant:   msg.ToParticipant,
		Denom:           msg.Denom,
		Amount:          msg.Amount,
	}); err != nil {
		return nil, err
	}

	return &types.MsgSubmitObligationResponse{Id: id, CycleId: cycleID, Mode: mode}, nil
}

// settlementModeFor decides whether an obligation nets or settles gross.
//
// Every path that is not an explicit decision to net ends in gross settlement:
// netting switched off chain-wide, a currency governance has never enabled, a
// threshold of zero, or an amount at or above the threshold. That direction is
// chosen deliberately — gross settlement moves the money immediately and
// leaves nobody carrying an exposure, so a misconfiguration costs liquidity
// rather than creating credit risk nobody agreed to.
func settlementModeFor(params types.Params, denom string, amount math.Int) types.SettlementMode {
	if !params.NettingEnabled() {
		return types.SETTLEMENT_MODE_GROSS
	}
	threshold, configured := params.GrossThresholdFor(denom)
	if !configured || !threshold.IsPositive() {
		return types.SETTLEMENT_MODE_GROSS
	}
	if amount.GTE(threshold) {
		return types.SETTLEMENT_MODE_GROSS
	}
	return types.SETTLEMENT_MODE_NET
}

// applyToPositions moves the two participants' running positions and enforces
// the sender's net debit cap.
//
// The cap is checked against the position *after* the obligation, not against
// the obligation's size, because that is what the participant will actually
// have to fund: a bank that is owed 900 and now owes 1000 has a net debit of
// 100, and requiring it to hold 1000 of collateral against a 100 exposure
// would make netting cost more liquidity than gross settlement rather than
// less. Saving that liquidity is the reason the deferred window exists at all.
func (k Keeper) applyToPositions(ctx context.Context, cycleID uint64, denom, from, to string, amount math.Int) error {
	fromBefore, err := k.GetPosition(ctx, cycleID, denom, from)
	if err != nil {
		return err
	}
	fromAfter := fromBefore.Sub(amount)

	locked, err := k.GetLocked(ctx, from, denom)
	if err != nil {
		return err
	}
	reserve, err := k.GetReserve(ctx, from, denom)
	if err != nil {
		return err
	}
	lockedAfter := locked.Add(debitOf(fromAfter)).Sub(debitOf(fromBefore))
	if lockedAfter.GT(reserve) {
		return errorsmod.Wrapf(types.ErrNetDebitCapExceeded,
			"%s would owe %s%s across unsettled windows against a reserve of %s",
			from, lockedAfter, denom, reserve)
	}

	toBefore, err := k.GetPosition(ctx, cycleID, denom, to)
	if err != nil {
		return err
	}
	toAfter := toBefore.Add(amount)

	if err := k.setPosition(ctx, cycleID, denom, from, fromAfter); err != nil {
		return err
	}
	if err := k.setPosition(ctx, cycleID, denom, to, toAfter); err != nil {
		return err
	}
	if err := k.adjustLocked(ctx, from, denom, fromBefore, fromAfter); err != nil {
		return err
	}
	// The creditor's collateral moves too, and downwards: a participant that
	// owed 100 and is now owed 50 has released everything it had committed.
	// Netting's liquidity saving is exactly this, and it has to be released
	// when the offsetting obligation arrives rather than at close, or the
	// window would hold collateral against exposures that no longer exist.
	return k.adjustLocked(ctx, to, denom, toBefore, toAfter)
}

// recordSubmission accumulates the currency's gross total on the open cycle.
//
// Kept on the cycle record and updated as obligations arrive rather than
// summed at close, so that closing a window costs what the netting costs —
// one pass over the positions — instead of a pass over every obligation in it.
// A settlement step whose cost grows with traffic is a settlement step that
// gets slowest exactly when the system is busiest.
func (k Keeper) recordSubmission(ctx context.Context, cycleID uint64, denom string, amount math.Int) error {
	cycle, err := k.Cycle.Get(ctx, cycleID)
	if err != nil {
		return err
	}

	for i := range cycle.Outcomes {
		if cycle.Outcomes[i].Denom == denom {
			cycle.Outcomes[i].GrossAmount = cycle.Outcomes[i].GrossAmount.Add(amount)
			cycle.Outcomes[i].ObligationCount++
			return k.Cycle.Set(ctx, cycleID, cycle)
		}
	}

	// Appended in arrival order rather than sorted, because the settlement path
	// never reads this list — it walks the position store, which is ordered by
	// the key bytes. Order here is presentational only.
	cycle.Outcomes = append(cycle.Outcomes, types.DenomOutcome{
		Denom:           denom,
		Status:          types.DENOM_STATUS_OPEN,
		GrossAmount:     amount,
		NetAmount:       math.ZeroInt(),
		ObligationCount: 1,
	})
	return k.Cycle.Set(ctx, cycleID, cycle)
}
