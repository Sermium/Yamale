package types

import (
	"context"
	"time"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// AuthKeeper is how the module recognises a module account.
//
// Module accounts hold the chain's own funds — the bonded pool, the fee
// collector, every custody account in this repository — and freezing one would
// stop staking, distribution or payments for everybody. There is no legitimate
// case against an address nobody holds a key for, so they are refused outright
// rather than left to the good judgement of whoever is voting at the time.
type AuthKeeper interface {
	AddressCodec() address.Codec
	GetAccount(context.Context, sdk.AccAddress) sdk.AccountI
}

// BankKeeper is what the module reads to know what is there, and uses to move
// it once the validators have said so.
type BankKeeper interface {
	GetAllBalances(ctx context.Context, addr sdk.AccAddress) sdk.Coins
	SpendableCoins(ctx context.Context, addr sdk.AccAddress) sdk.Coins
	SendCoins(ctx context.Context, from, to sdk.AccAddress, amt sdk.Coins) error
	BlockedAddr(addr sdk.AccAddress) bool
}

// StakingKeeper is how the module learns who may vote, how much their vote
// weighs, and what a target has staked.
//
// The voting power is the same power that secures the chain: this module hands
// the validator set an authority over other people's assets, and tying it to
// consensus weight at least means the authority cannot be bought more cheaply
// than the chain itself.
type StakingKeeper interface {
	GetLastTotalPower(ctx context.Context) (math.Int, error)
	Validator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.ValidatorI, error)
	BondDenom(ctx context.Context) (string, error)

	// GetDelegatorDelegations and Undelegate are what turn staked funds back
	// into recoverable ones. A scammer who delegates is not out of reach; they
	// have only bought the unbonding period.
	GetDelegatorDelegations(ctx context.Context, delegator sdk.AccAddress, maxRetrieve uint16) ([]stakingtypes.Delegation, error)
	Undelegate(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress, sharesAmount math.LegacyDec) (time.Time, math.Int, error)

	// GetUnbondingDelegations is how a sweep knows whether anything is still on
	// its way back, and therefore whether it is finished.
	GetUnbondingDelegations(ctx context.Context, delegator sdk.AccAddress, maxRetrieve uint16) ([]stakingtypes.UnbondingDelegation, error)
}
