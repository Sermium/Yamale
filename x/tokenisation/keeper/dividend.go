package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

/* ========================================================================== */
/*  The dividend token, and the exchange behind it                            */
/* ========================================================================== */
//
// A claim pays div/<asset>/<SYMBOL> and the stable it stands for moves out of
// the vehicle's spendable ledger into its dividend reserve. An exchange burns
// the token and pays the stable back, 1:1, out of that reserve alone.
//
// # What backs it, which is the only question that matters
//
// The invariant is one line, and it holds per vehicle at every height:
//
//	supply(div/<asset>/<SYMBOL>)  ==  vault.DividendReserve.AmountOf(vault.Denom)
//
// It holds because mintDividend and burnDividend below are the only two
// functions that touch either side, and each moves both sides by the same
// amount in the same write. Nothing else mints the denom; nothing else spends
// the reserve.
//
// The consequence is the property the product needs: the exchange cannot run
// dry against a correctly-funded vehicle. Every token that exists was issued
// against stable already collected, already in the module account, and already
// taken off the pool that a later distribution or a redemption could be paid
// from.
//
// # Why mintDividend takes the money OUT of Held rather than tagging it
//
// Held is the figure creditHolders checks before it will credit a
// distribution, and the figure release() checks before any payout. Leaving a
// claimed dividend inside it would mean a second distribution could be credited
// against stable already owed to a dividend token, and the vehicle would then
// owe the same unit twice: once to the token, once to the index. The first
// holder to reach either window gets paid and the second is refused — by which
// time the money is gone and the refusal looks like a bug rather than the theft
// it is. Segregation is what makes the double-promise impossible rather than
// merely unlikely.
//
// # Why this is not routed through x/treasury
//
// x/treasury is a real module on this chain and it is a genuinely good fit on
// paper: it holds funds in a module account, it keeps a per-treasury ledger
// keyed (treasury_id, denom), and it has a locked-versus-available split that
// looks very like reserved-versus-held. It was rejected, for three reasons.
//
//  1. Every outbound path in x/treasury is DISCRETIONARY. A spend needs an
//     admin or a ROLE_SPENDER, it is checked against a spend policy, and
//     Treasury.paused halts claiming outright. That is correct for an
//     operating budget and wrong for this: by the time a dividend token exists
//     the chain has already told its holder the money is theirs, and putting a
//     role check or a pause switch between them and it means the chain can
//     default on a debt it has already reserved for. An exchange that can be
//     paused is not a guarantee, it is an intention.
//
//  2. The invariant is about a VEHICLE, and x/treasury has no concept of one.
//     It cannot refuse a spend on the grounds that it would leave a dividend
//     token unbacked, because it does not know the token exists. Teaching it
//     would mean x/treasury importing x/tokenisation — the dependency running
//     the wrong way, from the general module to the specific one, which is the
//     same inversion the land bridge was built to avoid.
//
//  3. The money would have to physically move to the treasury module account,
//     and at that moment it leaves the per-vehicle Held ledger that
//     TestASaleNobodyPaidForCannotDrainAnotherVehicle exists to protect. One
//     module account serving every vehicle is exactly why that ledger was
//     written; handing the balance to a second module account serving every
//     TREASURY restates the problem one module over.
//
// What is kept from x/treasury is the idea rather than the code: a committed
// balance that no ordinary path can reach. DividendReserve is that idea, sized
// to one vehicle and enforced by the module that knows what a vehicle owes.

// dividendDenom is the denomination claims against this vehicle are paid in.
//
// Refuses rather than returning an empty denom, because minting against "" is
// minting one denomination across every unfractionalised vehicle on the chain.
func (k Keeper) dividendDenom(ctx context.Context, assetID uint64) (string, error) {
	asset, err := k.Assets.Get(ctx, assetID)
	if err != nil {
		return "", types.ErrAssetNotFound
	}
	denom := types.DividendDenom(asset.FractionDenom)
	if denom == "" {
		return "", types.ErrNoDividendDenom
	}
	return denom, nil
}

// mintDividend converts a settled entitlement into a dividend token.
//
// Both legs in one function because they must never happen separately. The
// stable comes off Held through release(), which is the existing check that a
// vehicle cannot pay out of a neighbour's money, and lands in DividendReserve;
// the token is minted for exactly the same amount. A caller that could do one
// without the other would be minting an unbacked claim or stranding stable
// nothing can reach.
func (k Keeper) mintDividend(
	ctx context.Context, assetID uint64, holder sdk.AccAddress, amount math.Int,
) (paid sdk.Coin, backing sdk.Coin, err error) {
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	denom, err := k.dividendDenom(ctx, assetID)
	if err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	if !amount.IsPositive() {
		return sdk.Coin{}, sdk.Coin{}, types.ErrNothingOwed
	}

	backing = sdk.NewCoin(vault.Denom, amount)

	// Off Held first. A claim this vehicle cannot cover fails here, before any
	// token exists, rather than succeeding into a reserve that is short.
	if err := k.release(ctx, assetID, backing); err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	// Re-read: release() wrote the vault, and holding the stale copy would undo
	// the debit it just made.
	vault, err = k.Vaults.Get(ctx, assetID)
	if err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	vault.DividendReserve = sdk.Coins(vault.DividendReserve).Add(backing)
	if err := k.Vaults.Set(ctx, assetID, vault); err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}

	paid = sdk.NewCoin(denom, amount)
	coins := sdk.NewCoins(paid)
	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, holder, coins); err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	return paid, backing, nil
}

