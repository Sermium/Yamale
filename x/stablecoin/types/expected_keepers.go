package types

import (
	"context"

	"cosmossdk.io/core/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	aliastypes "yamale/blockchain/x/alias/types"
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
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SetDenomMetaData(ctx context.Context, denomMetaData banktypes.Metadata)
}

// ScopeKeeper is the jurisdictional perimeter, and one method of it.
//
// Admitting the issuer of a currency is a monetary authority's act, and a
// monetary authority's powers stop at its border: Nigeria's central bank has no
// business admitting the issuer of a Senegalese currency. The target is the
// applicant account, whose country the chain holds — so this is the shape that
// resolves it, never the shape that lets the caller name it.
//
// Read-only, one method, and the dependency runs one way: x/alias knows nothing
// about this module.
type ScopeKeeper interface {
	// AssertScope is the gate. Nil means permitted; everything else refuses.
	AssertScope(ctx context.Context, actor string, role aliastypes.Role, target string) error

	// HoldsRole is for the error message and nothing else. This handler accepts
	// either governance or a monetary authority, so a signer that is neither has
	// to be told it is not an authority rather than told something about the
	// applicant's jurisdiction. It permits nothing: AssertScope still runs.
	HoldsRole(ctx context.Context, actor string, role aliastypes.Role) (bool, error)
}

// ParamSubspace defines the expected Subspace interface for parameters.
type ParamSubspace interface {
	Get(context.Context, []byte, interface{})
	Set(context.Context, []byte, interface{})
}
