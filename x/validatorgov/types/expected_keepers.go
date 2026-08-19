package types

import (
	"context"
	"time"

	"cosmossdk.io/core/address"
	authz "github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// StakingKeeper defines the expected interface for the Staking module.
type StakingKeeper interface {
	ConsensusAddressCodec() address.Codec
	ValidatorByConsAddr(context.Context, sdk.ConsAddress) (stakingtypes.ValidatorI, error)

	// GetValidator resolves the validator record behind an operator address.
	// A rotation may name an operator that was approved but never created a
	// validator, so the absence of a record is an ordinary outcome here rather
	// than an error.
	GetValidator(context.Context, sdk.ValAddress) (stakingtypes.Validator, error)

	// Jail and Unjail are how a contested validator is paused: removed from the
	// active set without being slashed and without its delegations unbonding.
	// Both panic in x/staking if the validator is already in the state being
	// asked for, so the module checks before it calls.
	Jail(context.Context, sdk.ConsAddress) error
	Unjail(context.Context, sdk.ConsAddress) error
}

// AuthzKeeper defines the expected interface for the Authz module.
//
// A completed rotation grants the incoming operator authorisation over the
// messages that operate the validator. There is no other way: x/staking keys a
// validator record — and every delegation pointing at it — by the operator
// address it was created with, and the SDK has no operation that re-keys one.
// Granting is what moves who signs while leaving the stake exactly where the
// guide says it stays.
type AuthzKeeper interface {
	SaveGrant(ctx context.Context, grantee, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) error
}

// AuthKeeper defines the expected interface for the Auth module.
type AuthKeeper interface {
	AddressCodec() address.Codec
	GetAccount(context.Context, sdk.AccAddress) sdk.AccountI // only used for simulation
	// Methods imported from account should be defined here
}

// BankKeeper defines the expected interface for the Bank module.
type BankKeeper interface {
	SpendableCoins(context.Context, sdk.AccAddress) sdk.Coins
	// Methods imported from bank should be defined here
}

// ParamSubspace defines the expected Subspace interface for parameters.
type ParamSubspace interface {
	Get(context.Context, []byte, interface{})
	Set(context.Context, []byte, interface{})
}
