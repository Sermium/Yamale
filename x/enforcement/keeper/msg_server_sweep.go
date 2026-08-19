package keeper

import (
	"context"

	"yamale/blockchain/x/enforcement/types"
)

// Sweep collects whatever a passed seizure can now reach.
//
// It is permissionless and repeatable, because a seizure against a staked
// target is not one event. The stake was unbonded when the case passed and
// arrives weeks later; somebody has to be able to collect it then, and needing
// another validator vote to finish a seizure that already passed would either
// leave the funds sitting there or turn every recovery into two votes.
//
// The destination comes from the parameters, so the sender gains nothing by
// being the one who calls this. It is a chore anybody can do.
func (k msgServer) Sweep(ctx context.Context, msg *types.MsgSweep) (*types.MsgSweepResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Sender); err != nil {
		return nil, err
	}

	enforcementCase, err := k.Case.Get(ctx, msg.CaseId)
	if err != nil {
		return nil, types.ErrCaseNotFound.Wrapf("case %d", msg.CaseId)
	}
	if enforcementCase.Action != types.CASE_ACTION_SEIZE {
		return nil, types.ErrNotSeizure.Wrapf("case %d ordered a freeze, not a seizure", msg.CaseId)
	}
	if enforcementCase.Status != types.CASE_STATUS_PASSED {
		return nil, types.ErrNotPassed.Wrapf("case %d is %s", msg.CaseId, enforcementCase.Status)
	}
	// A case marked complete is not sealed. The account stays frozen after a
	// seizure, so anything that arrives later — a late payment from an
	// accomplice, an unbonding that matured after the last sweep — is trapped
	// there. Refusing to sweep a "finished" case would leave those funds stuck
	// forever: unusable by the target and uncollectable by anyone else.

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// A destination that has been unset since the case passed would send the
	// funds to the zero address. Refused rather than guessed.
	if params.RecoveryDestination == "" {
		return nil, types.ErrInvalidCase.Wrap("no recovery destination is set")
	}

	// Delegations can reappear between sweeps: the target still holds their
	// keys until the account is empty, and nothing stops them staking again
	// with funds that arrive from elsewhere. Unbonding again on every sweep is
	// what keeps that from being a way to hold the seizure off indefinitely.
	if err := k.unbondEverything(ctx, enforcementCase.Target); err != nil {
		return nil, err
	}

	collected, complete, err := k.collect(ctx, &enforcementCase, params)
	if err != nil {
		return nil, err
	}
	if err := k.Case.Set(ctx, enforcementCase.Id, enforcementCase); err != nil {
		return nil, err
	}

	return &types.MsgSweepResponse{Collected: collected, Complete: complete}, nil
}
