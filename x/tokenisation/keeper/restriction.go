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
		// ByDenom holds fraction denoms only, which is what makes a DIVIDEND
		// denom fall out here — and it must. A div/… transfer settled as though
		// it were a share transfer would run the income arithmetic against a
		// balance in the wrong token: the sender's whole entitlement history
		// recomputed against however many dividend tokens they happened to
		// hold. Nothing errors; the positions are simply wrong afterwards.
		//
		// Leaving the dividend token unrestricted is also the right answer on
		// its own terms. The fraction token is halted at realisation because it
		// becomes a claim on a fixed pot that a pool would keep quoting a stale
		// price for; a dividend token is a claim on a fixed pot at a fixed 1:1
		// rate from the moment it is minted, so there is no stale price and no
		// free lunch. It is transferable on purpose — a holder who cannot
		// receive the stable has to be able to assign the claim to somebody who
		// can, and that is most of the reason the token exists.
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

		// Cost moves with the tokens, AFTER both sides are settled and before
		// the balances change. A transfer names no price, so there is nothing
		// to record as a purchase; what there is, is the sender's own cost for
		// the tokens leaving. See CarryBasis for why it is proportional and why
		// the rounding favours the sender.
		if err := k.CarryBasis(ctx, assetID, from, to, coin.Amount); err != nil {
			return to, err
		}
	}

	return to, nil
}

// IsRestrictedDenom reports whether a denom is one this module can halt.
//
// Every fraction denom qualifies, whatever its vehicle's current status. The
// question x/amm asks is not "is this halted now" but "could this ever be
// halted" — a pool created over an ACTIVE vehicle is exactly the trap, because
// the vehicle is realised later and the halt arrives after the LPs' money is
// in. So the answer is about the denom being a fraction denom at all, not about
// today's status.
//
// A store error is surfaced, not swallowed: x/amm treats it as a refusal, and a
// pool the chain could not vet is one it should not create.
func (k Keeper) IsRestrictedDenom(ctx context.Context, denom string) (bool, error) {
	has, err := k.ByDenom.Has(ctx, denom)
	if err != nil {
		return false, err
	}
	return has, nil
}
