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

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/enforcement/types"
)

// OpenCase accuses an address and freezes it on the spot.
//
// One signature is enough, and that is the deliberate asymmetry at the centre of
// this module: stopping the money has to be as fast as moving it, while taking
// it has to be slower than noticing a mistake. What one signature can do alone
// is stop transfers for a day, in public, under a name anybody can look up.
// Everything past that needs the supermajority.
//
// # Who may accuse
//
// A bonded validator, or a holder of ROLE_ENFORCEMENT_AUTHORITY covering the
// target's country. Two kinds of account, one message, and the widening is the
// point: the role could be granted before this and no message let it be used, so
// a country's enforcement office held a grant that did nothing — which
// role.proto itself calls a name in a registry pretending to be a control.
//
// It is a widening and not a shift. A bonded validator may do exactly what it
// could do before, and the perimeter check that was already here runs on both
// paths, so who may accuse grew and where anybody may accuse did not.
//
// # Why an office may accuse and cannot decide
//
// Accusation and decision were already separate here — the opener's own vote has
// never been assumed from opening — and that separation is what makes this safe
// to widen. An office that is not a validator has no vote at all: it can freeze
// an account provisionally, for as long as provisional_freeze_blocks, in public,
// and the validator set then confirms or refuses it. A national authority can
// stop money for a day; two thirds of bonded power decides whether it is taken.
func (k msgServer) OpenCase(ctx context.Context, msg *types.MsgOpenCase) (*types.MsgOpenCaseResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// The ombudsman may not open a case, and the bar is here rather than only
	// in the parameters because whether that key is also a bonded validator is
	// a fact about chain state that can change after the parameters were set.
	// Within the constitutional concentration ceilings AT THIS MOMENT, not as
	// of the last epoch sweep. See Keeper.assertWithinCaps: the sweep is
	// periodic and this power is not, so without this a group that had crossed
	// a ceiling held it for up to a whole epoch.
	if err := k.assertWithinCaps(ctx, msg.Opener); err != nil {
		return nil, err
	}
	if err := k.assertNotOmbudsman(params, msg.Opener); err != nil {
		return nil, err
	}

	// Who is accusing, and under what name it goes on the record. The opener's
	// own vote is not assumed from opening: a validator casts it like everyone
	// else, so a tally never contains a vote nobody sent, and an office has no
	// vote to cast at all.
	opener, err := k.openerOf(ctx, msg.Opener)
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
	// The perimeter, and it is the check that carries the whole of the widening
	// above. Being a bonded validator says you are trusted to secure this chain;
	// it does not say which country's accounts you may stop. Those are different
	// questions and this module used to answer only the first, which is how a
	// validator in one jurisdiction could freeze an account in another.
	//
	// It runs on both kinds of opener and it is the ONLY thing that permits
	// either of them. openerOf above decides what the accusation is called, not
	// whether it may be made — an office that holds the role somewhere and not
	// here reaches this line and is refused by it.
	//
	// Checked against the signing account rather than the operator address,
	// because a grant is made to an account and the operator address is derived
	// from it — granting to the derived form would be granting to an account
	// nobody holds a key for.
	if err := k.assertScope(ctx, msg.Opener, msg.Target); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	switch msg.Action {
	case types.CASE_ACTION_FREEZE:
		// A freeze needs no instrument: it takes nothing, and it has to be
		// openable in the minute a theft is noticed, which is not a minute in
		// which anybody has a court order. One may still be attached — a freeze
		// ordered by a regulator is an ordinary thing — and if it is, it is
		// checked, so the record never holds an instrument nobody validated.
		if !msg.LegalInstrument.IsZero() {
			if err := msg.LegalInstrument.Validate(sdkCtx.BlockTime().Unix()); err != nil {
				return nil, types.ErrLegalInstrumentRequired.Wrap(err.Error())
			}
		}
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
		// The external legal authority. Required always, with no parameter that
		// turns it off — unlike the evidence above, which governance can waive.
		//
		// That difference is the design. Evidence is the chain's own record of
		// what it was shown, and a deployment can reasonably decide how much of
		// it to demand. An instrument is somebody outside this chain ordering
		// that the assets be taken, and a requirement governance can vote away
		// is a default rather than a requirement. A validator set able to
		// remove its own need for a court order is a validator set that does
		// not need one.
		if msg.LegalInstrument.IsZero() {
			return nil, types.ErrLegalInstrumentRequired.Wrap(
				"a seizure must name the court order, regulatory direction or warrant it is carried out under, and the hash of that instrument")
		}
		if err := msg.LegalInstrument.Validate(sdkCtx.BlockTime().Unix()); err != nil {
			return nil, types.ErrLegalInstrumentRequired.Wrap(err.Error())
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
		LegalInstrument:    msg.LegalInstrument,
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
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	// Voting yes advances a case towards taking somebody's assets, so it is an
	// initiation power in everything but name and the ombudsman is barred from
	// it here — including from voting no, because an office that could vote at
	// all would be inside the process it is appointed to check.
	// Within the constitutional concentration ceilings AT THIS MOMENT, not as
	// of the last epoch sweep. See Keeper.assertWithinCaps: the sweep is
	// periodic and this power is not, so without this a group that had crossed
	// a ceiling held it for up to a whole epoch.
	if err := k.assertWithinCaps(ctx, msg.Voter); err != nil {
		return nil, err
	}
	if err := k.assertNotOmbudsman(params, msg.Voter); err != nil {
		return nil, err
	}

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

// WithdrawCase takes back an open case. Only whoever opened it may, and it lifts
// the freeze — the same party that was trusted to impose it alone is trusted to
// admit they were wrong alone.
//
// It asks for identity and for nothing else. A validator that has since unbonded
// and an office whose grant has since been revoked may both still withdraw the
// case they opened, and that is deliberate rather than an oversight: withdrawing
// is de-escalation, and a rule that made it conditional on still holding a power
// would leave somebody's account frozen precisely because the party that was
// wrong about them lost its authority afterwards.
//
// Which is why identity is resolved here without the authority check openerOf
// makes. See openerIdentity.
func (k msgServer) WithdrawCase(ctx context.Context, msg *types.MsgWithdrawCase) (*types.MsgWithdrawCaseResponse, error) {
	enforcementCase, err := k.Case.Get(ctx, msg.CaseId)
	if err != nil {
		return nil, types.ErrCaseNotFound.Wrapf("case %d", msg.CaseId)
	}
	if enforcementCase.Status != types.CASE_STATUS_VOTING {
		return nil, types.ErrCaseClosed.Wrapf("case %d is %s", msg.CaseId, enforcementCase.Status)
	}
	opener, err := k.openerIdentity(ctx, msg.Opener)
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
	// HELD as well as PASSED. A seizure the validators agreed to and that is
	// waiting out its delay is the case governance is most likely to want to
	// overturn, because it is the only point at which overturning it costs
	// nobody anything — nothing has moved yet. Refusing here until the funds
	// had gone would have made the appeal available only once it was too late
	// to be worth having.
	switch enforcementCase.Status {
	case types.CASE_STATUS_PASSED, types.CASE_STATUS_HELD:
	default:
		return nil, types.ErrNotPassed.Wrapf("case %d is %s", msg.CaseId, enforcementCase.Status)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	wasHeld := enforcementCase.Status == types.CASE_STATUS_HELD
	enforcementCase.Status = types.CASE_STATUS_REVERSED
	enforcementCase.ResolvedAtHeight = sdkCtx.BlockHeight()
	if msg.Reason != "" {
		enforcementCase.Reason = enforcementCase.Reason + "\n\nReversed by governance: " + msg.Reason
	}
	if err := k.Case.Set(ctx, enforcementCase.Id, enforcementCase); err != nil {
		return nil, err
	}
	// The execution queue entry has to go with it, or the end blocker would
	// carry out the seizure governance has just overturned.
	if wasHeld {
		if err := k.dequeueExecution(ctx, enforcementCase); err != nil {
			return nil, err
		}
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

// openerOf decides whether an account may accuse at all, and what name the
// accusation goes on the record under.
//
// Two kinds of opener, and they are recorded differently on purpose. A bonded
// validator is recorded by its OPERATOR address, because that is the name a
// validator is legible under and an accusation attributed to an account address
// nobody recognises is an accusation with no visible author. An office is
// recorded by its own address, because a group policy has no operator address
// and its own address is exactly what a role-holders query is read against.
//
// The validator branch is tried first, and the order is not arbitrary. It is the
// path that existed before offices could open cases, so trying it first means
// nothing a validator could do changed shape — including the error a validator
// gets when it has unbonded, which stays ErrUnknownValidator as long as it holds
// no grant.
//
// # What this function does not do
//
// It does not authorise anything. Holding ROLE_ENFORCEMENT_AUTHORITY *somewhere*
// is enough to get past it, and the perimeter check in the caller is what
// decides whether this office may act on this target. That split is deliberate:
// HoldsRole answers an honest boolean about the actor, AssertScope answers the
// question that permits, and collapsing them would mean deciding authority from
// a call that cannot see the target.
//
// It also fails closed. A missing perimeter keeper makes holdsEnforcementRole an
// error rather than a false, so a wiring mistake refuses an office rather than
// quietly reporting that it is not one.
func (k Keeper) openerOf(ctx context.Context, account string) (string, error) {
	operator, _, err := k.bondedValidatorOf(ctx, account)
	if err == nil {
		return operator, nil
	}
	validatorErr := err

	authority, roleErr := k.holdsEnforcementRole(ctx, account)
	if roleErr != nil {
		return "", roleErr
	}
	if authority {
		return account, nil
	}
	// Neither. The validator refusal is what comes back, because it is the
	// specific one — it distinguishes "not a validator" from "not bonded" — and
	// the sentence appended says what the other road would have been. A signer
	// told only "you hold no enforcement grant" when they meant to sign as a
	// validator would go looking for a proposal they never needed.
	return "", errorsmod.Wrapf(validatorErr,
		"%s is not a bonded validator and holds no grant of %s, so it may not open a case",
		account, aliastypes.RoleName(aliastypes.ROLE_ENFORCEMENT_AUTHORITY))
}

// openerIdentity renders an account the way a case records its opener, asking
// nothing about whether it may open one.
//
// It is openerOf with the authority removed, and the two are separate functions
// rather than one with a flag because they answer different questions and only
// one of them decides anything. This one is "what would this account be called
// on a case", and it is used to MATCH against a case that already exists — so
// the only account it can ever match is the one that actually opened it. There
// is no authority to check, because holding one is not what withdrawal turns on.
//
// A validator that has unbonded therefore renders as its own account address
// rather than as its operator address, and so no longer matches a case it
// opened. That is a real consequence and it is the honest one: the freeze it
// imposed still lapses by itself, the validators can still resolve the case, and
// governance can still reverse one that passed. What it does not get is a
// message asserting a name it no longer answers to.
func (k Keeper) openerIdentity(ctx context.Context, account string) (string, error) {
	if _, err := k.addressCodec.StringToBytes(account); err != nil {
		return "", errorsmod.Wrap(err, "invalid address")
	}
	operator, _, err := k.bondedValidatorOf(ctx, account)
	if err == nil {
		return operator, nil
	}
	return account, nil
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
