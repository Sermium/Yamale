package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/recovery"
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

// The second-factor test went with the signing gate.
//
// It asserted that the factor was demanded by AMOUNT rather than by session
// age, which was the right rule and the reason it was worth a test: gating the
// login protects the session, and the attacker already has the session.
//
// Nothing asks the question now, because nothing sees a payment before it
// happens. The rule survives where it still applies — a recovery, in
// factors_test.go — and does not survive here, where there is no longer a
// moment of signing for this service to be present at.

// ---------------------------------------------------------------- the store

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	key := []byte(strings.Repeat("k", 32))
	s, err := NewStore(dir, recovery.RoleCustodian, key)
	require.NoError(t, err)
	return s, dir
}

func custodianHolding(t *testing.T) recovery.Holding {
	t.Helper()
	// A real one. It used to be a role-shaped stub because generating a
	// threshold share cost four minutes of safe primes; Protect costs under a
	// second, so there is no longer any reason for the storage tests to run
	// against something that is not what production stores.
	holdings, err := recovery.Protect([]byte("a key, or something its length"), "seven horses eating apples")
	require.NoError(t, err)
	for _, h := range holdings {
		if h.Role == recovery.RoleCustodian {
			return h
		}
	}
	t.Fatal("no custodian holding")
	return recovery.Holding{}
}

func TestAStoredHoldingIsNotReadableFromTheFile(t *testing.T) {
	store, dir := testStore(t)
	acct := Account{Index: "idx-1", Address: "yml1test"}
	require.NoError(t, store.Put(acct, custodianHolding(t)))

	raw, err := os.ReadFile(filepath.Join(dir, "idx-1.json"))
	require.NoError(t, err)
	require.NotContains(t, string(raw), recovery.RoleCustodian,
		"the holding went to disk in a readable form")
}

func TestAHoldingCannotBeMovedBetweenAccounts(t *testing.T) {
	store, _ := testStore(t)
	require.NoError(t, store.Put(Account{Index: "idx-1"}, custodianHolding(t)))

	one, err := store.Get("idx-1")
	require.NoError(t, err)

	// Lift the sealed bytes into another account's record, which is what an
	// attacker with write access would try.
	forged := Account{Index: "idx-2", Holding: one.Holding}
	_, err = store.Holding(forged)
	require.Error(t, err, "a holding sealed for one account opened under another")
}

func TestTheWrongSealingKeyOpensNothing(t *testing.T) {
	dir := t.TempDir()
	right, err := NewStore(dir, recovery.RoleCustodian, []byte(strings.Repeat("k", 32)))
	require.NoError(t, err)
	require.NoError(t, right.Put(Account{Index: "idx-1"}, custodianHolding(t)))

	wrong, err := NewStore(dir, recovery.RoleCustodian, []byte(strings.Repeat("j", 32)))
	require.NoError(t, err)
	acct, err := wrong.Get("idx-1")
	require.NoError(t, err, "reading the record needs no key")
	_, err = wrong.Holding(acct)
	require.Error(t, err)
}

// The rule that keeps a custodian a custodian, checked on both sides.
//
// Put refuses so the bad file never exists; Holding refuses so a bad file that
// arrived some other way — written by the recovery deployment sharing a
// directory, or restored from the wrong backup — is caught on read. The
// read-side check is the one that matters, because a holding on disk is one
// that survives a restart, so it is exercised against a genuinely written file
// rather than a hand-forged one.
func TestAStoreRefusesToHandBackAnotherRolesHolding(t *testing.T) {
	store, dir := testStore(t)

	// The device's holding must not even be writable here. A service keeping it
	// would hold two of the three.
	require.Error(t, store.Put(Account{Index: "idx-1"}, recovery.Holding{Role: recovery.RoleDevice}),
		"a custodian store accepted the device's holding")

	// Now a file this store did not write. The recovery deployment is the
	// realistic source: same directory by misconfiguration, same sealing key by
	// copy-paste, and its holding opens fine — which is exactly why the role
	// has to be checked rather than inferred from the fact that it decrypted.
	otherService, err := NewStore(dir, recovery.RoleRecovery, []byte(strings.Repeat("k", 32)))
	require.NoError(t, err)
	require.NoError(t, otherService.Put(Account{Index: "idx-2"}, recovery.Holding{Role: recovery.RoleRecovery}))

	acct, err := store.Get("idx-2")
	require.NoError(t, err, "reading the record needs no role")
	_, err = store.Holding(acct)
	require.Error(t, err, "a custodian store handed back a recovery share")
	require.Contains(t, err.Error(), "can attempt a recovery on its own")
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
	require.NoError(t, store.Put(Account{Index: "idx-1", Address: "yml1x"}, custodianHolding(t)))
	require.NoError(t, store.Freeze("idx-1", "phone stolen at the market"))

	acct, err := store.Get("idx-1")
	require.NoError(t, err)
	require.True(t, acct.Frozen)
	require.Contains(t, acct.FrozenReason, "stolen")

	// And the share is still there: a freeze is not a deletion, because an
	// account that vanishes takes its own audit trail with it.
	_, err = store.Holding(acct)
	require.NoError(t, err)
}

