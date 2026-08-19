package keeper

import (
	"context"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
)

// The ombudsman is an office that can stop a case and can never start one, and
// the asymmetry is enforced structurally rather than by being told not to.
//
// "Structurally" means something specific here, and it is worth being exact
// about, because "the handler checks the signer" would not be it. There are
// exactly four ways a case can be opened or moved forward in this module, and
// the ombudsman is closed out of every one of them by what it is rather than by
// what it is allowed:
//
//  1. MsgOpenCase and MsgVoteCase require a *bonded validator*, resolved from
//     the signer through the staking module. The ombudsman is appointed outside
//     the validator set; if the same key were ever bonded, assertNotOmbudsman
//     below refuses it at the point of use, in the handler, on every call.
//  2. MsgEmergencyFreeze requires the emergency authority, and Params.Validate
//     refuses parameters in which the ombudsman and the emergency authority are
//     the same address. There is no configuration in which one key holds both.
//  3. MsgUpdateParams and MsgReverseCase require the governance authority — a
//     module account nobody signs for. This matters more than it looks: an
//     ombudsman who could reach UpdateParams could appoint itself emergency
//     authority and open cases from there, so the veto being unreachable from
//     governance is what stops the veto being an indirect way in.
//  4. MsgSweep is permissionless, and it advances collection on a seizure that
//     has already executed. The ombudsman is refused it anyway. It is the one
//     bar that is not strictly necessary, and it is here because the claim this
//     office makes is total: no message in this service that the ombudsman may
//     sign moves any case any distance towards taking anything.
//
// And the positive half of the same claim: MsgOmbudsmanVeto is the only message
// whose signer is the ombudsman, this file is the only handler for it, and the
// only case status it can write is a terminal one. There is no branch in it
// that creates a case, sets a case to VOTING or HELD, or increases any tally.
// The office cannot open a case because there is no code that would let it, not
// because there is code that stops it.

// OmbudsmanVeto stops a case before it takes anything.
func (k msgServer) OmbudsmanVeto(ctx context.Context, msg *types.MsgOmbudsmanVeto) (*types.MsgOmbudsmanVetoResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := k.assertOmbudsman(params, msg.Ombudsman); err != nil {
		return nil, err
	}

	// Grounds are required for the same reason every other act in this module
	// needs them. An office whose refusals need no explanation is not
	// accountable either, and this one is a check rather than a privilege.
	if err := params.ValidateReason(msg.Reason); err != nil {
		return nil, types.ErrInvalidCase.Wrap(err.Error())
	}

	enforcementCase, err := k.Case.Get(ctx, msg.CaseId)
	if err != nil {
		return nil, types.ErrCaseNotFound.Wrapf("case %d", msg.CaseId)
	}

	wasHeld := enforcementCase.Status == types.CASE_STATUS_HELD
	switch enforcementCase.Status {
	case types.CASE_STATUS_VOTING, types.CASE_STATUS_HELD:
	case types.CASE_STATUS_PASSED:
		// A seizure that has executed cannot be vetoed, and the refusal says so
		// rather than pretending. A veto cannot un-take money; the only thing
		// that returns what was taken is a transfer from the recovery
		// destination, by whoever controls it. Letting this succeed and marking
		// the case vetoed would put a comforting lie in the record.
		return nil, types.ErrCaseClosed.Wrapf(
			"case %d has already been carried out; a veto stops a seizure, it cannot return one", msg.CaseId)
	default:
		return nil, types.ErrCaseClosed.Wrapf(
			"case %d is %s, so there is nothing left to stop", msg.CaseId, enforcementCase.Status)
	}

	if strings.TrimSpace(msg.Reason) != "" {
		enforcementCase.Reason = enforcementCase.Reason + "\n\nVetoed by the ombudsman: " + msg.Reason
	}

	if err := k.stopCase(ctx, &enforcementCase, types.CASE_STATUS_VETOED); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCaseVetoed{
		CaseId:    enforcementCase.Id,
		Target:    enforcementCase.Target,
		Ombudsman: msg.Ombudsman,
		Reason:    msg.Reason,
		WasHeld:   wasHeld,
	}); err != nil {
		return nil, err
	}

	return &types.MsgOmbudsmanVetoResponse{}, nil
}

// assertOmbudsman refuses anyone but the address named in the parameters, and
// refuses everyone when no address is named.
//
// The empty case is the one to be careful about, exactly as it is for the
// emergency authority: an unappointed office must mean "nobody", never
// "anybody". A comparison that let an empty message field match an empty
// parameter would hand the veto to whoever noticed first — which, for a veto,
// means whoever wants to protect the account the chain is trying to freeze.
func (k Keeper) assertOmbudsman(params types.Params, signer string) error {
	if strings.TrimSpace(params.Ombudsman) == "" {
		return types.ErrNoOmbudsman.Wrap(
			"no ombudsman is appointed, so there is no veto; governance must appoint one first")
	}
	if signer != params.Ombudsman {
		return errorsmod.Wrapf(types.ErrInvalidSigner,
			"invalid ombudsman; expected %s, got %s", params.Ombudsman, signer)
	}
	return nil
}

// assertNotOmbudsman is the bar, applied at the point of use in every handler
// that opens or advances a case.
//
// It is checked in the handlers rather than only in Params.Validate because
// Params.Validate cannot ask the staking module anything: whether the
// ombudsman's key is also a bonded validator is a fact about chain state, not
// about a parameter struct, and it can become true after the parameters were
// last set — a validator admitted with that key, an operator changing hands.
// Guarding where the power is exercised is the only place the guard holds for
// every state the chain can reach.
func (k Keeper) assertNotOmbudsman(params types.Params, signer string) error {
	if strings.TrimSpace(params.Ombudsman) == "" {
		return nil
	}
	if signer == params.Ombudsman {
		return types.ErrOmbudsmanCannotInitiate.Wrapf(
			"%s is the ombudsman; that office exists to stop cases and holds no power to bring one", signer)
	}
	return nil
}
