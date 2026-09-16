package keeper

import (
	"bytes"
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/market/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns the write side of the market.
func NewMsgServerImpl(k Keeper) types.MsgServer {
	return &msgServer{Keeper: k}
}

var _ types.MsgServer = msgServer{}

// UpdateParams sets the market-wide limits. Governance only.
func (m msgServer) UpdateParams(
	ctx context.Context, msg *types.MsgUpdateParams,
) (*types.MsgUpdateParamsResponse, error) {
	authority, err := m.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(authority, m.GetAuthority()) {
		return nil, types.ErrNotAuthority.Wrapf("expected %s", m.GetAuthority())
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, types.ErrInvalidParams.Wrap(err.Error())
	}
	return &types.MsgUpdateParamsResponse{}, m.Params.Set(ctx, msg.Params)
}

/* ========================================================================== */
/*  The one thing every mechanism does                                        */
/* ========================================================================== */

// deliver hands escrowed shares to a buyer and records the trade against both
// sides' cost.
//
// Every fill in this module ends here, whichever mechanism produced it, and it
// is deliberately the only place that touches cost basis. Three mechanisms each
// writing their own would be three chances to write it differently, and the
// failure would be invisible: a cost basis that is wrong still renders as a
// percentage.
//
// The shares come OUT OF ESCROW rather than out of the seller's account, which
// is why every mechanism escrows first. That is not only about the seller being
// unable to offer the same shares twice — it is what makes this function safe to
// call in a loop. By the time it runs, the shares are already the module's, so
// the only way it can fail is a refusal the send restriction raises about the
// buyer, and it raises that before anything has moved.
//
// The send goes through the bank because x/tokenisation's restriction settles
// both sides of a share transfer before the balances change. A delivery by any
// other route would credit the seller's holding period to the buyer, and the
// next distribution would pay the wrong person.
func (k Keeper) deliver(
	ctx context.Context,
	assetID uint64,
	denom string,
	seller, buyer sdk.AccAddress,
	quantity math.Int,
	paid sdk.Coin,
) error {
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, buyer, sdk.NewCoins(sdk.NewCoin(denom, quantity)),
	); err != nil {
		return err
	}

	// The seller's cost for what they sold is consumed, and the buyer's is
	// written from the price. Both, and in that order, because they are two
	// halves of one fact: the shares that stopped being covered on one side are
	// the shares that started being covered on the other.
	//
	// This is the whole reason a marketplace is worth building on top of a
	// register that already transfers shares perfectly well. A bank send names
	// no price, so a register built only on sends knows what everybody holds and
	// nothing about what any of it cost — and "what did my stake return" is the
	// only question most holders ever ask.
	if err := k.tokenisationKeeper.ReleaseBasis(ctx, assetID, seller, quantity); err != nil {
		return err
	}
	if err := k.tokenisationKeeper.RecordPurchase(
		ctx, assetID, buyer, quantity, paid.Amount,
	); err != nil {
		return err
	}

	sellerStr, err := k.addressCodec.BytesToString(seller)
	if err != nil {
		return err
	}
	buyerStr, err := k.addressCodec.BytesToString(buyer)
	if err != nil {
		return err
	}
	// Emitted rather than stored. The pairing is what an indexer wants and what
	// consensus does not: keeping every fill for ever would grow the state by
	// the one thing that is pure history, and the chain already holds the orders
	// and the balances it was computed from.
	return sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.Fill{
		AssetId:  assetID,
		Denom:    denom,
		Buyer:    buyerStr,
		Seller:   sellerStr,
		Quantity: quantity,
		Paid:     paid,
	})
}