// The session tests went with the signing sessions.
//
// They covered three rules — one signature at a time, a two-minute expiry, and
// an unknown session refused — and all three were about a co-signing path that
// no longer exists. Deleted rather than adapted: there is nothing left for them
// to assert, and a test kept alive against a stub is worse than no test,
// because it reports coverage of a guarantee nobody is making any more.

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

// writeSecret writes a secret file the way an operator would, and skips the
// test on a filesystem that cannot hold unix permissions, where readSecret
// refuses everything for being world-readable.
func writeSecret(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, content, 0o600))
	if _, err := readSecret(path); err != nil && strings.Contains(err.Error(), "readable by others") {
		t.Skip("this filesystem does not carry unix permissions")
	}
	return path
}

// The bug this replaced. readSecret used to TrimSpace every file, so a raw
// random key whose first or last byte happened to be whitespace — 6 of the 256
// byte values, at either end, so about one key in twenty — came back 31 bytes
// long and the service refused to start on a key that was generated correctly.
// It surfaced deploying the account services on 2026-09-23 and looked like a
// mystery, because it depends on the draw.
func TestARawKeyIsNeverTrimmed(t *testing.T) {
	for _, edge := range []byte{'\n', '\r', ' ', '\t', '\v', '\f'} {
		key := []byte(strings.Repeat("k", 32))
		key[31] = edge
		got, err := readSecret(writeSecret(t, key))
		require.NoError(t, err)
		require.Equal(t, key, got, "a raw key ending in %q was altered", edge)

		key = []byte(strings.Repeat("k", 32))
		key[0] = edge
		got, err = readSecret(writeSecret(t, key))
		require.NoError(t, err)
		require.Equal(t, key, got, "a raw key starting with %q was altered", edge)
	}
}

// Hex is the format to write, and the one where trimming is safe: a hex digit
// is never whitespace, so the newline an editor or `openssl rand -hex` adds can
// be removed without ever touching the secret.
func TestAHexSecretIsDecoded(t *testing.T) {
	want := make([]byte, 32)
	for i := range want {
		want[i] = byte(i * 7)
	}
	for _, form := range []string{"%x", "%x\n", "  %x\r\n", "%X\n"} {
		got, err := readSecret(writeSecret(t, []byte(fmt.Sprintf(form, want))))
		require.NoError(t, err, "form %q", form)
		require.Equal(t, want, got, "form %q", form)
	}
}

// A longer hex pepper is fine; the pepper only has a minimum.
func TestALongHexPepperIsDecoded(t *testing.T) {
	want := []byte(strings.Repeat("p", 48))
	got, err := readSecret(writeSecret(t, []byte(fmt.Sprintf("%x\n", want))))
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// What used to be silently "fixed" is now refused, and told how to fix it. A
// 32-character typed secret plus the editor's newline is 33 bytes that are not
// hex: trimming it was right for that file and wrong for a random one, and
// readSecret cannot tell which it has been given.
func TestAnAmbiguousSecretIsRefusedWithTheFix(t *testing.T) {
	for name, content := range map[string][]byte{
		"typed secret with a newline": []byte(strings.Repeat("k", 32) + "\n"),
		"too short":                   []byte(strings.Repeat("k", 16)),
		"short hex":                   []byte(strings.Repeat("ab", 16) + "\n"),
		"odd-length hex":              []byte(strings.Repeat("a", 65)),
		"empty":                       {},
	} {
		_, err := readSecret(writeSecret(t, content))
		require.Error(t, err, name)
		require.Contains(t, err.Error(), "openssl rand -hex 32", name)
	}
}
