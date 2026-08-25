package types

import (
	"context"
	"time"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	aliastypes "yamale/blockchain/x/alias/types"
	constitutiontypes "yamale/blockchain/x/constitution/types"
)

// ScopeKeeper is the jurisdictional perimeter, and one method of it.
//
// The thing this module acts on is always an account, and the country an account
// is in is a fact the chain holds rather than one the accuser gets to state — so
// this is the shape that resolves the target itself. The other shape, where the
// jurisdiction is named in the message, is deliberately not here: an accuser who
// could tell the check which perimeter their target is in could reach anybody by
// naming their own.
//
// Read-only, one method. x/alias knows nothing about this module, so the
// dependency runs one way and the perimeter cannot be widened from inside the
// module it constrains.
type ScopeKeeper interface {
	// AssertScope returns nil only when the actor holds a grant of the role
	// covering the country the target account is recorded in. A target the chain
	// cannot place is an error, not a match.
	AssertScope(ctx context.Context, actor string, role aliastypes.Role, target string) error

	// HoldsRole reports whether an account holds a role in any jurisdiction at
	// all. It is NOT an authorisation check and nothing is permitted on the
	// strength of it — AssertScope still runs and is still the only thing that
	// permits anything.
	//
	// It is here for two jobs, and both are about telling the truth rather than
	// about deciding:
	//
	//   - OpenCase accepts either a bonded validator or an enforcement office, so
	//     it has to choose which refusal to report to somebody who is neither.
	//     Without this, an ordinary account opening a case would be told its
	//     target has no recorded jurisdiction, which sends the reader after a
	//     bug in the wrong module.
	//   - UpdateParams refuses an ombudsman that already holds
	//     ROLE_ENFORCEMENT_AUTHORITY, which is the parameter-time half of the
	//     exclusion the retired emergency_authority field used to carry. That
	//     question is about the holder and not about any target, so AssertScope
	//     is the wrong shape for it: there is no account to resolve a country
	//     from.
	HoldsRole(ctx context.Context, actor string, role aliastypes.Role) (bool, error)
}

// ConstitutionKeeper is where this module's non-negotiable parameters really
// live.
//
// One method, and it only reads. The point is that the module able to freeze
// and seize cannot move the numbers that bound it: the threshold, the
// destination and the two delays are held somewhere it can consult and nowhere
// it can write. x/constitution depends on x/staking and on nothing in this
// repository, so consulting it costs no dependency cycle.
type ConstitutionKeeper interface {
	GetInvariants(ctx context.Context) (constitutiontypes.Invariants, error)
}

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
