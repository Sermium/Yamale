package main

import (
	"encoding/base32"
	"net/http"
	"strings"
)

// Enrolling a second factor, and the two-step shape that keeps it from locking
// anybody out.
//
// # Why it takes two requests
//
// A TOTP secret is useless to the account holder until their authenticator has
// it, and there is no way for this service to know whether the QR code was
// actually scanned. Storing the secret as ENROLLED at the moment it is
// generated would mean any interrupted enrolment — a closed tab, a camera that
// would not focus, a phone that ran out of battery — silently arms a factor the
// person cannot produce.
//
// The next thing they would discover is that they cannot recover their account,
// at the moment they needed to, because of a screen they abandoned months
// earlier.
//
// So the secret is stored unconfirmed and does not count. `Strongest()` ignores
// it until `TOTPConfirmed`, which is set only by the holder reading a real code
// off it. Beginning an enrolment and walking away leaves the account exactly as
// it was.
//
// # Why these are customer endpoints
//
// The rest of the recovery flow is operator-driven on loopback, because a
// customer who could recover their own account is a customer whose email is
// enough to take it. This is the opposite: it is the account holder proving,
// with their password, that they are the person who had the account, and
// nobody else can do that for them.
//
//	POST /v1/factors            email + password -> what is enrolled
//	POST /v1/factors/totp/begin email + password -> a secret and an otpauth URI
//	POST /v1/factors/totp/confirm email + password + code -> armed

type factorRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

type factorState struct {
	// Strongest is what a recovery will demand. Always populated: the email
	// code is the floor, so there is always something.
	Strongest string `json:"strongest"`
	TOTP      bool   `json:"totp"`
	Passkey   bool   `json:"passkey"`
}

type totpBegin struct {
	// Secret is base32, for somebody typing it in by hand.
	Secret string `json:"secret"`
	// URI is the otpauth:// form an authenticator scans.
	URI string `json:"uri"`
	// Next says what has to happen for this to count, because a screen that
	// shows a QR code and says nothing else invites closing the tab.
	Next string `json:"next"`
}

// issuer is what an authenticator app shows beside the code.
const issuer = "Yamale"

// authFactor decodes the request and authenticates it, returning the account
// and the email that found it.
//
// The email is returned rather than re-read because it is needed for the
// otpauth URI, which shows the account holder which account an authenticator
// entry belongs to. Returning it from the one place that proved it beats
// trimming the raw field again at the call site and hoping they agree.
func (s *server) authFactor(w http.ResponseWriter, r *http.Request) (Account, factorRequest, bool) {
	var req factorRequest
	if !decode(w, r, &req) {
		return Account{}, req, false
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || req.Password == "" {
		refuse(w, http.StatusBadRequest, "an email and a password are required")
		return Account{}, req, false
	}
	req.Email = email
	account, ok := s.authenticate(w, email, req.Password)
	return account, req, ok
}

func (s *server) handleFactors(w http.ResponseWriter, r *http.Request) {
	account, _, ok := s.authFactor(w, r)
	if !ok {
		return
	}
	enrolled, err := s.store.Factors(account)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond(w, stateOf(enrolled))
}

func (s *server) handleTOTPBegin(w http.ResponseWriter, r *http.Request) {
	account, req, ok := s.authFactor(w, r)
	if !ok {
		return
	}
	enrolled, err := s.store.Factors(account)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A confirmed factor is not silently replaced. Overwriting it would mean a
	// caller who reached this endpoint — with the password, which is one of the
	// two things a thief has — could swap the factor for one of their own,
	// which is exactly what the factor exists to stop them doing.
	if enrolled.TOTPConfirmed {
		refuse(w, http.StatusConflict,
			"this account already has an authenticator. Removing one is a recovery, not a "+
				"setting, because a password alone must not be able to replace the factor that "+
				"exists to back the password up")
		return
	}

	secret, err := NewTOTPSecret()
	if err != nil {
		refuse(w, http.StatusInternalServerError, "no randomness for a secret")
		return
	}
	// Stored UNCONFIRMED. See the note at the top: it counts for nothing until
	// the holder reads a code off it.
	enrolled.TOTPSecret = secret
	enrolled.TOTPConfirmed = false
	if err := s.store.SetFactors(account.Index, enrolled); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}

	respond(w, totpBegin{
		Secret: base32Secret(secret),
		URI:    TOTPURI(issuer, req.Email, secret),
		Next:   "scan this, then send a code to /v1/factors/totp/confirm. Until you do, nothing has changed.",
	})
}

func (s *server) handleTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	account, req, ok := s.authFactor(w, r)
	if !ok {
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		refuse(w, http.StatusBadRequest, "a code from the authenticator is required")
		return
	}

	enrolled, err := s.store.Factors(account)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(enrolled.TOTPSecret) == 0 {
		refuse(w, http.StatusConflict, "no enrolment is in progress; start at /v1/factors/totp/begin")
		return
	}
	if _, ok := VerifyTOTP(enrolled.TOTPSecret, code, s.clock()); !ok {
		// The secret is left in place. A mistyped code is the common case and
		// discarding the enrolment would send somebody back to a fresh QR for a
		// typing error.
		refuse(w, http.StatusUnauthorized,
			"that code does not match. Check the clock on the phone running the authenticator: "+
				"these codes are derived from the time, so a phone several minutes out produces "+
				"codes that are correct and unusable")
		return
	}

	enrolled.TOTPConfirmed = true
	if err := s.store.SetFactors(account.Index, enrolled); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond(w, stateOf(enrolled))
}

// base32Secret is the form somebody types into an authenticator by hand when
// the camera will not read the QR code. Unpadded, because the padding is not
// part of what these apps accept.
func base32Secret(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

func stateOf(e Enrolled) factorState {
	return factorState{
		Strongest: string(e.Strongest()),
		TOTP:      e.TOTPConfirmed && len(e.TOTPSecret) > 0,
		Passkey:   len(e.PasskeyCredentialID) > 0,
	}
}
