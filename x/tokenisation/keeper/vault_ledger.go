package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

// The vault ledger: what the module account holds, and on whose behalf.
//
// One module account serves every vehicle on the chain, so its balance says
// nothing about who owns what. Without a per-vehicle record the bank keeper
// will happily pay a holder of vehicle A out of vehicle B's rent — it has no
// way to know there is a difference. Every function that moves the income
// denom goes through hold or release, and nothing credits the index that hold
// has not already covered.
//
// The whole invariant is one sentence: for each vehicle and denomination, what
// this ledger says is held is money that arrived for that vehicle and has not
// yet left it, and the sum across vehicles never exceeds the module balance.

// hold records that coins have arrived in the module account for one vehicle.
// Call it after the transfer, never instead of it.
func (k Keeper) hold(ctx context.Context, assetID uint64, amount sdk.Coin) error {
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return err
	}
	vault.Held = sdk.Coins(vault.Held).Add(amount)
	return k.Vaults.Set(ctx, assetID, vault)
}

// release takes coins off one vehicle's ledger before they are paid out, and
// refuses when the vehicle does not have them.
//
// The refusal is the point. A payout the ledger cannot cover is a payout from
// somebody else's vault, and it is far better to fail the transaction than to
// let the bank keeper succeed at it.
func (k Keeper) release(ctx context.Context, assetID uint64, amount sdk.Coin) error {
	if !amount.Amount.IsPositive() {
		return nil
	}
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return err
	}
	held := sdk.Coins(vault.Held)
	if held.AmountOf(amount.Denom).LT(amount.Amount) {
		return types.ErrVaultUnfunded.Wrapf(
			"vehicle %d holds %s and owes %s", assetID, held.AmountOf(amount.Denom), amount)
	}
	remaining, negative := held.SafeSub(amount)
	if negative {
		return types.ErrVaultUnfunded.Wrapf("vehicle %d holds %s and owes %s", assetID, held, amount)
	}
	vault.Held = remaining
	return k.Vaults.Set(ctx, assetID, vault)
}

// holderCut is the share of a gross figure the tokens carry.
//
// Truncated, so a fraction of a unit that cannot be divided stays with the
// sponsor rather than being conjured for the holders. Every caller computes it
// through this function, because a payment collected on one rounding and
// credited on another is a shortfall that only shows up at the last redemption.
func holderCut(asset types.Asset, gross math.Int) math.Int {
	return gross.MulRaw(int64(asset.HolderShareBps)).QuoRaw(10_000)
}
