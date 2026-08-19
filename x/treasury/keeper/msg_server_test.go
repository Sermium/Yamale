package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/treasury/types"
)

func TestCreateAndDeposit(t *testing.T) {
	f := initFixture(t)

	id, admin, _ := f.newTreasury(t, 1_000_000)

	treasury, err := f.keeper.Treasury.Get(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, "Test Treasury", treasury.Name)
	require.False(t, treasury.Paused)

	// The funds left the depositor and are held by the module, not by the admin.
	require.True(t, f.env.Balance(admin, denom).IsZero())
	require.Equal(t, math.NewInt(1_000_000), f.env.ModuleBalance(types.ModuleName, denom))
	require.Equal(t, math.NewInt(1_000_000), f.available(t, id))
	require.True(t, f.locked(t, id).IsZero())
}

// Depositing is permissionless and confers no control.
func TestDepositByStranger(t *testing.T) {
	f := initFixture(t)
	id, _, _ := f.newTreasury(t, 0)

	_, strangerStr := f.env.NewFundedAddr(t, coins(500))
	_, err := f.ms.Deposit(f.ctx, &types.MsgDeposit{
		Depositor: strangerStr, TreasuryId: id, Amount: coins(500),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(500), f.available(t, id))

	// The depositor gained no spending rights.
	recipient, recipientStr := f.env.Addr(t)
	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: strangerStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(100),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)
	require.True(t, f.env.Balance(recipient, denom).IsZero())
}

func TestSpendRequiresAuthorization(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	recipient, recipientStr := f.env.Addr(t)

	// A stranger may not spend.
	_, strangerStr := f.env.Addr(t)
	_, err := f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: strangerStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(100),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	// The admin may.
	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(100),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(100), f.env.Balance(recipient, denom))
	require.Equal(t, math.NewInt(999_900), f.available(t, id))

	// And so may an appointed spender.
	spender, spenderStr := f.env.Addr(t)
	_, err = f.ms.AssignRole(f.ctx, &types.MsgAssignRole{
		Admin: adminStr, TreasuryId: id, Address: spenderStr, Role: types.Role_ROLE_SPENDER,
	})
	require.NoError(t, err)

	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: spenderStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(50),
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(150), f.env.Balance(recipient, denom))

	// A spender may not reconfigure the treasury.
	_, err = f.ms.AssignRole(f.ctx, &types.MsgAssignRole{
		Admin: spenderStr, TreasuryId: id, Address: spenderStr, Role: types.Role_ROLE_ADMIN,
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)
	_ = spender
}

// This is the module's central promise: committed funds cannot be spent, by
// anyone, including the admin.
func TestLockedFundsCannotBeSpent(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	beneficiary, beneficiaryStr := f.env.Addr(t)

	_, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "800000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000,
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(800_000), f.locked(t, id))
	require.Equal(t, math.NewInt(200_000), f.available(t, id))

	_, recipientStr := f.env.Addr(t)

	// Spending the unlocked remainder is fine.
	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(200_000),
	})
	require.NoError(t, err)

	// One unit beyond it is not, even for the admin.
	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(1),
	})
	require.ErrorIs(t, err, types.ErrInsufficientFunds)

	// The module still physically holds the locked funds for the beneficiary.
	require.Equal(t, math.NewInt(800_000), f.env.ModuleBalance(types.ModuleName, denom))
	require.True(t, f.env.Balance(beneficiary, denom).IsZero())
}