// burnDividend is the other half: token in, stable out, 1:1.
//
// Ordered so that the token is destroyed before the stable moves. The reverse
// order would leave a window inside the handler where the holder has both, and
// although a failure later in the same message reverts everything, an ordering
// that is only safe because of the revert is one a future refactor breaks
// silently.
func (k Keeper) burnDividend(
	ctx context.Context, assetID uint64, holder sdk.AccAddress, amount math.Int,
) (paid sdk.Coin, burned sdk.Coin, err error) {
	// The denom first, so an asset that was never fractionalised is refused with
	// a reason rather than with the store's "no such key" for a vault that was
	// never created either. Two absences, and only one of them is the caller's
	// mistake.
	denom, err := k.dividendDenom(ctx, assetID)
	if err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	if !amount.IsPositive() {
		return sdk.Coin{}, sdk.Coin{}, types.ErrInvalidAmount
	}

	burned = sdk.NewCoin(denom, amount)
	paid = sdk.NewCoin(vault.Denom, amount)

	// The reserve is checked before the bank keeper is asked, for the same
	// reason release() is: the module account holds every vehicle's money, so a
	// payout the bank keeper can settle is not necessarily one this vehicle can
	// afford. This should be unreachable — the supply and the reserve move
	// together — which is exactly why it must fail rather than proceed.
	reserve := sdk.Coins(vault.DividendReserve)
	if reserve.AmountOf(paid.Denom).LT(amount) {
		return sdk.Coin{}, sdk.Coin{}, types.ErrReserveUnfunded.Wrapf(
			"vehicle %d has reserved %s%s and is being asked for %s",
			assetID, reserve.AmountOf(paid.Denom), paid.Denom, paid)
	}

	burnCoins := sdk.NewCoins(burned)
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, holder, types.ModuleName, burnCoins); err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, burnCoins); err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}

	remaining, negative := reserve.SafeSub(paid)
	if negative {
		return sdk.Coin{}, sdk.Coin{}, types.ErrReserveUnfunded.Wrapf(
			"vehicle %d has reserved %s and is being asked for %s", assetID, reserve, paid)
	}
	vault.DividendReserve = remaining
	if err := k.Vaults.Set(ctx, assetID, vault); err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}

	if err := k.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, holder, sdk.NewCoins(paid)); err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}
	return paid, burned, nil
}

// DividendOutstanding is what a vehicle still owes its dividend token, read
// from the bank keeper's supply rather than from this module's ledger.
//
// Deliberately the other source. The reserve figure beside it comes from this
// module's own state, so a query that took both from the same place would
// compare a number with itself and always agree — which is not a check.
func (k Keeper) DividendOutstanding(ctx context.Context, assetID uint64) (sdk.Coin, error) {
	denom, err := k.dividendDenom(ctx, assetID)
	if err != nil {
		return sdk.Coin{}, err
	}
	return k.bankKeeper.GetSupply(sdk.UnwrapSDKContext(ctx), denom), nil
}

// ExchangeDividend cashes a dividend token for the stable behind it.
//
// No status check, on purpose. Every other payout path in this module gates on
// the vehicle's status because it is deciding whether money is owed; this one
// is not deciding anything. The debt crystallised when the token was minted and
// the stable was set aside then. A vehicle that has since been REALISED or
// CLOSED changes what its SHARES are worth and changes nothing about a dividend
// declared before the sale — and redemption cannot have spent the reserve,
// because redemption pays out of Held and the reserve is not in Held.
func (m msgServer) ExchangeDividend(
	ctx context.Context, msg *types.MsgExchangeDividend,
) (*types.MsgExchangeDividendResponse, error) {
	holder, err := m.addressCodec.StringToBytes(msg.Holder)
	if err != nil {
		return nil, err
	}
	if msg.Amount.IsNil() || !msg.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	paid, burned, err := m.Keeper.burnDividend(ctx, msg.AssetId, holder, msg.Amount)
	if err != nil {
		return nil, err
	}
	return &types.MsgExchangeDividendResponse{Paid: paid, Burned: burned}, nil
}
