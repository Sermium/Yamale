package keeper

import (
	"bytes"
	"context"
	"strings"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/enforcement/types"
)

// OpenCase accuses an address and freezes it on the spot.
//
// One bonded validator is enough, and that is the deliberate asymmetry at the
// centre of this module: stopping the money has to be as fast as moving it,
// while taking it has to be slower than noticing a mistake. What one validator
// can do alone is stop transfers for a day, in public, under their own name.
// Everything past that needs the supermajority.
func (k msgServer) OpenCase(ctx context.Context, msg *types.MsgOpenCase) (*types.MsgOpenCaseResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Only a bonded validator may accuse. The opener's own vote is not assumed
	// from opening: they cast it like everyone else, so a tally never contains
	// a vote nobody sent.
	opener, _, err := k.bondedValidatorOf(ctx, msg.Opener)
	if err != nil {
		return nil, err
	}

	targetBz, err := k.addressCodec.StringToBytes(msg.Target)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid target address")
	}
	if err := k.assertTargetable(ctx, sdk.AccAddress(targetBz), msg.Target); err != nil {
		return nil, err
	}

	switch msg.Action {
	case types.CASE_ACTION_FREEZE:
	case types.CASE_ACTION_SEIZE:
		// Nowhere to send what is taken means the case cannot be carried out,
		// and a case that passes and then does nothing is worse than one that
		// was never opened: the record would say assets were recovered.
		if params.RecoveryDestination == "" {
			return nil, types.ErrInvalidCase.Wrap(
				"no recovery destination is set, so a seizure has nowhere to send what it takes; governance must set one first")
		}
		if params.SeizeRequiresEvidence && (strings.TrimSpace(msg.EvidenceUri) == "" || strings.TrimSpace(msg.EvidenceHash) == "") {
			return nil, types.ErrEvidenceRequired.Wrap("a seizure case needs both an evidence URI and its hash")
		}
	default:
		return nil, types.ErrInvalidCase.Wrapf("unknown action %s", msg.Action)
	}

	if err := params.ValidateReason(msg.Reason); err != nil {
		return nil, types.ErrInvalidCase.Wrap(err.Error())
	}
	if uint64(len(msg.EvidenceUri)) > params.MaxEvidenceUriLength {
		return nil, types.ErrLimitReached.Wrapf(
			"evidence_uri is %d characters, the maximum is %d", len(msg.EvidenceUri), params.MaxEvidenceUriLength)
	}

	// One case at a time per address. Without this, several validators could
	// each open their own against the same target, and withdrawing or losing
	// one would lift a freeze that another case still relies on.
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

	// The sequence is seeded at one in InitGenesis: a case id of zero is
	// indistinguishable from an unset field in every client that reads one, and
	// "frozen by case 0" is an accusation nobody could look up.
	id, err := k.CaseSeq.Next(ctx)
	if err != nil {
		return nil, err
	}

	newCase := types.Case{
		Id:                 id,
		Target:             msg.Target,
		Opener:             opener,
		Action:             msg.Action,
		Status:             types.CASE_STATUS_VOTING,
		Reason:             msg.Reason,
		EvidenceUri:        msg.EvidenceUri,
		EvidenceHash:       msg.EvidenceHash,
		OpenedAtHeight:     height,
		VotingEndsAtHeight: height + int64(params.VotingPeriodBlocks),
		TotalPowerAtOpen:   totalPower.Int64(),
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
		Opener:             opener,
		Action:             msg.Action,
		Reason:             msg.Reason,
		VotingEndsAtHeight: newCase.VotingEndsAtHeight,
	}); err != nil {
		return nil, err
	}

	return &types.MsgOpenCaseResponse{Id: id}, nil
}