func TestClaimVesting(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	beneficiary, beneficiaryStr := f.env.Addr(t)
	resp, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "1000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000,
	})
	require.NoError(t, err)

	// Nothing has vested at the start.
	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: resp.Id})
	require.ErrorIs(t, err, types.ErrNothingToClaim)

	// Half way through, half is claimable.
	f.at(1500)
	claim, err := f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: resp.Id})
	require.NoError(t, err)
	require.Equal(t, "500", claim.Released)
	require.Equal(t, math.NewInt(500), f.env.Balance(beneficiary, denom))
	require.Equal(t, math.NewInt(500), f.locked(t, id), "claimed funds must leave the locked balance")

	// Claiming again immediately yields nothing.
	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: resp.Id})
	require.ErrorIs(t, err, types.ErrNothingToClaim)

	// At the end the rest is claimable and the lock retires.
	f.at(2000)
	claim, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: resp.Id})
	require.NoError(t, err)
	require.Equal(t, "500", claim.Released)
	require.Equal(t, math.NewInt(1000), f.env.Balance(beneficiary, denom))
	require.True(t, f.locked(t, id).IsZero())

	lock, err := f.keeper.Lock.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.False(t, lock.Active, "a fully claimed lock should retire")
	require.Equal(t, "1000", lock.ReleasedAmount, "the lock is retained as an audit record")
}

func TestOnlyBeneficiaryMayClaim(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, beneficiaryStr := f.env.Addr(t)
	resp, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "1000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000,
	})
	require.NoError(t, err)

	f.at(1500)

	// Not the admin, and not a stranger.
	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: adminStr, LockId: resp.Id})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	_, strangerStr := f.env.Addr(t)
	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: strangerStr, LockId: resp.Id})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	require.Equal(t, math.NewInt(1000), f.locked(t, id))
}

// Revoking returns only the unvested part; what has already vested belongs to
// the beneficiary whether or not they claimed it.
func TestRevokeLockPaysVestedAndReturnsRest(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	beneficiary, beneficiaryStr := f.env.Addr(t)
	resp, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "1000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000, Revocable: true,
	})
	require.NoError(t, err)

	f.at(1400) // 40% vested, unclaimed

	revoke, err := f.ms.RevokeLock(f.ctx, &types.MsgRevokeLock{Admin: adminStr, LockId: resp.Id})
	require.NoError(t, err)
	require.Equal(t, "400", revoke.VestedToBeneficiary)
	require.Equal(t, "600", revoke.Returned)

	require.Equal(t, math.NewInt(400), f.env.Balance(beneficiary, denom),
		"already-vested funds must reach the beneficiary, not be clawed back")
	require.True(t, f.locked(t, id).IsZero())
	require.Equal(t, math.NewInt(999_600), f.available(t, id), "the unvested part returns to the treasury")

	lock, err := f.keeper.Lock.Get(f.ctx, resp.Id)
	require.NoError(t, err)
	require.False(t, lock.Active)

	// A revoked lock cannot be claimed or revoked again.
	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: resp.Id})
	require.ErrorIs(t, err, types.ErrLockInactive)
}

func TestIrrevocableLockCannotBeRevoked(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, beneficiaryStr := f.env.Addr(t)
	resp, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "1000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000, Revocable: false,
	})
	require.NoError(t, err)

	_, err = f.ms.RevokeLock(f.ctx, &types.MsgRevokeLock{Admin: adminStr, LockId: resp.Id})
	require.ErrorIs(t, err, types.ErrNotRevocable)
	require.Equal(t, math.NewInt(1000), f.locked(t, id))
}

func TestCreateLockRequiresAvailableFunds(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000)

	_, beneficiaryStr := f.env.Addr(t)
	_, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "1001", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000,
	})
	require.ErrorIs(t, err, types.ErrInsufficientFunds)

	// No lock was created for the failed commitment.
	require.True(t, f.locked(t, id).IsZero())
	require.Equal(t, math.NewInt(1_000), f.available(t, id))
}

func TestSpendPolicyPerTransactionLimit(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, err := f.ms.SetSpendPolicy(f.ctx, &types.MsgSetSpendPolicy{
		Admin: adminStr,
		Policy: types.SpendPolicy{
			TreasuryId: id, Denom: denom, PerTransactionLimit: "1000",
		},
	})
	require.NoError(t, err)

	_, recipientStr := f.env.Addr(t)

	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(1001),
	})
	require.ErrorIs(t, err, types.ErrSpendLimit)

	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(1000),
	})
	require.NoError(t, err)
}

