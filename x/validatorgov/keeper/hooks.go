package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// Hooks is the validator gate, enforced where the message router reaches rather
// than where transactions arrive.
//
// The ante decorator in x/validatorgov/ante is still there and still worth
// having — it refuses a bad MsgCreateValidator before any state is touched, and
// it descends into MsgExec so authz cannot carry one past it. What it cannot
// see is a road that does not go through the ante chain at all, and x/group
// execution is exactly that: a passed proposal dispatches its messages straight
// through the message router, after the ante handler has run. Both chain-wide
// ROLE_FOUNDATION_ADMINISTRATOR holders on this chain are x/group accounts, so
// either could have created a validator for an unapproved candidate. Interchain
// accounts arrive the same way.
//
// This is the argument x/alias makes for putting AssertScope on the keeper
// rather than in a decorator, and x/enforcement makes for putting the freeze on
// the bank keeper: a gate belongs at the narrowest point every path passes
// through, and for validator creation that is x/staking's own keeper.
type Hooks struct{ k Keeper }

var _ stakingtypes.StakingHooks = Hooks{}

// Hooks returns the staking hooks this module registers.
func (k Keeper) Hooks() Hooks { return Hooks{k: k} }

// AfterValidatorCreated refuses a validator whose operator governance has not
// approved.
//
// x/staking calls this inside CreateValidator and propagates the error, so a
// refusal fails the message and nothing is written — the same outcome the ante
// decorator produces, by a route no wrapper can avoid.
func (h Hooks) AfterValidatorCreated(ctx context.Context, valAddr sdk.ValAddress) error {
	// Genesis validators are onboarded through the gentx collection ceremony
	// rather than by vote, so the gate does not apply at height zero — the same
	// exemption, and the same reason, as the ante decorator's.
	if sdk.UnwrapSDKContext(ctx).BlockHeight() == 0 {
		return nil
	}

	candidate := sdk.AccAddress(valAddr).String()
	approved, err := h.k.ApprovedValidator.Has(ctx, candidate)
	if err != nil {
		return err
	}
	if !approved {
		return errorsmod.Wrapf(sdkerrors.ErrUnauthorized,
			"validator candidate %s is not approved by governance; submit a MsgApplyValidator and await a governance vote", candidate)
	}
	return nil
}

// The rest of the interface. This module has nothing to do on any of them, and
// they are written out rather than embedded so that a future SDK adding a hook
// is a compile error here instead of a silently unimplemented one.
func (Hooks) BeforeValidatorModified(context.Context, sdk.ValAddress) error { return nil }
func (Hooks) AfterValidatorRemoved(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}
func (Hooks) AfterValidatorBonded(context.Context, sdk.ConsAddress, sdk.ValAddress) error { return nil }
func (Hooks) AfterValidatorBeginUnbonding(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}
func (Hooks) BeforeDelegationCreated(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}
func (Hooks) BeforeDelegationSharesModified(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}
func (Hooks) BeforeDelegationRemoved(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}
func (Hooks) AfterDelegationModified(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}
func (Hooks) BeforeValidatorSlashed(context.Context, sdk.ValAddress, math.LegacyDec) error {
	return nil
}
func (Hooks) AfterUnbondingInitiated(context.Context, uint64) error { return nil }
