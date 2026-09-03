package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"

	"yamale/blockchain/mpc"
)

// Enrolment: how a member of the public gets an account, without any single
// service ever holding two shares.
//
// # The shape, because it is not the obvious one
//
// The DEVICE coordinates. It runs its own key-generation party in the browser,
// and talks to two independent services — this one as `custodian`, and a second
// deployment of this same binary as `recovery`. Each service runs its own
// party, computes its own share, and transmits none of it.
//
// That is why --role exists. Two deployments, two sealing keys, two
// directories, ideally two operators. One process able to hold both shares
// would be one process able to sign, and the whole design is the claim that no
// such process exists.
//
// # What this service can and cannot verify
//
// It cannot verify that the person enrolling is who they say. Nobody can, over
// HTTP, without an identity document or an agent — and pretending otherwise
// would be worse than saying so. What it CAN verify, and does:
//
//   - the email is not already enrolled, so enrolment can never overwrite an
//     existing account or replace its custodian share;
//   - the key generation actually completed, and the address the device claims
//     is the one this party computed. A device talking to a substituted
//     custodian, or through something rewriting the traffic, produces a
//     different key here and is refused;
//   - the password verifier is written only when both of the above hold.
//
// The identity question belongs to enrolment policy — an agent network, a
// document check, an invitation — and is deliberately not decided here.

// EnrolTTL bounds a half-finished enrolment.
//
// Longer than SessionTTL because key generation is genuinely slower than
// signing and a phone on a bad connection should not have to start again, and
// still short, because an abandoned enrolment holds a party mid-protocol with
// live key material in it.
const EnrolTTL = 10 * time.Minute

// MaxOpenEnrolments across the whole service.
//
// A global cap rather than a per-account one, because there is no account yet —
// that is the point of enrolling — so there is nothing to key a per-caller
// limit on that a caller cannot simply change. It bounds memory and, with the
// pre-parameter pool, bounds the CPU an unauthenticated caller can command.
const MaxOpenEnrolments = 32

var (
	ErrTooManyEnrolments = errors.New("too many enrolments are in progress; try again shortly")
	ErrNoEnrolment       = errors.New("no such enrolment")
	ErrEnrolmentOld      = errors.New("that enrolment has expired; start again")
	ErrAlreadyEnrolled   = errors.New("an account already exists for that email")
)

// Enrolment is one key generation in progress.
type Enrolment struct {
	ID    string
	Index string
	// Email is held only to be re-indexed at finish; it is never stored.
	Party    *mpc.KeygenParty
	Verifier Verifier
	Started  time.Time
	Done     bool
}

// Enrolments holds the open ones.
type Enrolments struct {
	mu   sync.Mutex
	open map[string]*Enrolment
	// byIndex stops one email opening many enrolments at once, which would
	// otherwise be the cheapest way to exhaust the cap above.
	byIndex map[string]string
	now     func() time.Time
}

func NewEnrolments() *Enrolments {
	return &Enrolments{
		open:    map[string]*Enrolment{},
		byIndex: map[string]string{},
		now:     time.Now,
	}
}

// Start begins this party's half of a new account's key generation.
//
// role is this deployment's role — custodian or recovery. The verifier is
// computed by the caller and held until finish, so a failed generation leaves
// nothing behind.
func (e *Enrolments) Start(
	role, index string, verifier Verifier, pre *keygen.LocalPreParams,
) (*Enrolment, []mpc.Outbound, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reapLocked()

	if existing, ok := e.byIndex[index]; ok {
		// Replaced rather than refused. A person whose phone lost connection
		// mid-enrolment would otherwise be locked out for EnrolTTL by their own
		// abandoned attempt, and there is no account yet to protect — the
		// account-exists check happens before this and is where overwriting is
		// actually prevented.
		e.closeLocked(existing)
	}
	if len(e.open) >= MaxOpenEnrolments {
		return nil, nil, ErrTooManyEnrolments
	}

	party, err := mpc.NewKeygenParty(role, pre)
	if err != nil {
		return nil, nil, err
	}
	out, err := party.Outbound()
	if err != nil {
		return nil, nil, err
	}

	id, err := newID()
	if err != nil {
		return nil, nil, err
	}
	en := &Enrolment{
		ID: id, Index: index, Party: party,
		Verifier: verifier, Started: e.now(),
	}
	e.open[id] = en
	e.byIndex[index] = id
	return en, out, nil
}

