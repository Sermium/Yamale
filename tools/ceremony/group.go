package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/group"
	groupkeeper "github.com/cosmos/cosmos-sdk/x/group/keeper"
)

// policyAddress derives the address x/group will give the seq-th group policy
// account ever created on a chain.
//
// This is the finding the whole genesis question turns on, so it is worth
// stating exactly what it does and does not depend on. x/group derives a policy
// account under ADR-028 from three things: the module name, a one-byte table
// prefix, and the group policy sequence number, big-endian. It does not depend
// on the members, the threshold, the admin, the metadata, the chain id, or when
// it was created.
//
// Two consequences, and the second is the uncomfortable one:
//
//   - The address IS knowable before genesis. Policy number one on a chain with
//     this bech32 prefix has exactly one possible address and this function
//     prints it offline. No two-phase genesis is needed to learn it.
//
//   - The address commits to NOTHING about who controls it. The same address is
//     produced by a 3-of-5 of the five custodians and by a 1-of-1 of an
//     attacker, because neither input reaches the derivation. So an address
//     pasted into genesis as recovery_destination while the group is created
//     afterwards by transaction is a promise that whoever wins the race to
//     create policy number one owns every asset the chain ever seizes.
//
// Which is why groupGenesis below exists and why the runbook uses it: genesis
// carries the group, its five members and the 3-of-5 policy, so the address and
// the membership are fixed by the same file at height zero and there is no
// window between them.
//
// The prefix comes from the SDK's own keeper rather than a literal, so that a
// change to it upstream breaks the build here instead of silently producing
// addresses this chain does not use.
func policyAddress(seq uint64) (string, error) {
	derivationKey := make([]byte, 8)
	binary.BigEndian.PutUint64(derivationKey, seq)

	credential, err := authtypes.NewModuleCredential(
		group.ModuleName,
		[]byte{groupkeeper.GroupPolicyTablePrefix},
		derivationKey,
	)
	if err != nil {
		return "", err
	}
	return sdk.AccAddress(credential.Address()).String(), nil
}

// runAddress prints the policy address for a sequence number, and nothing else.
//
// Separate from `group` so that anybody — a validator operator, an auditor, the
// observer at the ceremony — can independently derive the address that is about
// to be written into genesis, without the custodian files and without trusting
// the run that produced them.
func runAddress(args []string) error {
	flags := flag.NewFlagSet("address", flag.ExitOnError)
	seq := flags.Uint64("seq", 1, "group policy sequence number; the first policy on a chain is 1")
	if err := flags.Parse(args); err != nil {
		return err
	}

	address, err := policyAddress(*seq)
	if err != nil {
		return err
	}
	fmt.Println(address)
	return nil
}

// groupDocuments is everything `ceremony group` produces.
//
// constitution is empty for a country office. See groupPurpose.
type groupDocuments struct {
	members      []byte
	policy       []byte
	genesis      []byte
	msg          []byte
	constitution []byte
	policyAddr   string
}

// groupPurpose is which of the two groups this ceremony is building.
//
// Two things differ between the foundation's 3-of-5 and a country office's
// M-of-N, and neither is cosmetic:
//
//   - The label. It goes into the group metadata and the policy metadata, which
//     are recorded on chain permanently and are the one field a human reads to
//     find out what a group is. Senegal's payments authority recorded as "Yamale
//     foundation" is a lie in exactly the place nobody thinks to check.
//
//   - The constitutional invariants fragment. For the foundation it is the three
//     values that must agree with the group in genesis. For an office it is a
//     ready-to-splice JSON saying "send every seized asset on this chain to
//     Senegal's payments office", so it is not produced at all. Not produced
//     rather than produced-with-a-warning, because nobody should have to know
//     not to use a file that is sitting in the output directory.
//
// A struct rather than two bare parameters so that neither call site can pass
// them in the wrong order or read as a mystery boolean.
type groupPurpose struct {
	Label string
	// Office is true when this group belongs to a country office, in which case
	// the constitutional fragment is withheld.
	Office bool
}

// foundationPurpose is the foundation's 3-of-5.
//
// The label is the same constant the old code had inline, so the foundation's
// metadata bytes — and therefore its group fingerprint — do not move because the
// country path was added.
func foundationPurpose() groupPurpose {
	return groupPurpose{Label: foundationLabel}
}

// purposeFor reads the purpose out of a ceremony's parameters.
func purposeFor(params ceremonyParams) groupPurpose {
	return groupPurpose{Label: groupLabel(params), Office: params.Office != nil}
}

