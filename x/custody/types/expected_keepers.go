package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper is the narrowest surface this module needs.
//
// Deliberately not the whole bank keeper: custody mints claims against attested
// deposits and burns them on redemption, and it has no business moving anyone
// else's money. A wide interface here would be a capability grant disguised as
// a dependency.
type BankKeeper interface {
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipient sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, sender sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	GetSupply(ctx context.Context, denom string) sdk.Coin
	// GetBalance is how the module reads its own accumulated fee revenue.
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}
