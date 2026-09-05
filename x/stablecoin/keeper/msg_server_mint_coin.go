package keeper

import (
	"context"

	"yamale/blockchain/x/stablecoin/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MintCoin lets a denom's approved issuer mint new supply directly to a
// recipient. Only the issuer recorded by ApproveIssuer may mint that denom.
func (k msgServer) MintCoin(ctx context.Context, msg *types.MsgMintCoin) (*types.MsgMintCoinResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Issuer); err != nil {
		return nil, errorsmod.Wrap(err, "invalid issuer address")
	}
	recipientBz, err := k.addressCodec.StringToBytes(msg.Recipient)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid recipient address")
	}

	approved, err := k.ApprovedIssuer.Get(ctx, msg.Denom)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedIssuer, "%s has no approved issuer", msg.Denom)
	}
	if approved.Issuer != msg.Issuer {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedIssuer, "%s is not the approved issuer for %s", msg.Issuer, msg.Denom)
	}

	amount, ok := math.NewIntFromString(msg.Amount)
	if !ok || !amount.IsPositive() {
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid mint amount %s", msg.Amount)
	}
	coins := sdk.NewCoins(sdk.NewCoin(msg.Denom, amount))

	// The ceiling, and the reason it is on total supply rather than on the
	// mint: a per-transaction cap is a loop, and a per-period cap needs a clock
	// and a window to be reset against. What matters is how much of a currency
	// exists, so that is what is bounded.
	//
	// Until this check existed, being the recorded issuer was the whole of the
	// authorisation — no cap, no period limit, no reserve check anywhere in the
	// path. On a chain where one key was the approved issuer for all 43
	// currencies, that key could mint unlimited quantities of every national
	// currency the chain represented.
	//
	// A ceiling of zero refuses everything, which is the safe direction and the
	// one an upgraded chain lands in until governance states a figure.
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	ceiling := params.MintCeilingFor(msg.Denom)
	supply := k.bankKeeper.GetSupply(sdk.UnwrapSDKContext(ctx), msg.Denom).Amount
	if supply.Add(amount).GT(ceiling) {
		return nil, errorsmod.Wrapf(types.ErrMintCeiling,
			"%s has a ceiling of %s, %s already exists, and this would mint %s",
			msg.Denom, ceiling, supply, amount)
	}

	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		return nil, err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(recipientBz), coins); err != nil {
		return nil, err
	}

	return &types.MsgMintCoinResponse{}, nil
}