// maxGroupMetadata is x/group's limit, in bytes.
//
// Not ours: the keeper's assertMetadataLength refuses any metadata longer than
// Config.MaxMetadataLen, which DefaultConfig sets to 255 and this chain does not
// override. clients/foundation/foundation.js carries the same number for the
// proposal titles it composes.
//
// Checked here because the two paths fail differently and one of them fails
// silently. GroupInfo.ValidateBasic does NOT check metadata length and neither
// does x/group's GenesisState.Validate, so an over-long metadata string imports
// into a genesis file perfectly happily — the foundation path would never notice.
// A country office's group is created by MsgCreateGroupWithPolicy on a running
// chain, which is the path that IS checked, so the same string that launched a
// chain would fail a transaction after the whole ceremony had been held.
const maxGroupMetadata = 255

// constitutionFragment is the part of app_state.constitution.invariants this
// ceremony decides.
//
// Emitted as a document rather than left to whoever builds the genesis, because
// these three values have to agree with the group in group-genesis.json and
// with each other. A genesis where the constitution says three-of-five and the
// group is a two-of-four is one the chain will start from perfectly happily,
// and the ante gate would then spend its life refusing every legitimate change
// to a group that is already the wrong shape.
type constitutionFragment struct {
	EnforcementRecoveryDestination string `json:"enforcement_recovery_destination"`
	FoundationCustodianCount       int    `json:"foundation_custodian_count"`
	FoundationSignatureThreshold   int    `json:"foundation_signature_threshold"`
}

// decisionPolicy is the 3-of-5 itself.
//
// The voting period is the window in which the other custodians can vote once
// one has proposed. A week rather than a day: the custodians are five people in
// five places, this is not their job, and a policy whose window assumes everyone
// is at a desk turns "three of five agreed" into "three of five happened to be
// awake". The seizure it is downstream of already takes days to pass.
//
// min_execution_period is zero. The delay that protects anybody here is
// x/enforcement's, which is measured in blocks and already fixed by the
// constitution; a second delay stacked on top would only mean the foundation
// cannot act on an outcome the chain has already reached.
func decisionPolicy(threshold int, votingPeriod time.Duration) *group.ThresholdDecisionPolicy {
	return &group.ThresholdDecisionPolicy{
		Threshold: fmt.Sprintf("%d", threshold),
		Windows: &group.DecisionPolicyWindows{
			VotingPeriod:       votingPeriod,
			MinExecutionPeriod: 0,
		},
	}
}

