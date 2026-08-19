package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"yamale/blockchain/x/enforcement/types"
)

// stakingValidator is the shape of what the staking keeper hands back. Aliased
// so the signature of bondedValidator reads as what it is.
type stakingValidator = stakingtypes.ValidatorI

// maxDelegations bounds the delegations one seizure will unbond in a single
// block. A target with more than this keeps the rest until the next sweep,
// which is slower but cannot become a block that never finishes.
const maxDelegations = 100

// passCase carries out a case the validators have agreed to.
//
// A freeze case ends here: the provisional freeze becomes one that does not
// lapse. A seizure goes further — every delegation is unbonded so that staked
// funds start coming back, and whatever is liquid right now is taken. What is
// still unbonding is collected later, by Sweep, which is why this is not the
// end of the story for a seizure.
func (k Keeper) passCase(ctx context.Context, enforcementCase *types.Case) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	enforcementCase.Status = types.CASE_STATUS_PASSED
	enforcementCase.ResolvedAtHeight = sdkCtx.BlockHeight()

	if err := k.makePermanent(ctx, enforcementCase.Target); err != nil {
		return err
	}
	if err := k.dequeueVoting(ctx, *enforcementCase); err != nil {
		return err
	}

	passed, err := k.CasesPassed.Get(ctx)
	if err != nil && !isNotFound(err) {
		return err
	}
	if err := k.CasesPassed.Set(ctx, passed+1); err != nil {
		return err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}

	if enforcementCase.Action == types.CASE_ACTION_SEIZE {
		if err := k.unbondEverything(ctx, enforcementCase.Target); err != nil {
			return err
		}
		if _, _, err := k.collect(ctx, enforcementCase, params); err != nil {
			return err
		}
	}

	if err := k.Case.Set(ctx, enforcementCase.Id, *enforcementCase); err != nil {
		return err
	}

	return sdkCtx.EventManager().EmitTypedEvent(&types.EventCaseResolved{
		CaseId:        enforcementCase.Id,
		Target:        enforcementCase.Target,
		Status:        enforcementCase.Status,
		YesPower:      enforcementCase.YesPower,
		NoPower:       enforcementCase.NoPower,
		RequiredPower: params.RequiredPower(enforcementCase.TotalPowerAtOpen),
	})
}

// rejectCase closes a case without carrying it out, and gives the account back.
// Used for an outright rejection, a withdrawal, and a case that simply ran out
// of time — three different statuses, one consequence.
func (k Keeper) rejectCase(ctx context.Context, enforcementCase *types.Case, status types.CaseStatus) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	enforcementCase.Status = status
	enforcementCase.ResolvedAtHeight = sdkCtx.BlockHeight()

	if err := k.Case.Set(ctx, enforcementCase.Id, *enforcementCase); err != nil {
		return err
	}
	if err := k.dequeueVoting(ctx, *enforcementCase); err != nil {
		return err
	}
	if err := k.liftFreeze(ctx, enforcementCase.Target, enforcementCase.Id, status); err != nil {
		return err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}

	return sdkCtx.EventManager().EmitTypedEvent(&types.EventCaseResolved{
		CaseId:        enforcementCase.Id,
		Target:        enforcementCase.Target,
		Status:        status,
		YesPower:      enforcementCase.YesPower,
		NoPower:       enforcementCase.NoPower,
		RequiredPower: params.RequiredPower(enforcementCase.TotalPowerAtOpen),
	})
}

// liftFreeze unfreezes an address and says so, but only if the freeze on it
// belongs to this case. A freeze imposed by a later case must survive an
// earlier one being resolved.
func (k Keeper) liftFreeze(ctx context.Context, addr string, caseID uint64, status types.CaseStatus) error {
	freeze, found, err := k.FreezeOf(ctx, addr)
	if err != nil || !found || freeze.CaseId != caseID {
		return err
	}
	if err := k.unfreeze(ctx, addr); err != nil {
		return err
	}
	return sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventFreezeLifted{
		Address: addr,
		CaseId:  caseID,
		Status:  status,
	})
}

func (k Keeper) dequeueVoting(ctx context.Context, enforcementCase types.Case) error {
	return k.VotingQueue.Remove(ctx, collections.Join(enforcementCase.VotingEndsAtHeight, enforcementCase.Id))
}

