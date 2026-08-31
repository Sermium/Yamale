// Package mpc is threshold signing for consumer accounts: a secp256k1 key that
// exists in three shares and in no other form, anywhere, ever.
//
// # Why this exists
//
// A password cannot authorise a transaction. Only a key can. So every account
// design is an answer to where the key lives and who can use it, and the
// project took the answer deliberately (docs/guides/accounts.md, decided
// 2026-08-20): the operator must NOT be able to move a customer's funds. On a
// state-operated payments network "the authority can spend any citizen's
// balance" is a very different political object from "the authority runs the
// rails", and the second is the one being sold.
//
// That rules out holding keys server-side. Holding the key on the device under
// the password rules itself out for a different reason: a forgotten password
// would be a lost account, which is not viable for a national system serving
// people who lose phones.
//
// # Why three shares and not two
//
// Two — device and custodian — gives the security property and nothing else. If
// the password is gone the device share is gone, and the account is dead with
// the money in it. The spec requires that a password can be reset without any
// party ever reconstructing a whole key, and two shares cannot do that.
//
// So: three shares, any two of which sign.
//
//	device     wrapped by the user's password, on their own hardware
//	custodian  the operator's service
//	recovery   the foundation's offline custody, the same 3-of-5 that x/constitution
//	           pins as the seizure destination
//
// Ordinary signing is device + custodian. The operator holds one share and
// cannot sign. Recovery is recovery + custodian, and only after the workflow in
// Part 5 of the spec — two approvers, a delay, notice to every enrolled device.
// The recovery share is deliberately NOT held by the operator: an operator
// holding both custodian and recovery would be a custodian again, with an extra
// step.
//
// # Why threshold ECDSA rather than a Cosmos multisig
//
// A 2-of-3 x/group or legacy multisig account would give the same "no single
// party signs" property with no new cryptography, and it was the first thing
// considered. Two things ruled it out.
//
// The address. A multisig account's address is derived from the set of member
// keys, so rotating a share — which is exactly what a password reset is —
// changes the address. On this chain an address carries the account's x/alias
// identifier, and changing it retires that identifier: the handle a person has
// given to everyone who pays them stops resolving. Threshold ECDSA re-shares
// under the SAME public key, so a reset is invisible to everybody else.
//
// Enumeration. Every consumer account would carry the operator's share pubkey
// in its multisig, on a public chain, making the whole consumer base trivially
// countable and linkable by anyone. A threshold signature is an ordinary
// secp256k1 signature and an account holding one is indistinguishable from a
// self-custodied account.
//
// # What this package is not
//
// It is the protocol, not the service. It performs no authentication, decides
// nothing about who may hold a share, and moves no message between parties —
// the caller supplies the transport, because the transport is where the
// authentication lives and this package must not be the place that gets it
// wrong. See tools/custodian for the service that uses it.
package mpc

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// The three shares of one account. Named rather than numbered, because an
// operator reading a log needs to know which one they are looking at, and
// "share 2" tells them nothing about whether it should have been reachable.
const (
	RoleDevice    = "device"
	RoleCustodian = "custodian"
	RoleRecovery  = "recovery"
)

// Roles is the canonical order. Party identity in tss-lib is positional, so
// this ordering is part of the wire format and must not be rearranged.
var Roles = []string{RoleDevice, RoleCustodian, RoleRecovery}

// Threshold is tss-lib's t, where t+1 parties must cooperate. One means any
// two of the three sign, which is the whole design.
const Threshold = 1

// KeygenTimeout bounds the safe-prime search. It is generous because the
// alternative to waiting is a weaker prime.
const KeygenTimeout = 10 * time.Minute

// Share is one party's secret material plus everything it needs to know about
// the others. It is the thing that gets sealed and stored, and it is the thing
// that must never be transmitted.
type Share struct {
	Role string                    `json:"role"`
	Data keygen.LocalPartySaveData `json:"data"`
}

// PublicKey returns the account's public key, which every share agrees on and
// none of them can produce a signature from alone.
func (s Share) PublicKey() (*ecdsa.PublicKey, error) {
	if s.Data.ECDSAPub == nil {
		return nil, errors.New("share carries no public key")
	}
	return s.Data.ECDSAPub.ToECDSAPubKey(), nil
}

// partyIDs builds the fixed party set. Sorted by tss-lib's own rule so every
// participant derives the same ordering from the same roles.
func partyIDs() tss.SortedPartyIDs {
	ids := make(tss.UnSortedPartyIDs, 0, len(Roles))
	for i, role := range Roles {
		ids = append(ids, tss.NewPartyID(role, role, big.NewInt(int64(i+1))))
	}
	return tss.SortPartyIDs(ids)
}

