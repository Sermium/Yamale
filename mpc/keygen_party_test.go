package mpc_test

import (
	"crypto/ecdsa"
	"encoding/json"
	"testing"
	"time"

	"yamale/blockchain/mpc"
)

// deliver moves one party's outbound messages to the parties they address.
//
// Written out rather than hidden in a helper the tests share with the library,
// because the routing is the part a real deployment has to reimplement over
// HTTP and getting it wrong does not corrupt anything — it hangs. A broadcast
// that reaches two of three peers produces a protocol that waits forever with
// nothing logged, so the rule is worth seeing in full at least once.
func deliver(t *testing.T, from string, msgs []mpc.Outbound, parties map[string]*mpc.KeygenParty) {
	t.Helper()
	for _, m := range msgs {
		targets := m.To
		if m.Broadcast {
			targets = nil
			for role := range parties {
				if role != from {
					targets = append(targets, role)
				}
			}
		}
		for _, to := range targets {
			p, ok := parties[to]
			if !ok {
				t.Fatalf("message addressed to unknown party %q", to)
			}
			if err := p.Handle(m); err != nil {
				t.Fatalf("%s handling a message from %s: %v", to, from, err)
			}
		}
	}
}

// runKeygen drives three separate parties to completion, exchanging only the
// bytes a transport would carry.
//
// It polls with a short wait rather than declaring a stall the moment a pass
// produces nothing. That is not test-tuning: tss-lib starts each party in its
// own goroutine, so the first poll legitimately finds an empty queue, and an
// HTTP server driving this has exactly the same shape — a request arrives, the
// party may not have spoken yet, and the honest answer is "nothing to send
// yet" rather than "the protocol has failed".
func driveKeygen(t *testing.T, parties map[string]*mpc.KeygenParty, wire *[]byte) map[string]mpc.Share {
	t.Helper()
	shares := make(map[string]mpc.Share, len(mpc.Roles))
	deadline := time.Now().Add(3 * time.Minute)

	for len(shares) < len(mpc.Roles) {
		progressed := false
		for _, role := range mpc.Roles {
			p := parties[role]
			out, err := p.Outbound()
			if err != nil {
				t.Fatalf("%s: %v", role, err)
			}
			if len(out) > 0 {
				if wire != nil {
					for _, m := range out {
						*wire = append(*wire, m.Wire...)
					}
				}
				deliver(t, role, out, parties)
				progressed = true
			}
			if _, done := shares[role]; !done {
				if share, ok := p.Share(); ok {
					shares[role] = share
					progressed = true
				}
			}
		}
		if progressed {
			continue
		}
		for _, role := range mpc.Roles {
			if err := parties[role].Err(); err != nil {
				t.Fatalf("key generation failed in %s: %v", role, err)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("key generation stalled with %d of %d shares", len(shares), len(mpc.Roles))
		}
		time.Sleep(20 * time.Millisecond)
	}
	return shares
}

func startParties(t *testing.T) map[string]*mpc.KeygenParty {
	t.Helper()
	parties := make(map[string]*mpc.KeygenParty, len(mpc.Roles))
	for _, role := range mpc.Roles {
		p, err := mpc.NewKeygenParty(role, preParamFor(t, role))
		if err != nil {
			t.Fatalf("starting %s: %v", role, err)
		}
		parties[role] = p
	}
	return parties
}

func runKeygen(t *testing.T) map[string]mpc.Share {
	t.Helper()
	return driveKeygen(t, startParties(t), nil)
}

// TestKeygenPartyProducesAUsableSharing is the claim the type exists for: three
// parties that never see each other's share nevertheless end up holding one
// sharing of one key, and any two of them can sign for it.
func TestKeygenPartyProducesAUsableSharing(t *testing.T) {
	shares := runKeygen(t)

	// One public key, agreed by everybody. If the parties disagreed here the
	// account would have three addresses and the failure would surface as
	// "signature verification failed" on a payment, days later.
	var first *ecdsa.PublicKey
	for _, role := range mpc.Roles {
		pub, err := shares[role].PublicKey()
		if err != nil {
			t.Fatalf("%s public key: %v", role, err)
		}
		if first == nil {
			first = pub
			continue
		}
		if pub.X.Cmp(first.X) != 0 || pub.Y.Cmp(first.Y) != 0 {
			t.Fatalf("%s computed a different public key from %s", role, mpc.Roles[0])
		}
	}

	// And the sharing actually signs, with the pair a deployment uses: the
	// customer's device and the custodian. Generating a key nobody can sign
	// with is a failure this test would otherwise pass.
	digest := make([]byte, 32)
	for i := range digest {
		digest[i] = byte(i + 1)
	}
	sig, pub, err := mpc.Sign(digest, map[string]mpc.Share{
		"device":    shares["device"],
		"custodian": shares["custodian"],
	})
	if err != nil {
		t.Fatalf("signing with the shares just generated: %v", err)
	}
	if len(sig) != 64 {
		t.Fatalf("expected a 64-byte signature, got %d", len(sig))
	}
	if pub.X.Cmp(first.X) != 0 || pub.Y.Cmp(first.Y) != 0 {
		t.Fatal("the signature is for a different key than the one generated")
	}
}

// TestKeygenPartyKeepsItsShareToItself is the property the whole design rests
// on, asserted rather than assumed: nothing a party puts on the wire lets
// another party reconstruct its share.
//
// Not a proof — that is tss-lib's to make — but it catches the failure that
// would actually happen in this repository, which is somebody "simplifying" the
// transport by putting the save data in an Outbound.
func TestKeygenPartyKeepsItsShareToItself(t *testing.T) {
	var wire []byte
	shares := driveKeygen(t, startParties(t), &wire)
	if len(wire) == 0 {
		t.Fatal("no protocol traffic was captured, so this test proves nothing")
	}

	// The secret scalar of each share must not appear anywhere in the traffic.
	// Xi is the value that, with one other, reconstructs the key.
	for _, role := range mpc.Roles {
		xi := shares[role].Data.Xi
		if xi == nil {
			t.Fatalf("%s has no secret share", role)
		}
		if containsBytes(wire, xi.Bytes()) {
			t.Fatalf("%s's secret share appears in the protocol traffic", role)
		}
	}
}

// TestKeygenPartyRefusesItsOwnMessage guards the mistake that produces a hang
// rather than an error: a transport that echoes a broadcast back to its sender.
func TestKeygenPartyRefusesItsOwnMessage(t *testing.T) {
	p, err := mpc.NewKeygenParty("custodian", preParamFor(t, "custodian"))
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	if err := p.Handle(mpc.Outbound{From: "custodian", Broadcast: true, Wire: []byte{1}}); err == nil {
		t.Fatal("a party accepted its own message")
	}
}

// TestKeygenPartyRefusesAnUnknownRole checks the other half of the same guard.
func TestKeygenPartyRefusesAnUnknownRole(t *testing.T) {
	if _, err := mpc.NewKeygenParty("auditor", nil); err == nil {
		t.Fatal("a party was started for a role that does not exist")
	}
	p, err := mpc.NewKeygenParty("custodian", preParamFor(t, "custodian"))
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	if err := p.Handle(mpc.Outbound{From: "auditor", Broadcast: true, Wire: []byte{1}}); err == nil {
		t.Fatal("a party accepted a message from a role that does not exist")
	}
}

// TestKeygenPartyShareSurvivesSerialisation: the custodian seals its share to
// disk as JSON, so a share that cannot survive that round trip is one that
// works in a test and fails on the first restart.
func TestKeygenPartyShareSurvivesSerialisation(t *testing.T) {
	shares := runKeygen(t)

	raw, err := json.Marshal(shares["custodian"])
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	var back mpc.Share
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}

	before, err := shares["custodian"].PublicKey()
	if err != nil {
		t.Fatalf("public key before: %v", err)
	}
	after, err := back.PublicKey()
	if err != nil {
		t.Fatalf("public key after: %v", err)
	}
	if before.X.Cmp(after.X) != 0 || before.Y.Cmp(after.Y) != 0 {
		t.Fatal("the share changed key across a JSON round trip")
	}

	digest := make([]byte, 32)
	digest[0] = 9
	if _, _, err := mpc.Sign(digest, map[string]mpc.Share{
		"device":    shares["device"],
		"custodian": back,
	}); err != nil {
		t.Fatalf("signing with a deserialised share: %v", err)
	}
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
outer:
	for i := 0; i+len(needle) <= len(haystack); i++ {
		for j := range needle {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return true
	}
	return false
}
