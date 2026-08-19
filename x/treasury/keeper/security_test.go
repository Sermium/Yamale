package keeper_test

import (
	"errors"
	"testing"

	"cosmossdk.io/collections"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/treasury/keeper"
	"yamale/blockchain/x/treasury/types"
)

// Findings from the pre-genesis audit of this module.
//
// The blocked-beneficiary and pagination tests fail outright against the code
// as it stood before their fixes. The lock-count and policy-bound tests would
// not have compiled, because the state and parameter they assert on did not
// exist — they guard the fix from being undone rather than proving it was
// needed, and the reasoning for each is in the comment above it.

// A lock pays out through the bank on both the claim and the revoke path, so a
// beneficiary that cannot receive funds makes the commitment permanently
// unreachable — by the beneficiary, by the admin, and by governance. The
// treasury's own module account is the easiest way to hit this by accident.
func TestCreateLockRejectsUnreceivableBeneficiary(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	// The env blocks nothing by default, so block one address explicitly to
	// stand in for the chain's real blocklist.
	blocked := f.env.AuthKeeper.GetModuleAddress(types.ModuleName)
	blockedStr, err := f.env.AddressCodec.BytesToString(blocked)
	require.NoError(t, err)
	f.env.Block(blocked)

	_, err = f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
		Admin: adminStr, TreasuryId: id, Beneficiary: blockedStr, Denom: denom,
		Amount: "500000", LockType: types.LockType_LOCK_TYPE_VESTING,
		StartTime: 1000, CliffTime: 1000, EndTime: 2000,
	})
	require.ErrorIs(t, err, types.ErrDestinationDenied)

	// Nothing was committed, so the funds remain spendable.
	require.True(t, f.locked(t, id).IsZero())
	require.Equal(t, math.NewInt(1_000_000), f.available(t, id))
}

// Enforcing the lock cap by walking the treasury's index would grow more
// expensive with every lock ever created, because retired locks stay indexed
// for the audit trail. The count is tracked instead, and has to stay accurate
// across the full lifecycle.
func TestActiveLockCountTracksLifecycle(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 10_000_000)
	_, beneficiaryStr := f.env.Addr(t)

	createLock := func(amount string, revocable bool) uint64 {
		resp, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
			Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
			Amount: amount, LockType: types.LockType_LOCK_TYPE_VESTING,
			StartTime: 1000, CliffTime: 1000, EndTime: 2000, Revocable: revocable,
		})
		require.NoError(t, err)
		return resp.Id
	}

	count := func() uint64 {
		v, err := f.keeper.ActiveLockCount.Get(f.ctx, id)
		if errors.Is(err, collections.ErrNotFound) {
			return 0
		}
		require.NoError(t, err)
		return v
	}

	first := createLock("1000000", true)
	second := createLock("1000000", false)
	require.Equal(t, uint64(2), count())

	// Claiming a lock to completion retires it.
	f.at(2000)
	_, err := f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: second})
	require.NoError(t, err)
	require.Equal(t, uint64(1), count())

	// Revoking retires the other.
	_, err = f.ms.RevokeLock(f.ctx, &types.MsgRevokeLock{Admin: adminStr, LockId: first})
	require.NoError(t, err)
	require.Equal(t, uint64(0), count())

	// Reaching zero removes the entry rather than storing a zero, so that a
	// live chain's bytes match the same chain exported and re-imported —
	// InitGenesis writes nothing for a treasury with no active locks.
	has, err := f.keeper.ActiveLockCount.Has(f.ctx, id)
	require.NoError(t, err)
	require.False(t, has, "a zero count must be removed, not stored")

	// The index still holds both, which is exactly why the count cannot be
	// derived from it cheaply.
	locks, err := f.keeper.LockByTreasury.Iterate(f.ctx, nil)
	require.NoError(t, err)
	defer locks.Close()
	keys, err := locks.Keys()
	require.NoError(t, err)
	require.Len(t, keys, 2, "retired locks stay indexed for the audit trail")
}

// The cap has to bind on live locks only: a treasury that has cycled through
// many completed locks should still be able to create new ones.
func TestLockCapCountsOnlyActiveLocks(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 10_000_000)
	_, beneficiaryStr := f.env.Addr(t)

	params := types.DefaultParams()
	params.MaxLocksPerTreasury = 2
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	create := func() (uint64, error) {
		resp, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
			Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
			Amount: "100000", LockType: types.LockType_LOCK_TYPE_VESTING,
			StartTime: 1000, CliffTime: 1000, EndTime: 2000,
		})
		if err != nil {
			return 0, err
		}
		return resp.Id, nil
	}

	first, err := create()
	require.NoError(t, err)
	_, err = create()
	require.NoError(t, err)

	_, err = create()
	require.ErrorIs(t, err, types.ErrLimitReached, "the cap must bind at the limit")

	// Retire one and a slot frees up.
	f.at(2000)
	_, err = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{Beneficiary: beneficiaryStr, LockId: first})
	require.NoError(t, err)

	_, err = create()
	require.NoError(t, err, "a completed lock must not occupy a slot forever")
}

