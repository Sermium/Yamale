package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"yamale/blockchain/recovery"
)

// Signing in on a new device: email, password and second factor, and nothing else.
//
// # What changed, and why it is still not custodial
//
// Until 2026-11-25 the only way back into an account from another device was
// the operator-run recovery: two approvers, 72 hours' notice, a 24-hour hold,
// and a recovery number to quote. People who signed out could not sign in
// again, and made a second account instead.
//
// Signing in releases this service's holding to whoever proves the password
// AND the account's second factor, at once. The holding is a share of a
// ciphertext sealed under that same password: useless alone, useless with the
// other service's share alone, openable only on a device that also has the
// password. So "nobody, us included, can use your key" stays true; what changed
// is that the password and the second factor are now enough to fetch the
// sealed copies, as they are for any ordinary account.
//
// The slow recovery stays, for somebody who has lost their second factor. While
// one is open for an account, signing in is refused: the open recovery is
// somebody's claim that the account is not in its owner's hands.
//
// # One emailed code, checked by two services
//
// Each service checks the second factor itself, so neither trusts the other's
// word. An authenticator is easy: the device enrols one secret at both. An
// emailed code is not, because the custodian is the only one with a mailer,
// and two emails with two codes would be a bad joke.
//
// So the code is not random. It is an HMAC, under a key both services hold
// (--signin-code-key-file), over the email, a random nonce the custodian picks
// and the time it was issued. The custodian emails it; the device sends the
// nonce and the time to both services with the code; each recomputes it. A
// nonce is spent once per service, and a wrong code counts as a wrong password
// in the per-account wait, so guessing six digits is as slow as guessing a
// password.
//
// Holding that key lets a service compute the code for any account, which is a
// second factor's worth of power and no more: the password is still checked
// first, and still needed to open what is released.

// signinCodeLifetime is how long an emailed code is accepted.
const signinCodeLifetime = 10 * time.Minute

// signinCodes computes and checks the emailed sign-in codes.
type signinCodes struct {
	key []byte

	mu    sync.Mutex
	spent map[string]time.Time   // nonce -> when it stops mattering
	sends map[string][]time.Time // blind index -> recent sends
}

func newSigninCodes(key []byte) *signinCodes {
	return &signinCodes{key: key, spent: map[string]time.Time{}, sends: map[string][]time.Time{}}
}

// code is the six digits for this email, nonce and issue time.
func (c *signinCodes) code(email, nonce string, issued int64) string {
	m := hmac.New(sha256.New, c.key)
	fmt.Fprintf(m, "yamale-signin-code/v1\x00%s\x00%s\x00%d", email, nonce, issued)
	n := binary.BigEndian.Uint32(m.Sum(nil)[:4])
	return fmt.Sprintf("%06d", n%1_000_000)
}

// pending is the record VerifyFactor checks an emailed code against, or nil
// when the nonce is unusable: spent, malformed, or outside its lifetime.
func (c *signinCodes) pending(email, nonce string, issued int64, now time.Time) *EmailCode {
	if len(nonce) < 16 || len(nonce) > 64 {
		return nil
	}
	at := time.Unix(issued, 0)
	if now.Before(at.Add(-time.Minute)) || now.After(at.Add(signinCodeLifetime)) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, used := c.spent[nonce]; used {
		return nil
	}
	sum := sha256.Sum256([]byte(c.code(email, nonce, issued)))
	return &EmailCode{Hash: sum[:], Expires: at.Add(signinCodeLifetime), Attempts: 1}
}

// spend marks a nonce used, so a code that worked cannot work again here.
func (c *signinCodes) spend(nonce string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for n, until := range c.spent {
		if now.After(until) {
			delete(c.spent, n)
		}
	}
	c.spent[nonce] = now.Add(signinCodeLifetime + time.Minute)
}

