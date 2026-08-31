package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"yamale/blockchain/mpc"
)

// A signing session: the custodian's half of one signature, and the rules about
// when it will run at all.
//
// The rules matter more than the machinery. A custodian that starts a session
// for anybody who asks has made the device share the only thing protecting the
// account, which is the arrangement this design was chosen to avoid.

// SessionTTL bounds how long a half-finished signature may sit open.
//
// Short on purpose. An abandoned session holds a party mid-protocol with the
// custodian's share loaded, and a service that accumulates those is a service
// whose memory is a pile of live key material waiting for a heap dump.
const SessionTTL = 2 * time.Minute

// MaxOpenSessions per account. One, and the reason is not resource use.
//
// Two open sessions for one account means two signatures in flight that the
// account holder may have authorised once. Concurrency here is not a
// performance question, it is the difference between paying somebody and
// paying them twice.
const MaxOpenSessions = 1

// Session is one signature in progress.
type Session struct {
	ID      string
	Index   string
	Digest  []byte
	Party   *mpc.SigningParty
	Started time.Time
	// Done is set once a signature exists, so a replay of the last message
	// cannot restart a finished protocol.
	Done bool
}

// Sessions holds the open ones.
type Sessions struct {
	mu   sync.Mutex
	open map[string]*Session
	// byAccount enforces MaxOpenSessions without scanning.
	byAccount map[string]int
	now       func() time.Time
}

func NewSessions() *Sessions {
	return &Sessions{
		open:      map[string]*Session{},
		byAccount: map[string]int{},
		now:       time.Now,
	}
}

var (
	// ErrBusy is returned rather than queueing. A caller told "busy" retries a
	// second later with the same intent; a caller silently queued behind
	// another signature finds out what it agreed to afterwards.
	ErrBusy       = errors.New("a signature is already in progress for this account")
	ErrNoSession  = errors.New("no such session")
	ErrFinished   = errors.New("that session has already produced a signature")
	ErrSessionOld = errors.New("that session has expired")
)

// Start opens a session, refusing if one is already open for this account.
func (s *Sessions) Start(index string, digest []byte, share mpc.Share) (*Session, []mpc.Outbound, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reapLocked()

	if s.byAccount[index] >= MaxOpenSessions {
		return nil, nil, ErrBusy
	}

	party, err := mpc.NewSigningParty(
		mpc.RoleCustodian, digest, share,
		[]string{mpc.RoleDevice, mpc.RoleCustodian},
	)
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
	sess := &Session{
		ID: id, Index: index, Digest: digest,
		Party: party, Started: s.now(),
	}
	s.open[id] = sess
	s.byAccount[index]++
	return sess, out, nil
}

// Handle feeds one device message in and returns the custodian's replies.
func (s *Sessions) Handle(id string, in mpc.Outbound) ([]mpc.Outbound, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, err := s.lookupLocked(id)
	if err != nil {
		return nil, err
	}
	if sess.Done {
		return nil, ErrFinished
	}
	if in.From != mpc.RoleDevice {
		// The only peer in this committee is the device. A message claiming to
		// be from the custodian is the custodian being asked to talk to itself,
		// which is either a bug or somebody probing.
		return nil, fmt.Errorf("a custodian session accepts messages from the device only, not %q", in.From)
	}
	if err := sess.Party.Handle(in); err != nil {
		return nil, err
	}
	out, err := sess.Party.Outbound()
	if err != nil {
		return nil, err
	}
	if _, done := sess.Party.Signature(); done {
		sess.Done = true
	}
	return out, nil
}

// Signature returns the signature if the protocol has finished.
//
// The custodian computes the same bytes the device does, which is worth stating
// because it is what lets the device check what it is about to broadcast rather
// than trusting whatever the custodian hands back.
func (s *Sessions) Signature(id string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, err := s.lookupLocked(id)
	if err != nil {
		return nil, false, err
	}
	sig, done := sess.Party.Signature()
	if done {
		sess.Done = true
	}
	return sig, done, nil
}

// Close drops a session and its party.
//
// Called on the happy path too. A finished session still holds the custodian's
// share and the intermediate values of a threshold protocol, and leaving those
// in a long-lived process is how a share outlives the moment it was needed.
func (s *Sessions) Close(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked(id)
}

func (s *Sessions) closeLocked(id string) {
	sess, ok := s.open[id]
	if !ok {
		return
	}
	delete(s.open, id)
	if n := s.byAccount[sess.Index]; n <= 1 {
		delete(s.byAccount, sess.Index)
	} else {
		s.byAccount[sess.Index] = n - 1
	}
}

func (s *Sessions) lookupLocked(id string) (*Session, error) {
	sess, ok := s.open[id]
	if !ok {
		return nil, ErrNoSession
	}
	if s.now().Sub(sess.Started) > SessionTTL {
		s.closeLocked(id)
		return nil, ErrSessionOld
	}
	return sess, nil
}

// reapLocked drops expired sessions.
func (s *Sessions) reapLocked() {
	now := s.now()
	for id, sess := range s.open {
		if now.Sub(sess.Started) > SessionTTL {
			s.closeLocked(id)
		}
	}
}

// Open reports how many sessions are live, for an operator's status line.
func (s *Sessions) Open() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reapLocked()
	return len(s.open)
}

func newID() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
