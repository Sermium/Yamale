package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
)

// EmergencyFreeze stops an account on an enforcement authority's signature
// alone.
//
// Everything about it is the ordinary path with the validator step removed from
// the front: it opens a real case, imposes the same provisional freeze, and
// goes into the same voting queue, where the validators confirm it or refuse
// it. What it skips is the wait for one of them to be awake.
//
// It cannot seize, and there is deliberately no message that lets it. Stopping
// money is recoverable — release the account and nothing was lost but time.
// Taking it is not, so that stays with the supermajority whoever is asking.
//
// # From one address to a grant
//
// The signer used to be the emergency_authority parameter: one address,
// chain-wide, able to freeze any account on this chain without a case. It is now
// ROLE_ENFORCEMENT_AUTHORITY scoped to the target's country, which is the same
// check every other authority action on the chain routes through.
//
// The old shape is worth naming because it is what the perimeter exists to
// abolish. A single parameter with no territorial limit made the one path that
// acts on one signature also the one path no border bounded — so the fastest
// power in the module was also the widest, which is the shape of every abuse
// this design refuses. What replaces it is not slower: an office signs, in one
// block, exactly as before, inside a country.
func (k msgServer) EmergencyFreeze(ctx context.Context, msg *types.MsgEmergencyFreeze) (*types.MsgEmergencyFreezeResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	// The ombudsman may not open a case, and this is where the exclusion is
	// actually held.
	//
	// It used to be belt and braces over Params.Validate, which refused
	// parameters naming the same address as ombudsman and emergency authority.
	// That comparison is gone with the parameter, and it could not have been
	// kept: the authority is a grant in another module now, so the parameters
	// cannot see it. UpdateParams asks the perimeter keeper before writing an
	// ombudsman, which catches the appointment order — and this check catches the
	// other one, a grant made to a sitting ombudsman afterwards, which the
	// parameters could never have caught even when the field existed.
	if err := k.assertNotOmbudsman(params, msg.Authority); err != nil {
		return nil, err
	}

	targetBz, err := k.addressCodec.StringToBytes(msg.Target)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid target address")
	}
	if err := k.assertTargetable(ctx, sdk.AccAddress(targetBz), msg.Target); err != nil {
		return nil, err
	}
	// The perimeter, and the emergency is not an exception to it. It is now the
	// only thing that authorises this message at all, where it used to run after
	// a comparison against a named address. Skipping the jurisdiction check
	// because the situation is urgent would mean the one path that acts on a
	// single signature is also the one path with no territorial limit. An
	// authority that needs to stop an account outside its perimeter needs the
	// authority of that perimeter, urgently or otherwise.
	if err := k.assertScope(ctx, msg.Authority, msg.Target); err != nil {
		return nil, err
	}

	// The grounds are required here for the same reason as everywhere else, and
	// more so: acting in an emergency is not a reason to leave the record blank,
	// it is the reason the record matters.
	if err := params.ValidateReason(msg.Reason); err != nil {
		return nil, types.ErrInvalidCase.Wrap(err.Error())
	}
	if uint64(len(msg.EvidenceUri)) > params.MaxEvidenceUriLength {
		return nil, types.ErrLimitReached.Wrapf(
			"evidence_uri is %d characters, the maximum is %d", len(msg.EvidenceUri), params.MaxEvidenceUriLength)
	}

	frozen, found, err := k.FreezeOf(ctx, msg.Target)
	if err != nil {
		return nil, err
	}
	if found {
		return nil, types.ErrAlreadyFrozen.Wrapf("case %d already freezes %s", frozen.CaseId, msg.Target)
	}

	totalPower, err := k.stakingKeeper.GetLastTotalPower(ctx)
	if err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()

	id, err := k.CaseSeq.Next(ctx)
	if err != nil {
		return nil, err
	}

	newCase := types.Case{
		Id:                 id,
		Target:             msg.Target,
		Opener:             msg.Authority,
		Action:             types.CASE_ACTION_FREEZE,
		Status:             types.CASE_STATUS_VOTING,
		Reason:             msg.Reason,
		EvidenceUri:        msg.EvidenceUri,
		EvidenceHash:       msg.EvidenceHash,
		OpenedAtHeight:     height,
		VotingEndsAtHeight: height + int64(params.VotingPeriodBlocks),
		TotalPowerAtOpen:   totalPower.Int64(),
		Emergency:          true,
	}
	if err := k.Case.Set(ctx, id, newCase); err != nil {
		return nil, err
	}
	if err := k.VotingQueue.Set(ctx, collections.Join(newCase.VotingEndsAtHeight, id)); err != nil {
		return nil, err
	}
	if err := k.freeze(ctx, msg.Target, id, height+int64(params.ProvisionalFreezeBlocks)); err != nil {
		return nil, err
	}

	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCaseOpened{
		CaseId:             id,
		Target:             msg.Target,
		Opener:             msg.Authority,
		Action:             types.CASE_ACTION_FREEZE,
		Reason:             msg.Reason,
		VotingEndsAtHeight: newCase.VotingEndsAtHeight,
		Emergency:          true,
	}); err != nil {
		return nil, err
	}

	return &types.MsgEmergencyFreezeResponse{Id: id}, nil
}

