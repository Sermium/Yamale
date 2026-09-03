package main

import (
	"net/http"
	"strings"

	"yamale/blockchain/mpc"
	mpccosmos "yamale/blockchain/mpc/cosmos"
)

// The three endpoints a device drives to create an account.
//
//	POST /v1/enrol/start    email + password        -> session, first messages
//	POST /v1/enrol/message  session + one message   -> replies (and a poll)
//	POST /v1/enrol/finish   session + the address   -> committed, or refused
//
// The device runs the same exchange against this service and against the
// recovery deployment, routing each message by its To/Broadcast fields. It is
// the only participant that speaks to both, and it is the only one that ever
// holds its own share.

type enrolStartRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type enrolStartResponse struct {
	Session  string         `json:"session"`
	Role     string         `json:"role"`
	Outbound []mpc.Outbound `json:"outbound"`
}

type enrolMessageRequest struct {
	Session string        `json:"session"`
	Message *mpc.Outbound `json:"message,omitempty"`
}

type enrolMessageResponse struct {
	Outbound []mpc.Outbound `json:"outbound"`
	Done     bool           `json:"done"`
}

type enrolFinishRequest struct {
	Session string `json:"session"`
	// Address is what the DEVICE computed from its own share. Checked, not
	// trusted: see handleEnrolFinish.
	Address string `json:"address"`
}

type enrolFinishResponse struct {
	Address string `json:"address"`
	Role    string `json:"role"`
}

func (s *server) handleEnrolStart(w http.ResponseWriter, r *http.Request) {
	var req enrolStartRequest
	if !decode(w, r, &req) {
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || req.Password == "" {
		refuse(w, http.StatusBadRequest, "an email and a password are required")
		return
	}
	if err := checkPassword(req.Password); err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}

	index := s.index.Of(email)

	// Refused before anything expensive happens, and this is the check that
	// makes enrolment safe to expose: without it, anybody could enrol over an
	// existing email and replace that account's custodian share, which does not
	// steal the money but does destroy the ability to ever move it again.
	if _, err := s.store.Get(index); err == nil {
		// Deliberately the same shape of answer as success would be slow to
		// distinguish from — but not the same answer. Hiding this would trade a
		// real usability failure ("your account was silently not created") for
		// a weak privacy gain, since anybody can test an email by trying to
		// sign in.
		refuse(w, http.StatusConflict, ErrAlreadyEnrolled.Error())
		return
	}

	pre, err := s.preParams.Take()
	if err != nil {
		code, msg := enrolmentError(err)
		refuse(w, code, msg)
		return
	}

	// Computed now and held in memory until finish, so a generation that fails
	// or is abandoned leaves nothing on disk at all.
	verifier, err := NewVerifier(req.Password)
	if err != nil {
		refuse(w, http.StatusInternalServerError, "the password could not be prepared")
		return
	}

	en, out, err := s.enrolments.Start(s.role, index, verifier, pre)
	if err != nil {
		code, msg := enrolmentError(err)
		refuse(w, code, msg)
		return
	}
	respond(w, enrolStartResponse{Session: en.ID, Role: s.role, Outbound: out})
}

func (s *server) handleEnrolMessage(w http.ResponseWriter, r *http.Request) {
	var req enrolMessageRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Session == "" {
		refuse(w, http.StatusBadRequest, "a session is required")
		return
	}

	// A nil message is a poll. Legitimate and necessary: a round's messages can
	// become available after the response that triggered them was already
	// written, and without a way to ask again the protocol stalls one round
	// short with nothing logged.
	if req.Message == nil {
		out, done, err := s.enrolments.Poll(req.Session)
		if err != nil {
			code, msg := enrolmentError(err)
			refuse(w, code, msg)
			return
		}
		respond(w, enrolMessageResponse{Outbound: out, Done: done})
		return
	}

	if req.Message.From == s.role {
		refuse(w, http.StatusBadRequest,
			"that message is from this party; a transport echoing a broadcast back to its sender "+
				"hangs the protocol rather than failing it")
		return
	}
	out, err := s.enrolments.Handle(req.Session, *req.Message)
	if err != nil {
		code, msg := enrolmentError(err)
		refuse(w, code, msg)
		return
	}
	_, done, _ := s.enrolments.Poll(req.Session)
	respond(w, enrolMessageResponse{Outbound: out, Done: done})
}

func (s *server) handleEnrolFinish(w http.ResponseWriter, r *http.Request) {
	var req enrolFinishRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Session == "" || req.Address == "" {
		refuse(w, http.StatusBadRequest,
			"a session and the address the device computed are both required")
		return
	}

	en, share, err := s.enrolments.Finish(req.Session)
	if err != nil {
		code, msg := enrolmentError(err)
		refuse(w, code, msg)
		return
	}

	pub, err := share.PublicKey()
	if err != nil {
		s.enrolments.Close(req.Session)
		refuse(w, http.StatusInternalServerError, "the generated share carries no public key")
		return
	}
	address, err := mpccosmos.Address(pub)
	if err != nil {
		s.enrolments.Close(req.Session)
		refuse(w, http.StatusInternalServerError, "the generated key has no address")
		return
	}

	// The device tells us what IT computed, and it must match. This is the only
	// check either side gets that they generated one key rather than two, and
	// it is what a device talking through something that rewrote the traffic
	// fails on. Nothing is written when it fails.
	if req.Address != address {
		s.enrolments.Close(req.Session)
		refuse(w, http.StatusConflict,
			"the device and this service generated different keys, so nothing was saved; "+
				"start again, and if it recurs the connection is not carrying the protocol intact")
		return
	}

	// Re-checked here, not only at start. Two enrolments for one email can
	// begin before either finishes, and the one that commits second would
	// otherwise overwrite the first — replacing the share of an account whose
	// owner has already been told it exists.
	if _, err := s.store.Get(en.Index); err == nil {
		s.enrolments.Close(req.Session)
		refuse(w, http.StatusConflict, ErrAlreadyEnrolled.Error())
		return
	}

	account := Account{
		Index:    en.Index,
		Address:  address,
		Password: en.Verifier,
		Created:  nowUTC(),
	}
	if err := s.store.Put(account, share); err != nil {
		s.enrolments.Close(req.Session)
		refuse(w, http.StatusInternalServerError, "the account could not be saved")
		return
	}

	// Closed only after the share is safely on disk. The other order loses the
	// share if the write fails, and the party is the only place it exists.
	s.enrolments.Close(req.Session)

	respond(w, enrolFinishResponse{Address: address, Role: s.role})
}
