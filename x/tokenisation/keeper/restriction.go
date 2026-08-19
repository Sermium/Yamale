package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

// SendRestrictionFn settles both sides of a transfer before the balances move,
// and stops transfers once an asset has been realised.
//
// This runs on every bank send on the chain, so the first thing it does is
// establish that the coins are none of its business and get out of the way.
//
// Why a restriction rather than a hook: entitlement is
// balance * (cumulative - last_index), which is only correct if the position is
// settled against the balance the holder actually held. Settling after the
// transfer would credit the sender's history to the recipient.
func (k Keeper) SendRestrictionFn(ctx context.Context, from, to sdk.AccAddress, amount sdk.Coins) (sdk.AccAddress, error) {
	for _, coin := range amount {
		assetID, err := k.ByDenom.Get(ctx, coin.Denom)
		if err != nil {
			continue // not a fraction denom
		}

		asset, err := k.Assets.Get(ctx, assetID)
		if err != nil {
			continue
		}

		// Once the asset is sold, every token is a claim on a known fixed pot.
		// A pool still quoting a price from its reserves is then a free lunch
		// that gets taken within a block, so trading stops and the only path
		// left is redemption.
		//
		// The module account is exempt because redemption itself moves tokens
		// to the module to be burned, and a rule that blocked that would strand
		// every holder in the asset.
		if asset.Status == types.STATUS_REALISED || asset.Status == types.STATUS_CLOSED {
			moduleAddr := k.ModuleAddress()
			if !to.Equals(moduleAddr) && !from.Equals(moduleAddr) {
				return to, types.ErrTradingHalted
			}
		}

		sdkCtx := sdk.UnwrapSDKContext(ctx)

		// Settle against the balances held *before* this transfer. Both sides,
		// because both are about to change.
		fromBalance := k.bankKeeper.GetBalance(sdkCtx, from, coin.Denom).Amount
		if err := k.Settle(ctx, assetID, from, fromBalance); err != nil {
			return to, err
		}

		toBalance := k.bankKeeper.GetBalance(sdkCtx, to, coin.Denom).Amount
		if err := k.Settle(ctx, assetID, to, toBalance); err != nil {
			return to, err
		}
	}

	return to, nil
}