// Both policy lists are scanned on every spend and stored indefinitely, so an
// unbounded list is a way to make a treasury's own payments expensive and bloat
// state permanently.
func TestSpendPolicyAddressListIsBounded(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	params := types.DefaultParams()
	params.MaxSpendPolicyAddresses = 3
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	addrs := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		_, a := f.env.Addr(t)
		addrs = append(addrs, a)
	}

	_, err := f.ms.SetSpendPolicy(f.ctx, &types.MsgSetSpendPolicy{
		Admin:  adminStr,
		Policy: types.SpendPolicy{TreasuryId: id, Denom: denom, Allowlist: addrs},
	})
	require.ErrorIs(t, err, types.ErrLimitReached)

	// Within the bound it is accepted, counting both lists together.
	_, err = f.ms.SetSpendPolicy(f.ctx, &types.MsgSetSpendPolicy{
		Admin: adminStr,
		Policy: types.SpendPolicy{
			TreasuryId: id, Denom: denom,
			Allowlist: addrs[:2], Blocklist: addrs[2:3],
		},
	})
	require.NoError(t, err)
}

// Queries are gas-free, so a list endpoint that ignores its page request lets
// anyone make a node walk an unbounded collection for nothing.
func TestListQueriesRespectPagination(t *testing.T) {
	f := initFixture(t)
	q := keeper.NewQueryServerImpl(f.keeper)

	id, _, adminStr := f.newTreasury(t, 100_000_000)
	_, beneficiaryStr := f.env.Addr(t)

	const total = 12
	for i := 0; i < total; i++ {
		_, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
			Admin: adminStr, TreasuryId: id, Beneficiary: beneficiaryStr, Denom: denom,
			Amount: "100000", LockType: types.LockType_LOCK_TYPE_VESTING,
			StartTime: 1000, CliffTime: 1000, EndTime: 2000,
		})
		require.NoError(t, err, "lock %d", i)
	}

	page := &query.PageRequest{Limit: 5}

	all, err := q.ListLock(f.ctx, &types.QueryAllLockRequest{Pagination: page})
	require.NoError(t, err)
	require.Len(t, all.Lock, 5, "ListLock must honour the page limit")
	require.NotNil(t, all.Pagination)

	byTreasury, err := q.LocksByTreasury(f.ctx, &types.QueryLocksByTreasuryRequest{TreasuryId: id, Pagination: page})
	require.NoError(t, err)
	require.Len(t, byTreasury.Lock, 5, "LocksByTreasury must honour the page limit")

	byBeneficiary, err := q.LocksByBeneficiary(f.ctx, &types.QueryLocksByBeneficiaryRequest{
		Beneficiary: beneficiaryStr, Pagination: page,
	})
	require.NoError(t, err)
	require.Len(t, byBeneficiary.Lock, 5, "LocksByBeneficiary must honour the page limit")

	// Paging through reaches every record exactly once.
	seen := map[uint64]bool{}
	var next []byte
	for {
		resp, err := q.LocksByTreasury(f.ctx, &types.QueryLocksByTreasuryRequest{
			TreasuryId: id,
			Pagination: &query.PageRequest{Limit: 5, Key: next},
		})
		require.NoError(t, err)
		for _, l := range resp.Lock {
			require.False(t, seen[l.Id], "lock %d returned twice", l.Id)
			seen[l.Id] = true
		}
		if resp.Pagination == nil || len(resp.Pagination.NextKey) == 0 {
			break
		}
		next = resp.Pagination.NextKey
	}
	require.Len(t, seen, total)
}

// A treasury's own module account must never be a spend destination: the coins
// would leave the ledger and land straight back in the account holding it,
// leaving the module account permanently ahead of what it can account for.
func TestSpendRejectsUnreceivableRecipient(t *testing.T) {
	f := initFixture(t)
	id, _, adminStr := f.newTreasury(t, 1_000_000)

	blocked := f.env.AuthKeeper.GetModuleAddress(authtypes.FeeCollectorName)
	blockedStr, err := f.env.AddressCodec.BytesToString(blocked)
	require.NoError(t, err)
	f.env.Block(blocked)

	_, err = f.ms.Spend(f.ctx, &types.MsgSpend{
		Spender: adminStr, TreasuryId: id, Recipient: blockedStr, Amount: coins(1000),
	})
	require.ErrorIs(t, err, types.ErrDestinationDenied)
	require.Equal(t, math.NewInt(1_000_000), f.available(t, id))
}
