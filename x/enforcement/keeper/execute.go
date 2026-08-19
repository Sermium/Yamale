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

// passCase is the validators agreeing. What that means depends on what was
// asked for.
//
// A freeze case ends here: the provisional freeze becomes one that does not
// lapse, and nothing else happens because a freeze takes nothing.
//
// A seizure does not end here. It is assessed, given a delay proportionate to
// what it would take, and left waiting — frozen, decided, and still stoppable
// for free. Nothing is unbonded and nothing is moved until that delay expires,
// which is deliberate and does cost something: a seizure against staked funds
// now waits the delay *and then* the unbonding period, rather than running them
// together. That is the price of the veto window being real. A case the
// ombudsman stops during the hold leaves its target exactly as they were —
// still staked, still earning — instead of unstaked by an accusation that was
// then withdrawn.
func (k Keeper) passCase(ctx context.Context, enforcementCase *types.Case) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()

	enforcementCase.ResolvedAtHeight = height

	if err := k.makePermanent(ctx, enforcementCase.Target); err != nil {
		return err
	}
	if err := k.dequeueVoting(ctx, *enforcementCase); err != nil {
		return err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}

	if enforcementCase.Action == types.CASE_ACTION_SEIZE {
		return k.holdSeizure(ctx, enforcementCase, params, height)
	}

	// Counted here, where a freeze case reaches PASSED, and for a seizure in
	// executeSeizure where it does. Not when the vote is won.
	//
	// The distinction is not pedantry, it is what keeps this counter equal to
	// what a genesis import rebuilds. InitGenesis has only the cases to work
	// from, so it counts the ones whose status is PASSED; a counter incremented
	// when the validators agreed would be one ahead for every seizure still
	// waiting out its delay, and would stay ahead for every one the ombudsman
	// then vetoed. Export, import, and the chain would quietly disagree with
	// itself about how often this power has been used.
	if err := k.countPassed(ctx); err != nil {
		return err
	}

	enforcementCase.Status = types.CASE_STATUS_PASSED
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

// holdSeizure puts an agreed seizure into the waiting state its size earns.
//
// The assessment is taken once, here, and recorded on the case. Re-measuring at
// execution would let the delay be shortened by anything that moved the balance
// afterwards, and would make "why did this one wait a week" unanswerable from
// the record once the state it was measured against had moved on.
func (k Keeper) holdSeizure(ctx context.Context, enforcementCase *types.Case, params types.Params, height int64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	targetBz, err := k.addressCodec.StringToBytes(enforcementCase.Target)
	if err != nil {
		return err
	}
	assessed, err := k.assessSeizableValue(ctx, sdk.AccAddress(targetBz))
	if err != nil {
		return err
	}

	delay := params.SeizureDelayFor(assessed)

	enforcementCase.Status = types.CASE_STATUS_HELD
	enforcementCase.AssessedValue = assessed
	enforcementCase.ExecuteAtHeight = height + int64(delay)

	if err := k.Case.Set(ctx, enforcementCase.Id, *enforcementCase); err != nil {
		return err
	}
	if err := k.ExecutionQueue.Set(ctx, collections.Join(enforcementCase.ExecuteAtHeight, enforcementCase.Id)); err != nil {
		return err
	}

	// EventCaseHeld and not EventCaseResolved, because HELD is not a resolution.
	// EventCaseResolved is documented as being emitted once, at a case's final
	// status, and anything watching case lifecycles keys off that — so emitting
	// it here would announce every agreed seizure as finished and then never
	// correct itself, leaving an explorer showing "resolved: held" for a case
	// whose money moved a week later.
	return sdkCtx.EventManager().EmitTypedEvent(&types.EventCaseHeld{
		CaseId:          enforcementCase.Id,
		Target:          enforcementCase.Target,
		AssessedValue:   assessed,
		ExecuteAtHeight: enforcementCase.ExecuteAtHeight,
		DelayBlocks:     delay,
	})
}

