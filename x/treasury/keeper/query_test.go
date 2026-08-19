package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"yamale/blockchain/x/treasury/keeper"
	"yamale/blockchain/x/treasury/types"
)

// The queries are the module's entire public surface: a beneficiary checking
// what they are owed, an auditor verifying a disbursement, a dashboard drawing
// a balance. A query that reports the wrong number is as damaging as a handler
// that moves the wrong amount, because people act on both.

func newQueryFixture(t *testing.T) (*fixture, types.QueryServer) {
	t.Helper()
	f := initFixture(t)
	return f, keeper.NewQueryServerImpl(f.keeper)
}

func TestQueryParams(t *testing.T) {
	f, q := newQueryFixture(t)

	resp, err := q.Params(f.ctx, &types.QueryParamsRequest{})
	require.NoError(t, err)
	require.Equal(t, types.DefaultParams(), resp.Params)

	_, err = q.Params(f.ctx, nil)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestQueryTreasury(t *testing.T) {
	f, q := newQueryFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	resp, err := q.GetTreasury(f.ctx, &types.QueryGetTreasuryRequest{Id: id})
	require.NoError(t, err)
	require.Equal(t, id, resp.Treasury.Id)
	require.Equal(t, adminStr, resp.Treasury.Admin)

	_, err = q.GetTreasury(f.ctx, &types.QueryGetTreasuryRequest{Id: 999})
	require.Equal(t, codes.NotFound, status.Code(err))

	list, err := q.ListTreasury(f.ctx, &types.QueryAllTreasuryRequest{})
	require.NoError(t, err)
	require.Len(t, list.Treasury, 1)
}

// The balance split is the number every treasury UI leads with, so it has to
// stay correct as funds are committed and released.
func TestQueryTreasuryBalances(t *testing.T) {
	f, q := newQueryFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	resp, err := q.TreasuryBalances(f.ctx, &types.QueryTreasuryBalancesRequest{TreasuryId: id})
	require.NoError(t, err)
	require.Len(t, resp.Balances, 1)
	require.Equal(t, denom, resp.Balances[0].Denom)
	require.Equal(t, "1000000", resp.Balances[0].Total)
	require.Equal(t, "0", resp.Balances[0].Locked)
	require.Equal(t, "1000000", resp.Balances[0].Available)

	_, beneficiaryStr := f.env.Addr(t)
	_, err = f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "400000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000,
	})
	require.NoError(t, err)

	resp, err = q.TreasuryBalances(f.ctx, &types.QueryTreasuryBalancesRequest{TreasuryId: id})
	require.NoError(t, err)
	require.Equal(t, "1000000", resp.Balances[0].Total, "committing funds does not remove them from the treasury")
	require.Equal(t, "400000", resp.Balances[0].Locked)
	require.Equal(t, "600000", resp.Balances[0].Available)

	// An unknown treasury reports nothing rather than erroring, so a client can
	// render an empty state without special-casing.
	empty, err := q.TreasuryBalances(f.ctx, &types.QueryTreasuryBalancesRequest{TreasuryId: 999})
	require.NoError(t, err)
	require.Empty(t, empty.Balances)
}

func TestQueryLocks(t *testing.T) {
	f, q := newQueryFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, beneficiaryStr := f.env.Addr(t)
	_, otherStr := f.env.Addr(t)

	first, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "100000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000,
	})
	require.NoError(t, err)

	_, err = f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: otherStr, Denom: denom,
		Amount: "200000", LockType: types.LockType_LOCK_TYPE_TIME,
		StartTime: 1000, CliffTime: 1000, EndTime: 3000,
	})
	require.NoError(t, err)

	one, err := q.GetLock(f.ctx, &types.QueryGetLockRequest{Id: first.Id})
	require.NoError(t, err)
	require.Equal(t, beneficiaryStr, one.Lock.Beneficiary)

	_, err = q.GetLock(f.ctx, &types.QueryGetLockRequest{Id: 999})
	require.Equal(t, codes.NotFound, status.Code(err))

	all, err := q.ListLock(f.ctx, &types.QueryAllLockRequest{})
	require.NoError(t, err)
	require.Len(t, all.Lock, 2)

	byTreasury, err := q.LocksByTreasury(f.ctx, &types.QueryLocksByTreasuryRequest{TreasuryId: id})
	require.NoError(t, err)
	require.Len(t, byTreasury.Lock, 2)

	// A beneficiary can find what is owed to them without knowing a treasury id.
	mine, err := q.LocksByBeneficiary(f.ctx, &types.QueryLocksByBeneficiaryRequest{Beneficiary: beneficiaryStr})
	require.NoError(t, err)
	require.Len(t, mine.Lock, 1)
	require.Equal(t, first.Id, mine.Lock[0].Id)

	// And sees nothing that is not theirs.
	_, strangerStr := f.env.Addr(t)
	none, err := q.LocksByBeneficiary(f.ctx, &types.QueryLocksByBeneficiaryRequest{Beneficiary: strangerStr})
	require.NoError(t, err)
	require.Empty(t, none.Lock)
}

