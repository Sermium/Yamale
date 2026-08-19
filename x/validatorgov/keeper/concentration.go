package keeper

import (
	"context"
	"errors"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	constitutiontypes "yamale/blockchain/x/constitution/types"
	"yamale/blockchain/x/validatorgov/types"
)

// ConcentrationEndBlocker enforces the ceilings, once an epoch.
//
// Every epoch and not at admission, because admission is an event and
// concentration is a state. Nothing applies to vote on when a state-owned bank
// acquires a participant, when two members merge, when an operator is
// nationalised, or when a governance proposal sets a validator's power above a
// ceiling: power concentrates and there is no message to refuse. A check that
// only ran when somebody joined would be a ceiling on joining.
//
// The order is restore first, then demote. A demoted validator carries no
// power, so its groups are inside their ceilings precisely because it was
// demoted; asking whether the breach has cleared means asking what would happen
// if it came back, which has to be settled before this epoch's demotions are
// computed or the same validator would be restored and demoted in one block.
func (k Keeper) ConcentrationEndBlocker(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()

	inv, err := k.constitutionKeeper.GetInvariants(ctx)
	if err != nil {
		// A chain whose constitution module was added by an upgrade has no
		// settlement until its genesis is written. Enforcing nothing is the
		// only safe reading of that: the alternative is an end blocker that
		// returns an error every block, which halts the chain over the absence
		// of a ceiling rather than over a breach of one.
		if errors.Is(err, constitutiontypes.ErrNoInvariants) {
			return nil
		}
		return err
	}

	// The modulus is guarded at the point of use rather than trusted to
	// Validate(). A zero reaching this line — from a hand-written genesis, from
	// an upgrade that added the field, from any path that did not go through
	// the validator — is a division by zero inside an end blocker, which halts
	// the chain in the first block rather than failing a message.
	epoch := inv.ConcentrationEpochBlocks
	if epoch == 0 || height <= 0 || height%int64(epoch) != 0 {
		return nil
	}

	// Params are read for the attestation interval and nothing else, so a store
	// that does not have them yet must not stop the ceilings from being
	// enforced — and must certainly not return an error out of an end blocker,
	// which is a halt. A module added by an upgrade whose migration has not run
	// is exactly that state.
	params, err := k.Params.Get(ctx)
	if err != nil && !isNotFound(err) {
		return err
	}

	holders, err := k.activeSeatHolders(ctx)
	if err != nil {
		return err
	}

	if err := k.reportStaleDeclarations(ctx, height, params.AttestationInterval()); err != nil {
		return err
	}

	caps := types.CapsFrom(inv)

	holders, err = k.restoreCleared(ctx, holders, caps)
	if err != nil {
		return err
	}

	return k.applyDemotions(ctx, holders, caps)
}

// activeSeatHolders is the bonded validator set with its declarations attached.
//
// A validator with no approval record — a genesis validator admitted through
// the gentx ceremony rather than through a vote — is counted in the total but
// belongs to no group, so it can never be demoted. That is arithmetic, not
// leniency: leaving it out of the denominator would inflate everybody else's
// share and demote validators for power a third party holds, while putting
// every undeclared validator into one blank group would demote most of a
// founding set at the first epoch. The way to close it is to declare the
// founding set at genesis, which is why ApprovedValidator entries in genesis
// are refused without a declaration.
func (k Keeper) activeSeatHolders(ctx context.Context) ([]types.SeatHolder, error) {
	validators, err := k.stakingKeeper.GetBondedValidatorsByPower(ctx)
	if err != nil {
		return nil, err
	}

	holders := make([]types.SeatHolder, 0, len(validators))
	for _, validator := range validators {
		if validator.IsJailed() {
			continue
		}
		operator, err := k.operatorAccountOf(validator)
		if err != nil {
			return nil, err
		}
		var declaration types.Declaration
		approved, err := k.ApprovedValidator.Get(ctx, operator)
		switch {
		case err == nil:
			declaration = approved.Declaration
		case isNotFound(err):
			// Left blank on purpose: it counts in the total and in no group.
		default:
			return nil, err
		}

		power := validator.ConsensusPower(sdk.DefaultPowerReduction)
		if power <= 0 {
			continue
		}
		holders = append(holders, types.SeatHolder{
			Operator:    operator,
			Power:       power,
			Declaration: declaration,
		})
	}

	// x/staking already returns these in power order, but the order is fixed
	// again here rather than relied on. Every node has to compute the same
	// demotions from the same set, and an ordering that came from another
	// module's iteration is an ordering this module does not control.
	sort.Slice(holders, func(i, j int) bool {
		if holders[i].Power != holders[j].Power {
			return holders[i].Power > holders[j].Power
		}
		return holders[i].Operator < holders[j].Operator
	})

	return holders, nil
}

