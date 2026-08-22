package types

import (
	"context"

	"cosmossdk.io/core/address"
	sdk "github.com/cosmos/cosmos-sdk/types"

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
	SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error
}

// ScopeKeeper is the jurisdictional perimeter, and one method of it.
//
// Admitting a payment service provider is a payments authority's act, bounded by
// the same border as everything else: the authority of the country the applicant
// is recorded in, and no other.
//
// This module is the one place the perimeter cannot arrive through depinject.
// x/alias consults *this* module to find out who onboarded an account, so an
// edge back the other way is a cycle and the dependency graph refuses to build.
// It is therefore handed over after construction — see Keeper.SetScopeKeeper —
// and the check fails closed until it is, so a chain assembled without that
// wiring admits participants by governance vote and by nobody else, rather than
// admitting them by anybody.
type ScopeKeeper interface {
	// AssertScope is the gate. Nil means permitted; everything else refuses.
	AssertScope(ctx context.Context, actor string, role aliastypes.Role, target string) error

	// HoldsRole is for the error message and nothing else. This handler accepts
	// either governance or a payments authority, so a signer that is neither has
	// to be told it is not an authority rather than told something about the
	// applicant's jurisdiction. It permits nothing: AssertScope still runs.
	HoldsRole(ctx context.Context, actor string, role aliastypes.Role) (bool, error)
}

// ParamSubspace defines the expected Subspace interface for parameters.
type ParamSubspace interface {
	Get(context.Context, []byte, interface{})
	Set(context.Context, []byte, interface{})
}
