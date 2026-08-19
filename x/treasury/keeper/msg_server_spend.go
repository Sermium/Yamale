package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/treasury/types"
)

// Spend moves funds out of a treasury to a recipient.
//
// This is the direct path: a spender acts alone, without an admin decision,
// bounded by the denom's SpendPolicy. It exists so routine operational payments
// do not need a governance round trip, and the policy is what makes handing out
// that power safe — a compromised spender key can lose at most one period's
// limit, not the treasury.
func (k msgServer) Spend(ctx context.Context, msg *types.MsgSpend) (*types.MsgSpendResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Spender); err != nil {
		return nil, errorsmod.Wrap(err, "invalid spender address")
	}
	recipientBz, err := k.addressCodec.StringToBytes(msg.Recipient)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid recipient address")
	}
	recipient := sdk.AccAddress(recipientBz)

	// Sending to a blocked address would burn the funds from the treasury's
	// point of view: they leave the ledger and cannot be retrieved.
	if k.bankKeeper.BlockedAddr(recipient) {
		return nil, types.ErrDestinationDenied.Wrapf("%s is not allowed to receive funds", msg.Recipient)
	}

	treasury, err := k.getTreasury(ctx, msg.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := requireNotPaused(treasury); err != nil {
		return nil, err
	}
	if err := k.requireSpender(ctx, treasury, msg.Spender); err != nil {
		return nil, err
	}

	if !msg.Amount.IsValid() || !msg.Amount.IsAllPositive() {
		return nil, types.ErrInvalidAmount.Wrapf("spend %s", msg.Amount)
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	// Check and record every denom before moving anything, so a tx that breaches
	// a limit on its second coin does not leave the first one already sent.
	for _, coin := range msg.Amount {
		if err := k.enforceSpendPolicy(ctx, treasury.Id, msg.Recipient, coin, now); err != nil {
			return nil, err
		}
		if err := k.debitAvailable(ctx, treasury.Id, coin.Denom, coin.Amount); err != nil {
			return nil, err
		}
	}

	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, recipient, msg.Amount); err != nil {
		return nil, err
	}

	return &types.MsgSpendResponse{}, nil
}

// enforceSpendPolicy applies the denom's policy and records consumption of the
// current period. A treasury with no policy for a denom is unconstrained beyond
// its available balance.
func (k Keeper) enforceSpendPolicy(ctx context.Context, treasuryID uint64, recipient string, coin sdk.Coin, now int64) error {
	key := collections.Join(treasuryID, coin.Denom)

	policy, err := k.SpendPolicy.Get(ctx, key)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil
		}
		return err
	}

	// Allowlist first: a non-empty list means only those destinations.
	if len(policy.Allowlist) > 0 && !contains(policy.Allowlist, recipient) {
		return types.ErrDestinationDenied.Wrapf("%s is not on treasury %d's allowlist for %s", recipient, treasuryID, coin.Denom)
	}
	// Blocklist second, so an address on both lists is denied.
	if contains(policy.Blocklist, recipient) {
		return types.ErrDestinationDenied.Wrapf("%s is on treasury %d's blocklist for %s", recipient, treasuryID, coin.Denom)
	}

	if limit, ok := parseLimit(policy.PerTransactionLimit); ok && coin.Amount.GT(limit) {
		return types.ErrSpendLimit.Wrapf("%s exceeds the per-transaction limit of %s%s", coin, limit, coin.Denom)
	}

	periodLimit, hasPeriodLimit := parseLimit(policy.PeriodLimit)
	if !hasPeriodLimit {
		return nil
	}

	spent, windowStart, err := k.spendWindowAt(ctx, key, policy, now)
	if err != nil {
		return err
	}

	updated := spent.Add(coin.Amount)
	if updated.GT(periodLimit) {
		return types.ErrSpendLimit.Wrapf(
			"spending %s would bring this period's total to %s%s, over the limit of %s",
			coin, updated, coin.Denom, periodLimit)
	}

	return k.SpendWindow.Set(ctx, key, types.SpendWindow{
		TreasuryId:  treasuryID,
		Denom:       coin.Denom,
		Spent:       updated.String(),
		WindowStart: windowStart,
	})
}

// SetSpendPolicy sets the spending constraints for one denom.
func (k msgServer) SetSpendPolicy(ctx context.Context, msg *types.MsgSetSpendPolicy) (*types.MsgSetSpendPolicyResponse, error) {
	treasury, err := k.getTreasury(ctx, msg.Policy.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := k.requireAdmin(ctx, treasury, msg.Admin); err != nil {
		return nil, err
	}

	if err := sdk.ValidateDenom(msg.Policy.Denom); err != nil {
		return nil, errorsmod.Wrap(err, "invalid policy denom")
	}
	if _, ok := parseLimit(msg.Policy.PerTransactionLimit); msg.Policy.PerTransactionLimit != "" && !ok {
		return nil, types.ErrInvalidAmount.Wrapf("invalid per-transaction limit %q", msg.Policy.PerTransactionLimit)
	}
	if _, ok := parseLimit(msg.Policy.PeriodLimit); msg.Policy.PeriodLimit != "" && !ok {
		return nil, types.ErrInvalidAmount.Wrapf("invalid period limit %q", msg.Policy.PeriodLimit)
	}
	if msg.Policy.PeriodLimit != "" && msg.Policy.PeriodSeconds == 0 {
		return nil, types.ErrInvalidAmount.Wrap("a period limit requires a non-zero period length")
	}
	// Both lists are scanned on every spend and stored forever, so their size
	// is bounded: an unbounded list would let an admin make each of their own
	// treasury's payments arbitrarily expensive to validate, and bloat state
	// permanently while doing it.
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	entries := len(msg.Policy.Allowlist) + len(msg.Policy.Blocklist)
	if uint64(entries) > params.MaxSpendPolicyAddresses {
		return nil, types.ErrLimitReached.Wrapf(
			"a spend policy may list at most %d addresses, got %d", params.MaxSpendPolicyAddresses, entries)
	}
	for _, addr := range append(append([]string{}, msg.Policy.Allowlist...), msg.Policy.Blocklist...) {
		if _, err := k.addressCodec.StringToBytes(addr); err != nil {
			return nil, errorsmod.Wrapf(err, "invalid address %q in spend policy", addr)
		}
	}

	policy := msg.Policy
	policy.TreasuryId = treasury.Id

	return &types.MsgSetSpendPolicyResponse{}, k.SpendPolicy.Set(ctx, collections.Join(treasury.Id, policy.Denom), policy)
}

// parseLimit reads an optional limit. An empty string means no limit.
func parseLimit(s string) (math.Int, bool) {
	if s == "" {
		return math.ZeroInt(), false
	}
	v, ok := math.NewIntFromString(s)
	if !ok || v.IsNegative() {
		return math.ZeroInt(), false
	}
	return v, true
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
