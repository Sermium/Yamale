package mpc

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sync"

	"github.com/bnb-chain/tss-lib/v2/crypto"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/resharing"
	"github.com/bnb-chain/tss-lib/v2/tss"
)

// Reshare replaces every share with a fresh one, under the same public key and
// therefore at the same address.
//
// # What it is for
//
// A password reset. The device share is wrapped by the user's password, so a
// forgotten password destroys it. Recovery and the custodian — the other two —
// reshare, and the device is handed a new share wrapped under the new password.
// The old share is not revoked so much as made arithmetically useless: it
// belongs to a sharing that no longer reconstructs anything.
//
// It is also proactive security independent of any incident. An attacker who
// has patiently obtained one share and is waiting for a second has to start
// again after every reshare, because shares from two different sharings do not
// combine. Running it on a schedule turns "one share stolen" from a permanent
// half-compromise into a race with a deadline.
//
// # What it does NOT do
//
// It does not change the account. The address, the public key and the x/alias
// identifier are untouched, and nobody who has ever paid this person needs to
// be told anything. That is the property a Cosmos multisig could not offer:
// there, rotating a member changes the address, which retires the identifier
// and silently breaks every saved payee.
//
// It decides nothing about authorisation. Whether this person may reset their
// password — two approvers, the delay, notice to every enrolled device — is the
// account service's business, and by the time Reshare is called that question
// is already answered. See docs/guides/accounts.md, Part 5, which is blunt that
// recovery rather than login is what actually gets robbed.
//
// # Why Threshold+1 old shares are enough, and what that cost to learn
//
// Resharing is not "generate new shares and hand them out". No party holds the
// secret, so no party can re-split it: the old committee runs a protocol that
// projects its sharing onto a new one. Threshold+1 of them suffice, which is
// what makes a password reset possible at all — the device share is exactly the
// one that is gone.
//
// Getting that right depends on a distinction tss-lib makes and does not
// explain. The old PEER CONTEXT must contain only the parties actually taking
// part, while the party COUNT passed alongside it is the original sharing's
// size. Build the context from all three when only two are present and the
// incoming committee waits for a third sender that will never speak: no error,
// no timeout, just a goroutine in select. That cost half a day and two
// abandoned test runs, and it is the reason this function is written against
// tss-lib's own resharing test rather than against its documentation.
//
// preParams supplies the incoming committee's Paillier material. Nil is
// correct and slow — each new party generates its own inside the protocol,
// minutes apiece — so in production the custodian passes a pool and a reset is
// seconds rather than a quarter of an hour somebody phones support about.
func Reshare(old map[string]Share, preParams map[string]*keygen.LocalPreParams) (map[string]Share, error) {
	if len(old) < Threshold+1 {
		return nil, fmt.Errorf(
			"resharing needs %d of the %d shares and was given %d", Threshold+1, len(Roles), len(old))
	}

	// The participating old parties, and ONLY those. See the note above.
	oldIDs, err := participating(old)
	if err != nil {
		return nil, err
	}

	// The incoming committee. Its ids differ from the outgoing ones because
	// both committees are live at once and tss-lib addresses by index within a
	// committee — identical ids make every routing decision a guess.
	newUnsorted := make(tss.UnSortedPartyIDs, 0, len(Roles))
	for i, role := range Roles {
		newUnsorted = append(newUnsorted, tss.NewPartyID(incoming(role), role, freshKey(oldIDs, i)))
	}
	newIDs := tss.SortPartyIDs(newUnsorted)

	oldCtx := tss.NewPeerContext(oldIDs)
	newCtx := tss.NewPeerContext(newIDs)
	curve := tss.S256()

	total := len(oldIDs) + len(newIDs)
	outCh := make(chan tss.Message, total*8)
	endCh := make(chan *keygen.LocalPartySaveData, total)
	errCh := make(chan error, total)

	// Held as slices, because tss-lib addresses a resharing message by the
	// destination's INDEX inside its committee rather than by its id.
	oldParties := make([]tss.Party, len(oldIDs))
	newParties := make([]tss.Party, len(newIDs))

	for i, id := range oldIDs {
		// A COPY, and this is not defensive style — it is required.
		//
		// tss-lib retires an outgoing party by zeroing its Xi in place, and
		// LocalPartySaveData is full of pointers, so handing it the caller's
		// share destroys the caller's share. In a test that shows up as a later
		// case failing for no visible reason. In production it is worse: a
		// reshare that dies halfway through would leave the custodian holding a
		// gutted share, and an account whose remaining shares can no longer
		// reach the threshold is an account nobody can ever spend from again.
		//
		// The old shares SHOULD be retired, but only once the new ones exist and
		// the caller has stored them — which is the caller's decision and not
		// this function's.
		share, err := cloneShare(old[id.Moniker])
		if err != nil {
			return nil, err
		}
		params := tss.NewReSharingParameters(
			curve, oldCtx, newCtx, id,
			// The original sharing's size, not the number turning up.
			len(Roles), Threshold, len(newIDs), Threshold,
		)
		oldParties[i] = resharing.NewLocalParty(params, share.Data, outCh, endCh)
	}
	for i, id := range newIDs {
		params := tss.NewReSharingParameters(
			curve, oldCtx, newCtx, id,
			len(Roles), Threshold, len(newIDs), Threshold,
		)
		blank := keygen.NewLocalPartySaveData(len(newIDs))
		if preParams != nil {
			if pre, ok := preParams[id.Moniker]; ok && pre != nil {
				blank.LocalPreParams = *pre
			}
		}
		newParties[i] = resharing.NewLocalParty(params, blank, outCh, endCh)
	}

	var mu sync.Mutex
	start := func(p tss.Party) {
		go func() {
			if err := p.Start(); err != nil {
				errCh <- err
			}
		}()
	}
	for _, p := range oldParties {
		start(p)
	}
	for _, p := range newParties {
		start(p)
	}

	fresh := make(map[string]Share, len(Roles))
	ended := 0
	for ended < total {
		select {
		case err := <-errCh:
			return nil, fmt.Errorf("resharing: %w", err)

		case msg := <-outCh:
			if err := routeReshare(msg, oldParties, newParties, &mu); err != nil {
				return nil, err
			}

		case save := <-endCh:
			ended++
			// An outgoing party finishing has its Xi zeroed: it is being
			// retired, not issued anything. That flag is how the two
			// committees are told apart, and it is not the share id — after a
			// reshare an old and a new party can report ids that look equally
			// plausible.
			if save.Xi == nil {
				continue
			}
			index, err := save.OriginalIndex()
			if err != nil {
				return nil, fmt.Errorf("a new share reports no index: %w", err)
			}
			if index < 0 || index >= len(newIDs) {
				return nil, fmt.Errorf("a new share reports index %d, outside the committee", index)
			}
			role := newIDs[index].Moniker
			fresh[role] = Share{Role: role, Data: *save}
		}
	}

	if len(fresh) != len(Roles) {
		return nil, fmt.Errorf(
			"resharing produced %d shares, expected %d", len(fresh), len(Roles))
	}
	// The point of the exercise, asserted rather than assumed: same key, same
	// address, different shares. A reshare that moved the public key would have
	// quietly orphaned every coin in the account.
	if err := sameKey(old, fresh); err != nil {
		return nil, err
	}
	return fresh, nil
}

