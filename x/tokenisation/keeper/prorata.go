package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

/* ========================================================================== */
/*  Pro rata temporis                                                         */
/* ========================================================================== */
//
// Hold January to June, sell, and still earn half the year. The whole problem
// is doing that WITHOUT iterating holders: the index this replaces exists so a
// distribution to a thousand accounts costs one write, and a design that walked
// the holder set to apportion a dividend would halt the chain on the vehicle
// this platform most wants to succeed with.
//
// What makes a bounded solution possible is that a holder whose balance was
// constant across a whole period earns `B * X / S` either way — exactly what
// the old index paid. Pro-rata temporis differs ONLY for a balance that moved
// mid-period, and that is the entire delta.
//
// So the state carries two things per distribution: a RATE per token-second,
// which pays the partial stretch a holder was present for, and a CUMULATIVE
// per token, which pays every whole period in between by subtraction. The
// second is what removes the loop.

// periodRate reads one period's record.
func (k Keeper) periodRate(ctx context.Context, assetID, period uint64) (types.PeriodRate, error) {
	return k.PeriodRates.Get(ctx, collections.Join(assetID, period))
}

// AccruePosition brings a position's token-seconds up to `now` and converts
// anything owed for periods that have since closed.
//
// Constant time whatever the gap. A holder who last touched their balance five
// distributions ago is settled with two map reads and a subtraction, not five
// iterations — which is the property that keeps this safe on a vehicle with a
// long history.
func (k Keeper) AccruePosition(
	ctx context.Context, assetID uint64, pos *types.Position, balance math.Int, now int64,
) error {
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return err
	}

	pos.TokenSeconds = zeroIfNil(pos.TokenSeconds)
	if pos.LastTouch == 0 {
		// A position that has never accrued starts now rather than at the epoch.
		// Starting at zero would credit it with fifty-six years of token-seconds
		// on its first settle.
		pos.LastTouch = now
		pos.TokenSecondsPeriod = vault.PeriodIndex
	}

	if pos.TokenSecondsPeriod == vault.PeriodIndex {
		// Still inside the same period: keep integrating balance over time.
		if elapsed := now - pos.LastTouch; elapsed > 0 && balance.IsPositive() {
			pos.TokenSeconds = pos.TokenSeconds.Add(balance.MulRaw(elapsed))
		}
		pos.LastTouch = now
		return nil
	}

	// The period the pending seconds belong to has closed. Three steps, none of
	// them a loop.

	// 1. Those seconds are paid at THEIR period's rate, however long ago it was.
	//    The rate is only known when a period closes, so seconds accrued in
	//    period 3 must be paid at period 3's rate even if the holder settles in
	//    period 9.
	closed, err := k.periodRate(ctx, assetID, pos.TokenSecondsPeriod)
	if err != nil {
		return err
	}

	// The seconds recorded on the position stop at the last touch, and a holder
	// who did not touch their balance between then and the end of the period
	// still held it throughout. Those seconds have to be added here or they are
	// simply lost — which is exactly what happened the first time these tests
	// ran: a holder through five distributions was paid for four of them, and
	// the conservation test found 600,000 of a 900,000 distribution stranded.
	secondsInClosed := pos.TokenSeconds
	if closed.EndedAt > pos.LastTouch && balance.IsPositive() {
		secondsInClosed = secondsInClosed.Add(balance.MulRaw(closed.EndedAt - pos.LastTouch))
	}
	// amount * held / total, in integers, truncated once at the end. A stored
	// rate would divide here and multiply back, and the two roundings paid a
	// holder owed exactly 100,000 a measured 99,999.
	switch {
	case zeroIfNil(closed.TokenSeconds).IsPositive():
		if secondsInClosed.IsPositive() {
			share := zeroIfNil(closed.Amount).Mul(secondsInClosed).Quo(closed.TokenSeconds)
			pos.Accrued = zeroIfNil(pos.Accrued).Add(share)
		}
	case balance.IsPositive():
		// The period had no length: two distributions in one block, or income
		// paid in the same block the vault was created. Nobody held for any
		// time, so token-seconds apportion nothing and the only sensible answer
		// is the old rule, by balance. Without this branch the money is written
		// into the cumulative figure and credited to nobody -- which is how a
		// distribution silently strands in the vault, and is what the existing
		// sale tests caught.
		if pt := closed.PerToken; !pt.IsNil() && pt.IsPositive() {
			pos.Accrued = zeroIfNil(pos.Accrued).Add(pt.MulInt(balance).TruncateInt())
		}
	}

	// 2. Every period fully spanned since is covered by one subtraction. The
	//    holder held `balance` throughout each of them, so they are owed
	//    `balance * (cumulative_after_last_closed - cumulative_after_theirs)`.
	if vault.PeriodIndex > pos.TokenSecondsPeriod+1 && balance.IsPositive() {
		previous, err := k.periodRate(ctx, assetID, vault.PeriodIndex-1)
		if err != nil {
			return err
		}
		spanned := previous.CumulativePerToken.Sub(closed.CumulativePerToken)
		if spanned.IsPositive() {
			pos.Accrued = zeroIfNil(pos.Accrued).Add(spanned.MulInt(balance).TruncateInt())
		}
	}

	// 3. Start accruing afresh in the current period, from the period's start
	//    rather than from the holder's last touch — they held throughout the
	//    part of it that has already elapsed.
	from := vault.PeriodStart
	if pos.LastTouch > from {
		from = pos.LastTouch
	}
	pos.TokenSeconds = math.ZeroInt()
	if elapsed := now - from; elapsed > 0 && balance.IsPositive() {
		pos.TokenSeconds = balance.MulRaw(elapsed)
	}
	pos.TokenSecondsPeriod = vault.PeriodIndex
	pos.LastTouch = now
	return nil
}

