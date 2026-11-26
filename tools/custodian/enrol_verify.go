package main

// Proving the email at registration (since 2026-11-26).
//
// Until then an enrolment proved everything about the key and nothing about
// the email: anybody could register an address they did not own, before its
// owner did. With money that waits for an email until somebody registers it,
// that would be theft, so an enrolment now carries a code sent to that email:
//
//	POST /v1/enrol/code   email (+ lang) -> a code is emailed; nonce, issued
//	POST /v1/enrol        the deposit + nonce, issued, code
//
// Both services check the code, with the same shared key as the sign-in codes,
// but under a subject of its own ("enrol:" + email): a registration code is not
// a sign-in code, nor the reverse. Wrong codes are throttled like passwords.

import (
	"net/http"
	"strings"
	"time"

	"yamale/blockchain/recovery"
)

type enrolCodeRequest struct {
	Email string `json:"email"`
	Lang  string `json:"lang"`
}

type enrolCodeResponse struct {
	Nonce  string `json:"nonce"`
	Issued int64  `json:"issued"`
}

func enrolCodeSubject(email string) string { return "enrol:" + strings.ToLower(email) }

// handleEnrolCode emails a registration code. Custodian only: it has the mailer.
func (s *server) handleEnrolCode(w http.ResponseWriter, r *http.Request) {
	if s.role != recovery.RoleCustodian {
		refuse(w, http.StatusNotFound, "a registration code is sent by the custodian")
		return
	}
	if s.codes == nil || s.mailer == nil {
		refuse(w, http.StatusServiceUnavailable, "this service cannot send a registration code")
		return
	}
	var req enrolCodeRequest
	if !decode(w, r, &req) {
		return
	}
	email := enrolEmail(req.Email)
	if !strings.Contains(email, "@") || strings.ContainsAny(email, " \r\n") {
		refuse(w, http.StatusBadRequest, "an email address is required")
		return
	}
	index := s.index.Of(email)
	if _, err := s.store.Get(index); err == nil {
		refuse(w, http.StatusConflict, "this email already has an account; sign in instead")
		return
	}
	now := s.clock()
	if ok, wait := s.codes.mayEmail("enrol:"+index, now); !ok {
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
	subject, body := enrolCodeEmail(req.Lang, s.codes.code(enrolCodeSubject(email), nonce, issued))
	if err := s.mailer.Send(email, subject, body); err != nil {
		refuse(w, http.StatusServiceUnavailable, "the code could not be sent; check the address, or try again in a few minutes")
		return
	}
	respond(w, enrolCodeResponse{Nonce: nonce, Issued: issued})
}

// checkEnrolCode verifies the registration code of an enrolment, writing the
// refusal itself. A deployment that requires it and cannot check it refuses.
func (s *server) checkEnrolCode(w http.ResponseWriter, email, nonce string, issued int64, code string) bool {
	if !s.enrolNeedsEmailCode {
		return true
	}
	if s.codes == nil {
		refuse(w, http.StatusServiceUnavailable, "this service cannot check the email code a registration needs")
		return false
	}
	now := s.clock()
	key := "enrol:" + s.index.Of(email)
	if wait := s.attempts.wait(key, now); wait > 0 {
		w.Header().Set("Retry-After", retryAfter(wait))
		refuse(w, http.StatusTooManyRequests, "too many wrong codes; wait "+wait.Round(time.Second).String())
		return false
	}
	pending := s.codes.pending(enrolCodeSubject(email), nonce, issued, now)
	if pending == nil {
		refuse(w, http.StatusUnauthorized, "the email code has expired or was already used; ask for a new one")
		return false
	}
	if err := pending.Verify(strings.TrimSpace(code), now); err != nil {
		s.attempts.failed(key, now)
		refuse(w, http.StatusUnauthorized, "that email code is not right")
		return false
	}
	s.attempts.succeeded(key)
	return true
}

func enrolCodeEmail(lang, code string) (string, string) {
	if strings.HasPrefix(lang, "fr") {
		return "Votre code Yamale : " + code,
			"Pour créer votre compte Yamale avec cette adresse, saisissez le code " + code + ".\n\n" +
				"Il est valable 10 minutes. Si vous n'êtes pas en train de créer un compte, ignorez ce message : sans ce code, personne ne peut utiliser votre adresse.\n"
	}
	return "Your Yamale code: " + code,
		"To create your Yamale account with this address, enter the code " + code + ".\n\n" +
			"It works for 10 minutes. If you are not creating an account, ignore this message: without this code nobody can use your address.\n"
}
