package types

import (
	"cosmossdk.io/math"
)

// VestedAmount returns how much of a lock's total has unlocked by now, in Unix
// seconds. It is deliberately a pure function of the lock and the time: the
// same inputs give the same answer on every validator, and it can be tested
// exhaustively without a chain.
//
// Two rules govern every branch:
//
// The cliff is a gate, not a restart. Vesting accrues from start_time, but
// nothing is claimable until cliff_time passes — at which point the whole
// accrued amount becomes available at once. This is the model people expect
// from employment and grant schedules: a one-year cliff on a four-year vest
// releases a year's worth on the anniversary, not a fresh start.
//
// Rounding always favours the treasury. Every division truncates, so a
// beneficiary can never claim a fraction more than the schedule has actually
// earned them. Over a full schedule they still receive the exact total, because
// reaching end_time short-circuits to the full amount rather than accumulating
// rounding error.
func VestedAmount(lock Lock, now int64) math.Int {
	total, ok := math.NewIntFromString(lock.TotalAmount)
	if !ok || !total.IsPositive() {
		return math.ZeroInt()
	}

	// Past the end, everything has vested — stated exactly, with no rounding.
	if now >= lock.EndTime {
		return total
	}

	// Before the cliff nothing is claimable, whatever has accrued behind it.
	if now < lock.CliffTime {
		return math.ZeroInt()
	}

	switch lock.LockType {
	case LockType_LOCK_TYPE_TIME:
		// A time lock is all-or-nothing: it releases only at end_time, which
		// the check above already handled.
		return math.ZeroInt()

	case LockType_LOCK_TYPE_VESTING:
		duration := lock.EndTime - lock.StartTime
		if duration <= 0 {
			// A zero-length schedule that has not reached end_time cannot have
			// vested anything; guard the division regardless.
			return math.ZeroInt()
		}

		elapsed := now - lock.StartTime
		if elapsed <= 0 {
			return math.ZeroInt()
		}

		// Discrete tranches: only whole intervals release. With 4 intervals over
		// a year, three months and 29 days still pays one quarter, not almost
		// two.
		if lock.ReleaseIntervals > 1 {
			intervals := int64(lock.ReleaseIntervals)
			completed := (elapsed * intervals) / duration
			if completed <= 0 {
				return math.ZeroInt()
			}
			if completed >= intervals {
				return total
			}
			return total.MulRaw(completed).QuoRaw(intervals)
		}

		// Continuous: proportional to elapsed time.
		return total.MulRaw(elapsed).QuoRaw(duration)

	default:
		return math.ZeroInt()
	}
}

// ClaimableAmount returns what the beneficiary may withdraw right now: what has
// vested, less what they have already taken.
func ClaimableAmount(lock Lock, now int64) math.Int {
	released, ok := math.NewIntFromString(lock.ReleasedAmount)
	if !ok {
		released = math.ZeroInt()
	}

	claimable := VestedAmount(lock, now).Sub(released)
	if !claimable.IsPositive() {
		return math.ZeroInt()
	}
	return claimable
}

// RemainingAmount returns how much of the lock is still committed — the part
// that stays in the treasury's locked balance.
func RemainingAmount(lock Lock) math.Int {
	total, ok := math.NewIntFromString(lock.TotalAmount)
	if !ok {
		return math.ZeroInt()
	}
	released, ok := math.NewIntFromString(lock.ReleasedAmount)
	if !ok {
		released = math.ZeroInt()
	}

	remaining := total.Sub(released)
	if !remaining.IsPositive() {
		return math.ZeroInt()
	}
	return remaining
}

// ValidateSchedule checks a proposed lock's timings are coherent. Catching this
// at creation matters because a malformed schedule silently locks funds forever
// — there is no later point at which the chain could notice.
func ValidateSchedule(lockType LockType, start, cliff, end int64, intervals uint64, minSeconds uint64) error {
	switch lockType {
	case LockType_LOCK_TYPE_TIME, LockType_LOCK_TYPE_VESTING:
	default:
		return ErrInvalidSchedule.Wrap("lock type must be time or vesting")
	}

	if start <= 0 || end <= 0 || cliff <= 0 {
		return ErrInvalidSchedule.Wrap("start, cliff and end times must be positive Unix timestamps")
	}
	if end <= start {
		return ErrInvalidSchedule.Wrapf("end time %d must be after start time %d", end, start)
	}
	if cliff < start {
		return ErrInvalidSchedule.Wrapf("cliff time %d cannot precede start time %d", cliff, start)
	}
	if cliff > end {
		return ErrInvalidSchedule.Wrapf("cliff time %d cannot follow end time %d", cliff, end)
	}
	if uint64(end-start) < minSeconds {
		return ErrInvalidSchedule.Wrapf("lock duration %ds is shorter than the minimum %ds", end-start, minSeconds)
	}
	// More tranches than seconds would make some tranches unreachable.
	if intervals > uint64(end-start) {
		return ErrInvalidSchedule.Wrapf("release intervals %d exceed the schedule's %d seconds", intervals, end-start)
	}

	return nil
}
