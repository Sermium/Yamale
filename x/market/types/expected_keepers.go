package types

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper is the escrow, and every leg of every trade.
//
// Nothing in this module moves property any other way. That is not tidiness:
// x/tokenisation registers a send restriction that settles both sides of a
// share transfer BEFORE the balances change, because entitlement is an integral
// of balance over time and settling afterwards credits the seller's holding
// period to the buyer. A fill that bypassed the bank would be a transfer the
// register never saw, and the two accounts would disagree about who was owed
// the next distribution.
type BankKeeper interface {
	SendCoins(ctx context.Context, from, to sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, sender sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipient sdk.AccAddress, amt sdk.Coins) error
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}

// TokenisationKeeper is the register this market trades the shares of.
//
// # Why the market needs it at all
//
// Two things, and the second is the reason this module exists.
//
// The first is that a market has to know what it is pricing. A share
// denomination names a vehicle, that vehicle names the currency its income
// arrives in, and that currency is the only one a trade may settle in — because
// the cost this market records is carried in it, and a cost in one currency
// against a return in another is not a return, it is two unrelated numbers
// subtracted.
//
// The second is cost basis. A share moves by ordinary bank send and a send
// carries no consideration, so the register knows what everybody holds and
// nothing about what any of it cost. x/tokenisation permits exactly two events
// to write a cost: a subscription, where the party that took the money states
// the figure, and a marketplace fill, where nobody has to state anything
// because the price is in the trade. This module is the second one, and a fill
// that did not write it would leave the buyer's holding reading COST NOT
// RECORDED for ever — which is the whole question an investor has.
type TokenisationKeeper interface {
	// ShareAsset resolves a share denomination to the vehicle it belongs to,
	// the denomination that vehicle's income and recorded cost are carried in,
	// and whether its shares still move at all.
	//
	// Not found is an error rather than a zero asset id, because zero is a
	// perfectly good asset id everywhere else and a market that silently priced
	// the wrong vehicle would be worse than one that refused.
	ShareAsset(ctx context.Context, denom string) (assetID uint64, settlementDenom string, tradable bool, err error)

	// RecordPurchase writes what a buyer paid for shares a fill has just
	// delivered, adding to whatever they had already recorded.
	RecordPurchase(ctx context.Context, assetID uint64, buyer sdk.AccAddress, quantity, paid math.Int) error

	// ReleaseBasis consumes a seller's recorded cost for shares they have sold,
	// so their remaining cost stays matched to their remaining shares.
	ReleaseBasis(ctx context.Context, assetID uint64, seller sdk.AccAddress, quantity math.Int) error
}
