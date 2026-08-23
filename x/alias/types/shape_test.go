package types_test

import (
	"strings"
	"testing"

	"yamale/blockchain/x/alias/types"
)

// The unit tests for OfficeShape: the floor arithmetic and the rule about what a
// requirement may say, tested where they live.
//
// The keeper's tests already cover the behaviour end to end — an office voting
// itself smaller and losing its authority — and a mutation pass showed why these
// are here as well. Weakening either floor in Satisfies is caught only by the
// keeper's suite, so the arithmetic that decides whether a national authority may
// act had no test in the package that owns it. A rule tested only through the
// thing that calls it is a rule that survives being rewritten by somebody
// refactoring the caller.

func TestOfficeShapeFloorsAreFloors(t *testing.T) {
	want := &types.OfficeShape{Signatures: 3, Members: 5}

	for _, tc := range []struct {
		name              string
		signatures, membs uint32
		met               bool
	}{
		{"exactly as granted", 3, 5, true},
		{"a member joined", 3, 6, true},
		{"and the office tightened", 4, 6, true},
		{"far larger", 9, 20, true},
		{"unanimity: self-harm, not capture", 5, 5, true},
		{"a member left and was not replaced", 3, 4, false},
		{"the threshold was lowered", 2, 5, false},
		{"both", 1, 1, false},
		{"nothing at all", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := want.Satisfies(tc.signatures, tc.membs); got != tc.met {
				t.Fatalf("a %d-of-%d office against a required %s: got %v, want %v",
					tc.signatures, tc.membs, want.Rule(), got, tc.met)
			}
		})
	}
}

// Nil is "no requirement", and it satisfies everything.
//
// Asserted rather than assumed, because it is the decision about every grant made
// before required_shape existed: absence means unconstrained, and the honest cost
// is that those holders can still shrink to a single key.
func TestANilShapeIsNoRequirement(t *testing.T) {
	var none *types.OfficeShape
	if !none.Satisfies(1, 1) {
		t.Fatal("a grant with no recorded shape must not constrain its holder")
	}
	if !none.Satisfies(0, 0) {
		t.Fatal("and it must not constrain it at any size")
	}
	if err := none.Validate(); err != nil {
		t.Fatalf("an absent requirement is valid; got %v", err)
	}
	if none.Rule() != "no required shape" {
		t.Fatalf("nil rendered as %q; a blank or a 0-of-0 would read as a requirement", none.Rule())
	}
}

func TestOfficeShapeValidateRefusesWhatNoOfficeCouldMean(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shape types.OfficeShape
		says  string
	}{
		{"no signatures", types.OfficeShape{Signatures: 0, Members: 5}, "omit required_shape entirely"},
		{"nothing at all", types.OfficeShape{}, "omit required_shape entirely"},
		{"more signatures than members", types.OfficeShape{Signatures: 3, Members: 2}, "could ever satisfy"},
		{
			"past what a shape can be read from",
			types.OfficeShape{Signatures: 3, Members: types.MaxOfficeMembers + 1},
			"members this module can read",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.shape.Validate()
			if err == nil {
				t.Fatalf("%s was accepted", tc.shape.Rule())
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("the refusal of %s says %q, which does not mention %q",
					tc.shape.Rule(), err.Error(), tc.says)
			}
		})
	}

	for _, ok := range []types.OfficeShape{
		{Signatures: 1, Members: 1},
		{Signatures: 2, Members: 3},
		{Signatures: 3, Members: 5},
		{Signatures: types.MaxOfficeMembers, Members: types.MaxOfficeMembers},
	} {
		if err := ok.Validate(); err != nil {
			t.Fatalf("%s is a shape an office could have; got %v", ok.Rule(), err)
		}
	}
}

// A one-of-three is a legitimate requirement and is not refused here.
//
// It reads like a weak office and it is a real arrangement: any one of three
// deputies may sign, and there are three people in the office. The rule that a
// minimum of one signature is not worth having belongs in the ceremony, where the
// decision is taken — see officeMinimum.validate in tools/ceremony. Putting it
// here as well would mean the chain refusing a shape a country deliberately chose,
// and the chain is not the right place to have that argument.
func TestASingleSignatureRequirementIsTheCeremonysRuleAndNotTheChains(t *testing.T) {
	if err := (&types.OfficeShape{Signatures: 1, Members: 3}).Validate(); err != nil {
		t.Fatalf("a one-of-three is a shape a country may choose; got %v", err)
	}
}

// The grant validates its shape, by the same function genesis uses.
func TestARoleGrantValidatesItsShape(t *testing.T) {
	grant := types.RoleGrant{
		Holder:       "yml1holder",
		Role:         types.ROLE_ENFORCEMENT_AUTHORITY,
		Jurisdiction: "GH",
	}
	if err := grant.Validate(); err != nil {
		t.Fatalf("a grant with no requirement is valid; got %v", err)
	}

	grant.RequiredShape = &types.OfficeShape{Signatures: 3, Members: 5}
	if err := grant.Validate(); err != nil {
		t.Fatalf("a grant with a workable requirement is valid; got %v", err)
	}

	grant.RequiredShape = &types.OfficeShape{Signatures: 4, Members: 2}
	err := grant.Validate()
	if err == nil {
		t.Fatal("a grant requiring more signatures than members was accepted")
	}
	if !strings.Contains(err.Error(), "ROLE_ENFORCEMENT_AUTHORITY") {
		t.Fatalf("the refusal does not name the role: %v", err)
	}
}

func TestOfficeShapeRuleReadsTheWayTheRoomSaysIt(t *testing.T) {
	if got := (&types.OfficeShape{Signatures: 3, Members: 5}).Rule(); got != "3-of-5" {
		t.Fatalf("rendered as %q; a ceremony record says 3-of-5", got)
	}
}
