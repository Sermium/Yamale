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
	SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error
}

// StakingKeeper is consulted for one thing: what this chain's stake is called.
//
// The block reward used to be minted in sdk.DefaultBondDenom, a package-level
// variable that app/config.go's init() rewrites. It was correct only because
// the app package happened to be imported: a tool or test that linked
// x/emission without it would mint "stake", and a governance change to
// x/staking's bond_denom would leave emission minting the old denomination into
// the fee collector indefinitely, with nothing to notice.
//
// x/validatorgov and x/enforcement both ask the staking keeper for exactly this.
type StakingKeeper interface {
	BondDenom(context.Context) (string, error)
}

// ParamSubspace defines the expected Subspace interface for parameters.
type ParamSubspace interface {
	Get(context.Context, []byte, interface{})
	Set(context.Context, []byte, interface{})
}
