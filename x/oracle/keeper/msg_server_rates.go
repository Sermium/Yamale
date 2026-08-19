package keeper

import (
	"bytes"
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/oracle/types"
)

// SubmitExchangeRates records one validator's report for the current round.
//
// Nothing is aggregated here. A vote is stored as received and the rate is only
// agreed at the end of the period, in the EndBlocker, where every validator's
// report is visible at once. Deciding a price the moment a report arrives would
// let whoever transacts last in a block set it.
func (k msgServer) SubmitExchangeRates(ctx context.Context, msg *types.MsgSubmitExchangeRates) (*types.MsgSubmitExchangeRatesResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	valAddr, err := sdk.ValAddressFromBech32(msg.Validator)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid validator address")
	}

	// Only bonded validators may report, because only bonded stake carries
	// weight in the median. Accepting a vote that will be discarded at tally
	// time would tell the sender their report counted when it did not.
	validator, err := k.stakingKeeper.Validator(ctx, valAddr)
	if err != nil || validator == nil {
		return nil, errorsmod.Wrapf(types.ErrUnknownValidator, "%s", msg.Validator)
	}
	if !validator.IsBonded() {
		return nil, errorsmod.Wrapf(types.ErrUnknownValidator, "%s is not bonded", msg.Validator)
	}

	if err := k.assertFeeder(ctx, msg.Validator, valAddr, msg.Feeder); err != nil {
		return nil, err
	}

	if len(msg.Rates) == 0 {
		return nil, errorsmod.Wrap(types.ErrInvalidRate, "submission contains no rates")
	}
	// A submission may not exceed the accepted set, which also bounds the work
	// one transaction can create.
	if len(msg.Rates) > len(params.AcceptedDenoms) {
		return nil, errorsmod.Wrapf(types.ErrLimitReached,
			"submission has %d rates but only %d denoms are accepted", len(msg.Rates), len(params.AcceptedDenoms))
	}

	seen := make(map[string]bool, len(msg.Rates))
	for _, vote := range msg.Rates {
		if !params.Accepts(vote.Denom) {
			return nil, errorsmod.Wrapf(types.ErrDenomNotAccepted, "%s", vote.Denom)
		}
		// Two prices for one denom in one submission is not a report the chain
		// can act on, and silently keeping the last would make the outcome
		// depend on field order.
		if seen[vote.Denom] {
			return nil, errorsmod.Wrapf(types.ErrDenomNotAccepted, "%s appears twice in one submission", vote.Denom)
		}
		seen[vote.Denom] = true

		// LegacyDec caps the fractional side at 18 places but puts no limit on
		// the integer part, so an unbounded string here is bignum parsing that
		// every validator repeats — on submission, and again on every tally
		// until the round closes.
		if len(vote.Rate) > types.MaxRateLength {
			return nil, errorsmod.Wrapf(types.ErrLimitReached,
				"a rate must be at most %d bytes, got %d", types.MaxRateLength, len(vote.Rate))
		}

		rate, err := math.LegacyNewDecFromStr(vote.Rate)
		if err != nil {
			return nil, errorsmod.Wrapf(types.ErrInvalidRate, "%s: %s", vote.Denom, vote.Rate)
		}
		if !rate.IsPositive() {
			return nil, errorsmod.Wrapf(types.ErrInvalidRate, "%s must be positive, got %s", vote.Denom, vote.Rate)
		}

		// Setting replaces any earlier report for the same (validator, denom)
		// this round, so re-submitting is a correction rather than a second
		// vote. The alternative — rejecting the second — would leave a feeder
		// that noticed its own mistake unable to fix it.
		if err := k.Vote.Set(ctx, collections.Join(msg.Validator, vote.Denom), types.ExchangeRateVote{
			Validator: msg.Validator,
			Denom:     vote.Denom,
			Rate:      rate.String(),
		}); err != nil {
			return nil, err
		}
	}

	return &types.MsgSubmitExchangeRatesResponse{}, nil
}

// DelegateFeeder nominates the account allowed to vote for a validator.
//
// Signed by the validator's own account, never by the current feeder: a feeder
// that could nominate its successor would be able to keep the delegation alive
// after the validator tried to revoke it, which defeats the point of the hot
// key being disposable.
func (k msgServer) DelegateFeeder(ctx context.Context, msg *types.MsgDelegateFeeder) (*types.MsgDelegateFeederResponse, error) {
	valAddr, err := sdk.ValAddressFromBech32(msg.Validator)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid validator address")
	}

	validator, err := k.stakingKeeper.Validator(ctx, valAddr)
	if err != nil || validator == nil {
		return nil, errorsmod.Wrapf(types.ErrUnknownValidator, "%s", msg.Validator)
	}

	operator, err := k.addressCodec.StringToBytes(msg.Operator)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid operator address")
	}
	// The operator account is the validator address read as an account: same
	// bytes, different prefix.
	if !sdk.AccAddress(valAddr).Equals(sdk.AccAddress(operator)) {
		return nil, errorsmod.Wrapf(types.ErrNotTheFeeder,
			"only %s may change who votes for %s", sdk.AccAddress(valAddr).String(), msg.Validator)
	}

	feeder, err := k.addressCodec.StringToBytes(msg.Feeder)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid feeder address")
	}

	// Delegating back to the operator removes the delegation rather than
	// storing it, so the stored state means exactly one thing: a hot key is in
	// use. It also keeps derived state out of the export — see the same rule
	// applied to lock counts in x/treasury.
	if sdk.AccAddress(valAddr).Equals(sdk.AccAddress(feeder)) {
		return &types.MsgDelegateFeederResponse{}, k.Feeder.Remove(ctx, msg.Validator)
	}

	return &types.MsgDelegateFeederResponse{}, k.Feeder.Set(ctx, msg.Validator, msg.Feeder)
}

// assertFeeder checks that signer may vote on the validator's behalf.
func (k Keeper) assertFeeder(ctx context.Context, validator string, valAddr sdk.ValAddress, signer string) error {
	signerAddr, err := k.addressCodec.StringToBytes(signer)
	if err != nil {
		return errorsmod.Wrap(err, "invalid feeder address")
	}

	feeder, err := k.Feeder.Get(ctx, validator)
	switch {
	case err == nil:
		delegated, err := k.addressCodec.StringToBytes(feeder)
		if err != nil {
			return errorsmod.Wrap(err, "stored feeder delegation is unreadable")
		}
		if !bytes.Equal(delegated, signerAddr) {
			return errorsmod.Wrapf(types.ErrNotTheFeeder, "%s may not vote for %s", signer, validator)
		}
	case errors.Is(err, collections.ErrNotFound):
		// No delegation: the validator votes with its own account.
		if !sdk.AccAddress(valAddr).Equals(sdk.AccAddress(signerAddr)) {
			return errorsmod.Wrapf(types.ErrNotTheFeeder, "%s may not vote for %s", signer, validator)
		}
	default:
		return err
	}

	return nil
}

// FeederOf returns the account currently allowed to vote for a validator,
// which is the validator's own account when nothing has been delegated.
func (k Keeper) FeederOf(ctx context.Context, validator string) (string, error) {
	feeder, err := k.Feeder.Get(ctx, validator)
	if err == nil {
		return feeder, nil
	}
	if !errors.Is(err, collections.ErrNotFound) {
		return "", err
	}

	valAddr, err := sdk.ValAddressFromBech32(validator)
	if err != nil {
		return "", errorsmod.Wrap(err, "invalid validator address")
	}
	return k.addressCodec.BytesToString(sdk.AccAddress(valAddr))
}
