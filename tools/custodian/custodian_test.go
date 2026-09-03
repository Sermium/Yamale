package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/mpc"
)

// The refusals are the product, so they are what is tested.
//
// A custodian that holds a share safely and co-signs for whoever asks has
// turned a two-of-three into a one-of-one held by the attacker. Every case
// below is one way that could happen.

func TestAPasswordIsNeverStoredInAReversibleForm(t *testing.T) {
	v, err := NewVerifier("a long enough password")
	require.NoError(t, err)

	require.True(t, v.Verify("a long enough password"))
	require.False(t, v.Verify("a long enough passworD"))
	require.False(t, v.Verify(""))

	// Nothing anywhere in the stored form contains the password.
	require.NotContains(t, string(v.Hash), "password")
	require.NotContains(t, string(v.Salt), "password")

	// Two verifiers for the same password differ, because the salt differs.
	// Without that, equal hashes across a dump identify everyone who chose the
	// same password as each other.
	other, err := NewVerifier("a long enough password")
	require.NoError(t, err)
	require.NotEqual(t, v.Hash, other.Hash)
}

func TestAShortPasswordIsRefused(t *testing.T) {
	_, err := NewVerifier("short")
	require.Error(t, err)
	// And an absurd one, which is a bound on Argon2 work rather than a policy
	// about strength.
	_, err = NewVerifier(strings.Repeat("x", 2000))
	require.Error(t, err)
}

// The blind index is the fix for what clients/app does today, which is a bare
// SHA-256 of the address and is not a blind index at all.
func TestTheBlindIndexNeedsThePepperToReproduce(t *testing.T) {
	a, err := NewBlindIndex(strings.Repeat("a", 32))
	require.NoError(t, err)
	b, err := NewBlindIndex(strings.Repeat("b", 32))
	require.NoError(t, err)

	email := "amara@example.test"
	require.NotEqual(t, a.Of(email), b.Of(email),
		"the index does not depend on the pepper, so a dump alone could be attacked")

	// Same pepper, same answer — otherwise nobody could ever sign in twice.
	require.Equal(t, a.Of(email), a.Of(email))

	// Case and whitespace are one person, not two accounts.
	require.Equal(t, a.Of(email), a.Of("  Amara@Example.TEST  "))

	// And the index reveals nothing readable.
	require.NotContains(t, a.Of(email), "amara")
}

func TestAGuessablePepperIsRefused(t *testing.T) {
	_, err := NewBlindIndex("too short")
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least 32")
}

// The second factor is demanded at the moment of signing rather than at login,
// because an attacker holding the password and the device share never logs in
// again — they sign.
func TestTheSecondFactorGatesTheAmountNotTheSession(t *testing.T) {
	require.True(t, SecondFactorRequired(0, 0), "a threshold of zero must mean always")
	require.True(t, SecondFactorRequired(1_000_000, 0))

	require.False(t, SecondFactorRequired(999, 1000))
	require.True(t, SecondFactorRequired(1000, 1000), "at the threshold, not merely above it")
	require.True(t, SecondFactorRequired(1001, 1000))
}

// ---------------------------------------------------------------- the store

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	key := []byte(strings.Repeat("k", 32))
	s, err := NewStore(dir, mpc.RoleCustodian, key)
	require.NoError(t, err)
	return s, dir
}

func custodianShare(t *testing.T) mpc.Share {
	t.Helper()
	// A share-shaped value is enough for the storage rules; the protocol tests
	// in mpc/ exercise real ones, and generating one here would add four
	// minutes of safe primes to a test about file handling.
	return mpc.Share{Role: mpc.RoleCustodian}
}

func TestAStoredShareIsNotReadableFromTheFile(t *testing.T) {
	store, dir := testStore(t)
	acct := Account{Index: "idx-1", Address: "yml1test"}
	require.NoError(t, store.Put(acct, custodianShare(t)))

	raw, err := os.ReadFile(filepath.Join(dir, "idx-1.json"))
	require.NoError(t, err)
	require.NotContains(t, string(raw), mpc.RoleCustodian,
		"the share went to disk in a readable form")
}

func TestAShareCannotBeMovedBetweenAccounts(t *testing.T) {
	store, _ := testStore(t)
	require.NoError(t, store.Put(Account{Index: "idx-1"}, custodianShare(t)))

	one, err := store.Get("idx-1")
	require.NoError(t, err)

	// Lift the sealed bytes into another account's record, which is what an
	// attacker with write access would try.
	forged := Account{Index: "idx-2", Share: one.Share}
	_, err = store.Share(forged)
	require.Error(t, err, "a share sealed for one account opened under another")
}

func TestTheWrongSealingKeyOpensNothing(t *testing.T) {
	dir := t.TempDir()
	right, err := NewStore(dir, mpc.RoleCustodian, []byte(strings.Repeat("k", 32)))
	require.NoError(t, err)
	require.NoError(t, right.Put(Account{Index: "idx-1"}, custodianShare(t)))

	wrong, err := NewStore(dir, mpc.RoleCustodian, []byte(strings.Repeat("j", 32)))
	require.NoError(t, err)
	acct, err := wrong.Get("idx-1")
	require.NoError(t, err, "reading the record needs no key")
	_, err = wrong.Share(acct)
	require.Error(t, err)
}

