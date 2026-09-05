package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
)

// SendRestriction is the freeze, enforced where it cannot be walked around.
//
// It is registered on the bank keeper rather than in the ante chain on purpose.
// An ante decorator only sees messages that arrive as transactions, so a freeze
// enforced there would still be moved through by anything that reaches the
// bank another way: authz, interchain accounts, a treasury spend, a swap
// against a pool. Every one of those ends in a bank transfer, and this is the
// one place all of them pass through.
//
// It blocks sending, not receiving. A frozen account can still be paid — which
// is deliberate: refusing incoming funds would bounce payments from people who
// have done nothing wrong, and would hide the trail rather than preserve it.
func (k Keeper) SendRestriction(ctx context.Context, from, to sdk.AccAddress, amount sdk.Coins) (sdk.AccAddress, error) {
	fromStr, err := k.addressCodec.BytesToString(from)
	if err != nil {
		return to, err
	}

	freeze, found, err := k.FreezeOf(ctx, fromStr)
	if err != nil {
		return to, err
	}
	if !found {
		return to, nil
	}

	// The one way out is the way the case ordered: a seizure has to be able to
	// move the funds it seized, and it can only ever move them to the recovery
	// destination, which is set by governance and named in the parameters. So
	// this exception cannot be used to send anywhere else.
	params, err := k.Params.Get(ctx)
	if err != nil {
		return to, err
	}
	if params.RecoveryDestination != "" {
		toStr, err := k.addressCodec.BytesToString(to)
		if err != nil {
			return to, err
		}
		if toStr == params.RecoveryDestination {
			return to, nil
		}
	}

	return to, types.ErrFrozen.Wrapf(
		"account %s is frozen by enforcement case %d; query enforcement/v1/freeze/%s for the case and its grounds",
		fromStr, freeze.CaseId, fromStr,
	)
}

// reclaimWithdrawAddress points a frozen account's staking rewards back at
// itself.
//
// The send restriction above cannot see a reward withdrawal, because the sender
// on that transfer is the distribution module account. So the freeze reaches
// into x/distribution once, at the moment it is applied, and removes the
// redirect. Rewards then land in the frozen account and stop there.
//
// Deliberately not reversed when the freeze lifts. The account can set its
// withdraw address again itself; guessing which of possibly several earlier
// values to restore would be the module inventing an instruction nobody gave.
func (k Keeper) reclaimWithdrawAddress(ctx context.Context, addr string) error {
	if k.distrKeeper == nil {
		// A chain with no x/distribution has no rewards to redirect.
		return nil
	}
	account, err := k.addressCodec.StringToBytes(addr)
	if err != nil {
		return err
	}
	current, err := k.distrKeeper.GetDelegatorWithdrawAddr(ctx, account)
	if err != nil {
		// No record means it has never been redirected, which is the state we
		// would be putting it in anyway.
		return nil
	}
	if current.Equals(sdk.AccAddress(account)) {
		return nil
	}
	return k.distrKeeper.SetWithdrawAddr(ctx, account, account)
}
