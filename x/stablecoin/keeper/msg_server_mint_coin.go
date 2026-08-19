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

	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		return nil, err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sdk.AccAddress(recipientBz), coins); err != nil {
		return nil, err
	}

	return &types.MsgMintCoinResponse{}, nil
}
