package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/x/group"
)

func TestBuildReplacementEmitsTheSwapAndNothingElse(t *testing.T) {
	people := custodians(t, 6)
	current, incoming := people[:5], people[5]

	update, proposal, err := buildReplacement(current, current[1], incoming, firstPolicyAddress, 1)
	if err != nil {
		t.Fatal(err)
	}

	var msg struct {
		Admin         string                `json:"admin"`
		GroupID       string                `json:"group_id"`
		MemberUpdates []group.MemberRequest `json:"member_updates"`
	}
	if err := json.Unmarshal(update, &msg); err != nil {
		t.Fatal(err)
	}

	// Two updates, one of each. A message carrying only the removal is what
	// this whole command exists to make unavailable.
	if len(msg.MemberUpdates) != 2 {
		t.Fatalf("%d member updates, want a removal and an addition", len(msg.MemberUpdates))
	}
	removals, additions := 0, 0
	for _, member := range msg.MemberUpdates {
		switch member.Weight {
		case "0":
			removals++
			if member.Address != current[1].Address {
				t.Fatalf("removed %s, not the departing custodian", member.Address)
			}
		case "1":
			additions++
			if member.Address != incoming.Address {
				t.Fatalf("added %s, not the incoming custodian", member.Address)
			}
		default:
			t.Fatalf("unexpected weight %q", member.Weight)
		}
	}
	if removals != 1 || additions != 1 {
		t.Fatalf("%d removals and %d additions", removals, additions)
	}
	// The admin is the policy, because the group administers itself.
	if msg.Admin != firstPolicyAddress {
		t.Fatalf("admin = %q", msg.Admin)
	}

	// The proposal has to say why, because a change to who can move seized
	// property should be readable by somebody who was not in the room.
	if !strings.Contains(string(proposal), incoming.Fingerprint) {
		t.Fatal("the proposal does not carry the incoming custodian's fingerprint")
	}
	if !strings.Contains(string(proposal), "one decision") {
		t.Fatal("the proposal does not say that removal and replacement are one decision")
	}
}

func TestBuildReplacementRefusesAnIncomingCustodianWhoIsAlreadyOne(t *testing.T) {
	// The swap that is not a swap: the group would come out one member short
	// while the message looks like a proper replacement.
	people := custodians(t, 5)

	_, _, err := buildReplacement(people, people[1], people[2], firstPolicyAddress, 1)
	if err == nil {
		t.Fatal("an existing custodian was accepted as the replacement")
	}
	if !strings.Contains(err.Error(), "already a custodian") {
		t.Fatalf("the error does not explain what is wrong: %v", err)
	}
}

func TestBuildReplacementRefusesAnOutgoingCustodianWhoIsNotInTheGroup(t *testing.T) {
	people := custodians(t, 7)

	_, _, err := buildReplacement(people[:5], people[5], people[6], firstPolicyAddress, 1)
	if err == nil {
		t.Fatal("a replacement was built for somebody who is not a custodian")
	}
}

func TestBuildReplacementRefusesAValidatorOperatorKey(t *testing.T) {
	// An incoming custodian's key comes from the same ceremony as everybody
	// else's, not from whatever key the person already had.
	people := custodians(t, 6)
	incoming := people[5]
	incoming.Role = roleValidator

	if _, _, err := buildReplacement(people[:5], people[1], incoming, firstPolicyAddress, 1); err == nil {
		t.Fatal("a validator operator key was accepted as a custodian")
	}
}

func TestBuildReplacementRefusesAnUnreadableAddress(t *testing.T) {
	people := custodians(t, 6)
	incoming := people[5]
	incoming.Address = "yml1notanaddress"

	if _, _, err := buildReplacement(people[:5], people[1], incoming, firstPolicyAddress, 1); err == nil {
		t.Fatal("an address this chain cannot decode was accepted")
	}
}

func TestDescribeSwapNamesBothPeopleAndBothFingerprints(t *testing.T) {
	// This line goes into the ceremony record, which is the only place anybody
	// will look in five years to work out whose envelope is whose.
	people := custodians(t, 2)

	note := describeSwap(people[0], people[1])
	for _, want := range []string{
		people[0].Name, people[0].Fingerprint, people[1].Name, people[1].Fingerprint,
	} {
		if !strings.Contains(note, want) {
			t.Fatalf("the record note omits %q: %s", want, note)
		}
	}
}

func TestBuildGroupRefusesADerivedAddressAsACustodian(t *testing.T) {
	// A group policy sitting in this group could submit a proposal to it, and
	// the messages a proposal executes never pass the ante chain.
	people := custodians(t, 5)
	people[3].Address = firstPolicyAddress

	_, err := buildGroup(people, foundationPurpose(), 3, 0, 1, testTime())
	if err == nil {
		t.Fatal("a 32-byte derived address was accepted as a custodian")
	}
	if !strings.Contains(err.Error(), "derived account") {
		t.Fatalf("the error does not say what is wrong: %v", err)
	}
}
