package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/oracle/types"
)

// The implementation of what everything downstream actually consumes.
//
// Both methods return a Price carrying its own age and staleness rather than a
// bare number, and neither refuses to answer just because a value is old. That
// distinction is deliberate: a consumer needs to tell "there has never been a
// price" from "there was one and the feed stopped", because the second is an
// emergency and the first is only a configuration gap. Hiding a stale value
// makes them look identical.
var _ types.PriceSource = Keeper{}

// PriceOf values an amount of a fungible denom in the quote currency.
func (k Keeper) PriceOf(ctx context.Context, denom string, amount math.Int) (types.Price, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return types.Price{}, err
	}

	rate, err := k.ExchangeRate.Get(ctx, denom)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Price{}, types.ErrRateUnavailable.Wrapf("no rate has ever been agreed for %s", denom)
		}
		return types.Price{}, err
	}

	value, err := math.LegacyNewDecFromStr(rate.Rate)
	if err != nil {
		return types.Price{}, types.ErrInvalidRate.Wrapf("stored rate for %s is unreadable: %s", denom, rate.Rate)
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	exponent := k.exponentOf(denom)

	return types.Price{
		Value:      types.ValueOf(amount, exponent, value),
		Denom:      params.QuoteSymbol,
		ObservedAt: rate.UpdatedAt,
		AgeSeconds: types.AgeSeconds(rate.UpdatedAt, now),
		Stale:      types.IsStale(rate.UpdatedAt, now, params.MaxRateAgeSeconds),
		Source:     "oracle",
	}, nil
}

// ValueOfAsset returns the current appraised value of a tokenised asset.
//
// Staleness is measured from the valuation date rather than the submission
// date. A quarterly valuation filed three months late describes a quarter-old
// world however recently it was typed in, and treating the two as the same
// would make a stale number look current at exactly the wrong moment.
func (k Keeper) ValueOfAsset(ctx context.Context, classID, nftID string) (types.Price, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return types.Price{}, err
	}

	appraisal, err := k.Appraisal.Get(ctx, collections.Join(classID, nftID))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Price{}, types.ErrAppraisalMissing.Wrapf("%s/%s has never been valued", classID, nftID)
		}
		return types.Price{}, err
	}

	value, ok := math.NewIntFromString(appraisal.Value)
	if !ok {
		return types.Price{}, types.ErrInvalidValuation.Wrapf("stored valuation for %s/%s is unreadable", classID, nftID)
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	return types.Price{
		Value:      value,
		Denom:      appraisal.ValueDenom,
		ObservedAt: appraisal.ValuedAt,
		AgeSeconds: types.AgeSeconds(appraisal.ValuedAt, now),
		Stale:      types.IsStale(appraisal.ValuedAt, now, params.MaxAppraisalAgeSeconds),
		Source:     appraisal.Appraiser,
	}, nil
}

// RequireFreshPrice is the form a lending or liquidation path should use.
//
// It refuses rather than returning a stale number, because those callers act on
// the answer immediately and the safe failure is to stop. A feed that has gone
// quiet and a market that has genuinely moved look identical from the last
// stored value, and liquidating somebody on a price nobody currently stands
// behind is the harm this module exists to prevent.
func (k Keeper) RequireFreshPrice(ctx context.Context, denom string, amount math.Int) (types.Price, error) {
	price, err := k.PriceOf(ctx, denom, amount)
	if err != nil {
		return types.Price{}, err
	}
	if price.Stale {
		return price, types.ErrStale.Wrapf(
			"the rate for %s is %d seconds old; refusing to act on it", denom, price.AgeSeconds)
	}
	return price, nil
}

// RequireFreshAssetValue is the equivalent for a tokenised asset.
func (k Keeper) RequireFreshAssetValue(ctx context.Context, classID, nftID string) (types.Price, error) {
	price, err := k.ValueOfAsset(ctx, classID, nftID)
	if err != nil {
		return types.Price{}, err
	}
	if price.Stale {
		return price, types.ErrStale.Wrapf(
			"the valuation of %s/%s is %d seconds old; refusing to act on it", classID, nftID, price.AgeSeconds)
	}
	return price, nil
}

// exponentOf returns the decimal places of a denom.
//
// The oracle deliberately does not read x/bank's denom metadata: a rate is
// agreed for a denom in the accepted set, and those are the chain's own
// six-decimal base units. Reaching into bank would make the oracle's answer
// depend on metadata an approved issuer controls, which is a wider trust
// surface than this module needs.
func (k Keeper) exponentOf(denom string) uint32 {
	if len(denom) > 1 && denom[0] == 'u' {
		return 6
	}
	return 0
}