// Handle feeds in one message and returns this party's replies.
func (e *Enrolments) Handle(id string, in mpc.Outbound) ([]mpc.Outbound, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	en, err := e.lookupLocked(id)
	if err != nil {
		return nil, err
	}
	if err := en.Party.Handle(in); err != nil {
		return nil, err
	}
	out, err := en.Party.Outbound()
	if err != nil {
		return nil, err
	}
	if _, done := en.Party.Share(); done {
		en.Done = true
	}
	return out, nil
}

// Poll returns anything this party wants to send without feeding anything in.
//
// Needed because the first messages of a round can appear after the response to
// the message that triggered them has already been written. Without a way to
// ask again, the protocol stalls one round short — the same failure the mpc
// package documents on the signing side, arriving here through the transport
// instead of through a channel.
func (e *Enrolments) Poll(id string) ([]mpc.Outbound, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	en, err := e.lookupLocked(id)
	if err != nil {
		return nil, false, err
	}
	out, err := en.Party.Outbound()
	if err != nil {
		return nil, false, err
	}
	if _, done := en.Party.Share(); done {
		en.Done = true
	}
	return out, en.Done, nil
}

// Finish returns the completed share, leaving the enrolment open so the caller
// can commit it and close it in the right order.
func (e *Enrolments) Finish(id string) (*Enrolment, mpc.Share, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	en, err := e.lookupLocked(id)
	if err != nil {
		return nil, mpc.Share{}, err
	}
	share, ok := en.Party.Share()
	if !ok {
		if perr := en.Party.Err(); perr != nil {
			return nil, mpc.Share{}, perr
		}
		return nil, mpc.Share{}, errors.New("key generation has not finished yet")
	}
	return en, share, nil
}

// Close drops an enrolment and its party.
func (e *Enrolments) Close(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeLocked(id)
}

func (e *Enrolments) closeLocked(id string) {
	en, ok := e.open[id]
	if !ok {
		return
	}
	delete(e.open, id)
	if e.byIndex[en.Index] == id {
		delete(e.byIndex, en.Index)
	}
}

func (e *Enrolments) lookupLocked(id string) (*Enrolment, error) {
	en, ok := e.open[id]
	if !ok {
		return nil, ErrNoEnrolment
	}
	if e.now().Sub(en.Started) > EnrolTTL {
		e.closeLocked(id)
		return nil, ErrEnrolmentOld
	}
	return en, nil
}

// reapLocked drops expired enrolments.
//
// Called on every Start rather than from a timer: the work is proportional to
// what is open, which is capped, and a service with no background sweeper has
// one less thing that can silently stop running.
func (e *Enrolments) reapLocked() {
	now := e.now()
	for id, en := range e.open {
		if now.Sub(en.Started) > EnrolTTL {
			e.closeLocked(id)
		}
	}
}

// Open is how many are in progress, for the health endpoint.
func (e *Enrolments) Open() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.open)
}

// enrolmentError maps the failures above onto status codes.
func enrolmentError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNoEnrolment), errors.Is(err, ErrEnrolmentOld):
		return 404, err.Error()
	case errors.Is(err, ErrAlreadyEnrolled):
		return 409, err.Error()
	case errors.Is(err, ErrTooManyEnrolments), errors.Is(err, ErrNoPreParams):
		// 503 with a retry hint rather than 429: the caller is not being rate
		// limited for misbehaving, the service genuinely has nothing ready, and
		// telling them otherwise sends them to the wrong remedy.
		return 503, err.Error()
	default:
		return 400, fmt.Sprint(err)
	}
}
