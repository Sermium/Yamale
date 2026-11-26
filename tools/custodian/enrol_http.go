package main

import (
	"net/http"
	"strings"
)

// The one endpoint a device drives to create an account.
//
//	POST /v1/enrol   email + password + pubkey + holding + signature -> address
//
// It replaced three endpoints and a session store. The device generates a key,
// seals it under the password, splits the sealed blob, and posts one holding
// here and the other to the recovery deployment. Neither service talks to the
// other, and neither holds any state between requests.
//
// The device is still the only participant that speaks to both, and it is still
// the only one that ever holds its own holding. What changed is that there is
// no longer a protocol for anything to go wrong in the middle of.

type enrolRequest struct {
	Deposit
	// The registration code emailed by /v1/enrol/code (enrol_verify.go).
	Nonce  string `json:"nonce"`
	Issued int64  `json:"issued"`
	Code   string `json:"code"`
}

type enrolResponse struct {
	Address string `json:"address"`
	Role    string `json:"role"`
}

func (s *server) handleEnrol(w http.ResponseWriter, r *http.Request) {
	var req enrolRequest
	if !decode(w, r, &req) {
		return
	}
	if !s.checkEnrolCode(w, enrolEmail(req.Email), req.Nonce, req.Issued, req.Code) {
		return
	}
	address, err := s.Enrol(req.Deposit)
	if err == nil && s.enrolNeedsEmailCode && s.codes != nil {
		s.codes.spend(req.Nonce, s.clock())
	}
	if err != nil {
		code, msg := enrolmentError(err)
		refuse(w, code, msg)
		return
	}
	respond(w, enrolResponse{Address: address, Role: s.role})
}

// handleEnrolGone answers the endpoints that used to carry the threshold
// protocol.
//
// 410 rather than 404, and with the reason in it. A device built against the
// old three-endpoint exchange is out there in somebody's browser cache, and the
// difference between "this service is broken" and "this client is too old"
// costs an afternoon to work out from a 404.
func handleEnrolGone(w http.ResponseWriter, r *http.Request) {
	refuse(w, http.StatusGone,
		"this service no longer runs a threshold key generation; enrolment is a single "+
			"POST to /v1/enrol. Reload the application to get the current client.")
}

// enrolEmail is here so the tests and the handler agree on trimming.
func enrolEmail(raw string) string { return strings.TrimSpace(raw) }
