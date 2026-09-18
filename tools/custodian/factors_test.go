package main

import (
	"crypto/sha256"
	"testing"
	"time"
)

// The second factor.
//
// The tests that matter are the ones about what an ATTACKER can choose. A
// factor that verifies correctly and can be skipped, downgraded or replayed is
// a factor that is not there.

var when = time.Unix(1_700_000_000, 0)

/* ------------------------------------------------------------- the ladder */

func TestAnAccountIsHeldToItsStrongestFactor(t *testing.T) {
	secret, _ := NewTOTPSecret()

	if got := (Enrolled{}).Strongest(); got != FactorEmail {
		t.Errorf("an account with nothing enrolled should fall back to email, got %q", got)
	}
	if got := (Enrolled{TOTPSecret: secret, TOTPConfirmed: true}).Strongest(); got != FactorTOTP {
		t.Errorf("a confirmed TOTP should win over email, got %q", got)
	}
	if got := (Enrolled{
		TOTPSecret: secret, TOTPConfirmed: true, PasskeyCredentialID: []byte("cred"),
	}).Strongest(); got != FactorPasskey {
		t.Errorf("a passkey should win over TOTP, got %q", got)
	}
}

func TestAnUnconfirmedTOTPDoesNotLockTheAccountOut(t *testing.T) {
	// A secret is stored when the QR is shown. If that alone counted, somebody
	// who never finished scanning it could no longer sign at all — locked out
	// by a factor they do not have.
	secret, _ := NewTOTPSecret()
	if got := (Enrolled{TOTPSecret: secret}).Strongest(); got != FactorEmail {
		t.Errorf("an unconfirmed secret must not be demanded, got %q", got)
	}
}

func TestAWeakerFactorIsRefusedEvenIfItVerifies(t *testing.T) {
	// The downgrade attack. An account with TOTP enrolled must not be signable
	// by answering with an emailed code, however valid that code is — otherwise
	// enrolling a stronger factor buys nothing.
	secret, _ := NewTOTPSecret()
	e := Enrolled{TOTPSecret: secret, TOTPConfirmed: true}

	code, pending, err := NewEmailCode(when)
	if err != nil {
		t.Fatal(err)
	}
	_, err = VerifyFactor(e, FactorEmail, code, when, pending)
	if err != ErrFactorDowngrade {
		t.Fatalf("want ErrFactorDowngrade, got %v", err)
	}
}

func TestAPasskeyAccountCannotSignUntilPasskeysAreBuilt(t *testing.T) {
	// The honest direction. A credential the service cannot check must stop the
	// signature, not be waved through because the feature is unfinished.
	e := Enrolled{PasskeyCredentialID: []byte("cred")}
	if _, err := VerifyFactor(e, FactorPasskey, "anything", when, nil); err != ErrPasskeyNotBuilt {
		t.Fatalf("want ErrPasskeyNotBuilt, got %v", err)
	}
}

/* --------------------------------------------------------------- the rule */

func TestARecoveryAlwaysNeedsAFactor(t *testing.T) {
	// No threshold and no exemption. A recovery is somebody asking for an
	// account they cannot currently reach, which is the same sentence an
	// attacker says, so there is no version of it that needs less proof.
	if got := Required(Enrolled{}); got != FactorEmail {
		t.Fatalf("an account with nothing enrolled still owes the email floor, got %q", got)
	}
	secret, _ := NewTOTPSecret()
	if got := Required(Enrolled{TOTPSecret: secret, TOTPConfirmed: true}); got != FactorTOTP {
		t.Fatalf("want TOTP, got %q", got)
	}
}

/* ---------------------------------------------------------------- the TOTP */