func TestQueryClaimableAmount(t *testing.T) {
	f, q := newQueryFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, beneficiaryStr := f.env.Addr(t)
	lock, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
		Amount: "1000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000,
	})
	require.NoError(t, err)

	resp, err := q.ClaimableAmount(f.ctx, &types.QueryClaimableAmountRequest{LockId: lock.Id})
	require.NoError(t, err)
	require.Equal(t, "0", resp.Claimable)
	require.Equal(t, "1000", resp.Remaining)

	f.at(1500)
	resp, err = q.ClaimableAmount(f.ctx, &types.QueryClaimableAmountRequest{LockId: lock.Id})
	require.NoError(t, err)
	require.Equal(t, "500", resp.Claimable)
	require.Equal(t, "500", resp.Vested)
	require.Equal(t, "1000", resp.Remaining, "remaining counts what is unclaimed, not what is unvested")

	// After claiming, what was taken is no longer claimable or remaining.
	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: lock.Id})
	require.NoError(t, err)

	resp, err = q.ClaimableAmount(f.ctx, &types.QueryClaimableAmountRequest{LockId: lock.Id})
	require.NoError(t, err)
	require.Equal(t, "0", resp.Claimable)
	require.Equal(t, "500", resp.Vested)
	require.Equal(t, "500", resp.Remaining)

	_, err = q.ClaimableAmount(f.ctx, &types.QueryClaimableAmountRequest{LockId: 999})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestQueryRolesAndPolicy(t *testing.T) {
	f, q := newQueryFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	_, spenderStr := f.env.Addr(t)
	_, err := f.ms.AssignRole(f.ctx, &types.MsgAssignRole{
		Admin: adminStr, TreasuryId: id, Address: spenderStr, Role: types.Role_ROLE_SPENDER,
	})
	require.NoError(t, err)

	roles, err := q.ListRole(f.ctx, &types.QueryListRoleRequest{TreasuryId: id})
	require.NoError(t, err)
	require.Len(t, roles.Role, 1)
	require.Equal(t, types.Role_ROLE_SPENDER, roles.Role[0].Role)

	// No policy set yet.
	_, err = q.GetSpendPolicy(f.ctx, &types.QueryGetSpendPolicyRequest{TreasuryId: id, Denom: denom})
	require.Equal(t, codes.NotFound, status.Code(err))

	_, err = f.ms.SetSpendPolicy(f.ctx, &types.MsgSetSpendPolicy{
		Admin: adminStr,
		Policy: types.SpendPolicy{
			TreasuryId: id, Denom: denom,
			PerTransactionLimit: "1000", PeriodLimit: "5000", PeriodSeconds: 3600,
		},
	})
	require.NoError(t, err)

	policy, err := q.GetSpendPolicy(f.ctx, &types.QueryGetSpendPolicyRequest{TreasuryId: id, Denom: denom})
	require.NoError(t, err)
	require.Equal(t, "1000", policy.Policy.PerTransactionLimit)
	require.Equal(t, uint64(3600), policy.Policy.PeriodSeconds)
}