// mayEmail allows one code a minute and five an hour per account, and records
// the send when it allows one. Every send is an email to a real person.
func (c *signinCodes) mayEmail(index string, now time.Time) (bool, time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	recent := c.sends[index][:0]
	for _, at := range c.sends[index] {
		if now.Sub(at) < time.Hour {
			recent = append(recent, at)
		}
	}
	c.sends[index] = recent
	if n := len(recent); n > 0 && now.Sub(recent[n-1]) < time.Minute {
		return false, time.Minute - now.Sub(recent[n-1])
	}
	if len(recent) >= 5 {
		return false, time.Hour - now.Sub(recent[0])
	}
	c.sends[index] = append(recent, now)
	return true, 0
}

func newNonce() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

/* ------------------------------------------------------------- endpoints */

type signinStartRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Lang     string `json:"lang"`
}

type signinStartResponse struct {
	// Factor is what the sign-in will ask for: "totp" or "email".
	Factor string `json:"factor"`
	// Nonce and Issued identify the emailed code, when one was sent.
	Nonce  string `json:"nonce,omitempty"`
	Issued int64  `json:"issued,omitempty"`
}

// handleSigninStart checks the password and says which factor to answer with,
// emailing a code when that factor is the email. Custodian only: it is the one
// with a mailer. Naming the factor is safe after the password, for the reason
// factorReason gives.
func (s *server) handleSigninStart(w http.ResponseWriter, r *http.Request) {
	if s.role != recovery.RoleCustodian {
		refuse(w, http.StatusNotFound, "a sign-in starts at the custodian")
		return
	}
	var req signinStartRequest
	if !decode(w, r, &req) {
		return
	}
	email := enrolEmail(req.Email)
	if email == "" || req.Password == "" {
		refuse(w, http.StatusBadRequest, "the email and the password are required")
		return
	}
	account, ok := s.authenticate(w, email, req.Password)
	if !ok {
		return
	}
	if !s.mayReleaseFor(w, account) {
		return
	}
	enrolled, err := s.store.Factors(account)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	switch Required(enrolled) {
	case FactorTOTP:
		respond(w, signinStartResponse{Factor: string(FactorTOTP)})
		return
	case FactorPIN:
		respond(w, signinStartResponse{Factor: string(FactorPIN)})
		return
	case FactorPasskey:
		refuse(w, http.StatusServiceUnavailable, "this account's second factor is a passkey, which this service cannot check yet")
		return
	}
	if s.codes == nil || s.mailer == nil {
		refuse(w, http.StatusServiceUnavailable, "this account's second factor is an emailed code, and this service cannot send one")
		return
	}
	now := s.clock()
	if ok, wait := s.codes.mayEmail(account.Index, now); !ok {
		w.Header().Set("Retry-After", retryAfter(wait))
		refuse(w, http.StatusTooManyRequests, "a code was sent a moment ago; check your inbox, or try again in "+wait.Round(time.Second).String())
		return
	}
	nonce, err := newNonce()
	if err != nil {
		refuse(w, http.StatusInternalServerError, "no randomness for a code")
		return
	}
	issued := now.Unix()
	subject, body := signinCodeEmail(req.Lang, s.codes.code(email, nonce, issued))
	if err := s.mailer.Send(email, subject, body); err != nil {
		log.Printf("sign-in code not sent to %s: %v", account.Address, err)
		refuse(w, http.StatusServiceUnavailable, "the code could not be sent; try again in a few minutes")
		return
	}
	respond(w, signinStartResponse{Factor: string(FactorEmail), Nonce: nonce, Issued: issued})
}

type signinRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Factor   string `json:"factor"`
	Answer   string `json:"answer"`
	Nonce    string `json:"nonce"`
	Issued   int64  `json:"issued"`
	// Device names the browser in the new-sign-in notice, so its owner can tell
	// their own sign-in from somebody else's. Free text, shortened.
	Device string `json:"device"`
	Lang   string `json:"lang"`
}