func findParty(ids tss.SortedPartyIDs, role string) (*tss.PartyID, error) {
	for _, id := range ids {
		if id.Id == role {
			return id, nil
		}
	}
	return nil, fmt.Errorf("no party for role %q", role)
}

// GeneratePreParams produces the expensive half of key generation: the safe
// primes underlying a party's Paillier key.
//
// Split out because it takes minutes and depends on nothing — not the other
// parties, not the account, not the user. A custodian keeps a pool of these
// generated in advance, so that signing up does not mean watching a phone
// think for four minutes. It is the single biggest reason a naive threshold
// implementation feels unusable.
func GeneratePreParams(timeout time.Duration) (*keygen.LocalPreParams, error) {
	p, err := keygen.GeneratePreParams(timeout)
	if err != nil {
		return nil, fmt.Errorf("generating pre-parameters: %w", err)
	}
	return p, nil
}

// Keygen runs distributed key generation for all three parties.
//
// Every party runs in this process, which is right for a test and for a
// ceremony performed on one machine, and wrong for production — where the
// device's share must be generated on the device and never leave it. The
// production path drives the same tss-lib parties over a real transport; this
// function exists so the protocol can be exercised and asserted on without a
// network.
//
// preParams may be nil, in which case they are generated here and it will take
// minutes per party.
func Keygen(preParams map[string]*keygen.LocalPreParams) (map[string]Share, error) {
	ids := partyIDs()
	ctx := tss.NewPeerContext(ids)
	curve := tss.S256()

	outCh := make(chan tss.Message, len(Roles)*8)
	endCh := make(chan *keygen.LocalPartySaveData, len(Roles))

	parties := make(map[string]tss.Party, len(Roles))
	var mu sync.Mutex
	errCh := make(chan error, len(Roles))

	for _, role := range Roles {
		id, err := findParty(ids, role)
		if err != nil {
			return nil, err
		}
		params := tss.NewParameters(curve, ctx, id, len(ids), Threshold)
		var pre *keygen.LocalPreParams
		if preParams != nil {
			pre = preParams[role]
		}
		var p tss.Party
		if pre != nil {
			p = keygen.NewLocalParty(params, outCh, endCh, *pre)
		} else {
			p = keygen.NewLocalParty(params, outCh, endCh)
		}
		parties[role] = p
	}

	for _, role := range Roles {
		p := parties[role]
		go func() {
			if err := p.Start(); err != nil {
				errCh <- err
			}
		}()
	}

	saved := make(map[string]Share, len(Roles))
	for len(saved) < len(Roles) {
		select {
		case err := <-errCh:
			return nil, fmt.Errorf("key generation: %w", err)
		case msg := <-outCh:
			if err := route(msg, parties, &mu); err != nil {
				return nil, err
			}
		case data := <-endCh:
			// tss-lib does not tell us which party finished, so it is recovered
			// from the share index the party was given.
			role, err := roleOfSave(ids, data)
			if err != nil {
				return nil, err
			}
			saved[role] = Share{Role: role, Data: *data}
		}
	}
	return saved, nil
}

// roleOfSave identifies a finished party by its own share index.
func roleOfSave(ids tss.SortedPartyIDs, data *keygen.LocalPartySaveData) (string, error) {
	if data.ShareID == nil {
		return "", errors.New("save data carries no share id")
	}
	for _, id := range ids {
		if id.KeyInt().Cmp(data.ShareID) == 0 {
			return id.Id, nil
		}
	}
	return "", fmt.Errorf("share id %s belongs to no known party", data.ShareID)
}

