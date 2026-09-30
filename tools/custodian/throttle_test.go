package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The password check had nothing in front of it but nginx, which limits an
// ADDRESS: one request a minute, burst 20. An attacker with a thousand
// addresses gets a thousand guesses a minute at one account. That matters more
// here than on most sign-ins, because the password is also what seals the key -
// a password learned by guessing this endpoint is the one that opens the
// ciphertext if the ciphertext is ever obtained.

var throttleEpoch = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func TestTheFirstFewMistakesCostNothing(t *testing.T) {
	var th attemptThrottle
	for i := 0; i < throttleFree; i++ {
		require.Zero(t, th.wait("a", throttleEpoch), "attempt %d", i+1)
		th.failed("a", throttleEpoch)
	}
	require.Equal(t, throttleBase, th.wait("a", throttleEpoch),
		"the attempt after the free ones waits the base delay")
}

func TestTheWaitDoublesAndIsCapped(t *testing.T) {
	var th attemptThrottle
	now := throttleEpoch
	for i := 0; i < throttleFree; i++ {
		th.failed("a", now)
	}
	want := throttleBase
	for i := 0; i < 12; i++ {
		got := th.wait("a", now)
		require.Equal(t, want, got, "step %d", i)
		now = now.Add(got)
		require.Zero(t, th.wait("a", now), "the wait is over at step %d", i)
		th.failed("a", now)
		want *= 2
		if want > throttleMax {
			want = throttleMax
		}
	}
}

// A guess made during the wait must not be CHECKED, or the wait only slows the
// answer and not the guessing. The handler asks wait() first and never calls
// failed() for a refused attempt, so the wait does not grow while refused.
func TestARefusedAttemptDoesNotExtendTheWait(t *testing.T) {
	var th attemptThrottle
	for i := 0; i <= throttleFree; i++ {
		th.failed("a", throttleEpoch)
	}
	first := th.wait("a", throttleEpoch)
	require.Positive(t, first)
	for i := 0; i < 50; i++ {
		th.wait("a", throttleEpoch.Add(time.Second))
	}
	require.Equal(t, first-time.Second, th.wait("a", throttleEpoch.Add(time.Second)))
}

func TestTheRightPasswordClearsTheRecord(t *testing.T) {
	var th attemptThrottle
	for i := 0; i < throttleFree+3; i++ {
		th.failed("a", throttleEpoch)
	}
	th.succeeded("a")
	for i := 0; i < throttleFree; i++ {
		require.Zero(t, th.wait("a", throttleEpoch))
		th.failed("a", throttleEpoch)
	}
}

func TestAQuietDayForgivesTheMistakes(t *testing.T) {
	var th attemptThrottle
	for i := 0; i < throttleFree+4; i++ {
		th.failed("a", throttleEpoch)
	}
	later := throttleEpoch.Add(throttleForget + time.Minute)
	require.Zero(t, th.wait("a", later))
	th.failed("a", later)
	require.Zero(t, th.wait("a", later), "the count started again from nothing")
}

func TestOneAccountsMistakesDoNotSlowAnother(t *testing.T) {
	var th attemptThrottle
	for i := 0; i < throttleFree+2; i++ {
		th.failed("a", throttleEpoch)
	}
	require.Positive(t, th.wait("a", throttleEpoch))
	require.Zero(t, th.wait("b", throttleEpoch))
}

// Memory is bounded, and filling it cannot switch the throttle off: a full
// table evicts its stalest entry rather than refusing to record a new one,
// which is what an attacker who first sprays junk addresses would want.
func TestAFullTableEvictsTheStalestNotTheNewest(t *testing.T) {
	th := attemptThrottle{capacity: 3}
	th.failed("old", throttleEpoch)
	th.failed("mid", throttleEpoch.Add(time.Minute))
	th.failed("new", throttleEpoch.Add(2*time.Minute))
	for i := 0; i < throttleFree+1; i++ {
		th.failed("target", throttleEpoch.Add(3*time.Minute))
	}
	require.Len(t, th.entries, 3)
	require.NotContains(t, th.entries, "old")
	require.Positive(t, th.wait("target", throttleEpoch.Add(3*time.Minute)))
}

// Through the handler: the throttle sits in front of the password check.
func TestGuessingAnAccountIsSlowedDown(t *testing.T) {
	f := newRecoveryFixture(t)
	f.enrolAccount(t, "amara@example.com", "a long correct password")

	for i := 0; i < throttleFree; i++ {
		status, _ := f.factors(t, "amara@example.com", "wrong guess number "+strconv.Itoa(i))
		require.Equal(t, http.StatusUnauthorized, status, "guess %d", i+1)
	}

	// Now even the RIGHT password is refused, and refused without being
	// checked: answering 200 here would make every refused guess a free one.
	status, retry := f.factors(t, "amara@example.com", "a long correct password")
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, strconv.Itoa(int(throttleBase.Seconds())), retry)

	*f.clock = f.clock.Add(throttleBase)
	status, _ = f.factors(t, "amara@example.com", "a long correct password")
	require.Equal(t, http.StatusOK, status, "the wait is over and the password is right")

	status, _ = f.factors(t, "amara@example.com", "one more wrong guess")
	require.Equal(t, http.StatusUnauthorized, status, "a success cleared the record")
}

// An address with no account is slowed in exactly the same way. Otherwise the
// 429 would say what the careful 401 wording does not: that this email is
// enrolled.
func TestAnUnknownAccountIsSlowedTheSameWay(t *testing.T) {
	f := newRecoveryFixture(t)
	for i := 0; i < throttleFree; i++ {
		status, _ := f.factors(t, "nobody@example.com", "a guess that goes nowhere")
		require.Equal(t, http.StatusUnauthorized, status)
	}
	status, retry := f.factors(t, "nobody@example.com", "a guess that goes nowhere")
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, strconv.Itoa(int(throttleBase.Seconds())), retry)
}

// The same person, spelled twice, is one record - the throttle is keyed on the
// blind index, which folds case, so changing the capitals is not a fresh
// allowance.
func TestCapitalsAreNotAFreshAllowance(t *testing.T) {
	f := newRecoveryFixture(t)
	f.enrolAccount(t, "amara@example.com", "a long correct password")
	for i := 0; i < throttleFree; i++ {
		f.factors(t, "AMARA@example.com", "wrong guess "+strconv.Itoa(i))
	}
	status, _ := f.factors(t, "Amara@Example.com", "another wrong guess")
	require.Equal(t, http.StatusTooManyRequests, status)
}

// factors asks for an account's factors, which needs the password, and returns
// the status and any Retry-After.
func (f *recoveryFixture) factors(t *testing.T, email, password string) (int, string) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"email": email, "password": password})
	require.NoError(t, err)
	resp, err := http.Post(f.srv.URL+"/v1/factors", "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Retry-After")
}
