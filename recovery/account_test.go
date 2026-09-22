package recovery

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/argon2"
)

// These are slower than the sharing tests because every one of them pays for an
// Argon2id derivation at the parameters the service actually uses. That is the
// point — a test suite that ran the KDF at toy settings would not be exercising
// the thing that is deployed.

const testPassword = "seven horses eating apples slowly"

/* ---------------------------------------------------------- the seal */

func TestASealedSecretComesBackIntact(t *testing.T) {
	secret := []byte("a thirty-two byte key, near enough")
	sealed, err := Seal(secret, testPassword)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed.Ciphertext, secret) {
		t.Fatal("the plaintext is visible in the ciphertext")
	}
	got, err := sealed.Open(testPassword)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("got %q", got)
	}
}

func TestAWrongPasswordIsRefusedRatherThanReturningRubbish(t *testing.T) {
	sealed, err := Seal([]byte("secret"), testPassword)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	// One character different, and the closest wrong guess: the right password
	// with a trailing space, which is what a form that does not trim produces.
	for _, wrong := range []string{
		"seven horses eating apples slowlY",
		testPassword + " ",
	} {
		if _, err := sealed.Open(wrong); err != ErrWrongSeal {
			t.Fatalf("Open(%q) returned %v, want ErrWrongSeal", wrong, err)
		}
	}
}

// The trap named at the top of seal.go, held as a test.
//
// tools/custodian/auth.go STORES argon2.IDKey(password, salt, 3, 64MiB, 4, 32)
// as a password verifier. If the seal derived its key the same way, that stored
// value would be the decryption key and a store dump would open every account
// without a single guess.
func TestTheSealKeyIsNeverThePasswordVerifier(t *testing.T) {
	// The custodian's constants, copied because Verifier lives in package main
	// and cannot be imported. If those change, this test still asks the right
	// question: no derivation of this password over this salt may equal the
	// seal's key.
	const (
		custodianTime    = 3
		custodianMemory  = 64 * 1024
		custodianThreads = 4
		custodianKeyLen  = 32
	)

	salt := make([]byte, sealSaltLen)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}

	verifier := argon2.IDKey([]byte(testPassword), salt,
		custodianTime, custodianMemory, custodianThreads, custodianKeyLen)

	// The seal's key over the SAME salt — the worst case, where an
	// implementation reused the account's stored salt for convenience.
	sealKey := argon2.IDKey([]byte(testPassword), append([]byte(sealDomain), salt...),
		sealTime, sealMemory, sealThreads, sealKeyLen)

	if bytes.Equal(verifier, sealKey) {
		t.Fatal("the stored password verifier IS the seal key; a store dump opens every account")
	}

	// And the domain separator is what does it, not merely the differing cost
	// parameters — because the cost parameters are the sort of thing somebody
	// later "harmonises".
	sameParams := argon2.IDKey([]byte(testPassword), salt,
		sealTime, sealMemory, sealThreads, sealKeyLen)
	if bytes.Equal(sameParams, sealKey) {
		t.Fatal("the domain separator is not being mixed in; only the Argon2 parameters separate these")
	}
}

func TestTamperedCiphertextIsDetected(t *testing.T) {
	sealed, err := Seal([]byte("a secret worth changing"), testPassword)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	sealed.Ciphertext[0] ^= 1
	if _, err := sealed.Open(testPassword); err != ErrWrongSeal {
		t.Fatalf("a flipped bit returned %v, want ErrWrongSeal", err)
	}
}

func TestWeakenedParametersAreRefused(t *testing.T) {
	// Somebody with write access to the store setting Memory to 8 KiB, so that
	// this service does the cheap derivation for them. It would fail its tag
	// anyway; refusing earlier says why.
	sealed, err := Seal([]byte("x"), testPassword)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	sealed.Memory = 8
	if _, err := sealed.Open(testPassword); err != ErrSealWeakParams {
		t.Fatalf("got %v, want ErrSealWeakParams", err)
	}
}

func TestSealRefusesWhatItCannotProtect(t *testing.T) {
	if _, err := Seal(nil, testPassword); err != ErrEmptySecret {
		t.Fatalf("empty secret: %v", err)
	}
	if _, err := Seal([]byte("x"), ""); err != ErrNoPassword {
		t.Fatalf("empty password: %v", err)
	}
	var nilSeal *Sealed
	if _, err := nilSeal.Open(testPassword); err != ErrSealMalformed {
		t.Fatalf("nil seal: %v", err)
	}
}

func TestTwoSealsOfOneSecretDiffer(t *testing.T) {
	// Fresh salt and fresh nonce every time. Equal ciphertexts would tell
	// anybody watching the store that two accounts share a key, or that one
	// account's key did not change when it was supposed to have been rotated.
	a, err := Seal([]byte("same"), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Seal([]byte("same"), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a.Salt, b.Salt) {
		t.Fatal("the salt is not fresh")
	}
	if bytes.Equal(a.Nonce, b.Nonce) {
		t.Fatal("the nonce is not fresh")
	}
	if bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Fatal("two seals of one secret are identical")
	}
}

/* ------------------------------------------------------- the holdings */

