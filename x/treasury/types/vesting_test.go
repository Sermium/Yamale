package types_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/treasury/types"
)

// A schedule running from t=1000 to t=2000, so elapsed time and percentages
// are easy to read off.
const (
	start = int64(1000)
	end   = int64(2000)
)

func vestingLock(cliff int64, intervals uint64, total string) types.Lock {
	return types.Lock{
		TotalAmount:      total,
		ReleasedAmount:   "0",
		StartTime:        start,
		CliffTime:        cliff,
		EndTime:          end,
		ReleaseIntervals: intervals,
		LockType:         types.LockType_LOCK_TYPE_VESTING,
	}
}

func TestVestedAmountContinuous(t *testing.T) {
	lock := vestingLock(start, 0, "1000")

	testCases := []struct {
		name string
		now  int64
		want int64
	}{
		{name: "before start", now: 999, want: 0},
		{name: "at start", now: 1000, want: 0},
		{name: "ten percent", now: 1100, want: 100},
		{name: "half way", now: 1500, want: 500},
		{name: "ninety percent", now: 1900, want: 900},
		{name: "at end", now: 2000, want: 1000},
		{name: "long after end", now: 999999, want: 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, math.NewInt(tc.want), types.VestedAmount(lock, tc.now))
		})
	}
}

// The cliff gates claiming but does not restart the clock: once it passes, the
// whole amount accrued since start becomes available at once. This is what
// people expect from a one-year cliff on a four-year grant.
func TestVestedAmountCliffReleasesAccruedAmount(t *testing.T) {
	lock := vestingLock(1250, 0, "1000") // 25% of the way in

	require.True(t, types.VestedAmount(lock, 1249).IsZero(), "nothing may vest before the cliff")
	require.Equal(t, math.NewInt(250), types.VestedAmount(lock, 1250),
		"the cliff should release everything accrued up to it, not restart the schedule")
	require.Equal(t, math.NewInt(500), types.VestedAmount(lock, 1500))
}

// Discrete intervals release in whole tranches only.
func TestVestedAmountDiscreteIntervals(t *testing.T) {
	lock := vestingLock(start, 4, "1000") // quarterly

	testCases := []struct {
		name string
		now  int64
		want int64
	}{
		{name: "before first tranche", now: 1249, want: 0},
		{name: "first tranche", now: 1250, want: 250},
		{name: "still first tranche", now: 1499, want: 250},
		{name: "second tranche", now: 1500, want: 500},
		{name: "third tranche", now: 1750, want: 750},
		{name: "just before the last", now: 1999, want: 750},
		{name: "at end", now: 2000, want: 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, math.NewInt(tc.want), types.VestedAmount(lock, tc.now))
		})
	}
}

// A time lock is all-or-nothing: nothing before the end, everything after.
func TestVestedAmountTimeLock(t *testing.T) {
	lock := types.Lock{
		TotalAmount:    "1000",
		ReleasedAmount: "0",
		StartTime:      start,
		CliffTime:      start,
		EndTime:        end,
		LockType:       types.LockType_LOCK_TYPE_TIME,
	}

	require.True(t, types.VestedAmount(lock, 1500).IsZero(), "a time lock must not vest gradually")
	require.True(t, types.VestedAmount(lock, 1999).IsZero())
	require.Equal(t, math.NewInt(1000), types.VestedAmount(lock, 2000))
}

// Vesting must never exceed the total, and must never go backwards as time
// advances — a beneficiary's entitlement can only grow.
func TestVestedAmountIsMonotonicAndBounded(t *testing.T) {
	for _, intervals := range []uint64{0, 1, 3, 7, 100} {
		for _, cliff := range []int64{start, 1250, 1999} {
			lock := vestingLock(cliff, intervals, "999983") // awkward total, so truncation bites

			previous := math.ZeroInt()
			for now := start - 10; now <= end+10; now++ {
				vested := types.VestedAmount(lock, now)

				require.False(t, vested.IsNegative(), "vested went negative at t=%d", now)
				require.True(t, vested.LTE(math.NewInt(999983)),
					"vested %s exceeds the total at t=%d (intervals=%d, cliff=%d)", vested, now, intervals, cliff)
				require.True(t, vested.GTE(previous),
					"vested went backwards at t=%d: %s then %s", now, previous, vested)

				previous = vested
			}

			require.Equal(t, math.NewInt(999983), previous,
				"the full amount must vest by the end (intervals=%d, cliff=%d)", intervals, cliff)
		}
	}
}

// Rounding must favour the treasury: a beneficiary can never be credited more
// than the elapsed fraction of the schedule.
func TestVestedAmountRoundsTowardTheTreasury(t *testing.T) {
	lock := vestingLock(start, 0, "7") // 7 units over 1000 seconds

	// At 50%, exactly 3.5 units have accrued; the beneficiary gets 3, not 4.
	require.Equal(t, math.NewInt(3), types.VestedAmount(lock, 1500))
	// At 99.9%, 6.993 units; still 6.
	require.Equal(t, math.NewInt(6), types.VestedAmount(lock, 1999))
	// The remainder is paid in full at the end, so nothing is lost overall.
	require.Equal(t, math.NewInt(7), types.VestedAmount(lock, 2000))
}

func TestClaimableAmountSubtractsWhatWasAlreadyTaken(t *testing.T) {
	lock := vestingLock(start, 0, "1000")
	lock.ReleasedAmount = "300"

	require.Equal(t, math.NewInt(200), types.ClaimableAmount(lock, 1500))

	// Claiming cannot go negative if more was somehow released than has vested.
	lock.ReleasedAmount = "900"
	require.True(t, types.ClaimableAmount(lock, 1500).IsZero())
}

func TestValidateSchedule(t *testing.T) {
	const minSeconds = 60

	testCases := []struct {
		name    string
		start   int64
		cliff   int64
		end     int64
		ivals   uint64
		wantErr bool
	}{
		{name: "valid, no cliff", start: 1000, cliff: 1000, end: 2000},
		{name: "valid, with cliff", start: 1000, cliff: 1500, end: 2000},
		{name: "valid, with intervals", start: 1000, cliff: 1000, end: 2000, ivals: 4},
		{name: "end before start", start: 2000, cliff: 2000, end: 1000, wantErr: true},
		{name: "end equals start", start: 1000, cliff: 1000, end: 1000, wantErr: true},
		{name: "cliff before start", start: 1000, cliff: 900, end: 2000, wantErr: true},
		{name: "cliff after end", start: 1000, cliff: 2500, end: 2000, wantErr: true},
		{name: "shorter than the minimum", start: 1000, cliff: 1000, end: 1030, wantErr: true},
		{name: "zero timestamps", start: 0, cliff: 0, end: 0, wantErr: true},
		{name: "more intervals than seconds", start: 1000, cliff: 1000, end: 2000, ivals: 5000, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := types.ValidateSchedule(types.LockType_LOCK_TYPE_VESTING, tc.start, tc.cliff, tc.end, tc.ivals, minSeconds)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	t.Run("unspecified lock type is rejected", func(t *testing.T) {
		err := types.ValidateSchedule(types.LockType_LOCK_TYPE_UNSPECIFIED, 1000, 1000, 2000, 0, minSeconds)
		require.ErrorIs(t, err, types.ErrInvalidSchedule)
	})
}
