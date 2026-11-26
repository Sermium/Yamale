package main

import (
	"encoding/base32"
	"errors"
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
//	POST /v1/factors/totp/confirm email + password + code [+ proof] -> armed
//	POST /v1/factors/choose     email + password + proof + factor [+ pin] -> chosen
//
// A proof is the CURRENT factor answered (an emailed code with its nonce, an
// authenticator code, or the PIN). Choosing, and confirming an authenticator
// that is to become the chosen factor, need one: without it the password alone
// would be enough to swap the account's factor for a weaker one.

type factorRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code"`
	// Secret, optional on begin, is a base32 TOTP secret the device chose. The
	// device enrols the SAME secret at both services, so the holder keeps one
	// entry in their authenticator rather than two. It is no weaker than a
	// secret this service picks: the device is shown the secret either way.
	Secret string `json:"secret"`
	// Factor, on choose, is the factor to use from now on: email, totp or pin.
	Factor string `json:"factor"`
	// PIN, on choose with factor pin, is the new PIN.
	PIN string `json:"pin"`
	// Proof is the current factor, answered.
	Proof *factorProof `json:"proof"`
}

type factorProof struct {
	Factor string `json:"factor"`
	Answer string `json:"answer"`
	Nonce  string `json:"nonce"`
	Issued int64  `json:"issued"`
}

type factorState struct {
	// Strongest is what a recovery and a sign-in will demand: the chosen
	// factor, or the strongest enrolled. Always populated: the email code is
	// the floor, so there is always something. (Named before factors could be
	// chosen; Current says the same.)
	Strongest string `json:"strongest"`
	Current   string `json:"current"`
	TOTP      bool   `json:"totp"`
	PIN       bool   `json:"pin"`
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

	var secret []byte
	if req.Secret != "" {
		got, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimRight(strings.TrimSpace(req.Secret), "=")))
		if err != nil || len(got) != 20 {
			refuse(w, http.StatusBadRequest, "a chosen secret is 20 random bytes in base32")
			return
		}
		secret = got
	} else {
		var err error
		if secret, err = NewTOTPSecret(); err != nil {
			refuse(w, http.StatusInternalServerError, "no randomness for a secret")
			return
		}
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

	// With a proof of the current factor the authenticator also becomes the
	// chosen one. Without it, it counts only for an account that never chose
	// (the strongest wins there, as before); a chosen factor stays chosen.
	if req.Proof != nil {
		proved, ok := s.verifyProof(w, account, req.Email, enrolled, req.Proof)
		if !ok {
			return
		}
		enrolled = proved
		enrolled.Preferred = FactorTOTP
		enrolled.PINHash, enrolled.PINSalt = nil, nil
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
	current := string(Required(e))
	return factorState{
		Strongest: current,
		Current:   current,
		TOTP:      e.HasTOTP(),
		PIN:       e.HasPIN(),
		Passkey:   len(e.PasskeyCredentialID) > 0,
	}
}

// verifyProof checks the current factor, answered, and spends an emailed code.
// It writes the refusal itself, so a caller returns when it is not ok.
func (s *server) verifyProof(w http.ResponseWriter, account Account, email string, enrolled Enrolled, p *factorProof) (Enrolled, bool) {
	if p == nil || strings.TrimSpace(p.Answer) == "" {
		refuse(w, http.StatusForbidden, "answer the factor this account uses now before changing it")
		return enrolled, false
	}
	now := s.clock()
	var pending *EmailCode
	if s.codes != nil && FactorKind(p.Factor) == FactorEmail {
		pending = s.codes.pending(email, p.Nonce, p.Issued, now)
	}
	updated, err := VerifyFactor(enrolled, FactorKind(p.Factor), strings.TrimSpace(p.Answer), now, pending)
	switch {
	case err == nil:
	case errors.Is(err, ErrFactorDowngrade):
		refuse(w, http.StatusForbidden, "answer with the factor this account uses now: "+string(Required(enrolled)))
		return enrolled, false
	default:
		if errors.Is(err, ErrFactorWrong) {
			// As in a sign-in: a wrong code costs what a wrong password costs.
			s.attempts.failed(account.Index, now)
		}
		refuse(w, http.StatusUnauthorized, "that code does not open this account's factor")
		return enrolled, false
	}
	if pending != nil {
		s.codes.spend(p.Nonce, now)
	}
	return updated, true
}

// handleFactorChoose sets the factor the account answers with: the emailed
// code, a PIN (set here), or an authenticator already confirmed.
func (s *server) handleFactorChoose(w http.ResponseWriter, r *http.Request) {
	account, req, ok := s.authFactor(w, r)
	if !ok {
		return
	}
	enrolled, err := s.store.Factors(account)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	target := FactorKind(strings.TrimSpace(req.Factor))
	switch target {
	case FactorEmail, FactorPIN:
	case FactorTOTP:
		if !enrolled.HasTOTP() {
			refuse(w, http.StatusConflict, "confirm an authenticator first, at /v1/factors/totp/begin and /confirm")
			return
		}
	default:
		refuse(w, http.StatusBadRequest, "factor is email, totp or pin")
		return
	}
	if target == FactorPIN && !validPIN(req.PIN) {
		refuse(w, http.StatusBadRequest, ErrPINShape.Error())
		return
	}
	proved, ok := s.verifyProof(w, account, req.Email, enrolled, req.Proof)
	if !ok {
		return
	}
	enrolled = proved
	switch target {
	case FactorEmail:
		// Leaving the authenticator and the PIN behind: re-enrolling one later
		// starts from a fresh secret.
		enrolled.TOTPSecret, enrolled.TOTPConfirmed = nil, false
		enrolled.PINHash, enrolled.PINSalt = nil, nil
	case FactorPIN:
		if enrolled, err = enrolled.SetPIN(req.PIN); err != nil {
			refuse(w, http.StatusInternalServerError, err.Error())
			return
		}
		enrolled.TOTPSecret, enrolled.TOTPConfirmed = nil, false
	case FactorTOTP:
		enrolled.PINHash, enrolled.PINSalt = nil, nil
	}
	enrolled.Preferred = target
	if err := s.store.SetFactors(account.Index, enrolled); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond(w, stateOf(enrolled))
}
