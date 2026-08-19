package keeper_test

import (
	"fmt"
	"math/rand"
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/treasury/types"
)

// The treasury's whole security argument rests on its ledger matching reality.
// If the module account holds less than the ledger says, some treasury's funds
// are really someone else's; if `locked` drifts from what the live locks
// actually commit, a beneficiary's schedule can be spent out from under them.
//
// Neither failure would announce itself. Both would surface as a claim or a
// spend that inexplicably fails, long after the transaction that caused it. So
// these properties are checked after every single operation of a randomized
// sequence rather than at the end.

// assertLedgerInvariants checks every accounting rule the treasury must satisfy.
func assertLedgerInvariants(t *testing.T, f *fixture, treasuryIDs []uint64) {
	t.Helper()

	// Rebuild what the locks actually commit, per treasury and denom.
	committed := map[uint64]map[string]math.Int{}
	require.NoError(t, f.keeper.Lock.Walk(f.ctx, nil, func(_ uint64, l types.Lock) (bool, error) {
		if !l.Active {
			return false, nil
		}
		if committed[l.TreasuryId] == nil {
			committed[l.TreasuryId] = map[string]math.Int{}
		}
		prev, ok := committed[l.TreasuryId][l.Denom]
		if !ok {
			prev = math.ZeroInt()
		}
		committed[l.TreasuryId][l.Denom] = prev.Add(types.RemainingAmount(l))
		return false, nil
	}))

	ledgerTotal := math.ZeroInt()

	for _, id := range treasuryIDs {
		bal, err := f.keeper.Balance.Get(f.ctx, collections.Join(id, denom))
		total, locked := math.ZeroInt(), math.ZeroInt()
		if err == nil {
			total, _ = math.NewIntFromString(bal.Total)
			locked, _ = math.NewIntFromString(bal.Locked)
		}

		// Property 1: a treasury can never have committed more than it holds.
		// Violating this means it has promised funds that do not exist.
		require.True(t, locked.LTE(total),
			"treasury %d has %s locked but only %s held", id, locked, total)

		// Property 2: the locked figure equals what the live locks commit.
		// Drift here means either a spend could reach committed funds, or funds
		// are stranded that nobody can ever release.
		want := math.ZeroInt()
		if m, ok := committed[id]; ok {
			if v, ok := m[denom]; ok {
				want = v
			}
		}
		require.Equal(t, want.String(), locked.String(),
			"treasury %d's locked balance disagrees with its active locks", id)

		ledgerTotal = ledgerTotal.Add(total)
	}

	// Property 3: the module account holds exactly what the ledger claims across
	// all treasuries — no more (funds nobody can withdraw) and no less (funds
	// the module cannot honour).
	require.Equal(t, ledgerTotal.String(), f.env.ModuleBalance(types.ModuleName, denom).String(),
		"the treasury module account does not match the sum of its ledger")
}

func TestTreasuryLedgerPropertiesUnderRandomOperations(t *testing.T) {
	const (
		numTreasuries = 3
		numOperations = 150
	)

	for _, seed := range []int64{1, 7, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			r := rand.New(rand.NewSource(seed))
			f := initFixture(t)

			treasuryIDs := make([]uint64, numTreasuries)
			admins := make([]string, numTreasuries)
			for i := range treasuryIDs {
				id, _, adminStr := f.newTreasury(t, 1_000_000)
				treasuryIDs[i] = id
				admins[i] = adminStr
			}

			// A pool of beneficiaries, so locks and claims interleave across them.
			beneficiaries := make([]string, 4)
			for i := range beneficiaries {
				_, beneficiaries[i] = f.env.Addr(t)
			}

			var lockIDs []uint64
			now := int64(1000)

			assertLedgerInvariants(t, f, treasuryIDs)

			for op := 0; op < numOperations; op++ {
				i := r.Intn(numTreasuries)
				id, admin := treasuryIDs[i], admins[i]

				switch r.Intn(6) {
				case 0: // deposit
					_, depositorStr := f.env.NewFundedAddr(t, coins(int64(r.Intn(50_000)+1)))
					amount := int64(r.Intn(1000) + 1)
					_, _ = f.ms.Deposit(f.ctx, &types.MsgDeposit{
						Depositor: depositorStr, TreasuryId: id, Amount: coins(amount),
					})

				case 1: // spend
					_, recipientStr := f.env.Addr(t)
					_, _ = f.ms.Spend(f.ctx, &types.MsgSpend{
						Spender: admin, TreasuryId: id, Recipient: recipientStr,
						Amount: coins(int64(r.Intn(200_000) + 1)),
					})

				case 2: // create a lock
					start := now
					end := start + int64(r.Intn(2000)+100)
					cliff := start + int64(r.Intn(int(end-start)))
					resp, err := f.ms.CreateLock(f.ctx, &types.MsgCreateLock{
						Admin: admin, TreasuryId: id, Beneficiary: beneficiaries[r.Intn(len(beneficiaries))],
						Denom: denom, Amount: fmt.Sprintf("%d", r.Intn(300_000)+1),
						LockType:         lockKind(r),
						StartTime:        start,
						CliffTime:        cliff,
						EndTime:          end,
						ReleaseIntervals: uint64(r.Intn(5)),
						Revocable:        r.Intn(2) == 0,
					})
					if err == nil {
						lockIDs = append(lockIDs, resp.Id)
					}

				case 3: // claim
					if len(lockIDs) == 0 {
						break
					}
					lockID := lockIDs[r.Intn(len(lockIDs))]
					lock, err := f.keeper.Lock.Get(f.ctx, lockID)
					if err != nil {
						break
					}
					_, _ = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{
						Beneficiary: lock.Beneficiary, LockId: lockID,
					})

				case 4: // revoke
					if len(lockIDs) == 0 {
						break
					}
					lockID := lockIDs[r.Intn(len(lockIDs))]
					lock, err := f.keeper.Lock.Get(f.ctx, lockID)
					if err != nil {
						break
					}
					_, _ = f.ms.RevokeLock(f.ctx, &types.MsgRevokeLock{
						Admin: admins[indexOfTreasury(treasuryIDs, lock.TreasuryId)], LockId: lockID,
					})

				case 5: // let time pass, so schedules advance
					now += int64(r.Intn(400))
					f.at(now)
				}

				assertLedgerInvariants(t, f, treasuryIDs)
			}

			// Wind the clock past every schedule and drain what is left, so the
			// end state is exercised too rather than only the middle.
			f.at(now + 100_000)
			for _, lockID := range lockIDs {
				lock, err := f.keeper.Lock.Get(f.ctx, lockID)
				if err != nil || !lock.Active {
					continue
				}
				_, _ = f.ms.ClaimLock(f.ctx, &types.MsgClaimLock{
					Beneficiary: lock.Beneficiary, LockId: lockID,
				})
				assertLedgerInvariants(t, f, treasuryIDs)
			}

			// Every lock is settled, so nothing should remain committed.
			for _, id := range treasuryIDs {
				require.True(t, f.locked(t, id).IsZero(),
					"treasury %d still has funds locked after every schedule completed", id)
			}
		})
	}
}

func lockKind(r *rand.Rand) types.LockType {
	if r.Intn(2) == 0 {
		return types.LockType_LOCK_TYPE_TIME
	}
	return types.LockType_LOCK_TYPE_VESTING
}

func indexOfTreasury(ids []uint64, want uint64) int {
	for i, id := range ids {
		if id == want {
			return i
		}
	}
	return 0
}
