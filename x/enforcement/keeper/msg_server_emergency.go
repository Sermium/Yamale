package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
)

// EmergencyFreeze stops an account on the founders' signature alone.
//
// Everything about it is the ordinary path with the validator step removed from
// the front: it opens a real case, imposes the same provisional freeze, and
// goes into the same voting queue, where the validators confirm it or refuse
// it. What it skips is the wait for one of them to be awake.
//
// It cannot seize, and there is deliberately no message that lets it. Stopping
// money is recoverable — release the account and nothing was lost but time.
// Taking it is not, so that stays with the supermajority whoever is asking.
func (k msgServer) EmergencyFreeze(ctx context.Context, msg *types.MsgEmergencyFreeze) (*types.MsgEmergencyFreezeResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := k.assertEmergencyAuthority(params, msg.Authority); err != nil {
		return nil, err
	}

	targetBz, err := k.addressCodec.StringToBytes(msg.Target)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid target address")
	}
	if err := k.assertTargetable(ctx, sdk.AccAddress(targetBz), msg.Target); err != nil {
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
func (k msgServer) EmergencyRelease(ctx context.Context, msg *types.MsgEmergencyRelease) (*types.MsgEmergencyReleaseResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := k.assertEmergencyAuthority(params, msg.Authority); err != nil {
		return nil, err
	}

	enforcementCase, err := k.Case.Get(ctx, msg.CaseId)
	if err != nil {
		return nil, types.ErrCaseNotFound.Wrapf("case %d", msg.CaseId)
	}
	switch enforcementCase.Status {
	case types.CASE_STATUS_VOTING, types.CASE_STATUS_PASSED:
	default:
		return nil, types.ErrCaseClosed.Wrapf(
			"case %d is %s, so there is nothing to release", msg.CaseId, enforcementCase.Status)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	wasVoting := enforcementCase.Status == types.CASE_STATUS_VOTING

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

// assertEmergencyAuthority refuses anyone but the address named in the
// parameters, and refuses everyone when no address is named.
//
// The empty case is the one worth being careful about: an unset authority must
// mean "nobody", never "anybody". A comparison that let an empty message field
// match an empty parameter would hand this power to whoever noticed first.
func (k Keeper) assertEmergencyAuthority(params types.Params, signer string) error {
	if strings.TrimSpace(params.EmergencyAuthority) == "" {
		return types.ErrNoEmergencyAuthority.Wrap(
			"no emergency authority is set, so there is no emergency path; governance must name one first")
	}
	if signer != params.EmergencyAuthority {
		return errorsmod.Wrapf(types.ErrInvalidSigner,
			"invalid emergency authority; expected %s, got %s", params.EmergencyAuthority, signer)
	}
	return nil
}
