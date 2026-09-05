package types

import (
	"context"

	"cosmossdk.io/core/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// AuthKeeper defines the expected interface for the Auth module.
type AuthKeeper interface {
	AddressCodec() address.Codec
	GetAccount(context.Context, sdk.AccAddress) sdk.AccountI // only used for simulation
	// Methods imported from account should be defined here
}

// BankKeeper defines the expected interface for the Bank module.
type BankKeeper interface {
	SpendableCoins(context.Context, sdk.AccAddress) sdk.Coins
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}

// RestrictedDenomKeeper answers whether a denomination carries a transfer
// restriction the AMM cannot live with.
//
// A pool pays out both reserve legs in one SendCoins, so a denom whose transfer
// can be refused takes the whole payout — and the other leg — down with it. The
// tokenisation module's fraction denoms are exactly that: once a vehicle is
// realised, its send restriction halts every transfer of the fraction token
// that is not to the tokenisation module account, and a pool is a different
// module account. An LP's counter-asset is then locked in the pool forever.
//
// So CreatePool refuses a denom this reports as restricted. The dependency is
// read-only and optional: on a build without the tokenisation module there are
// no fraction denoms and nothing to refuse.
type RestrictedDenomKeeper interface {
	// IsRestrictedDenom reports whether a denom must be kept out of a pool. An
	// error is a refusal, not a pass: a pool the AMM could not vet is a pool it
	// should not have created.
	IsRestrictedDenom(ctx context.Context, denom string) (bool, error)
}

// ParamSubspace defines the expected Subspace interface for parameters.
type ParamSubspace interface {
	Get(context.Context, []byte, interface{})
	Set(context.Context, []byte, interface{})
}
