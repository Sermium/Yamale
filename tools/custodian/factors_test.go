package main

import (
	"crypto/sha256"
	"testing"
	"time"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"cosmossdk.io/math"
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

func TestAnUnknownValueAlwaysDemandsTheFactor(t *testing.T) {
	// The whole point of reading the preimage: when the service cannot tell
	// what a transaction moves, it asks for more proof rather than assuming the
	// amount was small.
	need, kind := Required(0, false, 1_000_000, Enrolled{})
	if !need {
		t.Fatal("an unvaluable transaction must demand a factor")
	}
	if kind != FactorEmail {
		t.Fatalf("want the email floor, got %q", kind)
	}
}

func TestAThresholdOfZeroDemandsItForEverything(t *testing.T) {
	if need, _ := Required(1, true, 0, Enrolled{}); !need {
		t.Fatal("zero means every signature")
	}
}

func TestBelowTheThresholdNeedsNothing(t *testing.T) {
	if need, _ := Required(999_999, true, 1_000_000, Enrolled{}); need {
		t.Fatal("a small, known payment should not demand a factor")
	}
	if need, _ := Required(1_000_000, true, 1_000_000, Enrolled{}); !need {
		t.Fatal("at the threshold is above it")
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

/* ------------------------------------------------------------ the preimage */

func signDocFor(t *testing.T, msgs ...sdk.Msg) []byte {
	t.Helper()
	body := &txtypes.TxBody{}
	for _, m := range msgs {
		any, err := codectypes.NewAnyWithValue(m)
		if err != nil {
			t.Fatal(err)
		}
		body.Messages = append(body.Messages, any)
	}
	bodyBytes, err := body.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	doc := &txtypes.SignDoc{BodyBytes: bodyBytes, ChainId: "yamale-devnet-2"}
	out, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAPreimageThatDoesNotHashToTheDigestIsRefused(t *testing.T) {
	// The check that makes everything else here worth doing. Without it the
	// caller could send an honest-looking preimage and a digest for something
	// else entirely.
	preimage := signDocFor(t, banktypes.NewMsgSend(
		sdk.AccAddress("from________________"), sdk.AccAddress("to__________________"),
		sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(5)))))

	wrong := sha256.Sum256([]byte("a different transaction"))
	if _, err := ValueOfPreimage(preimage, wrong[:]); err != ErrPreimageMismatch {
		t.Fatalf("want ErrPreimageMismatch, got %v", err)
	}
}

func TestABankSendIsValuedFromTheBytesRatherThanAsserted(t *testing.T) {
	preimage := signDocFor(t, banktypes.NewMsgSend(
		sdk.AccAddress("from________________"), sdk.AccAddress("to__________________"),
		sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(2_500_000)))))
	digest := sha256.Sum256(preimage)

	got, err := ValueOfPreimage(preimage, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if !got.Known {
		t.Fatalf("a bank send should be valuable: %s", got.Why)
	}
	if got.PerDenom["uyml"] != 2_500_000 {
		t.Fatalf("want 2500000 uyml, got %d", got.PerDenom["uyml"])
	}
	if got.Highest() != 2_500_000 {
		t.Fatalf("highest should be 2500000, got %d", got.Highest())
	}
}

func TestAnUnrecognisedMessageMakesTheWholeTransactionUnvaluable(t *testing.T) {
	// The attack this closes: hide the payment behind one message this binary
	// has never heard of, and a service that valued the rest would wave it
	// through as small.
	preimage := signDocFor(t,
		banktypes.NewMsgSend(
			sdk.AccAddress("from________________"), sdk.AccAddress("to__________________"),
			sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(1)))),
		// A governance vote stands in for "a type this file does not list".
		&banktypes.MsgSetSendEnabled{Authority: "yml1authority"},
	)
	digest := sha256.Sum256(preimage)

	got, err := ValueOfPreimage(preimage, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if got.Known {
		t.Fatal("one unvaluable message must make the whole transaction unvaluable")
	}
	if got.Why == "" {
		t.Fatal("it should name what it could not value")
	}
	if need, _ := Required(got.Highest(), got.Known, 1_000_000_000, Enrolled{}); !need {
		t.Fatal("and that must demand the factor, however small the rest was")
	}
}

func TestTheFeeCountsBecauseItLeavesTheAccountToo(t *testing.T) {
	body := &txtypes.TxBody{}
	bodyBytes, _ := body.Marshal()
	authInfo := &txtypes.AuthInfo{Fee: &txtypes.Fee{
		Amount: sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(9_000))),
	}}
	authBytes, _ := authInfo.Marshal()
	doc := &txtypes.SignDoc{BodyBytes: bodyBytes, AuthInfoBytes: authBytes}
	preimage, _ := doc.Marshal()
	digest := sha256.Sum256(preimage)

	got, err := ValueOfPreimage(preimage, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if got.PerDenom["uyml"] != 9_000 {
		t.Fatalf("the fee should be counted, got %d", got.PerDenom["uyml"])
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