// SpendCapacity has to report the tighter of the two bounds, or a client would
// show an allowance the chain will refuse.
func TestQuerySpendCapacity(t *testing.T) {
	f, q := newQueryFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	// With no policy, capacity is simply what is available.
	resp, err := q.SpendCapacity(f.ctx, &types.QuerySpendCapacityRequest{TreasuryId: id, Denom: denom})
	require.NoError(t, err)
	require.Equal(t, "1000000", resp.Available)
	require.Equal(t, "1000000", resp.RemainingThisPeriod)

	_, err = f.ms.SetSpendPolicy(f.ctx, &types.MsgSetSpendPolicy{
		Admin: adminStr,
		Policy: types.SpendPolicy{
			TreasuryId: id, Denom: denom,
			PerTransactionLimit: "400", PeriodLimit: "1000", PeriodSeconds: 3600,
		},
	})
	require.NoError(t, err)

	resp, err = q.SpendCapacity(f.ctx, &types.QuerySpendCapacityRequest{TreasuryId: id, Denom: denom})
	require.NoError(t, err)
	require.Equal(t, "1000", resp.RemainingThisPeriod, "the period limit binds before the balance does")
	require.Equal(t, "400", resp.PerTransactionLimit)

	// Spending consumes the period allowance.
	_, recipientStr := f.env.Addr(t)
	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: id, Recipient: recipientStr, Amount: coins(400),
	})
	require.NoError(t, err)

	resp, err = q.SpendCapacity(f.ctx, &types.QuerySpendCapacityRequest{TreasuryId: id, Denom: denom})
	require.NoError(t, err)
	require.Equal(t, "600", resp.RemainingThisPeriod)
	require.Equal(t, int64(1000+3600), resp.PeriodResetsAt)

	// Once the window rolls over, the full allowance is back.
	f.at(1000 + 3600)
	resp, err = q.SpendCapacity(f.ctx, &types.QuerySpendCapacityRequest{TreasuryId: id, Denom: denom})
	require.NoError(t, err)
	require.Equal(t, "1000", resp.RemainingThisPeriod)
}

// When the treasury holds less than the period allowance, the balance is the
// binding constraint and must be what is reported.
func TestQuerySpendCapacityBoundedByBalance(t *testing.T) {
	f, q := newQueryFixture(t)
	id, _, adminStr := f.newTreasury(t, 500)

	_, err := f.ms.SetSpendPolicy(f.ctx, &types.MsgSetSpendPolicy{
		Admin: adminStr,
		Policy: types.SpendPolicy{
			TreasuryId: id, Denom: denom, PeriodLimit: "1000000", PeriodSeconds: 3600,
		},
	})
	require.NoError(t, err)

	resp, err := q.SpendCapacity(f.ctx, &types.QuerySpendCapacityRequest{TreasuryId: id, Denom: denom})
	require.NoError(t, err)
	require.Equal(t, "500", resp.Available)
	require.Equal(t, "500", resp.RemainingThisPeriod)
}

func TestQueryRejectsNilRequests(t *testing.T) {
	f, q := newQueryFixture(t)

	calls := map[string]func() error{
		"GetTreasury":        func() error { _, err := q.GetTreasury(f.ctx, nil); return err },
		"ListTreasury":       func() error { _, err := q.ListTreasury(f.ctx, nil); return err },
		"TreasuryBalances":   func() error { _, err := q.TreasuryBalances(f.ctx, nil); return err },
		"GetLock":            func() error { _, err := q.GetLock(f.ctx, nil); return err },
		"ListLock":           func() error { _, err := q.ListLock(f.ctx, nil); return err },
		"LocksByTreasury":    func() error { _, err := q.LocksByTreasury(f.ctx, nil); return err },
		"LocksByBeneficiary": func() error { _, err := q.LocksByBeneficiary(f.ctx, nil); return err },
		"ClaimableAmount":    func() error { _, err := q.ClaimableAmount(f.ctx, nil); return err },
		"ListRole":           func() error { _, err := q.ListRole(f.ctx, nil); return err },
		"GetSpendPolicy":     func() error { _, err := q.GetSpendPolicy(f.ctx, nil); return err },
		"SpendCapacity":      func() error { _, err := q.SpendCapacity(f.ctx, nil); return err },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, codes.InvalidArgument, status.Code(call()))
		})
	}
}
