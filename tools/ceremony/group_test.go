package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/group"
)

// firstPolicyAddress is the address x/group gives the first group policy ever
// created on a chain using this bech32 prefix.
//
// Captured from a running chain — a devnet where `tx group
// create-group-with-policy` was broadcast and the resulting policy address
// read back out of the transaction response — not computed by the function it
// is testing. It is a constant because the derivation deliberately does not
// depend on the members, the threshold, the admin or the chain id: only on the
// sequence number. That is the property the genesis argument in group.go rests
// on, so it is pinned here rather than assumed.
const firstPolicyAddress = "yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj"

func TestPolicyAddressMatchesTheChain(t *testing.T) {
	configureAddresses()

	got, err := policyAddress(1)
	if err != nil {
		t.Fatal(err)
	}
	if got != firstPolicyAddress {
		t.Fatalf("policy 1 = %q, want %q — this address is written into genesis as the destination for every seizure", got, firstPolicyAddress)
	}
}

func TestPolicyAddressChangesWithTheSequence(t *testing.T) {
	configureAddresses()

	first, err := policyAddress(1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := policyAddress(2)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two sequence numbers derive the same address, so a second group policy would collide with the foundation's")
	}
}

func custodians(t *testing.T, n int) []identity {
	t.Helper()
	configureAddresses()

	people := make([]identity, 0, n)
	for i := 0; i < n; i++ {
		s, err := newSecret()
		if err != nil {
			t.Fatal(err)
		}
		priv, path, err := s.derive(0)
		if err != nil {
			t.Fatal(err)
		}
		id, err := identityOf(fmt.Sprintf("Custodian %d", i+1), roleCustodian, priv, path, time.Unix(1700000000, 0))
		if err != nil {
			t.Fatal(err)
		}
		people = append(people, id)
		s.zero()
	}
	return people
}