// VoteCase records one validator's judgement, and resolves the case as soon as
// the answer can no longer change.
//
// Resolving early is not an optimisation. A frozen account waiting out a voting
// period whose outcome is already decided is somebody's money held for no
// reason, and a scammer whose seizure is already agreed should not get the rest
// of the period to argue with the unbonding queue.
func (k msgServer) VoteCase(ctx context.Context, msg *types.MsgVoteCase) (*types.MsgVoteCaseResponse, error) {
	operator, validator, err := k.bondedValidatorOf(ctx, msg.Voter)
	if err != nil {
		return nil, err
	}

	switch msg.Option {
	case types.VOTE_OPTION_YES, types.VOTE_OPTION_NO, types.VOTE_OPTION_ABSTAIN:
	default:
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "a vote must be yes, no or abstain")
	}

	enforcementCase, err := k.Case.Get(ctx, msg.CaseId)
	if err != nil {
		return nil, types.ErrCaseNotFound.Wrapf("case %d", msg.CaseId)
	}
	if enforcementCase.Status != types.CASE_STATUS_VOTING {
		return nil, types.ErrCaseClosed.Wrapf("case %d is %s", msg.CaseId, enforcementCase.Status)
	}

	voteKey := collections.Join(msg.CaseId, operator)
	if has, err := k.Vote.Has(ctx, voteKey); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrAlreadyVoted.Wrapf("validator %s on case %d", operator, msg.CaseId)
	}

	power := validator.GetConsensusPower(sdk.DefaultPowerReduction)
	if err := k.Vote.Set(ctx, voteKey, types.Vote{
		CaseId:    msg.CaseId,
		Validator: operator,
		Option:    msg.Option,
		Power:     power,
	}); err != nil {
		return nil, err
	}

	switch msg.Option {
	case types.VOTE_OPTION_YES:
		enforcementCase.YesPower += power
	case types.VOTE_OPTION_NO:
		enforcementCase.NoPower += power
	case types.VOTE_OPTION_ABSTAIN:
		enforcementCase.AbstainPower += power
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCaseVoted{
		CaseId:    msg.CaseId,
		Validator: operator,
		Option:    msg.Option,
		Power:     power,
	}); err != nil {
		return nil, err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	required := params.RequiredPower(enforcementCase.TotalPowerAtOpen)

	switch {
	case enforcementCase.YesPower >= required:
		if err := k.passCase(ctx, &enforcementCase); err != nil {
			return nil, err
		}
	case enforcementCase.TotalPowerAtOpen-enforcementCase.NoPower < required:
		// Enough power has voted no that the threshold is unreachable even if
		// everyone left voted yes.
		if err := k.rejectCase(ctx, &enforcementCase, types.CASE_STATUS_REJECTED); err != nil {
			return nil, err
		}
	default:
		if err := k.Case.Set(ctx, enforcementCase.Id, enforcementCase); err != nil {
			return nil, err
		}
	}

	return &types.MsgVoteCaseResponse{}, nil
}

// WithdrawCase takes back an open case. Only the validator that opened it may,
// and it lifts the freeze — the same person who was trusted to impose it alone
// is trusted to admit they were wrong alone.
func (k msgServer) WithdrawCase(ctx context.Context, msg *types.MsgWithdrawCase) (*types.MsgWithdrawCaseResponse, error) {
	enforcementCase, err := k.Case.Get(ctx, msg.CaseId)
	if err != nil {
		return nil, types.ErrCaseNotFound.Wrapf("case %d", msg.CaseId)
	}
	if enforcementCase.Status != types.CASE_STATUS_VOTING {
		return nil, types.ErrCaseClosed.Wrapf("case %d is %s", msg.CaseId, enforcementCase.Status)
	}
	opener, _, err := k.bondedValidatorOf(ctx, msg.Opener)
	if err != nil {
		return nil, err
	}
	if enforcementCase.Opener != opener {
		return nil, types.ErrNotTheOpener.Wrapf("case %d was opened by %s", msg.CaseId, enforcementCase.Opener)
	}

	if err := k.rejectCase(ctx, &enforcementCase, types.CASE_STATUS_WITHDRAWN); err != nil {
		return nil, err
	}

	return &types.MsgWithdrawCaseResponse{}, nil
}

