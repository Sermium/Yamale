package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

// MigrateToProRata brings every vault and position written before pro-rata
// temporis onto the new mechanism.
//
// # The part that would have lost people money
//
// Under the old rule a holder's unsettled income was `balance * (cumulative -
// last_index)`, computed at read time. Pro-rata temporis no longer reads that
// delta at all — entitlement now comes from the per-period records, and there
// are none for any period before the upgrade.
//
// So a position carrying an unsettled delta at the halt height would simply
// stop being owed it. Not an error, not a halt: money quietly ceasing to exist,
// for every holder who had not happened to transfer since the last
// distribution. This settles that delta into `accrued` at the OLD rule before
// switching mechanisms, which is the whole reason this function exists rather
// than a few field initialisations.
//
// # The fields that would have halted the chain
//
// `math.Int`'s zero value is nil and PANICS on arithmetic rather than
// returning an error. Every position written before today decodes with
// `cost_basis`, `cost_qty` and `token_seconds` nil, so the first transfer after
// the upgrade would take down every validator at once. They are set explicitly
// here, and the keeper guards them again at every use — a migration that
// misses a row must not be the difference between a running chain and a stopped
// one.
//
// # The discontinuity, stated rather than hidden
//
// The first period of a migrated vehicle starts AT THE UPGRADE, not at
// issuance. The chain holds no record of when balances moved before that, so
// token-seconds for the past cannot be reconstructed — and assuming everyone
// held from issuance would credit a buyer from last week with a year of
// accrual. Income declared after the upgrade is apportioned by time from the
// upgrade forward. Everything before it was already paid by the old rule, which
// is what the settle above preserves.
//
// # Bounded
//
// This walks every vault and every position once. That is unbounded in
// principle and small in fact — a devnet with a handful of vehicles — but it is
// a one-off at a halt height with no other work in the block, which is the only
// place in this module where such a walk is acceptable.
func (k Keeper) MigrateToProRata(ctx context.Context) (vaults int, positions int, settled math.Int, err error) {
	settled = math.ZeroInt()
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	// Vaults first: a position's new period has to point at a period index the
	// vault already carries.
	if err := k.Vaults.Walk(ctx, nil, func(assetID uint64, v types.Vault) (bool, error) {
		v.PeriodIndex = 0
		v.PeriodStart = now
		if err := k.Vaults.Set(ctx, assetID, v); err != nil {
			return true, err
		}
		vaults++
		return false, nil
	}); err != nil {
		return 0, 0, settled, err
	}

	type update struct {
		key collections.Pair[uint64, string]
		pos types.Position
	}
	var pending []update

	if err := k.Positions.Walk(ctx, nil,
		func(key collections.Pair[uint64, string], pos types.Position) (bool, error) {
			assetID := key.K1()

			vault, err := k.Vaults.Get(ctx, assetID)
			if err != nil {
				// A position without a vault cannot be settled and cannot accrue.
				// It is normalised so it can never panic, and left otherwise alone.
				pos.CostBasis = zeroIfNil(pos.CostBasis)
				pos.CostQty = zeroIfNil(pos.CostQty)
				pos.Accrued = zeroIfNil(pos.Accrued)
				pos.TokenSeconds = math.ZeroInt()
				pos.LastTouch = now
				pending = append(pending, update{key, pos})
				return false, nil
			}

			asset, err := k.Assets.Get(ctx, assetID)
			if err != nil {
				return true, err
			}

			holder, err := k.addressCodec.StringToBytes(pos.Holder)
			if err != nil {
				return true, err
			}

			pos.CostBasis = zeroIfNil(pos.CostBasis)
			pos.CostQty = zeroIfNil(pos.CostQty)
			pos.Accrued = zeroIfNil(pos.Accrued)

			// Settle at the OLD rule. This is the money that would otherwise
			// vanish: nothing after this point reads the index delta.
			if !pos.LastIndex.IsNil() && asset.FractionDenom != "" {
				balance := k.bankKeeper.GetBalance(
					sdk.UnwrapSDKContext(ctx), holder, asset.FractionDenom).Amount
				if delta := vault.CumulativePerToken.Sub(pos.LastIndex); delta.IsPositive() && balance.IsPositive() {
					earned := delta.MulInt(balance).TruncateInt()
					pos.Accrued = pos.Accrued.Add(earned)
					settled = settled.Add(earned)
				}
			}
			pos.LastIndex = vault.CumulativePerToken

			// And start accruing time from here.
			pos.TokenSeconds = math.ZeroInt()
			pos.TokenSecondsPeriod = vault.PeriodIndex
			pos.LastTouch = now

			pending = append(pending, update{key, pos})
			return false, nil
		}); err != nil {
		return vaults, 0, settled, err
	}

	// Written after the walk rather than inside it: mutating a collection while
	// iterating it is undefined, and an upgrade is the worst possible place to
	// find that out.
	for _, u := range pending {
		if err := k.Positions.Set(ctx, u.key, u.pos); err != nil {
			return vaults, positions, settled, err
		}
		positions++
	}
	return vaults, positions, settled, nil
}

// AssertProRataReady refuses an upgrade that left anything able to panic.
//
// Checked rather than assumed, because the failure it guards is not a wrong
// number: a nil math.Int panics on arithmetic, and the first transfer of a
// missed position would stop every validator at once. Better to refuse the
// upgrade in one place than to discover it in a block.
func (k Keeper) AssertProRataReady(ctx context.Context) error {
	return k.Positions.Walk(ctx, nil,
		func(key collections.Pair[uint64, string], pos types.Position) (bool, error) {
			if pos.CostBasis.IsNil() || pos.CostQty.IsNil() ||
				pos.TokenSeconds.IsNil() || pos.Accrued.IsNil() {
				return true, fmt.Errorf(
					"position (asset %d, %s) still carries an unset amount; the first "+
						"transfer of it would panic every validator", key.K1(), pos.Holder)
			}
			if pos.LastTouch == 0 {
				return true, fmt.Errorf(
					"position (asset %d, %s) has no last_touch; its first accrual would "+
						"be balance times fifty-six years", key.K1(), pos.Holder)
			}
			return false, nil
		})
}