// restoreCleared gives the seats back to the demoted validators whose breach
// has cleared, and returns the active set with them counted in.
//
// Restoration is automatic and nobody votes on it. That is the difference
// between a concentration ceiling and an expulsion: the entity's remedy is in
// its own hands — restructure, or let the set grow around it — and it does not
// have to ask the validators who benefited from the demotion for permission to
// come back.
//
// Considered one at a time, in address order, and each restored validator is
// counted in before the next is considered. Restoring two at once could put
// their shared group back over its ceiling in the same block it was let out of.
func (k Keeper) restoreCleared(ctx context.Context, holders []types.SeatHolder, caps types.CapSet) ([]types.SeatHolder, error) {
	demotions := make([]types.Demotion, 0)
	if err := k.Demotion.Walk(ctx, nil, func(_ string, demotion types.Demotion) (bool, error) {
		demotions = append(demotions, demotion)
		return false, nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(demotions, func(i, j int) bool { return demotions[i].Operator < demotions[j].Operator })

	for _, demotion := range demotions {
		candidate, found, err := k.demotedSeatHolder(ctx, demotion.Operator)
		if err != nil {
			return nil, err
		}
		if !found {
			// The validator has gone entirely — removed, or its approval
			// withdrawn. There is nothing to restore and nothing to keep
			// holding down, so the record goes rather than pinning a demotion
			// on an operator that no longer exists.
			if err := k.Demotion.Remove(ctx, demotion.Operator); err != nil {
				return nil, err
			}
			continue
		}
		if !types.WithinCaps(candidate, holders, caps) {
			continue
		}

		unjailed, err := k.releaseDemotion(ctx, demotion)
		if err != nil {
			return nil, err
		}
		if err := k.Demotion.Remove(ctx, demotion.Operator); err != nil {
			return nil, err
		}
		if err := sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(&types.EventValidatorRestored{
			Operator:          demotion.Operator,
			Cap:               demotion.Cap,
			Group:             demotion.Group,
			UnjailedValidator: unjailed,
		}); err != nil {
			return nil, err
		}

		holders = append(holders, candidate)
	}

	sort.Slice(holders, func(i, j int) bool {
		if holders[i].Power != holders[j].Power {
			return holders[i].Power > holders[j].Power
		}
		return holders[i].Operator < holders[j].Operator
	})
	return holders, nil
}

// applyDemotions carries out the plan and records what it could not correct.
func (k Keeper) applyDemotions(ctx context.Context, holders []types.SeatHolder, caps types.CapSet) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()

	plans, uncorrected := types.Plan(holders, caps)

	for _, plan := range plans {
		jailed, err := k.jailForDemotion(ctx, plan.Operator)
		if err != nil {
			return err
		}

		demotion := types.Demotion{
			Operator:        plan.Operator,
			Cap:             plan.Cap,
			Group:           plan.Group,
			GroupPowerBps:   plan.GroupPowerBps,
			CapBps:          plan.CapBps,
			DemotedAtHeight: height,
			JailedValidator: jailed,
		}
		if err := k.Demotion.Set(ctx, plan.Operator, demotion); err != nil {
			return err
		}
		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventValidatorDemoted{
			Operator:        plan.Operator,
			Cap:             plan.Cap,
			Group:           plan.Group,
			GroupPowerBps:   plan.GroupPowerBps,
			CapBps:          plan.CapBps,
			JailedValidator: jailed,
		}); err != nil {
			return err
		}
	}

	for _, breach := range uncorrected {
		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventConcentrationUncorrected{
			Cap:                 breach.Cap,
			Group:               breach.Group,
			GroupPowerBps:       breach.GroupPowerBps,
			CapBps:              breach.CapBps,
			ActiveValidators:    breach.Active,
			MinActiveValidators: caps.MinActive,
		}); err != nil {
			return err
		}
	}

	return nil
}