// buildGroup turns the custodians' public records into the documents that
// create the group.
//
// The admin is the group policy itself. That is the point of the whole design:
// an admin outside the group would be a single key that could rewrite the
// membership, which is the single key this ceremony exists to abolish. With the
// policy as its own admin, changing who the custodians are needs the same
// three-of-five as spending, and there is no address anywhere that can do it
// alone.
func buildGroup(custodians []identity, purpose groupPurpose, threshold int, votingPeriod time.Duration, seq uint64, now time.Time) (groupDocuments, error) {
	if len(custodians) < threshold {
		return groupDocuments{}, fmt.Errorf(
			"threshold %d cannot be met by %d custodians: this group could never act",
			threshold, len(custodians))
	}
	if threshold < 2 {
		return groupDocuments{}, fmt.Errorf(
			"threshold %d means one custodian acts alone, which is the single key this ceremony replaces",
			threshold)
	}
	// A threshold equal to the membership has no redundancy at all: one lost
	// key, one custodian who resigns, one person who cannot be reached, and the
	// foundation account is frozen permanently with the chain still sending
	// seized assets to it. Refused rather than warned about, because it looks
	// like the safest choice and is the least safe one available.
	if threshold == len(custodians) {
		return groupDocuments{}, fmt.Errorf(
			"threshold %d of %d requires every custodian: losing one key would freeze the foundation account forever, with the chain still sending seizures to it",
			threshold, len(custodians))
	}

	// Sorted on a copy, by address, so the documents depend on the set of
	// custodians and not on the order a shell glob or a scribe reading from a
	// list happened to supply them in. On a launch day that is what lets a
	// second person rebuild the genesis fragment from the same five files and
	// compare it byte for byte, rather than taking the first run on trust.
	custodians = append([]identity(nil), custodians...)
	sort.Slice(custodians, func(i, j int) bool { return custodians[i].Address < custodians[j].Address })

	seen := map[string]string{}
	for _, custodian := range custodians {
		if custodian.Role != roleCustodian {
			return groupDocuments{}, fmt.Errorf(
				"%s is recorded as %q, not a custodian; a validator operator key does not belong in the foundation group",
				custodian.Name, custodian.Role)
		}
		address, err := sdk.AccAddressFromBech32(custodian.Address)
		if err != nil {
			return groupDocuments{}, fmt.Errorf("%s has an address this chain cannot read: %w", custodian.Name, err)
		}
		// Twenty bytes is a key; thirty-two is something derived — a module
		// account or another group's policy. A custodian has to be a person
		// holding a key, because a group policy sitting in this group could
		// submit a proposal to it, and the messages a proposal executes never
		// pass the ante chain. Refused here rather than left to the on-chain
		// gate, which by then would be arguing with a group that already exists.
		if len(address) != 20 {
			return groupDocuments{}, fmt.Errorf(
				"%s's address is %d bytes, so it is a derived account rather than a key. "+
					"Custodians are people with keys; a module or group account in this group would be a member nobody signs for",
				custodian.Name, len(address))
		}
		// Two members at one address is a 3-of-5 that is really a 3-of-4 with
		// somebody holding two votes. The SDK would reject the duplicate; this
		// says which two records collided, which is the thing the room needs to
		// know.
		if other, duplicate := seen[custodian.Address]; duplicate {
			return groupDocuments{}, fmt.Errorf(
				"%s and %s have the same address %s — one of them holds two votes, and this is not the group you think it is",
				other, custodian.Name, custodian.Address)
		}
		seen[custodian.Address] = custodian.Name
	}

	policyAddr, err := policyAddress(seq)
	if err != nil {
		return groupDocuments{}, err
	}
	admin, err := sdk.AccAddressFromBech32(policyAddr)
	if err != nil {
		return groupDocuments{}, err
	}

	members := make([]group.MemberRequest, 0, len(custodians))
	for _, custodian := range custodians {
		members = append(members, group.MemberRequest{
			Address: custodian.Address,
			// Equal weight, always. Weighting custodians would make some
			// signatures worth more than others, and the reason five people
			// hold this is that no one of them is more trusted than the rest.
			Weight:   "1",
			Metadata: fmt.Sprintf("%s (%s)", custodian.Name, custodian.Fingerprint),
		})
	}

	policy := decisionPolicy(threshold, votingPeriod)
	if err := policy.ValidateBasic(); err != nil {
		return groupDocuments{}, err
	}

	// Composed once and checked once, then used for the group, the policy and the
	// genesis fragment, so the three cannot disagree.
	metadata := groupMetadata(purpose.Label, custodians, threshold)
	policyMetadata := fmt.Sprintf("%s %d-of-%d", purpose.Label, threshold, len(custodians))
	for _, candidate := range []struct {
		what  string
		value string
	}{
		{"group metadata", metadata},
		{"group policy metadata", policyMetadata},
	} {
		if size := len(candidate.value); size > maxGroupMetadata {
			return groupDocuments{}, fmt.Errorf(
				"the %s is %d bytes and x/group refuses anything over %d, so the transaction that creates this "+
					"group would fail after the ceremony was over. Shorten the office name: it is the part of "+
					"%q that this ceremony chose",
				candidate.what, size, maxGroupMetadata, purpose.Label)
		}
	}

	registry := codectypes.NewInterfaceRegistry()
	group.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	documents := groupDocuments{policyAddr: policyAddr}

	// members.json and policy.json, in the shape `blockchaind tx group
	// create-group-with-policy` reads. The runtime path, for a chain that is
	// already running.
	documents.members, err = json.MarshalIndent(struct {
		Members []group.MemberRequest `json:"members"`
	}{members}, "", "  ")
	if err != nil {
		return groupDocuments{}, err
	}
	// MarshalInterfaceJSON, not MarshalJSON, and the difference is the whole
	// difference between a file the CLI reads and a file it refuses.
	//
	// A decision policy travels inside an Any, so the document has to carry an
	// "@type". MarshalJSON on the concrete *ThresholdDecisionPolicy emits its
	// fields and no type URL, and `blockchaind tx group create-group-with-policy`
	// then fails with "failed to parse decision policy: Any JSON doesn't have
	// '@type'" — which is what this file did for as long as it existed. It went
	// unnoticed because the foundation's launch path splices the genesis fragment
	// and never reads this document; the first thing that actually needed it was a
	// country office, whose group is created by a transaction on a running chain.
	//
	// Nothing downstream of the fingerprint moves. assembled.canonical() covers
	// the parameters, the policy address, the custodians, the genesis fragment and
	// the constitution fragment — not this document — so the value five people read
	// aloud is unchanged, and the browser never computes it at all.
	documents.policy, err = cdc.MarshalInterfaceJSON(policy)
	if err != nil {
		return groupDocuments{}, err
	}

	msg, err := group.NewMsgCreateGroupWithPolicy(
		admin.String(),
		members,
		metadata,
		policyMetadata,
		// The group policy is its own admin, so nothing outside the group can
		// change the membership or the threshold.
		true,
		policy,
	)
	if err != nil {
		return groupDocuments{}, err
	}
	documents.msg, err = cdc.MarshalJSON(msg)
	if err != nil {
		return groupDocuments{}, err
	}

	documents.genesis, err = groupGenesis(cdc, metadata, members, policy, admin, seq, now)
	if err != nil {
		return groupDocuments{}, err
	}

	// Withheld for a country office, and left nil rather than emptied to a "{}"
	// somebody could still splice. The three fields in it are the FOUNDATION's:
	// where the chain sends every seizure, how many custodians the foundation
	// has, and how many of them must sign. Produced for Senegal's payments
	// office, it is a document that reads as an instruction to hand that office
	// the whole chain's seized assets — and a genesis built from it would start
	// perfectly happily.
	if !purpose.Office {
		documents.constitution, err = json.MarshalIndent(constitutionFragment{
			EnforcementRecoveryDestination: policyAddr,
			FoundationCustodianCount:       len(custodians),
			FoundationSignatureThreshold:   threshold,
		}, "", "  ")
		if err != nil {
			return groupDocuments{}, err
		}
	}

	return documents, nil
}

