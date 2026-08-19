package keeper

import (
	"context"

	"yamale/blockchain/x/stablecoin/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BurnCoin lets a denom's approved issuer burn supply from their own
// balance. Only the issuer recorded by ApproveIssuer may burn that denom.
func (k msgServer) BurnCoin(ctx context.Context, msg *types.MsgBurnCoin) (*types.MsgBurnCoinResponse, error) {
	issuerBz, err := k.addressCodec.StringToBytes(msg.Issuer)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid issuer address")
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
		return nil, errorsmod.Wrapf(types.ErrInvalidAmount, "invalid burn amount %s", msg.Amount)
	}
	coins := sdk.NewCoins(sdk.NewCoin(msg.Denom, amount))

	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sdk.AccAddress(issuerBz), types.ModuleName, coins); err != nil {
		return nil, err
	}
	if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
		return nil, err
	}

	return &types.MsgBurnCoinResponse{}, nil
}
