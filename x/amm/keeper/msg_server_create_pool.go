package keeper

import (
	"context"

	"yamale/blockchain/x/amm/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// CreatePool creates a new constant-product (x*y=k) pool for two denoms,
// funded by the creator's initial deposit. Initial LP shares are minted as
// sqrt(amountA*amountB), the standard Uniswap-v2-style bootstrap.
//
// # Permissionless, on a chain where nothing else that moves value is
//
// This is a deliberate inconsistency and it is worth stating rather than
// leaving to be discovered. Any signer may open a pool over any two valid
// denominations, including the 43 licensed fiat currencies and the LP shares of
// other pools — while x/paymsg, x/netting and x/stablecoin all gate the same
// currencies on approved participants or issuers and on the x/alias perimeter.
// Transferable LP shares are also a route for value to move between accounts
// that the perimeter never sees.
//
// The reasoning for leaving it open: this module exists to price the chain's
// own assets against each other, an AMM whose pools are a governance decision
// is not a market, and every path that could be used to launder value through a
// pool still ends in a bank transfer — which is where the freeze, the blocked
// module accounts and the tokenisation settle restriction all sit. What is NOT
// argued is that the perimeter covers this: it does not, and a deployment that
// needs it to will have to gate CreatePool on an approved-issuer check the way
// x/stablecoin gates minting.
//
// Recorded in docs/scope/gaps.md so the decision is reversible by whoever
// disagrees, rather than being an omission nobody made.
func (k msgServer) CreatePool(ctx context.Context, msg *types.MsgCreatePool) (*types.MsgCreatePoolResponse, error) {
	creatorBz, err := k.addressCodec.StringToBytes(msg.Creator)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}
	if msg.DenomA == msg.DenomB {
		return nil, types.ErrSameDenom
	}

	// Both denoms reach sdk.NewCoin below, which panics rather than returning
	// an error on an invalid one. That panic is recovered into a failed
	// transaction rather than halting the chain, but a handler should refuse
	// its own bad input instead of relying on a recover several layers up.
	for _, denom := range []string{msg.DenomA, msg.DenomB} {
		if err := sdk.ValidateDenom(denom); err != nil {
			return nil, errorsmod.Wrapf(types.ErrInvalidDenom, "%q: %s", denom, err)
		}
	}

	// The fee is chosen by whoever opens the pool and is never revisited, so it
	// is bounded here or nowhere. Above 10,000 basis points the fee arithmetic
	// in a swap goes negative, and the only thing that stopped the result being
	// exploitable was a guard documented as unreachable — not a margin worth
	// keeping. The cap is well below that: a pool taking more than a tenth of
	// every trade is a trap rather than a market.
	if msg.SwapFeeBps > types.MaxSwapFeeBps {
		return nil, errorsmod.Wrapf(types.ErrInvalidSwapFee,
			"swap fee must be at most %d basis points, got %d", types.MaxSwapFeeBps, msg.SwapFeeBps)
	}

	amountA, ok := math.NewIntFromString(msg.AmountA)
	if !ok || !amountA.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid amountA %s", msg.AmountA)
	}
	amountB, ok := math.NewIntFromString(msg.AmountB)
	if !ok || !amountB.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid amountB %s", msg.AmountB)
	}

	id, err := k.PoolSeq.Next(ctx)
	if err != nil {
		return nil, err
	}

	deposit := sdk.NewCoins(sdk.NewCoin(msg.DenomA, amountA), sdk.NewCoin(msg.DenomB, amountB))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sdk.AccAddress(creatorBz), types.ModuleName, deposit); err != nil {
		return nil, err
	}

	sqrtDec, err := math.LegacyNewDecFromInt(amountA).Mul(math.LegacyNewDecFromInt(amountB)).ApproxSqrt()
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to compute initial LP shares")
	}
	initialShares := sqrtDec.TruncateInt()
	if !initialShares.IsPositive() {
		return nil, errorsmod.Wrap(types.ErrInvalidAmount, "initial deposit too small to mint any LP shares")
	}

	if err := k.Pool.Set(ctx, id, types.Pool{
		Id:          id,
		DenomA:      msg.DenomA,
		DenomB:      msg.DenomB,
		ReserveA:    amountA.String(),
		ReserveB:    amountB.String(),
		TotalShares: initialShares.String(),
		SwapFeeBps:  msg.SwapFeeBps,
	}); err != nil {
		return nil, err
	}

	lpCoins := sdk.NewCoins(sdk.NewCoin(types.LPDenom(id), initialShares))
	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, lpCoins); err != nil {
		return nil, err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(creatorBz), lpCoins); err != nil {
		return nil, err
	}

	return &types.MsgCreatePoolResponse{}, nil
}