// groupMetadata is the string recorded on chain, permanently, as what this group
// is.
//
// The label is a parameter rather than a literal because it is the only thing a
// human reads to tell one group from another. For the foundation it is
// "Yamale foundation" and the bytes are unchanged from before the country path
// existed; for a country office it names the office and its perimeter.
func groupMetadata(label string, custodians []identity, threshold int) string {
	names := make([]string, len(custodians))
	for i, custodian := range custodians {
		names[i] = fmt.Sprintf("%s %s", custodian.Name, custodian.Fingerprint)
	}
	return fmt.Sprintf("%s, %d of %d: %s", label, threshold, len(custodians), strings.Join(names, "; "))
}

// groupGenesis is the app_state.group fragment that puts the group in the
// genesis file itself.
//
// This is the answer to "the address must be known before genesis is
// finalised". It is known — policyAddress computes it offline — but knowing it
// is not enough, because the derivation does not depend on the membership. A
// genesis naming the address while the group is created later by transaction
// leaves a window in which recovery_destination points at an account whose
// controlling policy has not been decided, and the first MsgCreateGroupPolicy
// to land decides it.
//
// Seeding it here closes the window rather than shortening it: at height zero
// the address exists, the five members exist, and the threshold is 3. There is
// no race because there is no interval.
//
// One thing this deliberately does not do is create the auth account. The
// runtime path in x/group's keeper stores a BaseAccount carrying a
// ModuleCredential at the policy address; genesis import does not, because the
// group module's InitGenesis imports its own tables and nothing else. That
// address is unspendable either way — it is a hash of a module name and a
// sequence number, not of any public key, so no private key produces it — and
// the genesis ceremony adds it with `add-genesis-account` so that it holds a
// balance from block one. Verified against a running chain: a genesis-seeded
// group executes proposals exactly as a transaction-created one does.
// The metadata is passed in rather than recomposed here, so the string in the
// genesis fragment is the same object as the string in the create message and in
// the length check above. Two compositions of "the same" metadata is how a
// genesis-seeded group and a transaction-created one come to differ by a byte.
func groupGenesis(
	cdc codec.Codec,
	metadata string,
	members []group.MemberRequest,
	policy group.DecisionPolicy,
	policyAddr sdk.AccAddress,
	seq uint64,
	now time.Time,
) ([]byte, error) {
	const groupID = 1

	// Truncated to the second. Genesis JSON is compared byte for byte across
	// every validator in the ceremony, and a timestamp carrying nanoseconds is
	// a value nobody can retype from a runbook if the file has to be rebuilt.
	created := now.UTC().Truncate(time.Second)

	info := group.GroupInfo{
		Id:          groupID,
		Admin:       policyAddr.String(),
		Metadata:    metadata,
		Version:     1,
		TotalWeight: fmt.Sprintf("%d", len(members)),
		CreatedAt:   created,
	}

	groupMembers := make([]*group.GroupMember, 0, len(members))
	for _, member := range members {
		groupMembers = append(groupMembers, &group.GroupMember{
			GroupId: groupID,
			Member: &group.Member{
				Address:  member.Address,
				Weight:   member.Weight,
				Metadata: member.Metadata,
				AddedAt:  created,
			},
		})
	}

	policyInfo, err := group.NewGroupPolicyInfo(
		policyAddr, groupID, policyAddr, info.Metadata, 1, policy, created,
	)
	if err != nil {
		return nil, err
	}

	state := &group.GenesisState{
		GroupSeq:       groupID,
		Groups:         []*group.GroupInfo{&info},
		GroupMembers:   groupMembers,
		GroupPolicySeq: seq,
		GroupPolicies:  []*group.GroupPolicyInfo{&policyInfo},
	}
	// Validated here rather than discovered by a validator at height one. The
	// group module panics in InitGenesis on a fragment it cannot import, which
	// on a launch day is every node in the network failing at once with a
	// stack trace.
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("the genesis fragment this would produce is invalid: %w", err)
	}

	return cdc.MarshalJSON(state)
}

