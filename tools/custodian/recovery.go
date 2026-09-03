package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Account recovery: the path these systems are actually robbed through.
//
// # Why this is the most conservative thing in the repository
//
// Every custodial system's real attack surface is the route that exists for
// people who lost their password. It is used rarely, tested less, and it grants
// exactly what an attacker wants. Support-desk social engineering is the most
// reliable attack on consumer finance there is, and it does not need a single
// cryptographic weakness to work.
//
// So this implements Part 5 of docs/guides/accounts.md literally, and each rule
// below is a rule because removing it produces a system where a convincing
// phone call is worth every account it holds:
//
//   - TWO APPROVERS, from different teams, and the initiator may not approve.
//     One approver is one person to deceive.
//   - A 72-HOUR DELAY, with notice at initiation. An attacker must keep the
//     owner unaware for three days; a legitimate owner needs one moment of
//     attention to cancel.
//   - OUTBOUND PAYMENTS FROZEN for 24 hours after completion. Recovery restores
//     access, not immediate spending — so a recovery that did succeed against
//     the wrong person still does not convert to money that afternoon.
//   - A STANDARD OF PROOF that is not public knowledge. Recorded as free text
//     because the service cannot check it; what it can do is refuse to proceed
//     without it and make it something a human signed their name to.
//   - PUBLISHED IN AGGREGATE, so an unusual rate is visible without exposing
//     who was recovered.
//
// # What this deliberately does not do
//
// It does not re-key anything. Completing a recovery marks the account
// recoverable and freezes outbound spending; the actual reshare — mpc.Reshare,
// which produces new shares under the SAME public key, so the address and the
// x/alias identifier survive — is driven by the device once the customer is
// back. Putting the reshare in here would mean this service holding a new share
// it generated on behalf of somebody who was not present, which is the property
// the whole design refuses.

// RecoveryDelay is the wait between initiation and eligibility.
//
// Three days, from the design. Not a tunable: the number is doing the work.
// Shorten it and the attacker's problem — keeping the real owner unaware —
// shrinks to something a weekend covers.
const RecoveryDelay = 72 * time.Hour

// PostRecoveryFreeze is how long outbound payments stay frozen afterwards.
const PostRecoveryFreeze = 24 * time.Hour

// RequiredApprovals, from different teams, neither of them the initiator.
const RequiredApprovals = 2

// RecoveryExpiry drops a request nobody completed.
//
// A request that sits open forever is a standing authorisation to take an
// account, waiting for the day somebody stops paying attention to the list.
const RecoveryExpiry = 30 * 24 * time.Hour

type RecoveryState string

const (
	RecoveryPending   RecoveryState = "pending"
	RecoveryApproved  RecoveryState = "approved"
	RecoveryCompleted RecoveryState = "completed"
	RecoveryCancelled RecoveryState = "cancelled"
	RecoveryRefused   RecoveryState = "refused"
)

// Approval is one operator's signature on a recovery.
type Approval struct {
	Operator string    `json:"operator"`
	Team     string    `json:"team"`
	At       time.Time `json:"at"`
}

