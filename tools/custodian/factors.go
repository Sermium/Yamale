package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The second factor, demanded when a RECOVERY is asked for.
//
// # Where this is used, and where it deliberately is not
//
// It was written to gate signing, on the reasoning that gating the login
// protects the session while gating the signature protects the money. That
// reasoning is sound and it is not the architecture this platform chose.
//
// The decision (2026-09-18) is that a deal is between a buyer and a seller and
// the custodian appears only in a recovery. The device signs alone, so there is
// no second party in an ordinary payment to demand anything — and a factor that
// nobody is in a position to ask for is not a control, it is a field.
//
// What remains, and what this file is for, is RECOVERY: the one moment the
// services are in the path by design. Recovery already requires two approvers
// from different teams over 72 hours, and every one of those is somebody other
// than the account holder. This is the account holder's own half of that — the
// proof that the person asking for the account back is the person who had it,
// rather than somebody who learned an email address and a password.
//
// Nothing here is wired into the signing path and nothing should be. See
// docs/scope/decisions-2026-09-18.md.
//
// # A ladder, not a setting
//
// An account uses the STRONGEST factor it has enrolled, and the service decides
// which that is rather than the caller:
//
//	passkey  >  TOTP  >  emailed code
//
// The caller says which factor it is answering with, and the service checks
// that against the ladder. A client that offered the emailed code to an account
// with a passkey would be a client choosing the weaker of two factors on the
// account holder's behalf, which is the whole of a downgrade attack.
//
// The email code is the floor rather than an option: every account has an email
// — it is the account — so there is always something to demand. That makes the
// factor unskippable, which matters more than any one factor being strong,
// because the alternative in a system where enrolment is optional is an
// attacker simply finding an account that enrolled nothing.
//
// It is also the weakest rung by some distance, and worth saying why it is here
// at all: the account IS an email address, so somebody reading the mailbox has
// both factors. It defends against a stolen password and a stolen device. It
// does not defend against a compromised mailbox, and nothing that sends codes
// to an email can.

// FactorKind is a rung on the ladder. The zero value is no factor at all.
type FactorKind string

const (
	FactorNone    FactorKind = ""
	FactorEmail   FactorKind = "email"
	FactorTOTP    FactorKind = "totp"
	FactorPasskey FactorKind = "passkey"
)

// strength orders the ladder. Higher wins.
func (k FactorKind) strength() int {
	switch k {
	case FactorPasskey:
		return 3
	case FactorTOTP:
		return 2
	case FactorEmail:
		return 1
	default:
		return 0
	}
}

// Enrolled is what an account has registered, beyond its password.
//
// Stored on the account record. The TOTP secret is sealed with the rest of it,
// which is worth being clear about: a custodian whose store is stolen has every
// TOTP secret it holds. That is a real weakening and it is survivable for one
// reason only — the custodian still holds one share of three and cannot sign
// with it, so the second factor is not what stands between a thief and the
// money. It is what stands between a thief WITH THE OTHER TWO and the money.
type Enrolled struct {
	// TOTPSecret is the shared secret, raw. Empty means not enrolled.
	TOTPSecret []byte `json:"totp_secret,omitempty"`
	// TOTPConfirmed guards against an account locked out by an enrolment that
	// was never completed: a secret is stored when the QR is shown, and only
	// counts once the holder has proved they can read a code from it.
	TOTPConfirmed bool `json:"totp_confirmed,omitempty"`

	// Passkey is reserved and deliberately inert. See Required.
	PasskeyCredentialID []byte `json:"passkey_credential_id,omitempty"`

	// LastUsedCounter is the TOTP time step most recently accepted, which is
	// what stops a code being replayed inside its own validity window. A code
	// is valid for thirty seconds; without this, so is an attacker who reads it
	// over a shoulder.
	LastUsedCounter uint64 `json:"last_used_counter,omitempty"`
}

// Strongest is the rung this account will be held to.
func (e Enrolled) Strongest() FactorKind {
	switch {
	case len(e.PasskeyCredentialID) > 0:
		return FactorPasskey
	case e.TOTPConfirmed && len(e.TOTPSecret) > 0:
		return FactorTOTP
	default:
		return FactorEmail
	}
}

var (
	// ErrFactorRequired is returned when the value needs a factor and none was
	// offered.
	ErrFactorRequired = errors.New("this recovery needs a second factor")
	// ErrFactorWrong is a factor that was offered and did not verify.
	ErrFactorWrong = errors.New("the second factor did not verify")
	// ErrFactorDowngrade is an attempt to answer with a weaker factor than the
	// account has enrolled.
	ErrFactorDowngrade = errors.New(
		"this account has a stronger factor enrolled; answer with that one")
	// ErrPasskeyNotBuilt is honest rather than silent. An account that somehow
	// carries a passkey credential cannot sign until the verification exists,
	// which is the correct direction to fail in.
	ErrPasskeyNotBuilt = errors.New(
		"passkey verification is not implemented; this account cannot sign until it is")
)

// Required says which factor an account must answer with.
//
// A recovery always needs one — there is no threshold and no exemption, because
// a recovery is by definition somebody asking for an account they cannot
// currently reach, which is the same sentence an attacker says.
func Required(e Enrolled) FactorKind {
	return e.Strongest()
}