// cloneShare deep-copies a share, so the protocol can retire its copy without
// touching the caller's.
//
// Through JSON rather than field by field: LocalPartySaveData is a large
// structure of big.Int pointers that tss-lib is free to extend, and a
// hand-written copy silently stops being deep the next time it gains a field.
func cloneShare(in Share) (Share, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return Share{}, fmt.Errorf("copying the %s share: %w", in.Role, err)
	}
	var out Share
	if err := json.Unmarshal(raw, &out); err != nil {
		return Share{}, fmt.Errorf("copying the %s share: %w", in.Role, err)
	}
	return out, nil
}

// participating builds the peer context from the shares actually present.
//
// Their party ids are the ones the original sharing gave them — taken from each
// share rather than reconstructed — because a reshare against invented ids
// projects the wrong polynomial and produces a key nobody's coins are under.
func participating(shares map[string]Share) (tss.SortedPartyIDs, error) {
	unsorted := make(tss.UnSortedPartyIDs, 0, len(shares))
	for _, role := range Roles {
		s, present := shares[role]
		if !present {
			continue
		}
		if s.Data.ShareID == nil {
			return nil, fmt.Errorf("the %s share carries no share id", role)
		}
		// Moniker carries the role so the caller's map can be indexed by it
		// later; Id stays unique per party.
		unsorted = append(unsorted, tss.NewPartyID(role, role, s.Data.ShareID))
	}
	if len(unsorted) < Threshold+1 {
		return nil, fmt.Errorf("only %d usable shares", len(unsorted))
	}
	return tss.SortPartyIDs(unsorted), nil
}

