package main

// Which account an email opens, for the platform's own accounts API only.
//
// Money can be asked of a person by their email (traceabilityapi requests.go),
// and only an account that exists may be asked. The custodian is the one place
// that knows which emails have an account, and since 2026-11-26 every one of
// them was proved with a code at registration. This endpoint answers that one
// question for one caller: the accounts API, holding a token shared by file.
// The gate refuses the path from outside as well (yamale-account.conf), so the
// token is the second lock rather than the only one: a public "is this email
// registered" would let anybody list who has an account.
//
//	POST /v1/internal/address   X-Yamale-Lookup-Token, {email} -> {address} | 404

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"yamale/blockchain/recovery"
)

type lookupRequest struct {
	Email string `json:"email"`
}

func (s *server) handleLookup(w http.ResponseWriter, r *http.Request) {
	if s.role != recovery.RoleCustodian || len(s.lookupToken) < 32 {
		refuse(w, http.StatusNotFound, "not here")
		return
	}
	got := r.Header.Get("X-Yamale-Lookup-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.lookupToken)) != 1 {
		refuse(w, http.StatusNotFound, "not here")
		return
	}
	var req lookupRequest
	if !decode(w, r, &req) {
		return
	}
	email := enrolEmail(req.Email)
	if !strings.Contains(email, "@") {
		refuse(w, http.StatusBadRequest, "an email address is required")
		return
	}
	account, err := s.store.Get(s.index.Of(email))
	if err != nil {
		refuse(w, http.StatusNotFound, "no account opens with this email")
		return
	}
	respond(w, map[string]string{"address": account.Address})
}
