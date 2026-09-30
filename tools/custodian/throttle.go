package main

import (
	"sync"
	"time"
)

// attemptThrottle slows down password guessing against one account.
//
// nginx in front of this service limits an ADDRESS, and an attacker has as
// many addresses as they care to rent. This limits the ACCOUNT, which is the
// thing being guessed at, and it matters more here than on an ordinary sign-in:
// the password is also what seals the key, so a password learned by guessing
// this endpoint is the one that opens the ciphertext if the ciphertext is ever
// obtained.
//
// A growing wait, not a lockout. A lockout hands anybody who knows an email
// address a way to shut its owner out - and the one thing an owner may need
// urgently here is a freeze, after a theft, which is exactly when an attacker
// would want them shut out. So the first few mistakes cost nothing, each one
// after that doubles the wait, and the wait never exceeds an hour. At an hour a
// guess, a ten-character minimum is out of reach; an owner is never kept out
// longer than that.
//
// Three properties the tests hold:
//
//   - Keyed on the blind index, which an unknown address has too, so an email
//     with no account is slowed exactly like one with an account. Otherwise
//     the 429 would say what the careful 401 wording does not.
//   - Consulted BEFORE the password is checked. A guess made during the wait
//     is refused unread; answering it would make the wait slow the reply and
//     not the guessing.
//   - In memory. A restart forgets it, which costs an attacker nothing they
//     can arrange from outside, and keeps a record of who failed to sign in
//     out of the account store.
//
// The zero value is ready to use, so a server built anywhere - including by a
// test that knows nothing about this - is throttled.
type attemptThrottle struct {
	mu      sync.Mutex
	entries map[string]*attemptRecord
	// capacity bounds the table; zero means throttleCapacity.
	capacity int
}

type attemptRecord struct {
	failures int
	last     time.Time // the most recent failure
	until    time.Time // no attempt is read before this
}

const (
	// Mistakes allowed before any wait. A person mistyping gets several.
	throttleFree = 5
	// The first wait, doubled for each failure after it.
	throttleBase = time.Minute
	// The longest wait. What an owner can be kept out for, at most.
	throttleMax = time.Hour
	// A record with no failure for this long is forgotten.
	throttleForget = 24 * time.Hour
	// Accounts tracked at once. A record is a few dozen bytes; this is a few
	// megabytes, and filling it evicts the stalest record rather than
	// switching the throttle off.
	throttleCapacity = 100_000
)

// wait reports how long before an attempt on this key may be checked. Zero
// means now. It records nothing, so asking does not lengthen the wait.
func (t *attemptThrottle) wait(key string, now time.Time) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.entries[key]
	if !ok {
		return 0
	}
	if now.Sub(rec.last) >= throttleForget {
		delete(t.entries, key)
		return 0
	}
	if now.Before(rec.until) {
		return rec.until.Sub(now)
	}
	return 0
}

// failed records a wrong password and sets the wait for the next attempt.
func (t *attemptThrottle) failed(key string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.entries == nil {
		t.entries = make(map[string]*attemptRecord)
	}
	rec, ok := t.entries[key]
	if ok && now.Sub(rec.last) >= throttleForget {
		ok = false
	}
	if !ok {
		t.makeRoom(key)
		rec = &attemptRecord{}
		t.entries[key] = rec
	}
	rec.failures++
	rec.last = now
	// The wait starts with the attempt AFTER the last free mistake: five free
	// mistakes means the sixth attempt waits. Counting from the sixth FAILURE
	// instead gave six, which the tests caught.
	if over := rec.failures - throttleFree + 1; over > 0 {
		rec.until = now.Add(backoff(over))
	}
}

// succeeded clears the record: the owner is in, and their earlier typing
// mistakes should not follow them.
func (t *attemptThrottle) succeeded(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

// backoff is the wait after the n-th failure beyond the free ones.
func backoff(n int) time.Duration {
	d := throttleBase
	for i := 1; i < n; i++ {
		d *= 2
		if d >= throttleMax {
			return throttleMax
		}
	}
	return d
}

// makeRoom evicts the stalest record if the table is full. Called with the
// lock held, and only when a NEW key is about to be recorded.
//
// Evicting rather than refusing is the point: a table that stopped recording
// when full would let an attacker spray junk addresses first and then guess at
// the real one unthrottled. The scan is linear, but it runs only when the table
// is full, which takes a hundred thousand distinct failing addresses inside a
// day behind nginx's per-address limit.
func (t *attemptThrottle) makeRoom(key string) {
	limit := t.capacity
	if limit <= 0 {
		limit = throttleCapacity
	}
	if len(t.entries) < limit {
		return
	}
	var stalest string
	var oldest time.Time
	for k, rec := range t.entries {
		if stalest == "" || rec.last.Before(oldest) {
			stalest, oldest = k, rec.last
		}
	}
	delete(t.entries, stalest)
}