// unbondEverything starts the return of staked funds.
//
// Delegating is the obvious way to put money out of reach of a seizure, and
// this is the answer to it: the stake is unbonded on the spot, and the sweep
// collects it when the unbonding period ends. It does not shorten that period —
// nothing here overrides the chain's security assumptions — so a seizure
// against a staked target is not finished on the day it passes.
func (k Keeper) unbondEverything(ctx context.Context, target string) error {
	targetBz, err := k.addressCodec.StringToBytes(target)
	if err != nil {
		return err
	}
	delegator := sdk.AccAddress(targetBz)

	delegations, err := k.stakingKeeper.GetDelegatorDelegations(ctx, delegator, maxDelegations)
	if err != nil {
		return err
	}

	for _, delegation := range delegations {
		valAddr, err := sdk.ValAddressFromBech32(delegation.ValidatorAddress)
		if err != nil {
			return err
		}
		// A delegation that cannot be unbonded — one already at the maximum
		// number of unbonding entries — must not stop the rest of the seizure.
		// It stays staked and the next sweep tries again.
		if _, _, err := k.stakingKeeper.Undelegate(ctx, delegator, valAddr, delegation.Shares); err != nil {
			continue
		}
	}

	return nil
}

// collect takes what is liquid in the target's account right now and sends it
// to the recovery destination, returning what it moved and whether anything is
// still outstanding.
//
// Spendable, not total: a vesting account's unvested balance cannot be moved by
// anyone, including this module, and attempting it would fail the whole
// transaction rather than collecting the part that can be taken.
func (k Keeper) collect(ctx context.Context, enforcementCase *types.Case, params types.Params) (sdk.Coins, bool, error) {
	targetBz, err := k.addressCodec.StringToBytes(enforcementCase.Target)
	if err != nil {
		return nil, false, err
	}
	destinationBz, err := k.addressCodec.StringToBytes(params.RecoveryDestination)
	if err != nil {
		return nil, false, err
	}

	target := sdk.AccAddress(targetBz)
	spendable := k.bankKeeper.SpendableCoins(ctx, target)

	if spendable.IsAllPositive() && !spendable.IsZero() {
		if err := k.bankKeeper.SendCoins(ctx, target, sdk.AccAddress(destinationBz), spendable); err != nil {
			return nil, false, err
		}
		enforcementCase.Recovered = enforcementCase.Recovered.Add(spendable...)
		if err := k.addRecovered(ctx, spendable); err != nil {
			return nil, false, err
		}
	} else {
		spendable = sdk.NewCoins()
	}

	outstanding, err := k.stillOutstanding(ctx, target)
	if err != nil {
		return nil, false, err
	}
	enforcementCase.SweepComplete = !outstanding

	if err := sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventSeized{
		CaseId:      enforcementCase.Id,
		Target:      enforcementCase.Target,
		Destination: params.RecoveryDestination,
		Collected:   spendable,
		Complete:    enforcementCase.SweepComplete,
	}); err != nil {
		return nil, false, err
	}

	return spendable, enforcementCase.SweepComplete, nil
}

// stillOutstanding reports whether anything remains that a later sweep could
// collect: a balance that was not spendable yet, a delegation that could not be
// unbonded, or an unbonding that has not matured.
func (k Keeper) stillOutstanding(ctx context.Context, target sdk.AccAddress) (bool, error) {
	if !k.bankKeeper.GetAllBalances(ctx, target).IsZero() {
		return true, nil
	}
	delegations, err := k.stakingKeeper.GetDelegatorDelegations(ctx, target, 1)
	if err != nil {
		return false, err
	}
	if len(delegations) > 0 {
		return true, nil
	}
	unbonding, err := k.stakingKeeper.GetUnbondingDelegations(ctx, target, 1)
	if err != nil {
		return false, err
	}
	return len(unbonding) > 0, nil
}

// addRecovered maintains the chain-wide total of what this module has taken.
func (k Keeper) addRecovered(ctx context.Context, coins sdk.Coins) error {
	for _, coin := range coins {
		current, err := k.Recovered.Get(ctx, coin.Denom)
		if err != nil {
			if !isNotFound(err) {
				return err
			}
			current = math.ZeroInt()
		}
		if err := k.Recovered.Set(ctx, coin.Denom, current.Add(coin.Amount)); err != nil {
			return err
		}
	}
	return nil
}
