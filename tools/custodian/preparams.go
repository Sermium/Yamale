package main

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"

	"yamale/blockchain/mpc"
)

// The pool of pre-computed safe primes that makes enrolment possible at all.
//
// # Why this is not an optimisation
//
// A party's Paillier key needs two safe primes, and finding them takes minutes
// on the hardware this runs on. Generating them inside the enrolment request
// gives you two things, both bad: a person creating an account watches a
// spinner for four minutes, and an unauthenticated endpoint that costs minutes
// of CPU per call, which is a denial-of-service gift — a handful of requests
// pins every core and nobody else can enrol or sign.
//
// So the primes are made in advance, in the background, and enrolment either
// takes one from the shelf immediately or is refused immediately. Both answers
// are fast, and "refused immediately" is a far better failure than a service
// that becomes slow for everybody.
//
// # The refusal is deliberate and should not be softened
//
// The tempting change is to block until one is ready. That converts a clean
// "come back in a minute" into an unbounded queue whose length is set by
// whoever is attacking, and restores the whole problem behind a nicer error
// message.

// ErrNoPreParams is returned when the shelf is empty.
//
// Its text is written for the person enrolling rather than the operator,
// because it is the one error here that reaches an end user.
var ErrNoPreParams = errors.New(
	"this service cannot start a new account right now; try again in a minute")

// PreParamPool keeps a small number of pre-computed parameter sets ready.
type PreParamPool struct {
	mu    sync.Mutex
	ready []*keygen.LocalPreParams

	want int
	// generate is the expensive call, injected so tests can run without
	// spending minutes per case.
	generate func() (*keygen.LocalPreParams, error)

	// wake is signalled when one is taken, so the filler tops up promptly
	// rather than on a timer.
	wake chan struct{}
	stop chan struct{}
	once sync.Once
}

// NewPreParamPool starts a pool that keeps `want` sets ready.
//
// Small on purpose. Each set is a few kilobytes but costs minutes of CPU, and a
// pool sized for a launch-day rush would spend the preceding week of CPU
// producing parameters for accounts nobody creates.
func NewPreParamPool(want int, generate func() (*keygen.LocalPreParams, error)) *PreParamPool {
	if want < 1 {
		want = 1
	}
	if generate == nil {
		generate = func() (*keygen.LocalPreParams, error) {
			return mpc.GeneratePreParams(mpc.KeygenTimeout)
		}
	}
	p := &PreParamPool{
		want:     want,
		generate: generate,
		wake:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
	}
	go p.fill()
	return p
}

// Take removes one set from the shelf, or refuses.
//
// Never blocks on generation. See the note above about why turning this into a
// wait would undo the whole point.
func (p *PreParamPool) Take() (*keygen.LocalPreParams, error) {
	p.mu.Lock()
	if len(p.ready) == 0 {
		p.mu.Unlock()
		p.nudge()
		return nil, ErrNoPreParams
	}
	last := len(p.ready) - 1
	out := p.ready[last]
	// Cleared so the slice does not keep the parameters alive after the pool
	// has handed them over — this is key material, and a pool that quietly
	// retains a copy of everything it ever issued is a pool worth stealing.
	p.ready[last] = nil
	p.ready = p.ready[:last]
	p.mu.Unlock()

	p.nudge()
	return out, nil
}

// Ready is how many sets are on the shelf, for the health endpoint.
//
// Worth exposing: a pool that has been empty for an hour means enrolment has
// been failing for an hour, and nothing else in this service would say so.
func (p *PreParamPool) Ready() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.ready)
}

// Close stops the background filler.
func (p *PreParamPool) Close() {
	p.once.Do(func() { close(p.stop) })
}

func (p *PreParamPool) nudge() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// fill tops the shelf up, one set at a time.
//
// Serial rather than parallel: the point is to have parameters ready before
// anybody asks, not to have them all at once, and a filler that saturates every
// core defeats the reason the pool exists by making the service slow in exactly
// the situation it was meant to stay fast.
func (p *PreParamPool) fill() {
	for {
		p.mu.Lock()
		need := p.want - len(p.ready)
		p.mu.Unlock()

		if need <= 0 {
			select {
			case <-p.stop:
				return
			case <-p.wake:
				continue
			case <-time.After(time.Minute):
				continue
			}
		}

		set, err := p.generate()
		if err != nil {
			// Logged and retried rather than fatal. A failure here means new
			// accounts cannot be created; it does not mean existing accounts
			// cannot sign, and taking the service down would turn a partial
			// outage into a total one.
			log.Printf("pre-parameters: %v (enrolment is unavailable until this succeeds)", err)
			select {
			case <-p.stop:
				return
			case <-time.After(10 * time.Second):
			}
			continue
		}

		p.mu.Lock()
		p.ready = append(p.ready, set)
		n := len(p.ready)
		p.mu.Unlock()
		log.Printf("pre-parameters: %d of %d ready", n, p.want)

		select {
		case <-p.stop:
			return
		default:
		}
	}
}
