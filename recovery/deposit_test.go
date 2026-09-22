package recovery

import (
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func depositFixture(t *testing.T) (*secp256k1.PrivateKey, Holding) {
	t.Helper()
	key, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	holdings, err := Protect(key.Serialize(), testPassword)
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	for _, h := range holdings {
		if h.Role == RoleCustodian {
			return key, h
		}
	}
	t.Fatal("no custodian holding")
	return nil, Holding{}
}

func TestAHonestDepositVerifies(t *testing.T) {
	key, h := depositFixture(t)
	sig, err := SignDeposit(key, "idx-1", RoleCustodian, h)
	if err != nil {
		t.Fatalf("SignDeposit: %v", err)
	}
	pub, err := VerifyDeposit("idx-1", RoleCustodian, key.PubKey().SerializeCompressed(), sig, h)
	if err != nil {
		t.Fatalf("VerifyDeposit: %v", err)
	}
	if !pub.IsEqual(key.PubKey()) {
		t.Fatal("the verified key is not the signing key")
	}
}

// The attack this exists to close: something between the device and the service
// swaps the holding, the account it is filed under, or the service it was meant
// for. Each has to fail, and each is a separate line in DepositDigest.
func TestEveryBoundFieldIsActuallyBound(t *testing.T) {
	key, h := depositFixture(t)
	pub := key.PubKey().SerializeCompressed()
	sig, err := SignDeposit(key, "idx-1", RoleCustodian, h)
	if err != nil {
		t.Fatal(err)
	}

	other, otherHolding := depositFixture(t)

	tampered := h
	tampered.Share.Y = append([]byte(nil), h.Share.Y...)
	tampered.Share.Y[0] ^= 1

	movedSalt := h
	movedSalt.Salt = otherHolding.Salt

	weakened := h
	weakened.Memory = 8

	for _, c := range []struct {
		name    string
		index   string
		role    string
		pub     []byte
		holding Holding
	}{
		{"a different account", "idx-2", RoleCustodian, pub, h},
		{"the other service", "idx-1", RoleRecovery, pub, h},
		{"somebody else's key", "idx-1", RoleCustodian, other.PubKey().SerializeCompressed(), h},
		{"a flipped bit in the share", "idx-1", RoleCustodian, pub, tampered},
		{"a substituted salt", "idx-1", RoleCustodian, pub, movedSalt},
		{"downgraded Argon2 parameters", "idx-1", RoleCustodian, pub, weakened},
		{"a swapped holding entirely", "idx-1", RoleCustodian, pub, otherHolding},
	} {
		if _, err := VerifyDeposit(c.index, c.role, c.pub, sig, c.holding); err == nil {
			t.Fatalf("%s was accepted", c.name)
		}
	}
}

func TestAMalformedSignatureIsRefusedRatherThanPanicking(t *testing.T) {
	key, h := depositFixture(t)
	pub := key.PubKey().SerializeCompressed()
	good, err := SignDeposit(key, "idx-1", RoleCustodian, h)
	if err != nil {
		t.Fatal(err)
	}

	zeroR := make([]byte, SignatureLen)
	copy(zeroR[32:], good[32:])
	zeroS := make([]byte, SignatureLen)
	copy(zeroS[:32], good[:32])

	// A scalar above the curve order. SetByteSlice reports the overflow and
	// this must refuse rather than reduce it — a reduced value would be a
	// second encoding of a signature that already verifies.
	overflowing := make([]byte, SignatureLen)
	copy(overflowing, good)
	for i := 0; i < 32; i++ {
		overflowing[i] = 0xff
	}

	for name, sig := range map[string][]byte{
		"empty":       {},
		"too short":   good[:63],
		"too long":    append(append([]byte{}, good...), 0),
		"all zeroes":  make([]byte, SignatureLen),
		"zero r":      zeroR,
		"zero s":      zeroS,
		"overflowing": overflowing,
	} {
		if _, err := VerifyDeposit("idx-1", RoleCustodian, pub, sig, h); err != ErrBadSignature {
			t.Fatalf("%s returned %v, want ErrBadSignature", name, err)
		}
	}
}

func TestAMalformedPublicKeyIsRefused(t *testing.T) {
	key, h := depositFixture(t)
	sig, err := SignDeposit(key, "idx-1", RoleCustodian, h)
	if err != nil {
		t.Fatal(err)
	}
	notOnCurve := make([]byte, secp256k1.PubKeyBytesLenCompressed)
	notOnCurve[0] = 0x02
	for i := 1; i < len(notOnCurve); i++ {
		notOnCurve[i] = 0xff
	}

	for name, pub := range map[string][]byte{
		"empty":        {},
		"uncompressed": key.PubKey().SerializeUncompressed(),
		"not a point":  notOnCurve,
	} {
		if _, err := VerifyDeposit("idx-1", RoleCustodian, pub, sig, h); err != ErrBadPubKey {
			t.Fatalf("%s returned %v, want ErrBadPubKey", name, err)
		}
	}
}

// Length prefixing, stated as the thing it prevents. Without it these two
// deposits hash identically and one signature covers both.
func TestFieldsCannotBeRecutAcrossTheirBoundaries(t *testing.T) {
	_, h := depositFixture(t)
	a := DepositDigest("ab", "c", make([]byte, 33), h)
	b := DepositDigest("a", "bc", make([]byte, 33), h)
	if a == b {
		t.Fatal("the digest concatenates without length prefixes")
	}
}

func TestTheDigestIsStable(t *testing.T) {
	// Signing and verifying happen in different processes, minutes apart, and
	// on the device in a different language entirely. A digest that varied by
	// map ordering or by a timestamp would fail intermittently, which is the
	// worst way for this to be wrong.
	_, h := depositFixture(t)
	pub := make([]byte, 33)
	first := DepositDigest("idx", RoleCustodian, pub, h)
	for i := 0; i < 50; i++ {
		if DepositDigest("idx", RoleCustodian, pub, h) != first {
			t.Fatal("the digest is not deterministic")
		}
	}
}