// handleSignin releases this service's holding to the account holder.
func (s *server) handleSignin(w http.ResponseWriter, r *http.Request) {
	var req signinRequest
	if !decode(w, r, &req) {
		return
	}
	email := enrolEmail(req.Email)
	if email == "" || req.Password == "" || req.Answer == "" {
		refuse(w, http.StatusBadRequest, "the email, the password and the code are all required")
		return
	}
	account, ok := s.authenticate(w, email, req.Password)
	if !ok {
		return
	}
	if !s.mayReleaseFor(w, account) {
		return
	}
	enrolled, err := s.store.Factors(account)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := s.clock()
	var pending *EmailCode
	if s.codes != nil && FactorKind(req.Factor) == FactorEmail {
		pending = s.codes.pending(email, req.Nonce, req.Issued, now)
	}
	updated, err := VerifyFactor(enrolled, FactorKind(req.Factor), req.Answer, now, pending)
	if err != nil {
		if errors.Is(err, ErrFactorWrong) {
			// A wrong code counts as a wrong password: six digits must not be
			// cheaper to guess than the password in front of them.
			s.attempts.failed(account.Index, now)
		}
		if errors.Is(err, ErrFactorRequired) && pending == nil && FactorKind(req.Factor) == FactorEmail && s.codes != nil {
			refuse(w, http.StatusUnauthorized, "that code has expired or was already used; ask for a new one")
			return
		}
		refuse(w, factorStatus(err), factorReason(err, Required(enrolled)))
		return
	}
	if pending != nil {
		s.codes.spend(req.Nonce, now)
	}
	if err := s.store.SetFactors(account.Index, updated); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	holding, err := s.store.Holding(account)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("signed in: %s", account.Address)
	if s.role == recovery.RoleCustodian && s.mailer != nil {
		subject, body := signinNoticeEmail(req.Lang, now, shorten(req.Device, 120))
		go func() {
			if err := s.mailer.Send(email, subject, body); err != nil {
				log.Printf("sign-in notice not sent to %s: %v", account.Address, err)
			}
		}()
	}
	respond(w, claimResponse{
		Role: s.role, Address: account.Address, Holding: holding,
		Next: "fetch the other service's holding the same way, then open both with this password on your device",
	})
}

// mayReleaseFor refuses while a slow recovery is open for the account.
func (s *server) mayReleaseFor(w http.ResponseWriter, account Account) bool {
	open, err := s.recoveries.OpenFor(account.Index)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if open != nil {
		refuse(w, http.StatusConflict, "a recovery is open for this account; signing in waits until it is finished or cancelled")
		return false
	}
	return true
}

func shorten(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, strings.TrimSpace(s))
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

/* ---------------------------------------------------------------- emails */

func signinCodeEmail(lang, code string) (string, string) {
	if strings.HasPrefix(lang, "fr") {
		return "Votre code Yamale : " + code,
			"Votre code de connexion Yamale est " + code + ".\n\n" +
				"Il est valable 10 minutes. Si vous n'essayez pas de vous connecter, quelqu'un connaît votre mot de passe : changez-le et ignorez ce message.\n"
	}
	return "Your Yamale code: " + code,
		"Your Yamale sign-in code is " + code + ".\n\n" +
			"It works for 10 minutes. If you are not signing in, somebody knows your password: change it, and ignore this message.\n"
}

func signinNoticeEmail(lang string, at time.Time, device string) (string, string) {
	when := at.UTC().Format("2006-01-02 15:04") + " UTC"
	if device == "" {
		device = "?"
	}
	if strings.HasPrefix(lang, "fr") {
		return "Nouvelle connexion à votre compte Yamale",
			"Votre compte Yamale a été ouvert sur un nouvel appareil le " + when + " (" + device + ").\n\n" +
				"Si c'était vous, il n'y a rien à faire. Sinon, contactez la plateforme sans attendre : elle peut geler le compte.\n"
	}
	return "New sign-in to your Yamale account",
		"Your Yamale account was opened on a new device on " + when + " (" + device + ").\n\n" +
			"If this was you, there is nothing to do. If not, contact the platform at once: it can freeze the account.\n"
}