// EmergencyRelease lifts a freeze immediately, whoever imposed it.
//
// This is the half that makes the emergency authority worth having. A case
// opened on a misread transaction otherwise sits on an account for the whole
// voting period, and "wait a day, it expires by itself" is not an answer anyone
// can give a business whose payroll is stuck.
//
// It works on a case that is still being voted on and on one that already
// passed. What it does not do is undo a seizure: if the funds have already
// moved, releasing the account gives back the ability to use it and nothing
// else. Those funds are the recovery destination's to return.
//
// # Scoped to the case's target, not to anything the message names
//
// The authority has to hold ROLE_ENFORCEMENT_AUTHORITY covering the country the
// TARGET is recorded in, which means the case has to be loaded before the signer
// can be checked. That ordering is forced and it is worth being explicit about:
// there is no country in this message, and taking one from the signer would be
// letting an actor name its own perimeter, which is exactly the claim the
// perimeter design refuses.
//
// Leaving release chain-wide was the defensible alternative — it is the smaller
// act, and a wrong freeze is an emergency of its own. It was not taken because
// an office able to release anywhere could lift the freeze another country's
// authority had just imposed, which is interference in that perimeter rather
// than mercy in its own.
//
// The cost of scoping it is real and should be stated rather than discovered: a
// target whose jurisdiction the chain cannot resolve cannot be released by this
// message at all, because AssertScope refuses an unplaceable target before any
// grant is read. What is left for that account is the provisional freeze
// lapsing by itself, and governance reversing a case that passed. Both are
// slower and both exist.
func (k msgServer) EmergencyRelease(ctx context.Context, msg *types.MsgEmergencyRelease) (*types.MsgEmergencyReleaseResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	enforcementCase, err := k.Case.Get(ctx, msg.CaseId)
	if err != nil {
		return nil, types.ErrCaseNotFound.Wrapf("case %d", msg.CaseId)
	}
	if err := k.assertScope(ctx, msg.Authority, enforcementCase.Target); err != nil {
		return nil, err
	}
	switch enforcementCase.Status {
	// HELD belongs here beside VOTING and PASSED: a seizure waiting out its
	// delay is an account still frozen, which is exactly the situation this
	// message exists for. Leaving it out would have made the founders' release
	// unable to reach the state a seizure now spends most of its life in.
	case types.CASE_STATUS_VOTING, types.CASE_STATUS_HELD, types.CASE_STATUS_PASSED:
	default:
		return nil, types.ErrCaseClosed.Wrapf(
			"case %d is %s, so there is nothing to release", msg.CaseId, enforcementCase.Status)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	wasVoting := enforcementCase.Status == types.CASE_STATUS_VOTING
	wasHeld := enforcementCase.Status == types.CASE_STATUS_HELD

	enforcementCase.Status = types.CASE_STATUS_REVERSED
	enforcementCase.ResolvedAtHeight = sdkCtx.BlockHeight()
	if strings.TrimSpace(msg.Reason) != "" {
		enforcementCase.Reason = enforcementCase.Reason + "\n\nReleased by the emergency authority: " + msg.Reason
	}
	if err := k.Case.Set(ctx, enforcementCase.Id, enforcementCase); err != nil {
		return nil, err
	}

	// A case that was still open has a queue entry waiting to resolve it at its
	// end height. Left there, the end blocker would find a case that is no
	// longer voting and would have to guess what to do with it.
	if wasVoting {
		if err := k.dequeueVoting(ctx, enforcementCase); err != nil {
			return nil, err
		}
	}
	// A held seizure has an entry waiting in the execution queue. Left there,
	// the end blocker would reach its height and carry out a seizure against an
	// account the founders had already released.
	if wasHeld {
		if err := k.dequeueExecution(ctx, enforcementCase); err != nil {
			return nil, err
		}
	}

	if err := k.liftFreeze(ctx, enforcementCase.Target, enforcementCase.Id, types.CASE_STATUS_REVERSED); err != nil {
		return nil, err
	}

	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCaseResolved{
		CaseId:        enforcementCase.Id,
		Target:        enforcementCase.Target,
		Status:        types.CASE_STATUS_REVERSED,
		YesPower:      enforcementCase.YesPower,
		NoPower:       enforcementCase.NoPower,
		RequiredPower: params.RequiredPower(enforcementCase.TotalPowerAtOpen),
	}); err != nil {
		return nil, err
	}

	return &types.MsgEmergencyReleaseResponse{}, nil
}

// There is deliberately no assertEmergencyAuthority any more.
//
// It compared the signer against one address named in the parameters and refused
// everyone when none was named, and the empty case was the careful part: an
// unset authority had to mean "nobody", never "anybody". Both halves are now
// carried by the perimeter check instead, and both are carried better — a
// missing grant refuses, a store failure refuses, and the refusal names the
// country the signer's grant did not reach rather than an address it is not.
//
// Written down rather than deleted silently because the empty-means-nobody
// property is easy to lose when a check moves. The path that replaces it is
// Keeper.assertScope, which fails closed on a missing registry and on a target
// the chain cannot place, so there is no configuration of this module in which
// the emergency path is open to whoever noticed first.