// reportStaleDeclarations emits an event for every approved validator that has
// not re-attested within the interval.
//
// The event is the whole action. Turning an expired attestation into a demotion
// would make an operator's inattention a consensus event, and a set that all
// forgot at once would demote itself; what the chain can honestly do about a
// statement it cannot verify is publish the date and say so loudly.
func (k Keeper) reportStaleDeclarations(ctx context.Context, height int64, interval uint64) error {
	manager := sdk.UnwrapSDKContext(ctx).EventManager()

	stale := make([]types.ApprovedValidator, 0)
	if err := k.ApprovedValidator.Walk(ctx, nil, func(_ string, approved types.ApprovedValidator) (bool, error) {
		if approved.Declaration.IsStale(height, interval) {
			stale = append(stale, approved)
		}
		return false, nil
	}); err != nil {
		return err
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].Candidate < stale[j].Candidate })

	for _, approved := range stale {
		if err := manager.EmitTypedEvent(&types.EventDeclarationStale{
			Operator:         approved.Candidate,
			AttestedAtHeight: approved.Declaration.AttestedAtHeight,
			StaleSinceHeight: approved.Declaration.AttestedAtHeight + int64(interval),
		}); err != nil {
			return err
		}
	}
	return nil
}

// demotedSeatHolder is what a demoted validator would carry if it came back.
//
// It reads PotentialConsensusPower rather than ConsensusPower deliberately:
// x/staking reports a jailed validator's consensus power as zero, so asking the
// ordinary way would say every demoted validator could be restored for free and
// the ceilings would let the whole set back in at the next epoch.
func (k Keeper) demotedSeatHolder(ctx context.Context, operator string) (types.SeatHolder, bool, error) {
	approved, err := k.ApprovedValidator.Get(ctx, operator)
	if err != nil {
		if isNotFound(err) {
			return types.SeatHolder{}, false, nil
		}
		return types.SeatHolder{}, false, err
	}

	validator, found, err := k.validatorOf(ctx, operator)
	if err != nil || !found {
		return types.SeatHolder{}, false, err
	}

	return types.SeatHolder{
		Operator:    operator,
		Power:       validator.PotentialConsensusPower(sdk.DefaultPowerReduction),
		Declaration: approved.Declaration,
	}, true, nil
}

// jailForDemotion takes a validator out of the active set.
//
// Jailing, rather than reporting a lower power to CometBFT. The SDK permits
// exactly one module to return validator updates and x/staking is that module,
// so a second source of power would have to be reconciled against x/staking's
// own record of what it last applied — and any drift between the two is a
// consensus failure rather than a bug. Under equal seats the objection that
// made jailing look blunt is gone anyway: a validator holds one seat, and
// jailing removes exactly one seat.
//
// It returns whether this call is what jailed it. A validator already jailed
// for downtime must stay jailed when the breach clears, or a concentration
// ceiling becomes a way of clearing somebody else's downtime.
func (k Keeper) jailForDemotion(ctx context.Context, operator string) (bool, error) {
	validator, found, err := k.validatorOf(ctx, operator)
	if err != nil || !found {
		return false, err
	}
	if validator.IsJailed() {
		return false, nil
	}

	consAddr, err := validator.GetConsAddr()
	if err != nil {
		return false, err
	}
	if err := k.stakingKeeper.Jail(ctx, consAddr); err != nil {
		return false, err
	}
	return true, nil
}

// releaseDemotion undoes what jailForDemotion did, and only that.
func (k Keeper) releaseDemotion(ctx context.Context, demotion types.Demotion) (bool, error) {
	if !demotion.JailedValidator {
		return false, nil
	}

	validator, found, err := k.validatorOf(ctx, demotion.Operator)
	if err != nil || !found {
		return false, err
	}
	if !validator.IsJailed() {
		return false, nil
	}

	consAddr, err := validator.GetConsAddr()
	if err != nil {
		return false, err
	}
	if err := k.stakingKeeper.Unjail(ctx, consAddr); err != nil {
		return false, err
	}
	return true, nil
}

// validatorOf resolves an operator address in its account form to the validator
// behind it. An approved candidate that never created a validator is an
// ordinary outcome here rather than an error.
func (k Keeper) validatorOf(ctx context.Context, operator string) (stakingtypes.Validator, bool, error) {
	bz, err := k.addressCodec.StringToBytes(operator)
	if err != nil {
		return stakingtypes.Validator{}, false, err
	}
	validator, err := k.stakingKeeper.GetValidator(ctx, sdk.ValAddress(bz))
	if err != nil {
		if isNotFound(err) || errors.Is(err, stakingtypes.ErrNoValidatorFound) {
			return stakingtypes.Validator{}, false, nil
		}
		return stakingtypes.Validator{}, false, err
	}
	return validator, true, nil
}

// operatorAccountOf renders a validator's operator address in the account form
// every record in this module is keyed by.
func (k Keeper) operatorAccountOf(validator stakingtypes.Validator) (string, error) {
	valAddr, err := sdk.ValAddressFromBech32(validator.OperatorAddress)
	if err != nil {
		return "", err
	}
	return k.addressCodec.BytesToString(valAddr.Bytes())
}
