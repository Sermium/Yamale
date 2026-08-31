package mpc

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/v2/tss"
)

// SigningParty is ONE participant in a signature, holding one share and
// nothing else.
//
// This is the type production uses, and Sign — which runs every party in one
// process — is the type tests use. The distinction is the whole security model
// and it is worth being blunt about why both exist.
//
// Sign needs every share in one address space. That is correct for a test and
// catastrophic anywhere else: a browser that called it would be holding the
// device share AND the custodian share, which is a whole key in all but name,
// and "the operator cannot move your money" would be false the moment anybody
// looked. An earlier WebAssembly build did exactly that, and it is the reason
// this type exists.
//
// A SigningParty never sees another share. It emits protocol messages, consumes
// the ones addressed to it, and eventually produces a signature. The caller
// moves the bytes — over HTTPS to the custodian, over a QR code to an
// air-gapped recovery holder, however the deployment does it — and this package
// stays out of that, because the transport is where authentication lives and a
// crypto module that opened its own sockets would be one nobody could audit
// separately from the network it happened to trust.
type SigningParty struct {
	role  string
	party tss.Party
	// The committee, kept because tss.Party does not expose its parameters and
	// a peer has to be resolved to a party id on every inbound message.
	ids  tss.SortedPartyIDs
	out  chan tss.Message
	end  chan *common.SignatureData
	errs chan error

	mu sync.Mutex
	// Messages drained from tss-lib but not yet handed to the caller.
	//
	// A queue rather than a bare channel read, because the completion check and
	// the send path both have to drain the same channels and only one of them
	// may consume. Without this, asking "is it finished yet" quietly swallowed
	// whatever the party was trying to send, and the protocol stalled one round
	// short with nothing logged — which is exactly how it failed the first time
	// two parties were run apart.
	queue []Outbound
	pub   *ecdsa.PublicKey
	sig   []byte
	done  bool
	err   error
}

// Outbound is one protocol message this party wants delivered.
//
// Broadcast messages go to every other participant; the rest go only to To.
// Getting that wrong does not corrupt a signature, it hangs the protocol — the
// round never completes and nothing is logged — so the distinction is carried
// explicitly rather than inferred by the caller.
type Outbound struct {
	// Wire is the serialised message, opaque to the transport.
	Wire []byte `json:"wire"`
	// From is this party's role.
	From string `json:"from"`
	// To is empty when Broadcast is true.
	To []string `json:"to,omitempty"`
	// Broadcast means every other participant must receive it.
	Broadcast bool `json:"broadcast"`
}

// NewSigningParty starts this party's half of a signature over digest.
//
// signers names every participant, this one included, and must hold exactly
// Threshold+1 entries: a signing committee is fixed before the first message,
// because a protocol that could grow a participant mid-run is one where the
// last party to join decides the outcome.
func NewSigningParty(role string, digest []byte, share Share, signers []string) (*SigningParty, error) {
	if len(digest) != 32 {
		return nil, fmt.Errorf("expected a 32-byte digest, got %d bytes", len(digest))
	}
	if len(signers) != Threshold+1 {
		return nil, fmt.Errorf(
			"a signing committee is exactly %d parties; %d were named", Threshold+1, len(signers))
	}
	if share.Data.ShareID == nil {
		return nil, fmt.Errorf("the %s share carries no share id", role)
	}

	ids, self, err := committeeOf(signers, role, share)
	if err != nil {
		return nil, err
	}

	p := &SigningParty{
		role: role,
		out:  make(chan tss.Message, 32),
		end:  make(chan *common.SignatureData, 1),
		errs: make(chan error, 4),
	}
	if pub, err := share.PublicKey(); err == nil {
		p.pub = pub
	}

	p.ids = ids
	params := tss.NewParameters(tss.S256(), tss.NewPeerContext(ids), self, len(ids), Threshold)
	p.party = signing.NewLocalParty(new(big.Int).SetBytes(digest), params, share.Data, p.out, p.end)

	go func() {
		if err := p.party.Start(); err != nil {
			p.errs <- err
		}
	}()
	return p, nil
}

