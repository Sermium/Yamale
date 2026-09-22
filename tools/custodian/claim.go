package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"yamale/blockchain/recovery"
)

// Claiming a holding back: the one endpoint that emits key material.
//
// # Why this exists now and did not before
//
// Under threshold signing a recovery ended with a RESHARE — the two services
// generated fresh shares under the same public key, and no share ever crossed
// the wire to anybody. That was the right shape and it is not available here,
// because the thing the account holder needs back is their own key, which they
// reassemble themselves from two holdings and their password.
//
// So this endpoint hands over one of the three. What it hands over is a share
// of a ciphertext: useless alone, useless with the other service's share alone,
// and openable only by somebody who also knows the password. That is the
// property the recovery package was built around, and it is what makes an
// endpoint like this one safe enough to have at all.
//
// # Why it is a CUSTOMER endpoint when the rest of the recovery flow is not
//
// initiate, approve and complete are operator actions on a loopback listener,
// deliberately, because a customer who could recover their own account is a
// customer whose email is enough to take it. Claiming is the opposite: it is
// the account holder proving, with the password, that they are the person the
// recovery was for.
//
// Splitting it this way also means the operator path never produces key
// material. An operator who drives a recovery end to end still finishes holding
// nothing they did not start with.
//
// # What the password is doing here
//
// Not authorising the recovery — the 72-hour notice and two approvals did that.
// It is here because the holder needs it anyway to open the seal, so demanding
// it costs them nothing, and because without it this endpoint would hand a
// ciphertext share to anybody who learned a recovery identifier.

type claimRequest struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type claimResponse struct {
	Role    string           `json:"role"`
	Address string           `json:"address"`
	Holding recovery.Holding `json:"holding"`
	// Reminder, because the client cannot work this out and the person reading
	// it has just lost a phone.
	Next string `json:"next"`
}

func (s *server) handleRecoveryClaim(w http.ResponseWriter, r *http.Request) {
	var req claimRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ID) == "" || req.Email == "" || req.Password == "" {
		refuse(w, http.StatusBadRequest,
			"the recovery identifier, the email and the password are all required")
		return
	}

	account, ok := s.authenticate(w, req.Email, req.Password)
	if !ok {
		return // authenticate has already written the refusal.
	}

	rec, err := s.recoveries.Get(req.ID)
	if err != nil {
		refuse(w, recoveryStatus(err), err.Error())
		return
	}
	// The recovery has to be FOR this account. Without this check a valid
	// recovery for one account would release the holding for any other whose
	// password the caller happened to know — which sounds like it needs the
	// password anyway, and does, but it would also let one completed recovery
	// be spent over and over against different accounts.
	if rec.Index != account.Index {
		refuse(w, http.StatusForbidden, "that recovery is not for this account")
		return
	}
	if rec.State != RecoveryCompleted {
		refuse(w, http.StatusConflict,
			"that recovery has not completed; nothing is released until it has")
		return
	}

	// The waiting period, now enforced HERE rather than at signing time.
	//
	// It used to mean "access restored, but this account cannot spend for 24
	// hours", and it was enforced by the custodian refusing to co-sign. Nothing
	// co-signs any more, so that enforcement is simply gone — this service
	// never sees a payment and could not stop one if it did.
	//
	// The only lever left is WHEN the holding is released, so the delay moved
	// there. The meaning changed with it, and the change is worth stating
	// rather than quietly keeping the old name: it is no longer "you have your
	// account back but cannot spend yet", it is "you get your account back in
	// 24 hours". Strictly later for the legitimate holder, and strictly the
	// same 24 hours of daylight for the real owner to notice and object.
	if rec.FrozenUntil != nil && rec.FrozenUntil.After(s.clock()) {
		w.Header().Set("Retry-After", retryAfter(rec.FrozenUntil.Sub(s.clock())))
		refuse(w, http.StatusConflict,
			"this recovery completed less than "+PostRecoveryFreeze.String()+" ago; the holding is "+
				"released after that, and until then the account holder can still cancel it")
		return
	}

	holding, err := s.store.Holding(account)
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}

	respond(w, claimResponse{
		Role:    s.role,
		Address: account.Address,
		Holding: holding,
		Next: "claim the other holding from the recovery service, then open both with this " +
			"password on your own device; neither service can do it for you",
	})
}

// retryAfter is seconds, rounded up, because a client that retries a fraction
// of a second early gets the same refusal and learns nothing.
func retryAfter(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return strconv.Itoa(int((d + time.Second - 1) / time.Second))
}