func runGroup(args []string) error {
	flags := flag.NewFlagSet("group", flag.ExitOnError)
	threshold := flags.Int("threshold", 3, "how many custodians must sign")
	seq := flags.Uint64("seq", 1, "group policy sequence number; 1 on a chain that has never had one")
	votingPeriod := flags.Duration("voting-period", 7*24*time.Hour, "how long the other custodians have to vote")
	out := flags.String("out", ".", "directory for the group documents")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return fmt.Errorf("give the custodians' public record files, e.g. ceremony group custodian-*.json")
	}

	custodians, err := readIdentities(flags.Args())
	if err != nil {
		return err
	}

	// The foundation, always. `ceremony group` builds the foundation's 3-of-5 from
	// five public records on one machine; a country office's group comes out of
	// the hosted flow, where the office and its perimeter are in the parameters
	// every super user read a fingerprint of. There is deliberately no flag for it
	// here, because a country office assembled from files on a coordinator's
	// laptop would have nobody who could say what they had generated a key for.
	documents, err := buildGroup(custodians, foundationPurpose(), *threshold, *votingPeriod, *seq, time.Now())
	if err != nil {
		return err
	}

	files := []struct {
		name string
		data []byte
	}{
		{"group-members.json", documents.members},
		{"group-policy.json", documents.policy},
		{"group-create-msg.json", documents.msg},
		{"group-genesis.json", documents.genesis},
		{"constitution-invariants.json", documents.constitution},
	}
	for _, file := range files {
		path := filepath.Join(*out, file.name)
		if err := os.WriteFile(path, append(file.data, '\n'), 0o644); err != nil {
			return err
		}
	}

	c := stdConsole()
	c.printf("=== foundation group: %d of %d ===\n\n", *threshold, len(custodians))
	for _, custodian := range custodians {
		c.printf("  %s\n", custodian.describe())
	}
	c.println()
	c.printf("  group policy address   %s\n", documents.policyAddr)
	c.println()
	c.println("That address is what goes into genesis, in BOTH of these places — they are")
	c.println("checked against each other and the chain refuses to start if they disagree:")
	c.println()
	c.println("  app_state.constitution.invariants.enforcement_recovery_destination")
	c.println("  app_state.enforcement.params.recovery_destination")
	c.println()
	c.println("Written:")
	c.println("  group-genesis.json            splice into app_state.group — the one to use")
	c.println("  constitution-invariants.json  the three values that must agree with it")
	c.println("  group-members.json            \\ for `tx group create-group-with-policy`")
	c.println("  group-policy.json             / on a chain that is already running")
	c.println("  group-create-msg.json         the same message, as JSON, for review")
	c.println()
	c.println("Use the genesis fragment for a launch. The address above is derived from the")
	c.println("policy sequence number alone — not from these five members — so a genesis that")
	c.println("named the address and left the group to be created afterwards would be handing")
	c.println("every future seizure to whoever created the first group policy on the chain.")
	return nil
}

// readIdentities loads the custodians' public records, in a stable order.
//
// Sorted by address rather than by filename or by the order the shell happened
// to glob them, so that two people building the group from the same five files
// produce byte-identical documents. On a launch day that is what lets the
// genesis be rebuilt and compared rather than taken on trust.
func readIdentities(paths []string) ([]identity, error) {
	identities := make([]identity, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var id identity
		// Strict, so a record from a different tool or a hand-edited file with
		// a misspelled field is a refusal rather than a member silently
		// created with an empty address.
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&id); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if id.Address == "" || id.Fingerprint == "" {
			return nil, fmt.Errorf("%s has no address or no fingerprint; it is not a ceremony record", path)
		}
		identities = append(identities, id)
	}

	sort.Slice(identities, func(i, j int) bool { return identities[i].Address < identities[j].Address })
	return identities, nil
}