// committeeOf builds the signing committee's party ids.
//
// Every id is derived from the share ids this party already holds — Ks, which
// every share carries for the whole sharing — rather than from anything a peer
// asserts. A committee assembled from what the custodian said it was would be a
// committee the custodian could quietly change.
func committeeOf(signers []string, role string, share Share) (tss.SortedPartyIDs, *tss.PartyID, error) {
	if len(share.Data.Ks) != len(Roles) {
		return nil, nil, fmt.Errorf(
			"the share knows %d parties, expected %d", len(share.Data.Ks), len(Roles))
	}

	// Ks is sorted the same way Roles is, so position maps one to the other.
	// The party's own entry is checked against its ShareID rather than trusted,
	// which is what catches a share paired with the wrong role.
	index := make(map[string]*big.Int, len(Roles))
	for i, role := range Roles {
		index[role] = share.Data.Ks[i]
	}
	if own, ok := index[role]; !ok || own.Cmp(share.Data.ShareID) != 0 {
		return nil, nil, fmt.Errorf(
			"the share presented as %s does not carry that role's share id", role)
	}

	names := append([]string(nil), signers...)
	sort.Strings(names)
	unsorted := make(tss.UnSortedPartyIDs, 0, len(names))
	for _, name := range names {
		key, ok := index[name]
		if !ok {
			return nil, nil, fmt.Errorf("%q is not a party in this sharing", name)
		}
		unsorted = append(unsorted, tss.NewPartyID(name, name, key))
	}
	ids := tss.SortPartyIDs(unsorted)

	for _, id := range ids {
		if id.Id == role {
			return ids, id, nil
		}
	}
	return nil, nil, fmt.Errorf("%s is not among the named signers", role)
}

// drain moves everything tss-lib has produced into this party's own state.
//
// Safe to call from anywhere and as often as you like: it consumes the
// channels once and remembers what it found, so a caller polling for
// completion cannot destroy a message a caller polling for output was about to
// send.
func (p *SigningParty) drain() {
	for {
		select {
		case err := <-p.errs:
			p.mu.Lock()
			if p.err == nil {
				p.err = err
			}
			p.mu.Unlock()
		case sig := <-p.end:
			p.mu.Lock()
			p.sig = normalise(sig)
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
// it. This is the only method that consumes the queue.
//
// Non-blocking: it returns what is ready and nothing more. A caller polls it
// after every Handle, which is what turns a protocol with rounds into a request
// and a response the transport can carry.
func (p *SigningParty) Outbound() ([]Outbound, error) {
	p.drain()
	p.mu.Lock()
	defer p.mu.Unlock()
	msgs := p.queue
	p.queue = nil
	if p.err != nil {
		return msgs, fmt.Errorf("signing as %s: %w", p.role, p.err)
	}
	return msgs, nil
}

// Handle feeds in one message from another participant.
func (p *SigningParty) Handle(in Outbound) error {
	if in.From == p.role {
		// A party fed its own message is a party that has been given a mirror,
		// and tss-lib's failure mode for it is a round that never completes.
		return fmt.Errorf("a party cannot receive its own message")
	}
	from := p.peer(in.From)
	if from == nil {
		return fmt.Errorf("%q is not in this signing committee", in.From)
	}
	if _, err := p.party.UpdateFromBytes(in.Wire, from, in.Broadcast); err != nil {
		return fmt.Errorf("handling a message from %s: %w", in.From, err)
	}
	return nil
}

func (p *SigningParty) peer(role string) *tss.PartyID {
	for _, id := range p.ids {
		if id.Id == role {
			return id
		}
	}
	return nil
}

// Signature returns the finished signature, and whether it is finished.
//
// The signature is the same 64-byte low-S form the chain expects, and every
// participant computes the identical bytes — so the custodian cannot hand the
// device a different signature from the one it helped produce, and the device
// can check what it is about to broadcast against its own copy.
func (p *SigningParty) Signature() ([]byte, bool) {
	// Drains, but does NOT consume the outbound queue. See drain().
	p.drain()
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.done {
		return nil, false
	}
	return append([]byte(nil), p.sig...), true
}

// PublicKey is the account key this party is signing for.
func (p *SigningParty) PublicKey() (*ecdsa.PublicKey, error) {
	if p.pub == nil {
		return nil, errors.New("this party knows no public key")
	}
	return p.pub, nil
}
