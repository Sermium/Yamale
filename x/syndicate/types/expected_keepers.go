package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/group"
)

// BankKeeper is what the syndication layer needs from x/bank.
//
// Minting is here because the deal token is a real bank denomination rather
// than a number in this module's own store, and that is a deliberate choice
// with a cost. A bookkeeping entry would be simpler and would need no send
// restriction — but it would also be invisible to every wallet, every explorer
// and every other module on the chain, and an investor would have to take the
// platform's word for what they hold. The whole complaint about a paper
// syndicate is that the cap table is a spreadsheet somebody else holds.
type BankKeeper interface {
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	SendCoins(ctx context.Context, from, to sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, from sdk.AccAddress, module string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, module string, to sdk.AccAddress, amt sdk.Coins) error
	MintCoins(ctx context.Context, module string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, module string, amt sdk.Coins) error
}

// GroupKeeper is asked one question, once, at admission: is this operator a
// group account?
//
// Checked rather than trusted to a human review step at proposal time. The
// alternative puts the whole protection in a step that will eventually be
// rushed, and the cost of checking is one lookup.
type GroupKeeper interface {
	GroupPolicyInfo(ctx context.Context, req *group.QueryGroupPolicyInfoRequest) (*group.QueryGroupPolicyInfoResponse, error)
}