// The period limit is what bounds a compromised spender key's damage.
func TestSpendPolicyPeriodLimitAndReset(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, err := f.ms.SetSpendPolicy(f.ctx, &types.MsgSetSpendPolicy{
		Admin: adminStr,
		Policy: types.SpendPolicy{
			TreasuryId: id, Denom: denom, PeriodLimit: "1000", PeriodSeconds: 3600,
		},
	})
	require.NoError(t, err)

	_, recipientStr := f.env.Addr(t)
	spend := func(amount int64) error {
		_, err := f.ms.Spend(f.ctx, &types.MsgSpend{
			Spender: adminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(amount),
		})
		return err
	}

	require.NoError(t, spend(600))
	require.NoError(t, spend(400)) // exactly at the limit
	require.ErrorIs(t, spend(1), types.ErrSpendLimit)

	// Still capped later in the same window.
	f.at(1000 + 3599)
	require.ErrorIs(t, spend(1), types.ErrSpendLimit)

	// The window resets and the allowance returns.
	f.at(1000 + 3600)
	require.NoError(t, spend(1000))
	require.ErrorIs(t, spend(1), types.ErrSpendLimit)
}

func TestSpendPolicyAllowlistAndBlocklist(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, allowedStr := f.env.Addr(t)
	_, blockedStr := f.env.Addr(t)
	_, otherStr := f.env.Addr(t)

	_, err := f.ms.SetSpendPolicy(f.ctx, &types.MsgSetSpendPolicy{
		Admin: adminStr,
		Policy: types.SpendPolicy{
			TreasuryId: id, Denom: denom,
			Allowlist: []string{allowedStr, blockedStr},
			Blocklist: []string{blockedStr},
		},
	})
	require.NoError(t, err)

	spendTo := func(to string) error {
		_, err := f.ms.Spend(f.ctx, &types.MsgSpend{
			Spender: adminStr, TreasuryId: id, Recipient: to, Amount: coins(10),
		})
		return err
	}

	require.NoError(t, spendTo(allowedStr))
	require.ErrorIs(t, spendTo(otherStr), types.ErrDestinationDenied, "a non-empty allowlist excludes everyone else")
	require.ErrorIs(t, spendTo(blockedStr), types.ErrDestinationDenied, "the blocklist must win over the allowlist")
}

// A pause is meant to contain a compromise, so it stops movement in every
// direction — including beneficiary claims.
func TestPauseHaltsValueMovement(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, beneficiaryStr := f.env.Addr(t)
	resp, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "1000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000,
	})
	require.NoError(t, err)

	pauser, pauserStr := f.env.Addr(t)
	_, err = f.ms.AssignRole(f.ctx, &types.MsgAssignRole{
		Admin: adminStr, TreasuryId: id, Address: pauserStr, Role: types.Role_ROLE_PAUSER,
	})
	require.NoError(t, err)

	_, err = f.ms.SetPaused(f.ctx, &types.MsgSetPaused{Sender: pauserStr, TreasuryId: id, Paused: true})
	require.NoError(t, err)

	f.at(1500)
	_, recipientStr := f.env.Addr(t)

	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(1),
	})
	require.ErrorIs(t, err, types.ErrTreasuryPaused)

	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: resp.Id})
	require.ErrorIs(t, err, types.ErrTreasuryPaused)

	// A pauser cannot move funds, only freeze them.
	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: pauserStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(1),
	})
	require.Error(t, err)

	// Unpausing restores normal operation.
	_, err = f.ms.SetPaused(f.ctx, &types.MsgSetPaused{Sender: adminStr, TreasuryId: id, Paused: false})
	require.NoError(t, err)
	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: resp.Id})
	require.NoError(t, err)
	_ = pauser
}