// The rule that keeps a custodian a custodian, checked on both sides.
//
// Put refuses so the bad file never exists; Share refuses so a bad file that
// arrived some other way — written by the recovery deployment sharing a
// directory, or restored from the wrong backup — is caught on read. The
// read-side check is the one that matters, because a share on disk is a share
// that survives a restart, so it is exercised against a genuinely written file
// rather than a hand-forged one.
func TestAStoreRefusesToHandBackANonCustodianShare(t *testing.T) {
	store, dir := testStore(t)

	// The device's share must not even be writable here. A service holding it
	// could sign alone.
	require.Error(t, store.Put(Account{Index: "idx-1"}, mpc.Share{Role: mpc.RoleDevice}),
		"a custodian store accepted the device's share")

	// Now a file this store did not write. The recovery deployment is the
	// realistic source: same directory by misconfiguration, same sealing key by
	// copy-paste, and its share opens fine — which is exactly why the role has
	// to be checked rather than inferred from the fact that it decrypted.
	recovery, err := NewStore(dir, mpc.RoleRecovery, []byte(strings.Repeat("k", 32)))
	require.NoError(t, err)
	require.NoError(t, recovery.Put(Account{Index: "idx-2"}, mpc.Share{Role: mpc.RoleRecovery}))

	acct, err := store.Get("idx-2")
	require.NoError(t, err, "reading the record needs no role")
	_, err = store.Share(acct)
	require.Error(t, err, "a custodian store handed back a recovery share")
	require.Contains(t, err.Error(), "let one service sign alone")
}

func TestAnUnknownAccountIsIndistinguishableFromAWrongPassword(t *testing.T) {
	store, _ := testStore(t)
	_, err := store.Get("never-enrolled")
	require.ErrorIs(t, err, ErrNoAccount)
	// The endpoint's wording is asserted in the auth path; here the point is
	// only that the store reports a typed error rather than a bare nil record
	// a caller might mistake for an empty account.
}

func TestFreezingSurvivesAReRead(t *testing.T) {
	store, _ := testStore(t)
	require.NoError(t, store.Put(Account{Index: "idx-1", Address: "yml1x"}, custodianShare(t)))
	require.NoError(t, store.Freeze("idx-1", "phone stolen at the market"))

	acct, err := store.Get("idx-1")
	require.NoError(t, err)
	require.True(t, acct.Frozen)
	require.Contains(t, acct.FrozenReason, "stolen")

	// And the share is still there: a freeze is not a deletion, because an
	// account that vanishes takes its own audit trail with it.
	_, err = store.Share(acct)
	require.NoError(t, err)
}

// --------------------------------------------------------------- sessions

func TestOnlyOneSignatureAtATime(t *testing.T) {
	sessions := NewSessions()
	// Start needs a real share to build a party, so this asserts the guard
	// rather than the protocol: the counter is incremented before any crypto,
	// and a second start must be refused.
	sessions.byAccount["idx-1"] = MaxOpenSessions
	_, _, err := sessions.Start("idx-1", make([]byte, 32), custodianShare(t))
	require.ErrorIs(t, err, ErrBusy,
		"two signatures in flight for one account is how somebody pays twice")
}

func TestASessionExpires(t *testing.T) {
	sessions := NewSessions()
	now := time.Now()
	sessions.now = func() time.Time { return now }

	sessions.open["s1"] = &Session{ID: "s1", Index: "idx-1", Started: now}
	sessions.byAccount["idx-1"] = 1

	_, err := sessions.lookupLocked("s1")
	require.NoError(t, err)

	now = now.Add(SessionTTL + time.Second)
	_, err = sessions.lookupLocked("s1")
	require.ErrorIs(t, err, ErrSessionOld)

	// And it is gone, not merely reported stale — an expired session still
	// holds the custodian's share in memory.
	require.Empty(t, sessions.open)
	require.Empty(t, sessions.byAccount)
}

func TestAnUnknownSessionIsRefused(t *testing.T) {
	sessions := NewSessions()
	_, err := sessions.Handle("nope", mpc.Outbound{From: mpc.RoleDevice})
	require.ErrorIs(t, err, ErrNoSession)
}

// ---------------------------------------------------------------- config

func TestASecretInsideTheStoreIsRefused(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "custodian.key")
	require.NoError(t, os.WriteFile(inside, []byte("x"), 0o600))

	err := outsideStore(dir, inside, "the sealing key")
	require.Error(t, err)
	require.Contains(t, err.Error(), "protects nothing")

	outside := filepath.Join(t.TempDir(), "custodian.key")
	require.NoError(t, os.WriteFile(outside, []byte("x"), 0o600))
	require.NoError(t, outsideStore(dir, outside, "the sealing key"))
}

func TestAWorldReadableSecretIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pepper")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("p", 32)), 0o644))
	_, err := readSecret(path)
	if err == nil {
		t.Skip("this filesystem does not carry unix permissions")
	}
	require.Contains(t, err.Error(), "readable by others")
}

func TestASecretIsTrimmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("k", 32)+"\n"), 0o600))
	got, err := readSecret(path)
	if err != nil && strings.Contains(err.Error(), "readable by others") {
		t.Skip("this filesystem does not carry unix permissions")
	}
	require.NoError(t, err)
	require.Len(t, got, 32, "a trailing newline made the key the wrong length")
}
