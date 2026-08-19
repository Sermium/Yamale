package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/treasury/types"
)

// getTreasury loads a treasury or reports a clean not-found.
func (k Keeper) getTreasury(ctx context.Context, id uint64) (types.Treasury, error) {
	t, err := k.Treasury.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Treasury{}, types.ErrTreasuryNotFound.Wrapf("treasury %d", id)
		}
		return types.Treasury{}, err
	}
	return t, nil
}

// roleOf returns the role an address holds over a treasury.
//
// The admin address always counts as an admin without needing an explicit
// assignment. Requiring the admin to also grant itself a role would be a
// footgun: transferring admin would silently strip the new admin of its
// powers until it remembered to re-grant them.
func (k Keeper) roleOf(ctx context.Context, treasury types.Treasury, address string) (types.Role, error) {
	if address == treasury.Admin {
		return types.Role_ROLE_ADMIN, nil
	}

	assignment, err := k.Role.Get(ctx, collections.Join(treasury.Id, address))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Role_ROLE_UNSPECIFIED, nil
		}
		return types.Role_ROLE_UNSPECIFIED, err
	}
	return assignment.Role, nil
}

// requireAdmin authorizes an action that reconfigures the treasury.
func (k Keeper) requireAdmin(ctx context.Context, treasury types.Treasury, address string) error {
	role, err := k.roleOf(ctx, treasury, address)
	if err != nil {
		return err
	}
	if role != types.Role_ROLE_ADMIN {
		return types.ErrUnauthorized.Wrapf("%s is not an admin of treasury %d", address, treasury.Id)
	}
	return nil
}

// requireSpender authorizes moving funds out. Admins may spend as well, since
// an admin can grant itself the spender role anyway and pretending otherwise
// would only add a step.
func (k Keeper) requireSpender(ctx context.Context, treasury types.Treasury, address string) error {
	role, err := k.roleOf(ctx, treasury, address)
	if err != nil {
		return err
	}
	if role != types.Role_ROLE_ADMIN && role != types.Role_ROLE_SPENDER {
		return types.ErrUnauthorized.Wrapf("%s may not spend from treasury %d", address, treasury.Id)
	}
	return nil
}

// requirePauser authorizes freezing and unfreezing.
func (k Keeper) requirePauser(ctx context.Context, treasury types.Treasury, address string) error {
	role, err := k.roleOf(ctx, treasury, address)
	if err != nil {
		return err
	}
	if role != types.Role_ROLE_ADMIN && role != types.Role_ROLE_PAUSER {
		return types.ErrUnauthorized.Wrapf("%s may not pause treasury %d", address, treasury.Id)
	}
	return nil
}

// requireNotPaused blocks value movement while a treasury is frozen.
//
// Pausing stops funds moving in either direction, including beneficiary claims.
// That is a real cost to beneficiaries, and it is the intended trade: a pause
// exists for the case where the treasury's control has been compromised, and a
// freeze that let an attacker keep draining through the lock path would not be
// a freeze.
func requireNotPaused(treasury types.Treasury) error {
	if treasury.Paused {
		return types.ErrTreasuryPaused.Wrapf("treasury %d is paused", treasury.Id)
	}
	return nil
}
