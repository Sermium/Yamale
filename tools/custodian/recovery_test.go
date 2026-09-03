package main

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/mpc"
)

// Recovery is the path these systems are actually robbed through, so these
// tests are written as attacks rather than as feature checks. Each one is a way
// somebody would try to take an account, and the assertion is that they cannot.

type fakeNotifier struct {
	started   int
	completed int
	fail      error
}

func (f *fakeNotifier) RecoveryStarted(Account, time.Duration) error {
	if f.fail != nil {
		return f.fail
	}
	f.started++
	return nil
}

func (f *fakeNotifier) RecoveryCompleted(Account, time.Time) error {
	f.completed++
	return nil
}

func (f *fakeNotifier) Problem(string, error) {}

// testRecoveries returns a log whose clock the test controls, because
// otherwise every case here would take three days.
func testRecoveries(t *testing.T) (*Recoveries, *time.Time) {
	t.Helper()
	rs, err := NewRecoveries(t.TempDir())
	require.NoError(t, err)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock := &now
	rs.now = func() time.Time { return *clock }
	return rs, clock
}

func noNotice() error { return nil }

// TestOneOperatorCannotRecoverAnAccountAlone is the attack: a single member of
// staff, or a single deceived member of staff, taking an account.
func TestOneOperatorCannotRecoverAnAccountAlone(t *testing.T) {
	rs, clock := testRecoveries(t)

	rec, err := rs.Initiate("idx-1", "amina", "support", "showed a prior device", noNotice)
	require.NoError(t, err)

	// The initiator may not approve. One person who both starts and approves is
	// one person to deceive.
	_, err = rs.Approve(rec.ID, "amina", "support")
	require.ErrorIs(t, err, ErrSameOperator)

	// Nor may a colleague on the same team: two approvals from one team are two
	// people with one manager and one set of pressures.
	_, err = rs.Approve(rec.ID, "boubacar", "support")
	require.ErrorIs(t, err, ErrSameTeam)

	// A second team can approve, and one approval is still not enough.
	_, err = rs.Approve(rec.ID, "boubacar", "risk")
	require.NoError(t, err)
	*clock = clock.Add(RecoveryDelay + time.Hour)
	_, err = rs.Complete(rec.ID)
	require.Error(t, err, "one approval completed a recovery")
	require.Contains(t, err.Error(), "approvals from different teams")

	// And the same person cannot approve twice to make up the number.
	_, err = rs.Approve(rec.ID, "boubacar", "compliance")
	require.ErrorIs(t, err, ErrAlreadyApproved)

	_, err = rs.Approve(rec.ID, "chinwe", "compliance")
	require.NoError(t, err)
	done, err := rs.Complete(rec.ID)
	require.NoError(t, err)
	require.Equal(t, RecoveryCompleted, done.State)
}

// TestRecoveryCannotBeRushed: the 72 hours are the attacker's real cost.
func TestRecoveryCannotBeRushed(t *testing.T) {
	rs, clock := testRecoveries(t)

	rec, err := rs.Initiate("idx-1", "amina", "support", "in person at an agent", noNotice)
	require.NoError(t, err)
	_, err = rs.Approve(rec.ID, "boubacar", "risk")
	require.NoError(t, err)
	_, err = rs.Approve(rec.ID, "chinwe", "compliance")
	require.NoError(t, err)

	// Fully approved, and still refused.
	_, err = rs.Complete(rec.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "notice period has not passed")

	// One hour short is still short.
	*clock = clock.Add(RecoveryDelay - time.Hour)
	_, err = rs.Complete(rec.ID)
	require.Error(t, err)

	*clock = clock.Add(2 * time.Hour)
	_, err = rs.Complete(rec.ID)
	require.NoError(t, err)
}

// TestTheOwnerCanCancel is the other half of the delay. Notice is worthless if
// the person who receives it cannot act on it.
func TestTheOwnerCanCancel(t *testing.T) {
	rs, clock := testRecoveries(t)

	rec, err := rs.Initiate("idx-1", "amina", "support", "answered security questions", noNotice)
	require.NoError(t, err)
	_, err = rs.Approve(rec.ID, "boubacar", "risk")
	require.NoError(t, err)
	_, err = rs.Approve(rec.ID, "chinwe", "compliance")
	require.NoError(t, err)

	cancelled, err := rs.Cancel(rec.ID, "the account holder", "I did not ask for this")
	require.NoError(t, err)
	require.Equal(t, RecoveryCancelled, cancelled.State)

	// And a cancelled recovery stays cancelled: the delay passing does not
	// revive it, and no further approval resurrects it.
	*clock = clock.Add(RecoveryDelay + time.Hour)
	_, err = rs.Complete(rec.ID)
	require.Error(t, err)
	_, err = rs.Approve(rec.ID, "dalia", "operations")
	require.ErrorIs(t, err, ErrRecoveryClosed)
}