// ReverseCase is governance overturning a case that passed.
//
// It lifts the freeze and says on the record that the chain got it wrong. It
// does not return what was already seized: those funds are in the recovery
// destination's hands, and only they can send them back. Claiming otherwise
// here would put a comforting lie in the state machine.
func (k msgServer) ReverseCase(ctx context.Context, msg *types.MsgReverseCase) (*types.MsgReverseCaseResponse, error) {
	authorityBytes, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authorityBytes) {
		expected, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expected, msg.Authority)
	}

	enforcementCase, err := k.Case.Get(ctx, msg.CaseId)
	if err != nil {
		return nil, types.ErrCaseNotFound.Wrapf("case %d", msg.CaseId)
	}
	if enforcementCase.Status != types.CASE_STATUS_PASSED {
		return nil, types.ErrNotPassed.Wrapf("case %d is %s", msg.CaseId, enforcementCase.Status)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	enforcementCase.Status = types.CASE_STATUS_REVERSED
	enforcementCase.ResolvedAtHeight = sdkCtx.BlockHeight()
	if msg.Reason != "" {
		enforcementCase.Reason = enforcementCase.Reason + "\n\nReversed by governance: " + msg.Reason
	}
	if err := k.Case.Set(ctx, enforcementCase.Id, enforcementCase); err != nil {
		return nil, err
	}
	if err := k.unfreeze(ctx, enforcementCase.Target); err != nil {
		return nil, err
	}

	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCaseResolved{
		CaseId: enforcementCase.Id,
		Target: enforcementCase.Target,
		Status: enforcementCase.Status,
	}); err != nil {
		return nil, err
	}

	return &types.MsgReverseCaseResponse{}, nil
}

// bondedValidatorOf resolves the account a validator signs with to that
// validator, and returns its operator address alongside it.
//
// Messages here are signed with the validator's account key rather than named
// by operator address, because that is the key a validator actually holds and
// the one every CLI and wallet knows how to use. The operator address is what
// gets recorded: a case attributed to an account address nobody recognises is
// an accusation with no visible author.
func (k Keeper) bondedValidatorOf(ctx context.Context, account string) (string, stakingValidator, error) {
	accountBz, err := k.addressCodec.StringToBytes(account)
	if err != nil {
		return "", nil, errorsmod.Wrap(err, "invalid address")
	}

	operatorAddr := sdk.ValAddress(accountBz)
	operator := operatorAddr.String()

	validator, err := k.stakingKeeper.Validator(ctx, operatorAddr)
	if err != nil || validator == nil {
		return "", nil, types.ErrUnknownValidator.Wrapf("%s is not a validator", account)
	}
	if !validator.IsBonded() {
		return "", nil, types.ErrUnknownValidator.Wrapf("%s is not bonded", operator)
	}
	return operator, validator, nil
}

// assertTargetable refuses the addresses that must never be frozen.
//
// Module accounts hold the chain's own money: the bonded pool, the fee
// collector, the treasury and payments custody accounts. Freezing one would
// stop staking, distribution or every payment on the chain for everybody, and
// there is nobody behind such an address to accuse in the first place.
func (k Keeper) assertTargetable(ctx context.Context, addr sdk.AccAddress, bech32 string) error {
	if k.bankKeeper.BlockedAddr(addr) {
		return types.ErrProtectedAddress.Wrapf("%s is a module account", bech32)
	}
	account := k.authKeeper.GetAccount(ctx, addr)
	if account != nil {
		if _, isModule := account.(sdk.ModuleAccountI); isModule {
			return types.ErrProtectedAddress.Wrapf("%s is a module account", bech32)
		}
		if _, isModule := account.(*authtypes.ModuleAccount); isModule {
			return types.ErrProtectedAddress.Wrapf("%s is a module account", bech32)
		}
	}
	return nil
}