// Recovery is one request to restore access to an account.
type Recovery struct {
	ID    string        `json:"id"`
	Index string        `json:"index"`
	State RecoveryState `json:"state"`

	// InitiatedBy and InitiatedTeam are recorded so the initiator can be
	// excluded from approving, which is the rule that stops one person
	// completing a recovery alone.
	InitiatedBy   string `json:"initiated_by"`
	InitiatedTeam string `json:"initiated_team"`
	// Proof is what the operator was shown. Free text, because no service can
	// check "they named their last three payments" — but required, and
	// attributed, so it is a thing somebody put their name to.
	Proof string `json:"proof"`

	InitiatedAt time.Time  `json:"initiated_at"`
	EligibleAt  time.Time  `json:"eligible_at"`
	Approvals   []Approval `json:"approvals,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	FrozenUntil *time.Time `json:"frozen_until,omitempty"`
	// Outcome explains a cancellation or a refusal.
	Outcome string `json:"outcome,omitempty"`
	// NotifiedAt records when notice went out. Nil is a defect, not a state:
	// see Recoveries.Initiate.
	NotifiedAt *time.Time `json:"notified_at,omitempty"`
}

// Eligible reports whether the delay has passed and the approvals are in.
func (r Recovery) Eligible(now time.Time) error {
	if r.State == RecoveryCancelled || r.State == RecoveryRefused {
		return fmt.Errorf("this recovery was %s", r.State)
	}
	if r.State == RecoveryCompleted {
		return errors.New("this recovery has already completed")
	}
	if now.Before(r.EligibleAt) {
		return fmt.Errorf(
			"the 72-hour notice period has not passed; this becomes eligible at %s",
			r.EligibleAt.UTC().Format(time.RFC3339))
	}
	if n := len(r.Approvals); n < RequiredApprovals {
		return fmt.Errorf("this needs %d approvals from different teams and has %d",
			RequiredApprovals, n)
	}
	return nil
}

// Recoveries is the log of them, one file per request.
//
// On disk rather than in memory, and unsealed rather than encrypted. It holds
// no key material and no email — only a blind index — and an audit trail that
// does not survive a restart is not an audit trail. Anybody with the directory
// learns how many recoveries happened and when, which is information this
// design publishes deliberately.
type Recoveries struct {
	mu  sync.Mutex
	dir string
	now func() time.Time
}

func NewRecoveries(dir string) (*Recoveries, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Recoveries{dir: dir, now: time.Now}, nil
}

var (
	ErrNoRecovery       = errors.New("no such recovery")
	ErrRecoveryClosed   = errors.New("that recovery is no longer open")
	ErrSameOperator     = errors.New("the operator who initiated a recovery may not approve it")
	ErrSameTeam         = errors.New("the two approvals must come from different teams")
	ErrAlreadyApproved  = errors.New("that operator has already approved this recovery")
	ErrProofRequired    = errors.New("a recovery needs a recorded standard of proof")
	ErrRecoveryOpen     = errors.New("a recovery is already open for this account")
	ErrOperatorRequired = errors.New("an operator name and team are required")
)

// Initiate opens a recovery and starts the clock.
//
// notify is called with the account so the deployment can send notice to the
// registered email and every enrolled device. It returns an error if notice
// could not be sent, and that error ABORTS the recovery — the delay only
// protects anybody if the owner was told it started, so a recovery nobody was
// notified of is worse than no recovery process at all.
func (rs *Recoveries) Initiate(
	index, operator, team, proof string, notify func() error,
) (*Recovery, error) {
	if operator == "" || team == "" {
		return nil, ErrOperatorRequired
	}
	if strings.TrimSpace(proof) == "" {
		return nil, ErrProofRequired
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()

	open, err := rs.openForLocked(index)
	if err != nil {
		return nil, err
	}
	if open != nil {
		// Refused rather than replaced. Two open recoveries for one account is
		// two independent clocks, and an attacker who can open a second one has
		// a way to reset the first.
		return nil, ErrRecoveryOpen
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := rs.now().UTC()
	r := &Recovery{
		ID: id, Index: index, State: RecoveryPending,
		InitiatedBy: operator, InitiatedTeam: team, Proof: proof,
		InitiatedAt: now, EligibleAt: now.Add(RecoveryDelay),
	}

	// Notice BEFORE the request is written. If notice fails, nothing was
	// started — which is the right order, because the alternative is a live
	// recovery clock the account's owner has no idea about.
	if notify != nil {
		if err := notify(); err != nil {
			return nil, fmt.Errorf(
				"notice could not be sent, so no recovery was started: %w", err)
		}
	}
	r.NotifiedAt = &now

	if err := rs.writeLocked(r); err != nil {
		return nil, err
	}
	return r, nil
}

// Approve records one operator's approval.
func (rs *Recoveries) Approve(id, operator, team string) (*Recovery, error) {
	if operator == "" || team == "" {
		return nil, ErrOperatorRequired
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()

	r, err := rs.readLocked(id)
	if err != nil {
		return nil, err
	}
	if r.State != RecoveryPending && r.State != RecoveryApproved {
		return nil, ErrRecoveryClosed
	}
	if operator == r.InitiatedBy {
		return nil, ErrSameOperator
	}
	for _, a := range r.Approvals {
		if a.Operator == operator {
			return nil, ErrAlreadyApproved
		}
		if a.Team == team {
			// Different teams, not merely different people. Two approvals from
			// one team are two people with one manager, one set of pressures
			// and one person to deceive.
			return nil, ErrSameTeam
		}
	}
	// The initiator's team counts as taken too: initiating and approving from
	// one team is the same concentration the rule exists to break.
	if team == r.InitiatedTeam {
		return nil, ErrSameTeam
	}

	r.Approvals = append(r.Approvals, Approval{
		Operator: operator, Team: team, At: rs.now().UTC(),
	})
	if len(r.Approvals) >= RequiredApprovals {
		r.State = RecoveryApproved
	}
	if err := rs.writeLocked(r); err != nil {
		return nil, err
	}
	return r, nil
}

// Cancel stops a recovery. Anybody who can prove they hold the account may do
// this, and so may an operator who has learnt the request was fraudulent.
func (rs *Recoveries) Cancel(id, by, reason string) (*Recovery, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	r, err := rs.readLocked(id)
	if err != nil {
		return nil, err
	}
	if r.State == RecoveryCompleted {
		return nil, errors.New("that recovery has already completed")
	}
	if r.State == RecoveryCancelled || r.State == RecoveryRefused {
		return nil, ErrRecoveryClosed
	}
	r.State = RecoveryCancelled
	r.Outcome = fmt.Sprintf("cancelled by %s: %s", by, reason)
	if err := rs.writeLocked(r); err != nil {
		return nil, err
	}
	return r, nil
}

// Complete marks a recovery done and returns the moment outbound payments
// become possible again.
func (rs *Recoveries) Complete(id string) (*Recovery, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	r, err := rs.readLocked(id)
	if err != nil {
		return nil, err
	}
	now := rs.now().UTC()
	if err := r.Eligible(now); err != nil {
		return nil, err
	}
	until := now.Add(PostRecoveryFreeze)
	r.State = RecoveryCompleted
	r.CompletedAt = &now
	r.FrozenUntil = &until
	if err := rs.writeLocked(r); err != nil {
		return nil, err
	}
	return r, nil
}

// FrozenUntil reports when this account may spend again, if a recovery has
// recently completed for it.
//
// Consulted on every signature. A recovery that restored access AND immediate
// spending would hand an attacker who got through the process the money the
// same afternoon, which is the outcome the delay was supposed to have made
// expensive.
func (rs *Recoveries) FrozenUntil(index string) (time.Time, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	var latest time.Time
	_ = rs.eachLocked(func(r *Recovery) {
		if r.Index != index || r.State != RecoveryCompleted || r.FrozenUntil == nil {
			return
		}
		if r.FrozenUntil.After(latest) {
			latest = *r.FrozenUntil
		}
	})
	if latest.IsZero() || !latest.After(rs.now().UTC()) {
		return time.Time{}, false
	}
	return latest, true
}

// Get returns one recovery.
func (rs *Recoveries) Get(id string) (*Recovery, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.readLocked(id)
}

// OpenFor returns the open recovery for an account, if there is one.
func (rs *Recoveries) OpenFor(index string) (*Recovery, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.openForLocked(index)
}

// Statistics is what gets published: counts and durations, never who.
//
// From the design: "recoveries published in aggregate — how many, how long they
// took — so an unusual rate is visible without exposing who". A number nobody
// looks at protects nobody, so this is an endpoint rather than a log line.
type Statistics struct {
	Pending   int `json:"pending"`
	Approved  int `json:"approved"`
	Completed int `json:"completed"`
	Cancelled int `json:"cancelled"`
	// MedianHours is initiation to completion, for completed recoveries.
	// Median rather than mean: one recovery that sat open for a month would
	// otherwise hide twenty that took exactly the minimum, and it is the
	// twenty that would be the interesting number.
	MedianHours float64 `json:"median_hours_to_complete"`
	// CompletedLast30Days is the rate worth alarming on.
	CompletedLast30Days int `json:"completed_last_30_days"`
}

func (rs *Recoveries) Statistics() Statistics {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	var stats Statistics
	var durations []float64
	cutoff := rs.now().UTC().Add(-30 * 24 * time.Hour)

	_ = rs.eachLocked(func(r *Recovery) {
		switch r.State {
		case RecoveryPending:
			stats.Pending++
		case RecoveryApproved:
			stats.Approved++
		case RecoveryCancelled, RecoveryRefused:
			stats.Cancelled++
		case RecoveryCompleted:
			stats.Completed++
			if r.CompletedAt != nil {
				durations = append(durations, r.CompletedAt.Sub(r.InitiatedAt).Hours())
				if r.CompletedAt.After(cutoff) {
					stats.CompletedLast30Days++
				}
			}
		}
	})

	if len(durations) > 0 {
		sort.Float64s(durations)
		mid := len(durations) / 2
		if len(durations)%2 == 1 {
			stats.MedianHours = durations[mid]
		} else {
			stats.MedianHours = (durations[mid-1] + durations[mid]) / 2
		}
	}
	return stats
}

// ---------------------------------------------------------------- storage

func (rs *Recoveries) path(id string) string {
	return filepath.Join(rs.dir, id+".json")
}

func (rs *Recoveries) writeLocked(r *Recovery) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(rs.path(r.ID), raw)
}

func (rs *Recoveries) readLocked(id string) (*Recovery, error) {
	// The id comes from a caller, so it must never be able to name a path.
	if id == "" || strings.ContainsAny(id, `/\.`) {
		return nil, ErrNoRecovery
	}
	raw, err := os.ReadFile(rs.path(id))
	if err != nil {
		return nil, ErrNoRecovery
	}
	var r Recovery
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("the recovery record does not decode: %w", err)
	}
	return &r, nil
}

func (rs *Recoveries) openForLocked(index string) (*Recovery, error) {
	var found *Recovery
	now := rs.now().UTC()
	err := rs.eachLocked(func(r *Recovery) {
		if r.Index != index || found != nil {
			return
		}
		if r.State != RecoveryPending && r.State != RecoveryApproved {
			return
		}
		if now.Sub(r.InitiatedAt) > RecoveryExpiry {
			return
		}
		found = r
	})
	return found, err
}

func (rs *Recoveries) eachLocked(fn func(*Recovery)) error {
	entries, err := os.ReadDir(rs.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(rs.dir, e.Name()))
		if err != nil {
			continue
		}
		var r Recovery
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		fn(&r)
	}
	return nil
}