// Handing the treasury to an x/group policy address is how a treasury moves
// from single-key to M-of-N control, without moving any funds.
func TestSetAdminTransfersControl(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, newAdminStr := f.env.Addr(t)
	_, err := f.ms.SetAdmin(f.ctx, &types.MsgSetAdmin{
		Admin: adminStr, TreasuryId: id, NewAdmin: newAdminStr,
	})
	require.NoError(t, err)

	_, recipientStr := f.env.Addr(t)

	// The old admin has lost control...
	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(1),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	// ...and the new one has it, without needing an explicit role grant.
	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: newAdminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(1),
	})
	require.NoError(t, err)
}

func TestRevokeRole(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, spenderStr := f.env.Addr(t)
	_, err := f.ms.AssignRole(f.ctx, &types.MsgAssignRole{
		Admin: adminStr, TreasuryId: id, Address: spenderStr, Role: types.Role_ROLE_SPENDER,
	})
	require.NoError(t, err)

	_, recipientStr := f.env.Addr(t)
	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: spenderStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(1),
	})
	require.NoError(t, err)

	_, err = f.ms.RevokeRole(f.ctx, &types.MsgRevokeRole{
		Admin: adminStr, TreasuryId: id, Address: spenderStr,
	})
	require.NoError(t, err)

	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: spenderStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(1),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)
}

// Treasuries must be isolated: an admin of one has no power over another.
func TestTreasuriesAreIsolated(t *testing.T) {
	f := initFixture(t)

	idA, _, adminAStr := f.newTreasury(t, 1_000_000)
	idB, _, adminBStr := f.newTreasury(t, 1_000_000)

	_, recipientStr := f.env.Addr(t)

	_, err := f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminAStr, TreasuryId: idB, Recipient: recipientStr, Amount: coins(1),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminBStr, TreasuryId: idA, Recipient: recipientStr, Amount: coins(1),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	require.Equal(t, math.NewInt(1_000_000), f.available(t, idA))
	require.Equal(t, math.NewInt(1_000_000), f.available(t, idB))
}

func TestUnknownTreasuryAndLock(t *testing.T) {
	f := initFixture(t)

	_, adminStr := f.env.Addr(t)
	_, recipientStr := f.env.Addr(t)

	_, err := f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: 99, Recipient: recipientStr, Amount: coins(1),
	})
	require.ErrorIs(t, err, types.ErrTreasuryNotFound)

	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: adminStr, LockId: 99})
	require.ErrorIs(t, err, types.ErrLockNotFound)
}

func TestGenesisRoundTrip(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, beneficiaryStr := f.env.Addr(t)
	_, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "400000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1200, EndTime: 2000, ReleaseIntervals: 4, Revocable: true,
	})
	require.NoError(t, err)

	_, spenderStr := f.env.Addr(t)
	_, err = f.ms.AssignRole(f.ctx, &types.MsgAssignRole{
		Admin: adminStr, TreasuryId: id, Address: spenderStr, Role: types.Role_ROLE_SPENDER,
	})
	require.NoError(t, err)

	_, err = f.ms.SetSpendPolicy(f.ctx, &types.MsgSetSpendPolicy{
		Admin:  adminStr,
		Policy: types.SpendPolicy{TreasuryId: id, Denom: denom, PeriodLimit: "5000", PeriodSeconds: 600},
	})
	require.NoError(t, err)

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate(), "an exported genesis must be valid input")

	// Re-import into a clean chain and confirm the ledger survived.
	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))

	require.Equal(t, math.NewInt(400_000), g.locked(t, id))
	require.Equal(t, math.NewInt(600_000), g.available(t, id))

	reexported, err := g.keeper.ExportGenesis(g.ctx)
	require.NoError(t, err)
	require.Equal(t, exported.TreasuryList, reexported.TreasuryList)
	require.Equal(t, exported.LockList, reexported.LockList)
	require.Equal(t, exported.BalanceList, reexported.BalanceList)
	require.Equal(t, exported.RoleList, reexported.RoleList)
	require.Equal(t, exported.SpendPolicyList, reexported.SpendPolicyList)
	require.Equal(t, exported.TreasuryCount, reexported.TreasuryCount)
	require.Equal(t, exported.LockCount, reexported.LockCount)
}