func TestATOTPCodeVerifiesAndThenCannotBeReplayed(t *testing.T) {
	// A thirty-second code read over a shoulder is a thirty-second window for
	// somebody else, unless the step it came from is spent.
	secret, _ := NewTOTPSecret()
	e := Enrolled{TOTPSecret: secret, TOTPConfirmed: true}

	step := uint64(when.Unix()) / 30
	code := totpAt(secret, step)

	after, err := VerifyFactor(e, FactorTOTP, code, when, nil)
	if err != nil {
		t.Fatalf("a correct code should verify: %v", err)
	}
	if after.LastUsedCounter != step {
		t.Fatalf("the step should be recorded, got %d want %d", after.LastUsedCounter, step)
	}

	if _, err := VerifyFactor(after, FactorTOTP, code, when, nil); err != ErrFactorWrong {
		t.Fatalf("the same code must not work twice, got %v", err)
	}
}

func TestATOTPCodeSurvivesAClockAFewSecondsOut(t *testing.T) {
	// One step either way. Zero rejects people whose phone drifts; more widens
	// the window an intercepted code is useful in.
	secret, _ := NewTOTPSecret()
	step := uint64(when.Unix()) / 30

	for _, delta := range []int{-1, 0, 1} {
		code := totpAt(secret, uint64(int(step)+delta))
		if _, ok := VerifyTOTP(secret, code, when); !ok {
			t.Errorf("a code %d steps out should be accepted", delta)
		}
	}
	for _, delta := range []int{-2, 2, 10} {
		code := totpAt(secret, uint64(int(step)+delta))
		if _, ok := VerifyTOTP(secret, code, when); ok {
			t.Errorf("a code %d steps out should NOT be accepted", delta)
		}
	}
}

func TestTheTOTPUriIsWhatAnAuthenticatorAppExpects(t *testing.T) {
	// SHA-1 and six digits, because that is what every authenticator actually
	// implements. A service that chose SHA-256 would produce QR codes whose
	// apps reject the codes they generate, and the holder would be told their
	// phone is wrong.
	secret := []byte("12345678901234567890")
	uri := TOTPURI("Yamale", "a@b.test", secret)
	for _, want := range []string{
		"otpauth://totp/Yamale:a@b.test",
		"algorithm=SHA1", "digits=6", "period=30", "issuer=Yamale",
	} {
		if !contains(uri, want) {
			t.Errorf("the uri should carry %q: %s", want, uri)
		}
	}
}

/* --------------------------------------------------------- the email code */

func TestAnEmailCodeIsSpentWhetherOrNotItWasRight(t *testing.T) {
	// Six digits is a million guesses. Without a cap an attacker gets all of
	// them, and Argon2id does not stand between them and this one.
	_, pending, _ := NewEmailCode(when)
	for i := 0; i < emailCodeAttempts; i++ {
		if err := pending.Verify("000000", when); err == nil {
			t.Fatal("a wrong code must not verify")
		}
	}
	if pending.Attempts != 0 {
		t.Fatalf("attempts should be exhausted, %d left", pending.Attempts)
	}
}

func TestACorrectEmailCodeDoesNotWorkTwice(t *testing.T) {
	code, pending, _ := NewEmailCode(when)
	if err := pending.Verify(code, when); err != nil {
		t.Fatalf("the code should verify: %v", err)
	}
	if err := pending.Verify(code, when); err == nil {
		t.Fatal("a spent code must not verify again")
	}
}

func TestAnExpiredEmailCodeIsRefused(t *testing.T) {
	code, pending, _ := NewEmailCode(when)
	if err := pending.Verify(code, when.Add(emailCodeLifetime+time.Second)); err == nil {
		t.Fatal("an expired code must not verify")
	}
}

func TestTheEmailCodeIsNotRecoverableFromItsRecord(t *testing.T) {
	// The record holds a hash. An operator reading the service's memory must
	// not be able to replay somebody's second factor.
	code, pending, _ := NewEmailCode(when)
	sum := sha256.Sum256([]byte(code))
	if string(pending.Hash) != string(sum[:]) {
		t.Fatal("the record should hold the hash")
	}
	if contains(string(pending.Hash), code) {
		t.Fatal("the code itself must not be in the record")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		})()
}