// TestRecoveryWithoutNoticeDoesNotStart. The delay protects an account only if
// its owner was told the clock started, so notice failing must abort — not be
// logged and shrugged off.
func TestRecoveryWithoutNoticeDoesNotStart(t *testing.T) {
	rs, _ := testRecoveries(t)

	_, err := rs.Initiate("idx-1", "amina", "support", "a prior device", func() error {
		return errors.New("the mail server refused")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no recovery was started")

	// Nothing was written, so the account has no open recovery at all.
	open, err := rs.OpenFor("idx-1")
	require.NoError(t, err)
	require.Nil(t, open, "a recovery was recorded despite notice failing")
}

// TestTheDefaultNotifierRefuses. A service with no way to reach people must not
// run the recovery process correctly and silently, which is the most dangerous
// configuration this code can be in.
func TestTheDefaultNotifierRefuses(t *testing.T) {
	rs, _ := testRecoveries(t)
	var n Notifier = refusingNotifier{}

	_, err := rs.Initiate("idx-1", "amina", "support", "a prior device", func() error {
		return n.RecoveryStarted(Account{}, RecoveryDelay)
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoNotifier)
}

// TestProofIsRequired. The service cannot check the proof; what it can do is
// refuse to proceed without one and make it something a named person recorded.
func TestProofIsRequired(t *testing.T) {
	rs, _ := testRecoveries(t)
	_, err := rs.Initiate("idx-1", "amina", "support", "   ", noNotice)
	require.ErrorIs(t, err, ErrProofRequired)

	_, err = rs.Initiate("idx-1", "", "support", "a prior device", noNotice)
	require.ErrorIs(t, err, ErrOperatorRequired)
	_, err = rs.Initiate("idx-1", "amina", "", "a prior device", noNotice)
	require.ErrorIs(t, err, ErrOperatorRequired)
}

// TestOnlyOneRecoveryPerAccount. Two open recoveries are two independent
// clocks, and an attacker who can open a second has a way to work around the
// first.
func TestOnlyOneRecoveryPerAccount(t *testing.T) {
	rs, _ := testRecoveries(t)
	_, err := rs.Initiate("idx-1", "amina", "support", "a prior device", noNotice)
	require.NoError(t, err)
	_, err = rs.Initiate("idx-1", "dalia", "operations", "a different story", noNotice)
	require.ErrorIs(t, err, ErrRecoveryOpen)

	// A different account is unaffected.
	_, err = rs.Initiate("idx-2", "dalia", "operations", "a prior device", noNotice)
	require.NoError(t, err)
}

// TestPaymentsAreHeldAfterARecovery is the rule that makes a successful attack
// still not pay: recovery restores access, not immediate spending.
func TestPaymentsAreHeldAfterARecovery(t *testing.T) {
	rs, clock := testRecoveries(t)

	rec, err := rs.Initiate("idx-1", "amina", "support", "in person", noNotice)
	require.NoError(t, err)
	_, err = rs.Approve(rec.ID, "boubacar", "risk")
	require.NoError(t, err)
	_, err = rs.Approve(rec.ID, "chinwe", "compliance")
	require.NoError(t, err)
	*clock = clock.Add(RecoveryDelay + time.Minute)

	_, frozen := rs.FrozenUntil("idx-1")
	require.False(t, frozen, "an account was frozen before its recovery completed")

	_, err = rs.Complete(rec.ID)
	require.NoError(t, err)

	until, frozen := rs.FrozenUntil("idx-1")
	require.True(t, frozen, "an account was not held after a recovery")

	// Held for the whole window, released after it, and never affecting anybody
	// else's account.
	*clock = clock.Add(PostRecoveryFreeze - time.Minute)
	_, frozen = rs.FrozenUntil("idx-1")
	require.True(t, frozen, "the hold ended early")
	_, frozen = rs.FrozenUntil("idx-2")
	require.False(t, frozen, "an unrelated account was held")

	*clock = clock.Add(2 * time.Minute)
	_, stillFrozen := rs.FrozenUntil("idx-1")
	require.False(t, stillFrozen, "the hold outlasted its window")
	require.True(t, until.After(time.Time{}))
}

// TestTheLogSurvivesARestart. An audit trail that lives in memory is not one.
func TestTheLogSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	rs, err := NewRecoveries(dir)
	require.NoError(t, err)

	rec, err := rs.Initiate("idx-1", "amina", "support", "a prior device", noNotice)
	require.NoError(t, err)
	_, err = rs.Approve(rec.ID, "boubacar", "risk")
	require.NoError(t, err)

	reopened, err := NewRecoveries(dir)
	require.NoError(t, err)
	back, err := reopened.Get(rec.ID)
	require.NoError(t, err)
	require.Equal(t, rec.ID, back.ID)
	require.Len(t, back.Approvals, 1)
	require.Equal(t, "boubacar", back.Approvals[0].Operator)
	require.Equal(t, "amina", back.InitiatedBy)

	// And the exclusion rules still hold across the restart, which they would
	// not if they depended on anything held in memory.
	_, err = reopened.Approve(rec.ID, "amina", "support")
	require.ErrorIs(t, err, ErrSameOperator)
	_, err = reopened.Approve(rec.ID, "chidi", "risk")
	require.ErrorIs(t, err, ErrSameTeam)
}

// TestAnIDCannotNameAPath. The id reaches this code from a caller.
func TestAnIDCannotNameAPath(t *testing.T) {
	rs, _ := testRecoveries(t)
	for _, bad := range []string{"", "../../etc/passwd", "a/b", `a\b`, "x.json"} {
		_, err := rs.Get(bad)
		require.ErrorIs(t, err, ErrNoRecovery, "id %q was accepted", bad)
	}
}

// TestStatisticsSayHowManyAndNotWho, which is what the design asks be
// published: an unusual rate visible without exposing anybody.
func TestStatisticsSayHowManyAndNotWho(t *testing.T) {
	rs, clock := testRecoveries(t)

	complete := func(index string) {
		t.Helper()
		rec, err := rs.Initiate(index, "amina", "support", "in person", noNotice)
		require.NoError(t, err)
		_, err = rs.Approve(rec.ID, "boubacar", "risk")
		require.NoError(t, err)
		_, err = rs.Approve(rec.ID, "chinwe", "compliance")
		require.NoError(t, err)
		*clock = clock.Add(RecoveryDelay + time.Hour)
		_, err = rs.Complete(rec.ID)
		require.NoError(t, err)
	}
	complete("idx-1")
	complete("idx-2")

	open, err := rs.Initiate("idx-3", "amina", "support", "a prior device", noNotice)
	require.NoError(t, err)
	_, err = rs.Cancel(open.ID, "the account holder", "not me")
	require.NoError(t, err)

	stats := rs.Statistics()
	require.Equal(t, 2, stats.Completed)
	require.Equal(t, 1, stats.Cancelled)
	require.Equal(t, 2, stats.CompletedLast30Days)
	// Both took at least the mandatory delay, which is the number an operator
	// would check first if they suspected the delay was being bypassed.
	require.GreaterOrEqual(t, stats.MedianHours, RecoveryDelay.Hours())
}

// TestRecoveryDoesNotTouchTheShare. Completing a recovery must not re-key
// anything: the reshare happens with the customer present, and a service that
// generated new shares on behalf of somebody who was not there would be a
// service that can sign alone.
func TestRecoveryDoesNotTouchTheShare(t *testing.T) {
	store, _ := testStore(t)
	rs, clock := testRecoveries(t)

	// A distinguishable share id, so "unchanged" is a real assertion rather
	// than nil compared with nil.
	original := custodianShare(t)
	original.Data.ShareID = big.NewInt(0xC0FFEE)
	require.NoError(t, store.Put(Account{Index: "idx-1"}, original))

	rec, err := rs.Initiate("idx-1", "amina", "support", "in person", noNotice)
	require.NoError(t, err)
	_, err = rs.Approve(rec.ID, "boubacar", "risk")
	require.NoError(t, err)
	_, err = rs.Approve(rec.ID, "chinwe", "compliance")
	require.NoError(t, err)
	*clock = clock.Add(RecoveryDelay + time.Hour)
	_, err = rs.Complete(rec.ID)
	require.NoError(t, err)

	acct, err := store.Get("idx-1")
	require.NoError(t, err)
	after, err := store.Share(acct)
	require.NoError(t, err)
	require.Equal(t, mpc.RoleCustodian, after.Role)
	require.NotNil(t, after.Data.ShareID)
	require.Zero(t, original.Data.ShareID.Cmp(after.Data.ShareID),
		"completing a recovery changed the custodian's share")
}