// Sign produces one secp256k1 signature over hash using exactly the shares
// given, which must be at least Threshold+1 of them.
//
// The signers are named rather than inferred. A function that quietly used
// whatever shares it could reach would be one that signs with the custodian and
// the recovery share on a day somebody left both on the same machine, which is
// the arrangement this whole design exists to prevent — so the caller says who
// is signing and the refusal below is the last check that they meant it.
func Sign(hash []byte, shares map[string]Share) ([]byte, *ecdsa.PublicKey, error) {
	if len(shares) < Threshold+1 {
		return nil, nil, fmt.Errorf(
			"%d share(s) cannot sign: %d of %d are needed", len(shares), Threshold+1, len(Roles))
	}
	if len(hash) != 32 {
		return nil, nil, fmt.Errorf("expected a 32-byte hash, got %d bytes", len(hash))
	}

	// Party identity comes from each share's own id, not from a table keyed by
	// role. A reshare issues new ids under the same public key — that is what
	// makes a password reset invisible to everybody else — so a signer looked up
	// by role in a fixed table would be handed the id it had before the reset,
	// which is in nobody's key list any more and produces a protocol that hangs
	// rather than an error anybody can read.
	unsorted := make(tss.UnSortedPartyIDs, 0, len(shares))
	roles := make([]string, 0, len(shares))
	for role := range shares {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	for _, role := range roles {
		id := shares[role].Data.ShareID
		if id == nil {
			return nil, nil, fmt.Errorf("the %s share carries no share id", role)
		}
		unsorted = append(unsorted, tss.NewPartyID(role, role, id))
	}
	ids := tss.SortPartyIDs(unsorted)
	ctx := tss.NewPeerContext(ids)
	curve := tss.S256()

	outCh := make(chan tss.Message, len(ids)*16)
	endCh := make(chan *common.SignatureData, len(ids))
	errCh := make(chan error, len(ids))
	parties := make(map[string]tss.Party, len(ids))
	var mu sync.Mutex

	msg := new(big.Int).SetBytes(hash)
	for _, id := range ids {
		share, ok := shares[id.Id]
		if !ok {
			return nil, nil, fmt.Errorf("no share supplied for %s", id.Id)
		}
		params := tss.NewParameters(curve, ctx, id, len(ids), Threshold)
		parties[id.Id] = signing.NewLocalParty(msg, params, share.Data, outCh, endCh)
	}
	for _, id := range ids {
		p := parties[id.Id]
		go func() {
			if err := p.Start(); err != nil {
				errCh <- err
			}
		}()
	}

	var sig *common.SignatureData
	done := 0
	for done < len(ids) {
		select {
		case err := <-errCh:
			return nil, nil, fmt.Errorf("signing: %w", err)
		case m := <-outCh:
			if err := route(m, parties, &mu); err != nil {
				return nil, nil, err
			}
		case s := <-endCh:
			sig = s
			done++
		}
	}
	if sig == nil {
		return nil, nil, errors.New("signing produced no signature")
	}

	var any Share
	for _, s := range shares {
		any = s
		break
	}
	pub, err := any.PublicKey()
	if err != nil {
		return nil, nil, err
	}
	return normalise(sig), pub, nil
}

// normalise returns the 64-byte [R||S] Cosmos expects, with S in the lower half
// of the curve order.
//
// A secp256k1 signature has two valid forms — (r, s) and (r, n-s) — and both
// verify. Cosmos rejects the high one, because a signature with two encodings
// is a transaction with two hashes, and a transaction with two hashes can be
// replayed under the identity it was not indexed under. tss-lib already
// normalises internally; this repeats it rather than trusting it, since the
// cost is one comparison and the failure is silent.
func normalise(sig *common.SignatureData) []byte {
	r := new(big.Int).SetBytes(sig.R)
	s := new(big.Int).SetBytes(sig.S)

	order := secp256k1.S256().N
	half := new(big.Int).Rsh(order, 1)
	if s.Cmp(half) > 0 {
		s = new(big.Int).Sub(order, s)
	}

	out := make([]byte, 64)
	r.FillBytes(out[:32])
	s.FillBytes(out[32:])
	return out
}

// route delivers one protocol message to its recipients.
//
// Broadcast messages go to every party except the sender; a point-to-point
// message goes only to the party it names. Getting this wrong does not produce
// a wrong signature, it produces a protocol that hangs — which is why the
// sender is excluded explicitly rather than by the parties ignoring their own.
func route(msg tss.Message, parties map[string]tss.Party, mu *sync.Mutex) error {
	data, _, err := msg.WireBytes()
	if err != nil {
		return fmt.Errorf("serialising a protocol message: %w", err)
	}
	from := msg.GetFrom()

	deliver := func(p tss.Party) error {
		mu.Lock()
		defer mu.Unlock()
		if _, err := p.UpdateFromBytes(data, from, msg.IsBroadcast()); err != nil {
			return fmt.Errorf("delivering to %s: %w", p.PartyID().Id, err)
		}
		return nil
	}

	if msg.IsBroadcast() {
		for id, p := range parties {
			if id == from.Id {
				continue
			}
			if err := deliver(p); err != nil {
				return err
			}
		}
		return nil
	}
	for _, to := range msg.GetTo() {
		p, ok := parties[to.Id]
		if !ok {
			return fmt.Errorf("message addressed to unknown party %s", to.Id)
		}
		if err := deliver(p); err != nil {
			return err
		}
	}
	return nil
}
