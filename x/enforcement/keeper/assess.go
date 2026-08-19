package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
)

// assessSeizableValue is what a seizure against this target would be worth:
// what is in the account, what is staked, and what is already on its way back.
//
// All three, not just the balance, and that is the point of the function. The
// delay a seizure waits and the room it takes in the rolling cap are both sized
// from this number, so a measure that counted only liquid funds would let
// anybody holding their money in a validator have the largest seizure on the
// chain treated as the smallest — a few minutes' delay and almost no room taken
// in the window — while the seizure went on to collect the whole stake weeks
// later through Sweep. The stake is out of reach for a while; it is not out of
// scope.
//
// Staked amounts are converted through the validator that holds them rather
// than being read as shares. Shares are not tokens: a validator that has been
// slashed returns fewer tokens per share, and sizing a delay from shares would
// over-state a seizure against a slashed validator's delegators and under-state
// one against nobody in particular.
//
// Bounded by the same delegation limit the seizure itself unbonds under. A
// target with more delegations than that is assessed at less than they hold,
// which sizes the delay short — but assessing funds the seizure would not reach
// in the same block would be sizing the delay against a number that is not what
// happens.
func (k Keeper) assessSeizableValue(ctx context.Context, target sdk.AccAddress) (sdk.Coins, error) {
	total := k.bankKeeper.GetAllBalances(ctx, target)

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	delegations, err := k.stakingKeeper.GetDelegatorDelegations(ctx, target, types.MaxDelegationsAssessed)
	if err != nil {
		return nil, err
	}
	staked := math.ZeroInt()
	for _, delegation := range delegations {
		valAddr, err := sdk.ValAddressFromBech32(delegation.ValidatorAddress)
		if err != nil {
			return nil, err
		}
		validator, err := k.stakingKeeper.Validator(ctx, valAddr)
		if err != nil || validator == nil {
			// A delegation to a validator the staking module no longer knows
			// about is skipped rather than fatal. It contributes nothing to the
			// assessment, which under-states the case; failing the whole
			// resolution instead would leave the case unresolvable and the
			// account frozen indefinitely, which is worse for the person it is
			// frozen against.
			continue
		}
		staked = staked.Add(validator.TokensFromShares(delegation.Shares).TruncateInt())
	}

	unbondings, err := k.stakingKeeper.GetUnbondingDelegations(ctx, target, types.MaxDelegationsAssessed)
	if err != nil {
		return nil, err
	}
	for _, unbonding := range unbondings {
		for _, entry := range unbonding.Entries {
			staked = staked.Add(entry.Balance)
		}
	}

	if staked.IsPositive() {
		total = total.Add(sdk.NewCoin(bondDenom, staked))
	}
	return total, nil
}