// ClosePeriod records what a distribution paid and starts the next period.
//
// Called by FundVault, after the money has arrived. The rate is
// `amount / (supply * elapsed)`: money per token-second, which is what a
// partial holder is paid at.
//
// A period of zero length — two distributions in one block — would divide by
// zero. It gets a zero rate and the whole amount goes into the cumulative
// figure instead, so the money is not lost: it is paid to whoever holds at the
// next settle, which is the old behaviour and the only sensible answer when no
// time has passed for anybody to have held through.
func (k Keeper) ClosePeriod(
	ctx context.Context, assetID uint64, amount math.Int, supply math.Int,
) error {
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	elapsed := now - vault.PeriodStart
	if elapsed < 0 {
		elapsed = 0
	}

	perToken := math.LegacyZeroDec()
	if supply.IsPositive() {
		perToken = math.LegacyNewDecFromInt(amount).QuoInt(supply)
	}

	// Supply times the period's length: the denominator every holder's share is
	// taken over. Zero when no time passed, which is what makes that case
	// detectable rather than a division by zero.
	tokenSeconds := math.ZeroInt()
	if elapsed > 0 && supply.IsPositive() {
		tokenSeconds = supply.MulRaw(elapsed)
	}

	vault.CumulativePerToken = vault.CumulativePerToken.Add(perToken)

	if err := k.PeriodRates.Set(ctx, collections.Join(assetID, vault.PeriodIndex), types.PeriodRate{
		AssetId:            assetID,
		PeriodIndex:        vault.PeriodIndex,
		Amount:             amount,
		TokenSeconds:       tokenSeconds,
		PerToken:           perToken,
		CumulativePerToken: vault.CumulativePerToken,
		EndedAt:            now,
	}); err != nil {
		return err
	}

	vault.PeriodIndex++
	vault.PeriodStart = now
	return k.Vaults.Set(ctx, assetID, vault)
}

// PendingEntitlement is what a holder has earned in the CURRENT, undeclared
// period — which is not money yet, because the rate is unknown until the period
// closes.
//
// Returned as token-seconds rather than as an amount, deliberately. An
// interface that converted them at the last period's rate would be publishing a
// forecast beside a settled figure in the same units, and a reader cannot tell
// those apart once they are both in dollars.
func (k Keeper) PendingTokenSeconds(
	ctx context.Context, assetID uint64, holder sdk.AccAddress, balance math.Int, now int64,
) (math.Int, error) {
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return math.ZeroInt(), err
	}
	pos, err := k.Positions.Get(ctx, collections.Join(assetID, holder.String()))
	if err != nil {
		// Never seen: they have accrued from the period's start if they hold
		// anything at all, which they cannot have without a position — so zero.
		return math.ZeroInt(), nil
	}

	seconds := zeroIfNil(pos.TokenSeconds)
	from := pos.LastTouch
	if pos.TokenSecondsPeriod != vault.PeriodIndex {
		// The pending seconds belong to a closed period and are money now, not
		// seconds; the current period starts fresh from whichever is later of
		// its start and the holder's last touch.
		seconds = math.ZeroInt()
		from = vault.PeriodStart
		if pos.LastTouch > from {
			from = pos.LastTouch
		}
	}
	if elapsed := now - from; elapsed > 0 && balance.IsPositive() {
		seconds = seconds.Add(balance.MulRaw(elapsed))
	}
	return seconds, nil
}
