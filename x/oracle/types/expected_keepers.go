package types

import (
	"context"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// AuthKeeper defines the expected interface for the Auth module.
type AuthKeeper interface {
	AddressCodec() address.Codec
	GetAccount(context.Context, sdk.AccAddress) sdk.AccountI
}

// BankKeeper is used only by the module's simulation, to size the fees a
// randomly generated transaction can afford. Nothing in the oracle moves funds:
// it records what things are worth and never holds any of them.
type BankKeeper interface {
	SpendableCoins(context.Context, sdk.AccAddress) sdk.Coins
}

// StakingKeeper is how the module learns who may vote and how much their vote
// weighs. Rates are agreed by the same stake that secures the chain, so
// capturing the oracle is no cheaper than capturing consensus.
type StakingKeeper interface {
	GetLastTotalPower(ctx context.Context) (math.Int, error)
	Validator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.ValidatorI, error)
	IterateBondedValidatorsByPower(ctx context.Context, fn func(index int64, validator stakingtypes.ValidatorI) (stop bool)) error
}

// NFTKeeper is how the module confirms that an appraised asset actually
// exists. Without this an appraiser could publish valuations for tokens that
// were never minted, and a lending module reading them would extend credit
// against nothing.
type NFTKeeper interface {
	HasClass(ctx context.Context, classID string) bool
	HasNFT(ctx context.Context, classID, nftID string) bool
	GetOwner(ctx context.Context, classID, nftID string) sdk.AccAddress
}

// Price is a value together with everything a consumer needs to decide whether
// to act on it.
//
// The staleness flag travels with the number deliberately. A caller that
// received only the value would have to remember to check the age separately,
// and the one time somebody forgets is the time the feed had stopped.
type Price struct {
	// Value is in base units of Denom.
	Value math.Int
	Denom string
	// ObservedAt is when the underlying observation was made — the vote round
	// for a rate, the inspection date for an appraisal — not when the chain
	// recorded it.
	ObservedAt int64
	AgeSeconds int64
	// Stale is true when the value is older than its kind's maximum age. It is
	// never safe to lend, liquidate or margin-call on a stale price.
	Stale bool
	// Source names where the number came from: the oracle's agreed rate, or the
	// address of the valuer who signed it.
	Source string
}

// PriceSource is the whole of what a lending module needs from this one.
//
// Keeping the boundary this narrow is what makes the trust model swappable. A
// consumer written against this interface neither knows nor cares whether a
// number arrived by validator vote, by an appraiser's signature, or over IBC
// from another chain — so the decision about where prices come from can change
// without touching the code that lends against them.
type PriceSource interface {
	// PriceOf values an amount of a fungible denom in the quote currency.
	// Returns ErrRateUnavailable when no rate has ever been agreed, and a
	// Price marked Stale when one has but is too old.
	PriceOf(ctx context.Context, denom string, amount math.Int) (Price, error)

	// ValueOfAsset returns the current appraised value of a tokenised asset.
	// Returns ErrAppraisalMissing when it has never been valued.
	ValueOfAsset(ctx context.Context, classID, nftID string) (Price, error)
}