// VerifyFactor checks an offered factor against what the account has enrolled.
//
// `offered` is the kind the caller says it is answering with, and it is checked
// against the ladder before the value is looked at. Accepting a weaker kind
// because it happened to verify would let a client downgrade an account with a
// passkey to an emailed code by simply asking to.
func VerifyFactor(
	e Enrolled, offered FactorKind, answer string, now time.Time, pending *EmailCode,
) (Enrolled, error) {
	want := e.Strongest()
	if offered.strength() < want.strength() {
		return e, ErrFactorDowngrade
	}

	switch want {
	case FactorPasskey:
		return e, ErrPasskeyNotBuilt

	case FactorTOTP:
		step, ok := VerifyTOTP(e.TOTPSecret, answer, now)
		if !ok {
			return e, ErrFactorWrong
		}
		// Replay inside the window. A thirty-second code read over a shoulder
		// is a thirty-second window for somebody else to use it, unless the
		// step it came from is refused once it has been spent.
		if step <= e.LastUsedCounter {
			return e, ErrFactorWrong
		}
		e.LastUsedCounter = step
		return e, nil

	default:
		if pending == nil {
			return e, ErrFactorRequired
		}
		if err := pending.Verify(answer, now); err != nil {
			return e, err
		}
		return e, nil
	}
}

/* ------------------------------------------------------------------ TOTP */

// TOTP, RFC 6238: HMAC-SHA1 over a thirty-second counter, six digits.
//
// SHA-1 rather than SHA-256 not because it is better but because it is what
// every authenticator application actually implements. A service that chose
// SHA-256 here would be one whose QR code produces codes that Google
// Authenticator rejects, and the user would be told their phone is wrong.

const (
	totpStep   = 30 * time.Second
	totpDigits = 6
	// totpSkew is how many steps either side are accepted, for clock drift. One
	// step each way is ninety seconds of validity in total, which is the usual
	// compromise: zero rejects people whose phone is a few seconds out, and
	// more than one widens the window an intercepted code is useful in.
	totpSkew = 1
)

// NewTOTPSecret makes a secret for a new enrolment.
func NewTOTPSecret() ([]byte, error) {
	secret := make([]byte, 20) // 160 bits, the RFC 4226 recommendation
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	return secret, nil
}

// TOTPURI is what goes into the QR code an authenticator app scans.
//
// The label carries the email so somebody with several accounts can tell them
// apart in the app, which means the email is in the QR — on the holder's own
// screen, for the holder's own phone. It is not stored anywhere by this service:
// see BlindIndex.
func TOTPURI(issuer, email string, secret []byte) string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	return fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		issuer, email, enc, issuer, totpDigits, int(totpStep.Seconds()))
}

// VerifyTOTP checks a code and returns the time step it came from.
//
// The step is returned so the caller can refuse to accept the same one twice.
func VerifyTOTP(secret []byte, code string, now time.Time) (uint64, bool) {
	code = strings.TrimSpace(code)
	if len(secret) == 0 || len(code) != totpDigits {
		return 0, false
	}
	counter := uint64(now.Unix()) / uint64(totpStep.Seconds())
	for delta := -totpSkew; delta <= totpSkew; delta++ {
		step := counter + uint64(delta)
		// Constant time, because a comparison that returns early tells an
		// attacker how much of a code they got right.
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, step)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func totpAt(secret []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// Dynamic truncation, RFC 4226 §5.4.
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])

	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, value%mod)
}

/* ------------------------------------------------------------ email codes */

// EmailCode is a one-time code sent to the account's address.
//
// Held in memory rather than in the store, deliberately: it is worthless after
// a few minutes, and writing it beside the share would put a live credential in
// the file an operator is told to back up.
type EmailCode struct {
	// Hash is the code's SHA-256, never the code.
	Hash []byte
	// Expires is when it stops being accepted.
	Expires time.Time
	// Attempts left before it is burned. Six digits is a million guesses, and
	// without a cap an attacker gets all of them.
	Attempts int
}

const (
	emailCodeDigits   = 6
	emailCodeLifetime = 10 * time.Minute
	emailCodeAttempts = 5
)

// NewEmailCode makes a code and the record that will check it.
//
// Returns the code itself exactly once, for sending. It is not recoverable
// afterwards — the record holds only its hash, so an operator reading the
// service's memory or its logs cannot replay somebody's second factor.
func NewEmailCode(now time.Time) (string, *EmailCode, error) {
	var n uint32
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, err
	}
	n = binary.BigEndian.Uint32(buf)

	mod := uint32(1)
	for i := 0; i < emailCodeDigits; i++ {
		mod *= 10
	}
	code := fmt.Sprintf("%0*d", emailCodeDigits, n%mod)

	sum := sha256.Sum256([]byte(code))
	return code, &EmailCode{
		Hash:     sum[:],
		Expires:  now.Add(emailCodeLifetime),
		Attempts: emailCodeAttempts,
	}, nil
}

// Verify checks a code, spending one attempt whether or not it is right.
func (c *EmailCode) Verify(answer string, now time.Time) error {
	if c == nil {
		return ErrFactorRequired
	}
	if now.After(c.Expires) {
		return ErrFactorWrong
	}
	if c.Attempts <= 0 {
		return ErrFactorWrong
	}
	c.Attempts--

	sum := sha256.Sum256([]byte(strings.TrimSpace(answer)))
	if subtle.ConstantTimeCompare(sum[:], c.Hash) != 1 {
		return ErrFactorWrong
	}
	// Spent. A correct code must not work twice, for the same reason a TOTP
	// step must not.
	c.Attempts = 0
	return nil
}
