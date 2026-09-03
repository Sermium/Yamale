package main

import (
	"net/http"
	"strings"
	"time"
)

// The recovery endpoints.
//
// These are OPERATOR endpoints, not customer ones, and the distinction is the
// point: a customer cannot recover their own account, because an attacker
// holding a customer's email could then do it too. What a customer can do is
// CANCEL — which needs no authority at all, because the worst a malicious
// cancellation achieves is that a real recovery has to be started again.
//
//	POST /v1/recovery/initiate   operator, team, proof -> the clock starts
//	POST /v1/recovery/approve    a second operator, a different team
//	POST /v1/recovery/complete   after 72 hours and two approvals
//	POST /v1/recovery/cancel     anybody, any time before completion
//	GET  /v1/recovery/statistics counts and durations, never who
//
// There is deliberately no authentication on these beyond the transport. This
// service listens on loopback and expects an operator console in front of it
// that knows who is logged in — putting a second, weaker identity system in
// here would be the thing everybody then trusts.

type recoveryInitiateRequest struct {
	Email    string `json:"email"`
	Operator string `json:"operator"`
	Team     string `json:"team"`
	Proof    string `json:"proof"`
}

type recoveryApproveRequest struct {
	ID       string `json:"id"`
	Operator string `json:"operator"`
	Team     string `json:"team"`
}

type recoveryIDRequest struct {
	ID     string `json:"id"`
	By     string `json:"by"`
	Reason string `json:"reason"`
}

// recoveryView is what these endpoints return. It never carries the email, the
// blind index or anything about the share.
type recoveryView struct {
	ID          string     `json:"id"`
	State       string     `json:"state"`
	InitiatedAt time.Time  `json:"initiated_at"`
	EligibleAt  time.Time  `json:"eligible_at"`
	Approvals   []Approval `json:"approvals,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	FrozenUntil *time.Time `json:"frozen_until,omitempty"`
	Outcome     string     `json:"outcome,omitempty"`
	// Blocking is what still stands between this request and completion, in
	// words. An operator staring at a request that will not complete should not
	// have to work out which of two rules is holding it.
	Blocking string `json:"blocking,omitempty"`
}

func viewOf(r *Recovery, now time.Time) recoveryView {
	v := recoveryView{
		ID: r.ID, State: string(r.State),
		InitiatedAt: r.InitiatedAt, EligibleAt: r.EligibleAt,
		Approvals: r.Approvals, CompletedAt: r.CompletedAt,
		FrozenUntil: r.FrozenUntil, Outcome: r.Outcome,
	}
	if r.State == RecoveryPending || r.State == RecoveryApproved {
		if err := r.Eligible(now); err != nil {
			v.Blocking = err.Error()
		}
	}
	return v
}

func (s *server) handleRecoveryInitiate(w http.ResponseWriter, r *http.Request) {
	var req recoveryInitiateRequest
	if !decode(w, r, &req) {
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		refuse(w, http.StatusBadRequest, "an email is required")
		return
	}
	index := s.index.Of(email)

	// The account must exist. Opening a recovery against an email nobody holds
	// would let anybody use this endpoint to find out which emails are enrolled.
	account, err := s.store.Get(index)
	if err != nil {
		refuse(w, http.StatusNotFound, "no account matches that email")
		return
	}

	rec, err := s.recoveries.Initiate(index, req.Operator, req.Team, req.Proof, func() error {
		return s.notifier.RecoveryStarted(account, s.recoveryDelay())
	})
	if err != nil {
		refuse(w, recoveryStatus(err), err.Error())
		return
	}
	respond(w, viewOf(rec, nowUTC()))
}

func (s *server) handleRecoveryApprove(w http.ResponseWriter, r *http.Request) {
	var req recoveryApproveRequest
	if !decode(w, r, &req) {
		return
	}
	rec, err := s.recoveries.Approve(req.ID, req.Operator, req.Team)
	if err != nil {
		refuse(w, recoveryStatus(err), err.Error())
		return
	}
	respond(w, viewOf(rec, nowUTC()))
}

func (s *server) handleRecoveryComplete(w http.ResponseWriter, r *http.Request) {
	var req recoveryIDRequest
	if !decode(w, r, &req) {
		return
	}
	rec, err := s.recoveries.Complete(req.ID)
	if err != nil {
		refuse(w, recoveryStatus(err), err.Error())
		return
	}
	// Notice again on completion. The owner who missed the first message has a
	// second chance to notice during the 24 hours in which nothing can leave.
	if account, err := s.store.Get(rec.Index); err == nil && rec.FrozenUntil != nil {
		if err := s.notifier.RecoveryCompleted(account, *rec.FrozenUntil); err != nil {
			// Logged, not fatal: the recovery IS complete, and pretending
			// otherwise would leave the record disagreeing with the state.
			s.notifier.Problem("notice of a completed recovery could not be sent", err)
		}
	}
	respond(w, viewOf(rec, nowUTC()))
}

func (s *server) handleRecoveryCancel(w http.ResponseWriter, r *http.Request) {
	var req recoveryIDRequest
	if !decode(w, r, &req) {
		return
	}
	if req.By == "" {
		req.By = "the account holder"
	}
	rec, err := s.recoveries.Cancel(req.ID, req.By, req.Reason)
	if err != nil {
		refuse(w, recoveryStatus(err), err.Error())
		return
	}
	respond(w, viewOf(rec, nowUTC()))
}

func (s *server) handleRecoveryStatistics(w http.ResponseWriter, _ *http.Request) {
	respond(w, s.recoveries.Statistics())
}

func (s *server) recoveryDelay() time.Duration { return RecoveryDelay }

func recoveryStatus(err error) int {
	switch {
	case err == ErrNoRecovery:
		return http.StatusNotFound
	case err == ErrRecoveryOpen, err == ErrRecoveryClosed,
		err == ErrSameOperator, err == ErrSameTeam, err == ErrAlreadyApproved:
		return http.StatusConflict
	case err == ErrProofRequired, err == ErrOperatorRequired:
		return http.StatusBadRequest
	default:
		return http.StatusConflict
	}
}