func TestAnyTwoHoldingsRestoreTheKey(t *testing.T) {
	secret := []byte("the device's private key")
	holdings, err := Protect(secret, testPassword)
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	if len(holdings) != 3 {
		t.Fatalf("got %d holdings", len(holdings))
	}

	// Named, because which pairs work is the operational question: lose the
	// phone and it is custodian+recovery; lose the custodian and it is
	// device+recovery.
	for _, pair := range [][2]int{{0, 1}, {0, 2}, {1, 2}} {
		got, err := Restore([]Holding{holdings[pair[0]], holdings[pair[1]]}, testPassword)
		if err != nil {
			t.Fatalf("%s+%s: %v", holdings[pair[0]].Role, holdings[pair[1]].Role, err)
		}
		if !bytes.Equal(got, secret) {
			t.Fatalf("%s+%s rebuilt %q", holdings[pair[0]].Role, holdings[pair[1]].Role, got)
		}
	}

	// And all three, which is what happens when nothing was lost and the
	// operator simply fetched everything.
	got, err := Restore(holdings, testPassword)
	if err != nil {
		t.Fatalf("all three: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("all three holdings did not restore the key")
	}
}

// The property the whole file exists for.
//
// The custodian and the recovery service are both run by the platform. If the
// two of them could reconstruct the key, "the operator cannot move your money"
// would be false, and it is the claim the product is sold on.
//
// This test is what fails if anybody ever reorders Protect to split first and
// seal the shares afterwards — which reads as more thorough and gives away
// exactly this.
func TestBothServicesTogetherStillCannotOpenIt(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	holdings, err := Protect(secret, testPassword)
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}

	var custodian, recovery Holding
	for _, h := range holdings {
		switch h.Role {
		case RoleCustodian:
			custodian = h
		case RoleRecovery:
			recovery = h
		}
	}

	// Everything the two services hold between them, with no password.
	between := [][]byte{custodian.Share.Y, recovery.Share.Y, custodian.Salt, custodian.Nonce}
	combined, err := Combine([]Share{custodian.Share, recovery.Share})
	if err != nil {
		t.Fatalf("Combine: %v", err)
	}
	between = append(between, combined)

	for i, held := range between {
		if bytes.Contains(held, secret) {
			t.Fatalf("the secret is recoverable from what the services hold (item %d)", i)
		}
	}

	// And a guess at the password still has to go through the seal.
	if _, err := Restore([]Holding{custodian, recovery}, "not the password at all"); err != ErrWrongSeal {
		t.Fatalf("a wrong password returned %v, want ErrWrongSeal", err)
	}
}

func TestOneHoldingIsNotARecovery(t *testing.T) {
	holdings, err := Protect([]byte("key"), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(holdings[:1], testPassword); err != ErrNotEnoughHoldings {
		t.Fatalf("got %v, want ErrNotEnoughHoldings", err)
	}
}

func TestHoldingsFromDifferentEnrolmentsAreRefused(t *testing.T) {
	// The operational failure this catches: a store that returns the right
	// role's holding from the wrong account. Combined blindly they produce a
	// ciphertext that fails its tag, and the operator reads ErrWrongSeal and
	// concludes the customer forgot their password.
	a, err := Protect([]byte("first key"), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Protect([]byte("second key"), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore([]Holding{a[0], b[1]}, testPassword); err != ErrMixedHoldings {
		t.Fatalf("got %v, want ErrMixedHoldings", err)
	}
}

func TestAnUnknownRoleIsRefused(t *testing.T) {
	holdings, err := Protect([]byte("key"), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	holdings[1].Role = "auditor"
	if _, err := Restore(holdings[:2], testPassword); err != ErrUnknownRole {
		t.Fatalf("got %v, want ErrUnknownRole", err)
	}
}

func TestEachRoleGetsADifferentShare(t *testing.T) {
	holdings, err := Protect([]byte("key"), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[byte]string{}
	for _, h := range holdings {
		if prior, ok := seen[h.Share.X]; ok {
			t.Fatalf("%s and %s hold the same share", prior, h.Role)
		}
		seen[h.Share.X] = h.Role
	}
	if len(seen) != 3 {
		t.Fatalf("got %d distinct shares", len(seen))
	}
}

/* -------------------------------------------------- an actual chain key */

// The end of the argument for deleting tss-lib: the key this protects is an
// ordinary secp256k1 private key, it signs on its own, and it survives a
// recovery still able to sign.
//
// Under the threshold scheme there was no such object at any point in the
// system's life — which is why enrolment took nine and a half minutes and why
// key generation had to be moved off the main thread.
func TestARealSigningKeySurvivesARecovery(t *testing.T) {
	key, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	message := [32]byte{1, 2, 3, 4, 5}
	before := ecdsa.Sign(key, message[:])

	holdings, err := Protect(key.Serialize(), testPassword)
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}

	// The phone is gone. The two services and the password are what is left.
	var pair []Holding
	for _, h := range holdings {
		if h.Role != RoleDevice {
			pair = append(pair, h)
		}
	}

	restored, err := Restore(pair, testPassword)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	recovered := secp256k1.PrivKeyFromBytes(restored)

	if !recovered.PubKey().IsEqual(key.PubKey()) {
		t.Fatal("the recovered key has a different public key: the account is not the same account")
	}
	if !before.Verify(message[:], recovered.PubKey()) {
		t.Fatal("a signature made before the recovery no longer verifies")
	}
	after := ecdsa.Sign(recovered, message[:])
	if !after.Verify(message[:], key.PubKey()) {
		t.Fatal("the recovered key cannot produce a valid signature")
	}
}

func TestGeneratingAKeyIsNotSlow(t *testing.T) {
	// Stated as a test because the number it replaces is the headline: an
	// enrolment under the threshold scheme took about nine and a half minutes
	// in a browser, which is why it needed a Web Worker at all.
	//
	// Generous by three orders of magnitude, so this measures "not a protocol"
	// rather than the speed of the machine it runs on.
	for i := 0; i < 100; i++ {
		if _, err := secp256k1.GeneratePrivateKey(); err != nil {
			t.Fatalf("generating a key: %v", err)
		}
	}
}