// incoming names a party of the new committee so it cannot be confused with the
// outgoing party of the same role.
func incoming(role string) string { return role + "'" }

// freshKey produces a party id for the new committee that cannot collide with
// any id in the old one.
func freshKey(old tss.SortedPartyIDs, i int) *big.Int {
	max := big.NewInt(0)
	for _, id := range old {
		if id.KeyInt().Cmp(max) > 0 {
			max = id.KeyInt()
		}
	}
	return new(big.Int).Add(max, big.NewInt(int64(i+1)))
}

func sameKey(old, fresh map[string]Share) error {
	var before, after *crypto.ECPoint
	for _, s := range old {
		before = s.Data.ECDSAPub
		break
	}
	for _, s := range fresh {
		after = s.Data.ECDSAPub
		break
	}
	if before == nil || after == nil {
		return fmt.Errorf("a sharing with no public key")
	}
	if !before.Equals(after) {
		return fmt.Errorf("resharing changed the public key, which would orphan the account")
	}
	for role, s := range fresh {
		if s.Data.ECDSAPub == nil || !s.Data.ECDSAPub.Equals(after) {
			return fmt.Errorf("the new %s share disagrees about the public key", role)
		}
	}
	return nil
}

// routeReshare delivers one message, following tss-lib's own rule.
//
// A resharing message is not simply broadcast or point-to-point. It is
// addressed to the old committee, the new one, or both, and the destination
// index is an index INTO THAT COMMITTEE. Treating it as an ordinary broadcast —
// everyone except the sender — delivers half the rounds to parties that have no
// use for them and none to the parties waiting, which presents as a protocol
// that hangs rather than one that fails.
func routeReshare(msg tss.Message, oldParties, newParties []tss.Party, mu *sync.Mutex) error {
	wire, _, err := msg.WireBytes()
	if err != nil {
		return fmt.Errorf("serialising a resharing message: %w", err)
	}
	from := msg.GetFrom()
	dest := msg.GetTo()
	if dest == nil {
		return fmt.Errorf("a resharing message with no destination, from %s", from.Id)
	}

	deliver := func(p tss.Party) error {
		if p == nil {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		if _, err := p.UpdateFromBytes(wire, from, msg.IsBroadcast()); err != nil {
			return fmt.Errorf("delivering to %s: %w", p.PartyID().Id, err)
		}
		return nil
	}

	if msg.IsToOldCommittee() || msg.IsToOldAndNewCommittees() {
		limit := len(oldParties)
		if len(dest) < limit {
			limit = len(dest)
		}
		for _, to := range dest[:limit] {
			if to.Index < 0 || to.Index >= len(oldParties) {
				continue
			}
			if err := deliver(oldParties[to.Index]); err != nil {
				return err
			}
		}
	}
	if !msg.IsToOldCommittee() || msg.IsToOldAndNewCommittees() {
		for _, to := range dest {
			if to.Index < 0 || to.Index >= len(newParties) {
				continue
			}
			if err := deliver(newParties[to.Index]); err != nil {
				return err
			}
		}
	}
	return nil
}