func TestBuildGroupProducesAThreeOfFive(t *testing.T) {
	people := custodians(t, 5)

	documents, err := buildGroup(people, foundationPurpose(), 3, 7*24*time.Hour, 1, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if documents.policyAddr != firstPolicyAddress {
		t.Fatalf("policy address = %q", documents.policyAddr)
	}

	var members struct {
		Members []group.MemberRequest `json:"members"`
	}
	if err := json.Unmarshal(documents.members, &members); err != nil {
		t.Fatal(err)
	}
	if len(members.Members) != 5 {
		t.Fatalf("%d members", len(members.Members))
	}
	for _, member := range members.Members {
		// Equal weight, always: the reason five people hold this is that no
		// one of them is more trusted than the rest.
		if member.Weight != "1" {
			t.Fatalf("member %s has weight %q", member.Address, member.Weight)
		}
	}
}

func TestGroupGenesisIsAValidImportableFragment(t *testing.T) {
	people := custodians(t, 5)
	now := time.Unix(1700000000, 0)

	documents, err := buildGroup(people, foundationPurpose(), 3, 7*24*time.Hour, 1, now)
	if err != nil {
		t.Fatal(err)
	}

	registry := codectypes.NewInterfaceRegistry()
	group.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	var state group.GenesisState
	if err := cdc.UnmarshalJSON(documents.genesis, &state); err != nil {
		t.Fatalf("the fragment does not round-trip through the group module's codec: %v", err)
	}
	// The group module panics in InitGenesis on a fragment it cannot import,
	// which on launch day is every validator failing at once.
	if err := state.Validate(); err != nil {
		t.Fatalf("the fragment would not import: %v", err)
	}

	if len(state.Groups) != 1 || len(state.GroupPolicies) != 1 {
		t.Fatalf("got %d groups and %d policies", len(state.Groups), len(state.GroupPolicies))
	}
	if len(state.GroupMembers) != 5 {
		t.Fatalf("%d members in the fragment", len(state.GroupMembers))
	}
	if state.GroupPolicySeq != 1 {
		t.Fatalf("group_policy_seq = %d; the derived address assumes 1", state.GroupPolicySeq)
	}
	if state.GroupPolicies[0].Address != firstPolicyAddress {
		t.Fatalf("the seeded policy sits at %q, not the address genesis names", state.GroupPolicies[0].Address)
	}

	// The policy is its own admin. Anything else is a single key that can
	// rewrite the membership, which is the key this ceremony abolishes.
	if state.Groups[0].Admin != firstPolicyAddress {
		t.Fatalf("group admin = %q, want the policy itself", state.Groups[0].Admin)
	}
	if state.GroupPolicies[0].Admin != firstPolicyAddress {
		t.Fatalf("policy admin = %q, want the policy itself", state.GroupPolicies[0].Admin)
	}

	policy, err := state.GroupPolicies[0].GetDecisionPolicy()
	if err != nil {
		t.Fatal(err)
	}
	threshold, ok := policy.(*group.ThresholdDecisionPolicy)
	if !ok {
		t.Fatalf("decision policy is %T", policy)
	}
	if threshold.Threshold != "3" {
		t.Fatalf("threshold = %q, want 3", threshold.Threshold)
	}

	// Every custodian is in it, exactly once.
	seen := map[string]bool{}
	for _, member := range state.GroupMembers {
		seen[member.Member.Address] = true
	}
	for _, person := range people {
		if !seen[person.Address] {
			t.Fatalf("custodian %s is not in the group this genesis would create", person.Name)
		}
	}
}

func TestTheConstitutionFragmentAgreesWithTheGroup(t *testing.T) {
	// A genesis where the constitution says three-of-five and the group is a
	// different shape starts perfectly happily, and the ante gate then spends
	// its life refusing every legitimate change to a group that was already
	// wrong. Both documents come from one call so they cannot disagree.
	people := custodians(t, 5)

	documents, err := buildGroup(people, foundationPurpose(), 3, 7*24*time.Hour, 1, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}

	var fragment constitutionFragment
	if err := json.Unmarshal(documents.constitution, &fragment); err != nil {
		t.Fatal(err)
	}
	if fragment.EnforcementRecoveryDestination != documents.policyAddr {
		t.Fatalf("the constitution names %q, the group is at %q", fragment.EnforcementRecoveryDestination, documents.policyAddr)
	}
	if fragment.FoundationCustodianCount != 5 || fragment.FoundationSignatureThreshold != 3 {
		t.Fatalf("the constitution fragment says %d-of-%d", fragment.FoundationSignatureThreshold, fragment.FoundationCustodianCount)
	}
}

func TestBuildGroupRefusesAThresholdNobodyCouldMeet(t *testing.T) {
	people := custodians(t, 3)
	if _, err := buildGroup(people, foundationPurpose(), 5, time.Hour, 1, time.Now()); err == nil {
		t.Fatal("a group that could never reach its threshold was accepted")
	}
}

func TestBuildGroupRefusesASingleSigner(t *testing.T) {
	people := custodians(t, 5)
	if _, err := buildGroup(people, foundationPurpose(), 1, time.Hour, 1, time.Now()); err == nil {
		t.Fatal("a 1-of-5 was accepted, which is the single key this ceremony replaces")
	}
}

func TestBuildGroupRefusesUnanimity(t *testing.T) {
	// 5-of-5 looks like the safest option and is the least safe available: one
	// lost key freezes the account the chain keeps sending seizures to.
	people := custodians(t, 5)
	if _, err := buildGroup(people, foundationPurpose(), 5, time.Hour, 1, time.Now()); err == nil {
		t.Fatal("a 5-of-5 was accepted")
	}
}

func TestBuildGroupRefusesADuplicateCustodian(t *testing.T) {
	people := custodians(t, 5)
	people[4].Address = people[0].Address

	_, err := buildGroup(people, foundationPurpose(), 3, time.Hour, 1, time.Now())
	if err == nil {
		t.Fatal("two members at one address were accepted: that is a 3-of-4 with somebody holding two votes")
	}
	if !strings.Contains(err.Error(), people[0].Address) {
		t.Fatalf("the error does not name the colliding address: %v", err)
	}
}

func TestBuildGroupRefusesAValidatorOperatorKey(t *testing.T) {
	people := custodians(t, 5)
	people[2].Role = roleValidator

	if _, err := buildGroup(people, foundationPurpose(), 3, time.Hour, 1, time.Now()); err == nil {
		t.Fatal("a validator operator key was accepted into the foundation group")
	}
}

func TestBuildGroupRefusesAnUnreadableAddress(t *testing.T) {
	people := custodians(t, 5)
	people[1].Address = "yml1definitelynotanaddress"

	if _, err := buildGroup(people, foundationPurpose(), 3, time.Hour, 1, time.Now()); err == nil {
		t.Fatal("an address this chain cannot decode was accepted")
	}
}

func TestBuildGroupIsDeterministic(t *testing.T) {
	people := custodians(t, 5)
	now := time.Unix(1700000000, 0)

	first, err := buildGroup(people, foundationPurpose(), 3, 7*24*time.Hour, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	// Same five people, different order — as a shell glob or a scribe reading
	// from a list would give them. The documents must come out byte-identical
	// so a genesis can be rebuilt and compared rather than taken on trust.
	shuffled := []identity{people[3], people[0], people[4], people[1], people[2]}
	second, err := buildGroup(shuffled, foundationPurpose(), 3, 7*24*time.Hour, 1, now)
	if err != nil {
		t.Fatal(err)
	}

	if string(first.genesis) != string(second.genesis) {
		t.Fatal("two runs over the same five custodians produced different genesis fragments")
	}
	if string(first.members) != string(second.members) {
		t.Fatal("two runs over the same five custodians produced different member lists")
	}
}

// writeAndReadBack round-trips identities through the files the ceremony really
// produces, which is where readIdentities' sort applies.
func writeAndReadBack(t *testing.T, people []identity) ([]identity, error) {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, 0, len(people))
	for i, person := range people {
		path := filepath.Join(dir, fmt.Sprintf("c%d.json", i))
		if err := writeIdentity(path, person); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return readIdentities(paths)
}

func TestReadIdentitiesSortsByAddress(t *testing.T) {
	people := custodians(t, 5)
	got, err := writeAndReadBack(t, people)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Address >= got[i].Address {
			t.Fatalf("records are not in address order: %q then %q", got[i-1].Address, got[i].Address)
		}
	}
}

func TestReadIdentitiesRejectsAFileThatIsNotACeremonyRecord(t *testing.T) {
	dir := t.TempDir()

	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"name":"nobody","role":"custodian","address":"","pubkey":{"@type":"x","key":"y"},"fingerprint":"","hd_path":"","generated_at":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readIdentities([]string{empty}); err == nil {
		t.Fatal("a record with no address was accepted")
	}

	// A hand-edited file with a misspelled field must be a refusal, not a
	// member silently created with an empty address.
	typo := filepath.Join(dir, "typo.json")
	if err := os.WriteFile(typo, []byte(`{"naem":"x","address":"yml1x","fingerprint":"AAAA-BBBB"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readIdentities([]string{typo}); err == nil {
		t.Fatal("a record with an unknown field was accepted")
	}
}

func TestWriteIdentityRefusesToOverwrite(t *testing.T) {
	people := custodians(t, 1)
	path := filepath.Join(t.TempDir(), "c.json")

	if err := writeIdentity(path, people[0]); err != nil {
		t.Fatal(err)
	}
	// Two custodians given the same name is an easy mistake in a room reading
	// from a list, and overwriting would leave a group with four members and a
	// fifth who believes they are in it.
	if err := writeIdentity(path, people[0]); err == nil {
		t.Fatal("an existing public record was overwritten")
	}
}

func TestPolicyAddressIsAValidChainAddress(t *testing.T) {
	configureAddresses()

	address, err := policyAddress(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sdk.AccAddressFromBech32(address); err != nil {
		t.Fatalf("the derived policy address is not decodable by this chain: %v", err)
	}
	if !strings.HasPrefix(address, accountPrefix+"1") {
		t.Fatalf("policy address %q is not under this chain's prefix", address)
	}
}
