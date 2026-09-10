package keeper

import (
	"context"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/syndicate/types"
)

// SendRestrictionFn is where the KYC gate is actually enforced, and where the
// distribution index is kept correct.
//
// # Why the gate lives here and nowhere else
//
// Every investor completes KYC/AML before joining the platform or taking part
// in a syndicate. It is tempting to enforce that in the application: the
// investor signs up, the platform checks them, the user interface refuses
// anybody else.
//
// That gate binds nobody. A deal token is a bank denomination, and a bank send
// does not go through a user interface — a holder can transfer to anyone with a
// wallet, and the platform would learn about it from a support ticket. Worse,
// it would learn about it while telling a regulator that its tokens are only
// held by onboarded investors, which would be false and would have been false
// from the first block.
//
// So the check runs on every transfer of every deal token, including transfers
// this module never sees, composed by software that has never heard of it.
//
// # Why a restriction rather than a hook
//
// Entitlement is balance * (index - position.index), which is only correct if
// each position is settled against the balance that holder ACTUALLY HELD over
// the interval. A hook that ran after the transfer would settle both sides
// against their new balances, crediting the seller's holding period to the
// buyer — silently, and in the buyer's favour, which is the direction nobody
// complains about until the pot runs short for the last claimant.
func (k Keeper) SendRestrictionFn(
	ctx context.Context, from, to sdk.AccAddress, amount sdk.Coins,
) (sdk.AccAddress, error) {
	for _, coin := range amount {
		// The cheapest possible exit for the overwhelming majority of transfers
		// on this chain, which have nothing to do with any syndicate: one
		// string prefix comparison, no store read. A restriction that made a
		// lookup out of every unrelated payment would be a tax on the network.
		if !strings.HasPrefix(coin.Denom, types.DenomPrefix) {
			continue
		}

		dealID, err := k.ByDenom.Get(ctx, coin.Denom)
		if err != nil {
			continue // a denom that merely looks like one of ours
		}
		deal, err := k.Deal.Get(ctx, dealID)
		if err != nil {
			continue
		}

		moduleAddr := k.ModuleAddress()
		involvesModule := to.Equals(moduleAddr) || from.Equals(moduleAddr)

		// Once realised, every token is a claim on a known fixed sum. A market
		// still quoting a price for a position that no longer changes is a trap
		// for whoever buys last, so trading stops.
		//
		// The module account is exempt, and it has to be: allocation at close,
		// listing escrow and cancellation all move tokens to or from the
		// module, and a blanket halt would strand a seller's own escrowed
		// property inside the module for ever.
		if deal.State == types.DEAL_STATE_REALISED && !involvesModule {
			return to, types.ErrTradingHalted.Wrapf("deal %d", dealID)
		}

		// The gate. Only on the RECEIVING side, and only for a real account:
		// sending TO the module is escrow or redemption, and requiring the
		// module account to hold a KYC record would be theatre.
		//
		// Note what is not checked: whether the SENDER is still onboarded. An
		// investor whose onboarding is withdrawn keeps what they hold and keeps
		// being paid on it — their money is in a company and the chain cannot
		// get it out — but they may not acquire more. Blocking their sales
		// instead would strand exactly the person the platform has just decided
		// it no longer wants as a customer.
		if !to.Equals(moduleAddr) {
			addr, err := k.addressCodec.BytesToString(to)
			if err != nil {
				return to, err
			}
			if err := k.eligible(ctx, addr); err != nil {
				return to, err
			}
		}

		sdkCtx := sdk.UnwrapSDKContext(ctx)

		// Settle against the balances held BEFORE this transfer. Both sides,
		// because both are about to change.
		fromBalance := k.bankKeeper.GetBalance(sdkCtx, from, coin.Denom).Amount
		if err := k.Settle(ctx, dealID, from, fromBalance); err != nil {
			return to, err
		}
		toBalance := k.bankKeeper.GetBalance(sdkCtx, to, coin.Denom).Amount
		if err := k.Settle(ctx, dealID, to, toBalance); err != nil {
			return to, err
		}
	}

	return to, nil
}

// IsRestrictedDenom reports whether a denomination is a deal token.
//
// Asked by x/amm before it creates a pool. The answer is about the denom being
// a deal token AT ALL, not about today's state: a pool created over an open
// deal is exactly the trap, because the deal is realised later and the halt
// arrives after the liquidity providers' money is in.
func (k Keeper) IsRestrictedDenom(ctx context.Context, denom string) (bool, error) {
	if !strings.HasPrefix(denom, types.DenomPrefix) {
		return false, nil
	}
	return k.ByDenom.Has(ctx, denom)
}
