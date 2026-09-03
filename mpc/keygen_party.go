package mpc

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"sync"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/tss"
)

// KeygenParty is ONE participant in distributed key generation.
//
// # Why this exists when Keygen already does
//
// Keygen runs all three parties in one process. That is right for a test and
// for a ceremony performed on one machine, and it is precisely wrong for
// enrolling a member of the public: whoever runs that process holds all three
// shares at once, for however long it takes, and the entire claim of this
// design — that the operator cannot move anybody's money — is false for the
// duration.
//
// It is not a small window either. The share that matters is written to disk by
// the same program that just held the other two.
//
// So this type is the same protocol with the parties pulled apart. Each side
// constructs its own, its share is computed locally and never transmitted, and
// the caller moves opaque bytes between them. At no point does any one process
// hold two shares, which makes the property structural rather than procedural.
//
// # The transport is deliberately not in here
//
// Same reason SigningParty leaves it out: authentication lives in the
// transport, and a crypto module that opened its own sockets would be one
// nobody could audit separately from the network it happened to trust.
//
// # What a caller must still get right
//
// Every participant must be started before the first message is delivered, and
// all three must run to completion. Key generation is not a threshold protocol
// — 2-of-3 describes signing; generating the sharing needs all three, because a
// party absent from generation has no share to sign with later.
type KeygenParty struct {
	role  string
	party tss.Party
	ids   tss.SortedPartyIDs
	out   chan tss.Message
	end   chan *keygen.LocalPartySaveData
	errs  chan error

	mu sync.Mutex
	// Drained from tss-lib but not yet handed to the caller. A queue rather
	// than a bare channel read, for the reason SigningParty documents at
	// length: the completion check and the send path drain the same channels
	// and only one of them may consume, or a poll for "is it done" silently
	// eats the message the protocol was about to send and it stalls one round
	// short with nothing logged.
	queue []Outbound
	share *Share
	done  bool
	err   error
}

// NewKeygenParty starts this participant's half of key generation.
//
// role must be one of Roles. preParams is the expensive half — the safe primes
// behind this party's Paillier key — and may be nil, in which case they are
// generated here and this call takes minutes rather than milliseconds. A
// service enrolling people should keep a pool of them: see GeneratePreParams.
func NewKeygenParty(role string, preParams *keygen.LocalPreParams) (*KeygenParty, error) {
	if !isRole(role) {
		return nil, fmt.Errorf("%q is not one of the roles %v", role, Roles)
	}

	// Built from the role table rather than from anything a peer asserts, which
	// is what lets three processes that have never spoken agree on the same
	// committee. A peer context assembled from what the custodian said it was
	// would be one the custodian could quietly change.
	ids := partyIDs()
	self, err := findParty(ids, role)
	if err != nil {
		return nil, err
	}

	p := &KeygenParty{
		role: role,
		ids:  ids,
		out:  make(chan tss.Message, len(Roles)*8),
		end:  make(chan *keygen.LocalPartySaveData, 1),
		errs: make(chan error, 4),
	}

	params := tss.NewParameters(tss.S256(), tss.NewPeerContext(ids), self, len(ids), Threshold)
	if preParams != nil {
		p.party = keygen.NewLocalParty(params, p.out, p.end, *preParams)
	} else {
		p.party = keygen.NewLocalParty(params, p.out, p.end)
	}

	go func() {
		if err := p.party.Start(); err != nil {
			p.errs <- err
		}
	}()
	return p, nil
}

// drain moves everything tss-lib has produced into this party's own state.
//
// Safe to call from anywhere and as often as you like. See SigningParty.drain.
func (p *KeygenParty) drain() {
	for {
		select {
		case err := <-p.errs:
			p.mu.Lock()
			if p.err == nil {
				p.err = err
			}
			p.mu.Unlock()
		case data := <-p.end:
			p.mu.Lock()
			p.share = &Share{Role: p.role, Data: *data}
			p.done = true
			p.mu.Unlock()
		case m := <-p.out:
			wire, _, err := m.WireBytes()
			if err != nil {
				p.mu.Lock()
				if p.err == nil {
					p.err = fmt.Errorf("serialising a message: %w", err)
				}
				p.mu.Unlock()
				continue
			}
			o := Outbound{Wire: wire, From: p.role, Broadcast: m.IsBroadcast()}
			if !o.Broadcast {
				for _, to := range m.GetTo() {
					o.To = append(o.To, to.Id)
				}
			}
			p.mu.Lock()
			p.queue = append(p.queue, o)
			p.mu.Unlock()
		default:
			return
		}
	}
}

// Outbound hands over whatever this party currently wants to send, and clears
// it. The only method that consumes the queue.
func (p *KeygenParty) Outbound() ([]Outbound, error) {
	p.drain()
	p.mu.Lock()
	defer p.mu.Unlock()
	msgs := p.queue
	p.queue = nil
	if p.err != nil {
		return msgs, fmt.Errorf("generating as %s: %w", p.role, p.err)
	}
	return msgs, nil
}

// Handle feeds in one message from another participant.
func (p *KeygenParty) Handle(in Outbound) error {
	if in.From == p.role {
		// A party fed its own message is a party that has been given a mirror,
		// and tss-lib's failure mode for it is a round that never completes.
		return errors.New("a party cannot receive its own message")
	}
	from := p.peer(in.From)
	if from == nil {
		return fmt.Errorf("%q is not a party in this key generation", in.From)
	}
	if _, err := p.party.UpdateFromBytes(in.Wire, from, in.Broadcast); err != nil {
		return fmt.Errorf("handling a message from %s: %w", in.From, err)
	}
	return nil
}

func (p *KeygenParty) peer(role string) *tss.PartyID {
	for _, id := range p.ids {
		if id.Id == role {
			return id
		}
	}
	return nil
}

// Share returns this party's finished share, and whether it is finished.
//
// The share is this party's alone. Nothing in it is transmitted, and that is
// the property the whole type exists for — so callers should note that handing
// the result to anybody, for any reason, undoes it.
func (p *KeygenParty) Share() (Share, bool) {
	// Drains, but does NOT consume the outbound queue. See drain().
	p.drain()
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.done || p.share == nil {
		return Share{}, false
	}
	return *p.share, true
}

// PublicKey is the account key the finished sharing controls.
//
// Every party computes the identical key, which is what lets the device check
// that the address it is about to be told to use is the one it helped generate,
// rather than one the custodian chose to report.
func (p *KeygenParty) PublicKey() (*ecdsa.PublicKey, error) {
	share, ok := p.Share()
	if !ok {
		return nil, errors.New("key generation has not finished")
	}
	return share.PublicKey()
}

// Err reports a protocol failure without consuming the outbound queue.
//
// Outbound also reports it, but a caller that is only polling for completion
// would otherwise sit on a party that failed in round one and never notice —
// which reads to whoever is enrolling as the service having hung.
func (p *KeygenParty) Err() error {
	p.drain()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err == nil {
		return nil
	}
	return fmt.Errorf("generating as %s: %w", p.role, p.err)
}

func isRole(role string) bool {
	for _, r := range Roles {
		if r == role {
			return true
		}
	}
	return false
}