// executeSeizure carries out a held seizure whose delay has expired.
//
// The rolling cap is checked here rather than when the case was decided,
// because the window it is checked against is the one in force at the moment
// the money would actually move. A case admitted at decision time and executed
// a week later would take room in a window that had already been spent.
//
// A case the cap refuses is not cancelled and not lost. It stays held, its
// target stays frozen, and it is re-queued for the height at which the window
// could next have room — with an event saying so every time, because a case
// quietly waiting is indistinguishable from a case that has been forgotten and
// the difference matters most to the person still frozen.
func (k Keeper) executeSeizure(ctx context.Context, enforcementCase *types.Case, params types.Params, height int64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	admitted, retryAt, refusal, err := k.admitSeizure(ctx, params, height, enforcementCase.AssessedValue)
	if err != nil {
		return err
	}
	if !admitted {
		// execute_at_height moves with the queue entry, so the two can never
		// disagree: it is where this case is queued now, not where it was
		// queued first. Keeping the original instead would leave every stop
		// path — veto, reversal, emergency release — deleting a key that is not
		// there, and a deferred case would stay queued after being stopped.
		// That is a released account being seized from, at a height nobody is
		// watching. What the original wait was remains on the record, in the
		// EventCaseHeld emitted when the case was agreed and in every
		// EventSeizureDeferred since.
		enforcementCase.ExecuteAtHeight = retryAt
		if err := k.Case.Set(ctx, enforcementCase.Id, *enforcementCase); err != nil {
			return err
		}
		if err := k.ExecutionQueue.Set(ctx, collections.Join(retryAt, enforcementCase.Id)); err != nil {
			return err
		}
		return sdkCtx.EventManager().EmitTypedEvent(&types.EventSeizureDeferred{
			CaseId:        enforcementCase.Id,
			Target:        enforcementCase.Target,
			RetryAtHeight: retryAt,
			Reason:        refusal,
		})
	}

	enforcementCase.Status = types.CASE_STATUS_PASSED
	if err := k.countPassed(ctx); err != nil {
		return err
	}

	if err := k.unbondEverything(ctx, enforcementCase.Target); err != nil {
		return err
	}
	collected, _, err := k.collect(ctx, enforcementCase, params)
	if err != nil {
		return err
	}
	if err := k.Case.Set(ctx, enforcementCase.Id, *enforcementCase); err != nil {
		return err
	}
	if err := k.recordSeizure(ctx, enforcementCase.Id, height, enforcementCase.AssessedValue, collected); err != nil {
		return err
	}

	// Now the case is finished, so now it is resolved. This is the event a
	// seizure's lifecycle ends on, and the only one it emits with a final
	// status.
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

// countPassed records that one more case has been carried out.
//
// Kept as state rather than computed on demand because the honest answer to
// "how often has this been used" should not require replaying the chain — and
// incremented in exactly the two places a case reaches CASE_STATUS_PASSED, so
// that it equals what InitGenesis rebuilds from the cases alone.
func (k Keeper) countPassed(ctx context.Context) error {
	passed, err := k.CasesPassed.Get(ctx)
	if err != nil && !isNotFound(err) {
		return err
	}
	return k.CasesPassed.Set(ctx, passed+1)
}

func (k Keeper) dequeueVoting(ctx context.Context, enforcementCase types.Case) error {
	return k.VotingQueue.Remove(ctx, collections.Join(enforcementCase.VotingEndsAtHeight, enforcementCase.Id))
}

// dequeueExecution takes a held seizure out of the execution queue.
//
// One removal, by exact key, because a case has exactly one entry in the queue
// at any time: execute_at_height is where it is queued *now*, and a deferral
// moves both together. An earlier version searched the whole queue for stale
// entries instead, which made stopping one case cost a walk over every held
// case — unbounded work reachable from the end blocker, which is the shape of
// problem that makes blocks late.
func (k Keeper) dequeueExecution(ctx context.Context, enforcementCase types.Case) error {
	return k.ExecutionQueue.Remove(ctx, collections.Join(enforcementCase.ExecuteAtHeight, enforcementCase.Id))
}

// stopCase closes a case that has taken nothing, whatever state it was in, and
// gives the account back.
//
// Used by the ombudsman's veto, which is the one instrument that reaches both a
// case still being argued and a seizure already decided and waiting. Everything
// else in this module acts on one or the other, so this is the only place that
// has to clear both queues without knowing which one the case was in.
func (k Keeper) stopCase(ctx context.Context, enforcementCase *types.Case, status types.CaseStatus) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	wasVoting := enforcementCase.Status == types.CASE_STATUS_VOTING
	wasHeld := enforcementCase.Status == types.CASE_STATUS_HELD

	enforcementCase.Status = status
	enforcementCase.ResolvedAtHeight = sdkCtx.BlockHeight()
	if err := k.Case.Set(ctx, enforcementCase.Id, *enforcementCase); err != nil {
		return err
	}

	if wasVoting {
		if err := k.dequeueVoting(ctx, *enforcementCase); err != nil {
			return err
		}
	}
	if wasHeld {
		if err := k.dequeueExecution(ctx, *enforcementCase); err != nil {
			return err
		}
	}

	if err := k.liftFreeze(ctx, enforcementCase.Target, enforcementCase.Id, status); err != nil {
		return err
	}

	// A veto is a terminal status like any other, so it announces itself the
	// same way. EventCaseVetoed is emitted alongside this by the handler and
	// says who stopped it and why; this is the one an indexer following case
	// lifecycles reads, and leaving it out would make a vetoed case the only
	// kind that ends without saying so.
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
