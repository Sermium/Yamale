package keeper

import (
	"context"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/treasury/types"
)

// CreateTreasury opens a new treasury. Creation is permissionless — a treasury
// with no funds in it grants nobody anything, and gating creation behind
// governance would make the feature unusable for the teams it is for.
func (k msgServer) CreateTreasury(ctx context.Context, msg *types.MsgCreateTreasury) (*types.MsgCreateTreasuryResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	admin := msg.Admin
	if admin == "" {
		admin = msg.Creator
	}
	if _, err := k.addressCodec.StringToBytes(admin); err != nil {
		return nil, errorsmod.Wrap(err, "invalid admin address")
	}

	id, err := k.TreasurySeq.Next(ctx)
	if err != nil {
		return nil, err
	}

	if err := k.Treasury.Set(ctx, id, types.Treasury{
		Id:              id,
		Name:            msg.Name,
		Admin:           admin,
		Paused:          false,
		CreatedAtHeight: uint64(sdk.UnwrapSDKContext(ctx).BlockHeight()),
	}); err != nil {
		return nil, err
	}

	return &types.MsgCreateTreasuryResponse{Id: id}, nil
}

// Deposit funds a treasury. Anyone may deposit into any treasury, the same way
// anyone may pay an invoice; the depositor gains no claim on the funds and no
// control over the treasury.
func (k msgServer) Deposit(ctx context.Context, msg *types.MsgDeposit) (*types.MsgDepositResponse, error) {
	depositorBz, err := k.addressCodec.StringToBytes(msg.Depositor)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid depositor address")
	}

	treasury, err := k.getTreasury(ctx, msg.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := requireNotPaused(treasury); err != nil {
		return nil, err
	}

	if !msg.Amount.IsValid() || !msg.Amount.IsAllPositive() {
		return nil, types.ErrInvalidAmount.Wrapf("deposit %s", msg.Amount)
	}

	// Take custody first, then record it. If the transfer fails there is
	// nothing to unwind.
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sdk.AccAddress(depositorBz), types.ModuleName, msg.Amount); err != nil {
		return nil, err
	}

	for _, coin := range msg.Amount {
		if err := k.creditBalance(ctx, treasury.Id, coin.Denom, coin.Amount); err != nil {
			return nil, err
		}
	}

	return &types.MsgDepositResponse{}, nil
}

// SetAdmin transfers administrative control. Point it at an x/group policy
// address to move a treasury from single-key to M-of-N control without moving
// any funds.
func (k msgServer) SetAdmin(ctx context.Context, msg *types.MsgSetAdmin) (*types.MsgSetAdminResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.NewAdmin); err != nil {
		return nil, errorsmod.Wrap(err, "invalid new admin address")
	}

	treasury, err := k.getTreasury(ctx, msg.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := k.requireAdmin(ctx, treasury, msg.Admin); err != nil {
		return nil, err
	}

	treasury.Admin = msg.NewAdmin
	return &types.MsgSetAdminResponse{}, k.Treasury.Set(ctx, treasury.Id, treasury)
}

// SetPaused freezes or unfreezes a treasury.
func (k msgServer) SetPaused(ctx context.Context, msg *types.MsgSetPaused) (*types.MsgSetPausedResponse, error) {
	treasury, err := k.getTreasury(ctx, msg.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := k.requirePauser(ctx, treasury, msg.Sender); err != nil {
		return nil, err
	}

	treasury.Paused = msg.Paused
	return &types.MsgSetPausedResponse{}, k.Treasury.Set(ctx, treasury.Id, treasury)
}

// AssignRole grants an address a role over a treasury.
func (k msgServer) AssignRole(ctx context.Context, msg *types.MsgAssignRole) (*types.MsgAssignRoleResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Address); err != nil {
		return nil, errorsmod.Wrap(err, "invalid address")
	}
	if msg.Role == types.Role_ROLE_UNSPECIFIED {
		return nil, types.ErrInvalidRole.Wrap("role must be specified; use RevokeRole to remove one")
	}

	treasury, err := k.getTreasury(ctx, msg.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := k.requireAdmin(ctx, treasury, msg.Admin); err != nil {
		return nil, err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	key := collections.Join(treasury.Id, msg.Address)
	existing, err := k.Role.Has(ctx, key)
	if err != nil {
		return nil, err
	}
	if !existing {
		count, err := k.countRoles(ctx, treasury.Id)
		if err != nil {
			return nil, err
		}
		if count >= params.MaxRoleAssignmentsPerTreasury {
			return nil, types.ErrLimitReached.Wrapf(
				"treasury %d already has the maximum of %d role assignments", treasury.Id, params.MaxRoleAssignmentsPerTreasury)
		}
	}

	return &types.MsgAssignRoleResponse{}, k.Role.Set(ctx, key, types.RoleAssignment{
		TreasuryId: treasury.Id,
		Address:    msg.Address,
		Role:       msg.Role,
	})
}

// RevokeRole removes an address's role.
func (k msgServer) RevokeRole(ctx context.Context, msg *types.MsgRevokeRole) (*types.MsgRevokeRoleResponse, error) {
	treasury, err := k.getTreasury(ctx, msg.TreasuryId)
	if err != nil {
		return nil, err
	}
	if err := k.requireAdmin(ctx, treasury, msg.Admin); err != nil {
		return nil, err
	}

	key := collections.Join(treasury.Id, msg.Address)
	has, err := k.Role.Has(ctx, key)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, types.ErrInvalidRole.Wrapf("%s holds no role on treasury %d", msg.Address, treasury.Id)
	}

	return &types.MsgRevokeRoleResponse{}, k.Role.Remove(ctx, key)
}

// countRoles counts a treasury's explicit role assignments.
func (k Keeper) countRoles(ctx context.Context, treasuryID uint64) (uint64, error) {
	iter, err := k.Role.Iterate(ctx, collections.NewPrefixedPairRange[uint64, string](treasuryID))
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	var count uint64
	for ; iter.Valid(); iter.Next() {
		count++
	}
	return count, nil
}
