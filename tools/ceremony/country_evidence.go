package main

// Reading the chain's own answers, and deciding whether they say what somebody
// claims they say.
//
// Every function in this file takes the output of one `blockchaind query ... -o
// json` and turns it into a fact this ceremony will act on, or into a refusal.
// Nothing here fetches anything; see the note at the top of country.go for why
// tools/ceremony has no outbound network code and what that costs.
//
// Two rules run through all of it.
//
// # A queried result, never a broadcast response
//
// `blockchaind tx ...` prints a response with a code in it, and that code is the
// result of CheckTx: code 0 there means the transaction was accepted into a
// mempool. It has not run. Four separate bugs in this project came from reading
// it as though it had — a transaction that was accepted and then failed in
// delivery reports code 0 on the way in and an error nobody looked at on the way
// out, and everything downstream proceeds on the assumption that it worked.
//
// So every function here expects the output of `query tx <hash>`, which is the
// delivered result, and refuses a document that looks like a broadcast response.
// The distinguishing feature is `height`: a broadcast response carries height 0,
// because it has not been included in a block yet.
//
// # Derived values are recomputed, claimed values are checked
//
// The same discipline as verifySubmission. A query response is a document that
// travelled, so anything in it that can be checked against something else is
// checked: the policy address against the address in the transaction's own
// events, the member set against the roster the office's super users signed for,
// the threshold against the one they attested to. What cannot be recomputed —
// a transaction hash, a height — is carried and named as carried.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	aliastypes "yamale/blockchain/x/alias/types"
)

// The typed events x/group emits. Named as constants because a typo in one of
// them would make confirmOffice report "this transaction created no group
// policy" for a transaction that created one, which sends an operator looking
// for a chain problem.
const (
	eventCreateGroup       = "cosmos.group.v1.EventCreateGroup"
	eventCreateGroupPolicy = "cosmos.group.v1.EventCreateGroupPolicy"
)

// decodeAccountAddress is the one place an address string becomes bytes.
func decodeAccountAddress(address string) (sdk.AccAddress, error) {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return nil, errors.New("empty")
	}
	return sdk.AccAddressFromBech32(trimmed)
}

// ---------------------------------------------------------------- json helpers

// flexUint64 reads an integer that protobuf JSON may have written as a number or
// as a quoted number.
//
// Not a convenience. gogoproto renders uint64 and int64 as strings and uint32 as
// a number, so a document mixing group ids and result codes mixes both forms —
// and a parser that assumed one would silently read zero for the other. Zero is
// the value that matters here: group 0 and policy sequence 0 both exist on this
// chain, so "unset" and "zero" are not the same answer and a parser must not turn
// one into the other.
type flexUint64 uint64

func (f *flexUint64) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return errors.New("no value where a number was expected")
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return fmt.Errorf("%q is not a number: %w", text, err)
	}
	*f = flexUint64(value)
	return nil
}

type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return errors.New("no value where a number was expected")
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("%q is not a number: %w", text, err)
	}
	*f = flexInt64(value)
	return nil
}

// readJSONFile loads a query response.
//
// Deliberately NOT strict about unknown fields, unlike everything else this tool
// reads. These documents are produced by another program whose response types
// grow between SDK versions, and refusing a field the node added would mean the
// ceremony stops working on an upgrade that changed nothing that matters. What
// replaces strictness is that every field this file reads is either cross-checked
// against another document or required to be present and non-empty — a missing
// field is an error, never a zero value carried forward.
func readJSONFile(path string, into any) error {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("%s is not a query response this can read: %w", path, err)
	}
	return nil
}

// ---------------------------------------------------------------- transactions

// txResult is the part of a queried transaction result this tool reads.
type txResult struct {
	Height    flexInt64  `json:"height"`
	TxHash    string     `json:"txhash"`
	Code      flexUint64 `json:"code"`
	Codespace string     `json:"codespace"`
	RawLog    string     `json:"raw_log"`
	Events    []struct {
		Type       string `json:"type"`
		Attributes []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"attributes"`
	} `json:"events"`
}

// readTxResult loads a queried transaction result and refuses anything that is
// not one, or that did not succeed.
//
// The height check is the one worth reading. A document with height 0 is a
// broadcast response: the transaction was accepted for inclusion and this is not
// evidence that it did anything. Accepting it here would make every later phase
// of the ceremony proceed on a transaction that may have failed in delivery,
// which is precisely the bug this project has hit four times.
func readTxResult(path string) (txResult, error) {
	var result txResult
	if err := readJSONFile(path, &result); err != nil {
		return txResult{}, err
	}

	if strings.TrimSpace(result.TxHash) == "" {
		return txResult{}, fmt.Errorf("%s has no txhash, so it is not a transaction result", path)
	}
	if result.Height <= 0 {
		return txResult{}, fmt.Errorf(
			"%s is at height %d, which means it is a BROADCAST response and not a queried result.\n"+
				"A broadcast that came back code 0 has been accepted into a mempool and has not executed — the\n"+
				"transaction can still fail in delivery and report a code nobody looked at. Query it instead:\n"+
				"  blockchaind query tx %s -o json > %s",
			path, int64(result.Height), result.TxHash, path)
	}
	if result.Code != 0 {
		return txResult{}, fmt.Errorf(
			"transaction %s failed: code %d in codespace %q.\n  %s",
			result.TxHash, uint64(result.Code), result.Codespace, strings.TrimSpace(result.RawLog))
	}
	return result, nil
}

// attribute finds one attribute of one typed event.
//
// Typed events carry their attribute values JSON-encoded, so a string arrives
// wrapped in quotes and a number arrives as a quoted number. Unwrapped here
// rather than at every call site, because a value read with its quotes still on
// compares unequal to the same address read from anywhere else, and the failure
// looks like a mismatched address rather than like a parsing bug.
func (t txResult) attribute(eventType, key string) (string, bool) {
	for _, event := range t.Events {
		if event.Type != eventType {
			continue
		}
		for _, attr := range event.Attributes {
			if attr.Key != key {
				continue
			}
			value := attr.Value
			var unquoted string
			if err := json.Unmarshal([]byte(value), &unquoted); err == nil {
				return unquoted, true
			}
			return value, true
		}
	}
	return "", false
}

// ---------------------------------------------------------------- group policy

// policyInfo is the part of `query group group-policy-info` this tool reads.
type policyInfo struct {
	Info struct {
		Address        string            `json:"address"`
		GroupID        flexUint64        `json:"group_id"`
		Admin          string            `json:"admin"`
		Metadata       string            `json:"metadata"`
		Version        flexUint64        `json:"version"`
		DecisionPolicy decisionPolicyDoc `json:"decision_policy"`
	} `json:"info"`
}

// decisionPolicyDoc is a decision policy as it arrives from a node, in either of
// the two shapes a Cosmos node renders a protobuf Any in.
//
// This is not defensive programming, it is the actual situation, and it was found
// by running the enrolment against a chain rather than by reading anything:
//
//	blockchaind query group group-policy-info <addr> -o json
//	  "decision_policy": {"type": "/cosmos.group.v1.ThresholdDecisionPolicy",
//	                      "value": {"threshold": "2", "windows": {…}}}
//
//	GET /cosmos/group/v1/group_policy_info/<addr>
//	  "decision_policy": {"@type": "/cosmos.group.v1.ThresholdDecisionPolicy",
//	                      "threshold": "2", "windows": {…}}
//
// The CLI goes through the codec's Any rendering and the REST gateway goes through
// protobuf JSON, so the type URL sits under a different key and the fields are
// nested in one and flat in the other. A tool reading only the second refuses
// every CLI answer, and one reading only the first refuses every REST answer —
// either way an operator with a perfectly good group is told their office does
// not match.
//
// What it must never do is treat an absent type as anything but absent. This
// leaves Type empty when neither key is present, and confirmOffice refuses an
// empty type against the one policy shape this ceremony accepts. So the failure
// direction is a refusal, which is how the mismatch above was noticed at all.
type decisionPolicyDoc struct {
	Type               string
	Threshold          string
	VotingPeriod       string
	MinExecutionPeriod string
}

func (d *decisionPolicyDoc) UnmarshalJSON(raw []byte) error {
	type windows struct {
		VotingPeriod       string `json:"voting_period"`
		MinExecutionPeriod string `json:"min_execution_period"`
	}
	var doc struct {
		AtType string `json:"@type"`
		Type   string `json:"type"`
		// The flat form, as protobuf JSON writes it.
		Threshold string   `json:"threshold"`
		Windows   *windows `json:"windows"`
		// The nested form, as the codec's Any rendering writes it.
		Value *struct {
			Threshold string   `json:"threshold"`
			Windows   *windows `json:"windows"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}

	// Whichever key carries it — but a document carrying both with DIFFERENT
	// values is refused rather than resolved. There is no honest way to choose
	// between two type URLs for one policy, and the whole reason this tool reads
	// the type is to refuse a policy shape whose threshold could move.
	switch {
	case doc.AtType != "" && doc.Type != "" && doc.AtType != doc.Type:
		return fmt.Errorf(
			"a decision policy names two different types, %q and %q. Whatever produced that document, it is not "+
				"an answer to confirm an office against", doc.AtType, doc.Type)
	case doc.AtType != "":
		d.Type = doc.AtType
	default:
		d.Type = doc.Type
	}

	d.Threshold = doc.Threshold
	if doc.Windows != nil {
		d.VotingPeriod = doc.Windows.VotingPeriod
		d.MinExecutionPeriod = doc.Windows.MinExecutionPeriod
	}
	if doc.Value != nil {
		if d.Threshold == "" {
			d.Threshold = doc.Value.Threshold
		}
		if doc.Value.Windows != nil && d.VotingPeriod == "" {
			d.VotingPeriod = doc.Value.Windows.VotingPeriod
			d.MinExecutionPeriod = doc.Value.Windows.MinExecutionPeriod
		}
	}
	return nil
}

// groupMembers is the part of `query group group-members` this tool reads.
type groupMembers struct {
	Members []struct {
		GroupID flexUint64 `json:"group_id"`
		Member  struct {
			Address  string `json:"address"`
			Weight   string `json:"weight"`
			Metadata string `json:"metadata"`
		} `json:"member"`
	} `json:"members"`
}

// thresholdDecisionPolicyType is the only decision policy this ceremony will
// accept for an office.
//
// Pinned rather than inferred. x/group also has a percentage policy, and a
// percentage over a membership that changes is a threshold that changes with it:
// an office admitted as "two of three" would become "three of four" the moment a
// fourth super user was added, which is a widening or a narrowing that nobody
// voted for and that no field on the record would show.
const thresholdDecisionPolicyType = "/cosmos.group.v1.ThresholdDecisionPolicy"

// confirmOffice is the whole of phase two, and the reason this tool exists in
// this shape.
//
// It takes the chain's answers about a group somebody has created and decides
// whether that group is this office. Not "does this address look like a group
// policy" — whether the policy at that address is administered by itself, needs
// the number of signatures this office's super users attested to, and has exactly
// their five addresses as its members and nobody else's.
//
// That is the check that makes it safe to name the address in a grant. A
// predicted address would have passed no check at all: the derivation does not
// depend on the members, so the address a sequence number produces is equally
// the address of a 2-of-3 of these super users and of a 1-of-1 of somebody who
// created a group policy first. The grant that would then be composed is a real
// grant of a country's payments or enforcement authority, made by the foundation,
// to a stranger, with every signature on it valid.
//
// Six refusals, and none of them is a warning:
//
//  1. the transaction did not create a group policy at all;
//  2. the policy info is for a different address than the transaction created;
//  3. the group id disagrees between the transaction, the policy and the members;
//  4. the policy is not self-administered, so something outside the office can
//     rewrite its membership;
//  5. the threshold or the policy type is not what the office attested to;
//  6. the member set is not exactly the office's roster, at equal weight.
// attestedGroup is the group a ceremony's custodians attested to, reduced to the
// four things the chain's answer has to be checked against.
//
// It exists so that there is ONE implementation of "is the policy at this address
// really the group these people generated". There were nearly two: the country
// enrolment's, and a copy for foundation administrators. Both decide who holds a
// power that cannot be taken back by the person who granted it — a national
// authority in one case, the ability to move any account out from under its
// regulator in the other — and a rule enforced in two places is a rule with two
// ways to be wrong.
type attestedGroup struct {
	// Name is what to call it in a refusal. "Banque Centrale du Sénégal", or the
	// administrator ceremony's name.
	Name string
	// Threshold is how many of Members must sign.
	Threshold int
	// Members are the member addresses, sorted, as the ceremony generated them.
	Members []string
}

// forbiddenAddress is an address this group must not turn out to be, and why.
//
// Carried as a reason rather than a bare list because the reason is the whole
// value of the check: for a country office, confirming at the foundation's own
// address would be the foundation granting itself a national authority; for an
// administrator group it would appoint the foundation, which on a live run is
// exactly what a predicted address produced, both being policy sequence 1.
type forbiddenAddress struct {
	Address string
	Reason  string
}

func confirmOffice(
	office *officeRecord,
	foundation string,
	tx txResult,
	policy policyInfo,
	members groupMembers,
	now time.Time,
) (onChainGroup, error) {
	return confirmGroup(
		attestedGroup{
			Name:      office.Name,
			Threshold: office.Threshold,
			Members:   office.memberAddresses(),
		},
		[]forbiddenAddress{{
			Address: strings.TrimSpace(foundation),
			Reason: "An office confirmed there would be the foundation granting itself a national " +
				"authority",
		}},
		tx, policy, members, now,
	)
}

// confirmGroup checks the chain's own answers against what a ceremony attested
// to, and returns the address only if every one of them agrees.
//
// The membership check at the end is the one the whole two-phase design exists
// for. An x/group policy address derives from the policy sequence number alone —
// not from the members, not from the threshold, not from the admin — so an
// address that looks right proves nothing at all about who controls it.
func confirmGroup(
	group attestedGroup,
	forbidden []forbiddenAddress,
	tx txResult,
	policy policyInfo,
	members groupMembers,
	now time.Time,
) (onChainGroup, error) {
	address, found := tx.attribute(eventCreateGroupPolicy, "address")
	if !found || strings.TrimSpace(address) == "" {
		return onChainGroup{}, fmt.Errorf(
			"transaction %s created no group policy — there is no %s event in it with an address.\n"+
				"That is the transaction that has to be queried here: the one that broadcast %s's "+
				"create-group-with-policy message",
			tx.TxHash, eventCreateGroupPolicy, group.Name)
	}
	if _, err := decodeAccountAddress(address); err != nil {
		return onChainGroup{}, fmt.Errorf(
			"transaction %s names %q as the group policy address, which is not an address this chain can read: %w",
			tx.TxHash, address, err)
	}

	// Refused outright, with the caller's reason. See forbiddenAddress.
	for _, banned := range forbidden {
		if banned.Address != "" && address == banned.Address {
			return onChainGroup{}, fmt.Errorf(
				"the group policy at %s is the foundation's own address. %s", address, banned.Reason)
		}
	}

	groupID, err := groupIDFrom(tx)
	if err != nil {
		return onChainGroup{}, err
	}

	if policy.Info.Address != address {
		return onChainGroup{}, fmt.Errorf(
			"the transaction created the policy at %s and the policy info given is for %s. Those are different "+
				"accounts, and the grant would land on whichever one this tool believed",
			address, policy.Info.Address)
	}
	if uint64(policy.Info.GroupID) != groupID {
		return onChainGroup{}, fmt.Errorf(
			"the transaction created group %d and the policy at %s belongs to group %d",
			groupID, address, uint64(policy.Info.GroupID))
	}

	// Self-administered, which is the property that makes the M-of-N mean
	// anything. An admin outside the group is a single account that can rewrite
	// the membership, so an office with an outside admin is an office whose
	// threshold is advisory — and the grant would still be to the policy address.
	if policy.Info.Admin != address {
		return onChainGroup{}, fmt.Errorf(
			"the group policy at %s is administered by %s rather than by itself. Whoever holds that address can "+
				"rewrite this office's membership without any of its super users agreeing, so the %d-of-%d is not "+
				"a %d-of-%d",
			address, policy.Info.Admin, group.Threshold, len(group.Members), group.Threshold, len(group.Members))
	}

	if policy.Info.DecisionPolicy.Type != thresholdDecisionPolicyType {
		return onChainGroup{}, fmt.Errorf(
			"the group policy at %s decides by %q, and this ceremony only recognises %s. A percentage policy over "+
				"a membership that can change is a threshold that changes with it",
			address, policy.Info.DecisionPolicy.Type, thresholdDecisionPolicyType)
	}
	threshold, err := strconv.Atoi(strings.TrimSpace(policy.Info.DecisionPolicy.Threshold))
	if err != nil {
		return onChainGroup{}, fmt.Errorf(
			"the group policy at %s has threshold %q, which is not a number",
			address, policy.Info.DecisionPolicy.Threshold)
	}
	if threshold != group.Threshold {
		return onChainGroup{}, fmt.Errorf(
			"the group policy at %s needs %d signatures and %s's super users attested to %d. The office on the "+
				"chain is not the office they agreed to",
			address, threshold, group.Name, group.Threshold)
	}

	if err := confirmMembers(group, address, groupID, members); err != nil {
		return onChainGroup{}, err
	}

	return onChainGroup{
		PolicyAddress: address,
		GroupID:       groupID,
		TxHash:        tx.TxHash,
		Height:        int64(tx.Height),
		ConfirmedAt:   now.UTC().Truncate(time.Second).Format(time.RFC3339),
	}, nil
}

// groupIDFrom reads the group id out of the transaction that created it.
//
// From the event rather than from the policy info, so that the id and the address
// come from the same document and the policy info is checked against both rather
// than being the source of either.
func groupIDFrom(tx txResult) (uint64, error) {
	raw, found := tx.attribute(eventCreateGroup, "group_id")
	if !found {
		return 0, fmt.Errorf(
			"transaction %s has no %s event, so it did not create a group. A create-group-with-policy "+
				"transaction emits both that and %s",
			tx.TxHash, eventCreateGroup, eventCreateGroupPolicy)
	}
	id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("transaction %s reports group id %q, which is not a number: %w", tx.TxHash, raw, err)
	}
	return id, nil
}

// confirmMembers is the check the whole two-phase design comes down to.
//
// Set equality, both ways, and equal weight. Not "the office's members are among
// the group's" — a group with a sixth member has a sixth vote, and a 3-of-5 with
// six members is a 3-of-6. Not "the group's members are among the office's" — a
// group missing one is a 3-of-4, which is a different arrangement with more
// authority concentrated in whoever remains.
func confirmMembers(group attestedGroup, address string, groupID uint64, members groupMembers) error {
	expected := group.Members

	actual := make([]string, 0, len(members.Members))
	for _, entry := range members.Members {
		if uint64(entry.GroupID) != groupID {
			return fmt.Errorf(
				"the member list given is for group %d and this ceremony's group is %d",
				uint64(entry.GroupID), groupID)
		}
		// Equal weight, because a member with two votes turns a 3-of-5 into
		// something no field on the record describes. The ceremony writes weight
		// 1 for everybody; anything else means the group on chain is not the one
		// it composed.
		if strings.TrimSpace(entry.Member.Weight) != "1" {
			return fmt.Errorf(
				"%s holds weight %q in the group at %s. Equal weight is what makes %d-of-%d mean what it says; a "+
					"member with two votes is a threshold nobody agreed to",
				entry.Member.Address, entry.Member.Weight, address, group.Threshold, len(group.Members))
		}
		actual = append(actual, entry.Member.Address)
	}
	sort.Strings(actual)

	if len(actual) != len(expected) || !equalStrings(actual, expected) {
		return fmt.Errorf(
			"the group at %s is not %s.\n"+
				"  it has %d members: %s\n"+
				"  the ceremony generated %d: %s\n"+
				"This is the check the two-phase design exists for. An x/group policy address derives from the "+
				"policy sequence number alone, so an address that looks right proves nothing about who controls "+
				"it — the membership is the only thing that does. Do not grant anything to this address.",
			address, group.Name,
			len(actual), strings.Join(actual, ", "),
			len(expected), strings.Join(expected, ", "))
	}
	return nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- constitution

// invariantsResponse is the part of `query constitution invariants` this reads.
type invariantsResponse struct {
	Invariants struct {
		EnforcementRecoveryDestination string     `json:"enforcement_recovery_destination"`
		FoundationCustodianCount       flexUint64 `json:"foundation_custodian_count"`
		FoundationSignatureThreshold   flexUint64 `json:"foundation_signature_threshold"`
	} `json:"invariants"`
}

// requireFoundation checks that the address the dossier calls the foundation is
// the address the chain's constitution pins.
//
// This is not a formality, because the whole reason x/alias reads the foundation
// out of an invariant rather than out of a parameter list is that an invariant
// cannot be moved by an ordinary vote. A dossier naming some other address would
// compose a proposal that three custodians vote for and the chain then refuses,
// after the vote — which is the expensive way to find out.
// Returns the number of custodian signatures the constitution says the foundation
// needs, so the tool can tell an operator how many votes to collect instead of
// guessing or leaving a blank.
func requireFoundation(dossier countryDossier, path string) (int, error) {
	var response invariantsResponse
	if err := readJSONFile(path, &response); err != nil {
		return 0, err
	}
	pinned := strings.TrimSpace(response.Invariants.EnforcementRecoveryDestination)
	if pinned == "" {
		return 0, fmt.Errorf(
			"%s names no enforcement_recovery_destination. On a chain that started correctly this cannot happen "+
				"— x/constitution refuses a genesis without one — so either this is not an invariants response or "+
				"it is from somewhere else", path)
	}
	if pinned != strings.TrimSpace(dossier.Foundation) {
		return 0, fmt.Errorf(
			"the constitution pins %s as the foundation and this enrolment names %s.\n"+
				"Only the pinned account may admit a country, so a proposal built for the second would be voted "+
				"through by its custodians and then refused by the chain",
			pinned, dossier.Foundation)
	}

	// The threshold is read rather than assumed to be three. It is a
	// constitutional invariant, so it is the chain's answer to "how many
	// custodians have to agree", and a tool that printed a number of its own
	// would be telling an operator to collect the wrong number of votes.
	threshold := int(response.Invariants.FoundationSignatureThreshold)
	if threshold < 2 {
		return 0, fmt.Errorf(
			"the constitution says the foundation needs %d signatures. Below two there is no foundation to "+
				"speak of, and this is not a chain to enrol a country on", threshold)
	}
	return threshold, nil
}


// ---------------------------------------------------------------- alias params

// aliasParamsResponse is the part of `query alias params` this reads.
type aliasParamsResponse struct {
	Params struct {
		// PayloadLength is read as well as the list, because the appointment
		// ceremony has to RESUBMIT it: MsgUpdateParams carries a Params message
		// rather than a field mask, so setting it replaces the whole object and a
		// parameter this tool did not read is a parameter it would zero.
		//
		// flexUint64 because the two producers disagree: the CLI's JSON renders a
		// uint32 as a number and the REST gateway has rendered it as a string. A
		// type that accepted only one of them would read zero from the other, and
		// zero is the value that means "unknown" here.
		PayloadLength            flexUint64 `json:"payload_length"`
		FoundationAdministrators []string   `json:"foundation_administrators"`
	} `json:"params"`
}

// requireFoundationAdministrator checks that the foundation may record a
// jurisdiction.
//
// This is a coupling worth being explicit about, because it is the one place
// where the two things called "the foundation" have to be the same account.
// MsgGrantRole recognises the address x/constitution pins. MsgSetJurisdiction
// recognises the addresses in x/alias's own foundation_administrators parameter.
// They are different mechanisms with different amendment costs, deliberately —
// but an enrolment needs both, because an office's group account was onboarded by
// no participant and so nobody but an administrator or governance can record
// where it is.
//
// So the ceremony refuses rather than composing a proposal that will fail. The
// fix is a governance proposal adding the foundation's policy address to
// alias.params.foundation_administrators, once, before any country is enrolled.
func requireFoundationAdministrator(dossier countryDossier, path string) error {
	var response aliasParamsResponse
	if err := readJSONFile(path, &response); err != nil {
		return err
	}
	foundation := strings.TrimSpace(dossier.Foundation)
	for _, administrator := range response.Params.FoundationAdministrators {
		if strings.TrimSpace(administrator) == foundation {
			return nil
		}
	}
	return fmt.Errorf(
		"%s is not in alias.params.foundation_administrators, so it cannot record a jurisdiction.\n"+
			"An office's group account was onboarded by no participant, so the only accounts that may place it "+
			"are a foundation administrator and governance — and %q is currently %s.\n"+
			"This is a one-off governance proposal, not part of the enrolment: add the foundation's policy "+
			"address to that parameter before enrolling any country. It is a separate mechanism from the "+
			"constitutional invariant that decides who may grant a role, and both are needed.",
		foundation, "foundation_administrators",
		describeList(response.Params.FoundationAdministrators))
}

func describeList(values []string) string {
	if len(values) == 0 {
		return "empty"
	}
	return strings.Join(values, ", ")
}

// ---------------------------------------------------------------- role grants

// roleGrantsResponse is the part of `query alias role-grants` this reads.
type roleGrantsResponse struct {
	Grants []struct {
		Holder          string    `json:"holder"`
		Role            string    `json:"role"`
		Jurisdiction    string    `json:"jurisdiction"`
		GrantedBy       string    `json:"granted_by"`
		GrantedAtHeight flexInt64 `json:"granted_at_height"`

		// RequiredShape is a pointer because the chain omits it when a grant
		// records no requirement, and those two states must not read the same. A
		// struct value would decode an absent field as zero-of-zero, and a grant
		// pinning nothing would then look like a grant pinning a shape whose
		// numbers happened to be zero — which is the whole reason the chain's own
		// field is a message rather than two integers.
		RequiredShape *struct {
			Signatures flexUint64 `json:"signatures"`
			Members    flexUint64 `json:"members"`
		} `json:"required_shape"`
	} `json:"grants"`
}

// verifyGrants reads an office's grants back off the chain.
//
// Every role the dossier says the office should hold has to be present, in this
// country, granted by the foundation — and the check is that last part as much as
// the first. A grant of the right role in the right country made by some other
// authority is a grant this ceremony did not make, and reporting it as this
// ceremony's would put a signature on somebody else's act.
//
// Grants the office holds that the dossier does not describe are reported rather
// than refused. They are not this enrolment's business — an office may
// legitimately have been granted something else by governance — but an operator
// verifying an enrolment should see them, because an office holding more than
// the record says is the thing a reader of the record would want to know.
func verifyGrants(office *officeRecord, country, foundation, path string, now time.Time) ([]grantEvidence, []string, error) {
	if office.OnChain == nil {
		return nil, nil, fmt.Errorf(
			"%s has no confirmed group, so there is no holder address to look grants up for", office.Name)
	}

	var response roleGrantsResponse
	if err := readJSONFile(path, &response); err != nil {
		return nil, nil, err
	}

	expected, err := rolesOf(office.Roles)
	if err != nil {
		return nil, nil, err
	}
	minimum, err := requireOfficeMinimum(*office)
	if err != nil {
		return nil, nil, err
	}

	stamp := now.UTC().Truncate(time.Second).Format(time.RFC3339)
	verified := make([]grantEvidence, 0, len(expected))
	extra := []string{}
	matched := map[string]bool{}

	for _, grant := range response.Grants {
		if strings.TrimSpace(grant.Holder) != office.OnChain.PolicyAddress {
			return nil, nil, fmt.Errorf(
				"%s lists a grant held by %s, and %s's confirmed address is %s. That response is about a "+
					"different account",
				path, grant.Holder, office.Name, office.OnChain.PolicyAddress)
		}
		key := grant.Role + "@" + grant.Jurisdiction
		matched[key] = true
	}

	for _, role := range expected {
		name := aliastypes.RoleName(role)
		var found *grantEvidence
		for _, grant := range response.Grants {
			if grant.Role != name || grant.Jurisdiction != country {
				continue
			}
			if strings.TrimSpace(grant.GrantedBy) != strings.TrimSpace(foundation) {
				return nil, nil, fmt.Errorf(
					"%s holds %s in %s, and it was granted by %s rather than by the foundation %s.\n"+
						"That grant is somebody else's act, and recording it on this enrolment's record would put "+
						"this ceremony's name on it",
					office.Name, name, country, grant.GrantedBy, foundation)
			}
			// The shape the chain records, checked against the minimum this
			// enrolment agreed. Two failures and they are different failures:
			//
			//   - absent. The grant landed without a required shape at all, so the
			//     office is unconstrained and could vote itself to a single key.
			//     That is a grant made by a chain, a proposal or a tool that did not
			//     carry the requirement, and recording it as verified would put this
			//     ceremony's signature on an office nothing holds to anything.
			//   - present and lower than agreed. Somebody edited the proposal, or the
			//     dossier and the proposal disagree. Either way what is on the chain
			//     is not what was decided.
			//
			// A shape HIGHER than the minimum is accepted and recorded as what the
			// chain says. It constrains the office more than agreed, which is not
			// this check's business to reverse, and the record names the real number
			// rather than the one that was asked for.
			if grant.RequiredShape == nil {
				return nil, nil, fmt.Errorf(
					"%s holds %s in %s and the grant records no required shape, though this enrolment agreed %s.\n"+
						"An office with no recorded shape can vote itself down to a single key and go on holding "+
						"the authority, so this grant does not constrain it. Re-make it with the required shape:\n"+
						"  ceremony country grants --dossier <file> --proposer <custodian>",
					office.Name, name, country, minimum.rule())
			}
			onChain := officeMinimum{
				Signatures: int(grant.RequiredShape.Signatures),
				Members:    int(grant.RequiredShape.Members),
			}
			if onChain.Signatures < minimum.Signatures || onChain.Members < minimum.Members {
				return nil, nil, fmt.Errorf(
					"%s holds %s in %s with a required shape of %s, and this enrolment agreed %s.\n"+
						"The grant on the chain is weaker than the decision on the record, so one of the two is "+
						"wrong and neither can be signed as though it described the other",
					office.Name, name, country, onChain.rule(), minimum.rule())
			}
			found = &grantEvidence{
				Role:            name,
				Jurisdiction:    country,
				GrantedBy:       strings.TrimSpace(grant.GrantedBy),
				GrantedAtHeight: int64(grant.GrantedAtHeight),
				VerifiedAt:      stamp,
				RequiredShape:   onChain.rule(),
			}
			break
		}
		if found == nil {
			return nil, nil, fmt.Errorf(
				"%s does not hold %s in %s.\n"+
					"The proposal may have been accepted and not executed: a broadcast code of 0 means a "+
					"transaction entered a mempool, and an x/group proposal additionally has to reach its "+
					"threshold and then be executed. Check the proposal:\n"+
					"  blockchaind query group proposals-by-group-policy %s -o json",
				office.Name, name, country, foundation)
		}
		verified = append(verified, *found)
	}

	for key := range matched {
		parts := strings.SplitN(key, "@", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[1] == country && office.holdsRole(aliastypes.Role(aliastypes.Role_value[parts[0]])) {
			continue
		}
		extra = append(extra, fmt.Sprintf("%s in %s", parts[0], parts[1]))
	}
	sort.Strings(extra)

	return verified, extra, nil
}

// ---------------------------------------------------------------- jurisdictions

// jurisdictionResponse is the part of `query alias jurisdiction` this reads.
type jurisdictionResponse struct {
	Jurisdiction struct {
		Address          string    `json:"address"`
		Country          string    `json:"country"`
		RecordedBy       string    `json:"recorded_by"`
		RecordedAtHeight flexInt64 `json:"recorded_at_height"`
	} `json:"jurisdiction"`
}

// verifyPlacement reads a jurisdiction record back off the chain.
//
// The recorder is checked as well as the country, and by whom is the whole point
// of the record: a country recorded by the account itself would be a
// self-declaration, and a country recorded by an institution that no longer banks
// the account is a perimeter stamped with a relationship that has ended. The
// chain refuses both; this refuses to write either onto a signed record.
func verifyPlacement(account, country, path string, now time.Time) (placement, error) {
	var response jurisdictionResponse
	if err := readJSONFile(path, &response); err != nil {
		return placement{}, err
	}

	record := response.Jurisdiction
	if strings.TrimSpace(record.Address) == "" {
		return placement{}, fmt.Errorf(
			"%s carries no jurisdiction record. An account the chain cannot place is an account no authority's "+
				"perimeter contains, so nothing can be done to it and nothing can be done for it", path)
	}
	if strings.TrimSpace(record.Address) != strings.TrimSpace(account) {
		return placement{}, fmt.Errorf(
			"%s is the record for %s and this step is about %s", path, record.Address, account)
	}
	if aliastypes.NormaliseCountry(record.Country) != aliastypes.NormaliseCountry(country) {
		return placement{}, fmt.Errorf(
			"%s is recorded in %s and this enrolment is for %s. An office in the wrong perimeter would hold "+
				"authority over a country it is not in",
			account, record.Country, country)
	}
	if strings.TrimSpace(record.RecordedBy) == "" {
		return placement{}, fmt.Errorf(
			"the record for %s names nobody as having recorded it. \"Who says this account is in %s\" is the "+
				"question asked when the answer turns out to be wrong",
			account, country)
	}
	if strings.TrimSpace(record.RecordedBy) == strings.TrimSpace(account) {
		return placement{}, fmt.Errorf(
			"%s recorded its own jurisdiction. An account free to name its own perimeter would name the one "+
				"with no authority watching it", account)
	}

	return placement{
		Account:    strings.TrimSpace(record.Address),
		Country:    aliastypes.NormaliseCountry(record.Country),
		RecordedBy: strings.TrimSpace(record.RecordedBy),
		VerifiedAt: now.UTC().Truncate(time.Second).Format(time.RFC3339),
	}, nil
}

// ---------------------------------------------------------------- participants

// approvedParticipantResponse is the part of `query paymsg get-approved-
// participant` this reads.
type approvedParticipantResponse struct {
	ApprovedParticipant struct {
		Participant string `json:"participant"`
		Code        string `json:"code"`
		Name        string `json:"name"`
	} `json:"approved_participant"`
}

// verifyAdmission reads an approved participant back off the chain.
func verifyAdmission(participant, path string, now time.Time) (admission, error) {
	var response approvedParticipantResponse
	if err := readJSONFile(path, &response); err != nil {
		return admission{}, err
	}
	record := response.ApprovedParticipant
	if strings.TrimSpace(record.Participant) == "" {
		return admission{}, fmt.Errorf(
			"%s carries no approved participant. An application that was submitted and never approved looks "+
				"exactly like this, and so does an approval transaction that was accepted and then failed in "+
				"delivery", path)
	}
	if strings.TrimSpace(record.Participant) != strings.TrimSpace(participant) {
		return admission{}, fmt.Errorf(
			"%s is the record for %s and this step is about %s", path, record.Participant, participant)
	}
	if strings.TrimSpace(record.Code) == "" {
		return admission{}, fmt.Errorf(
			"%s is approved with no participant code, which is the identifier it appears under on every payment "+
				"instruction", participant)
	}
	return admission{
		Participant: strings.TrimSpace(record.Participant),
		Code:        strings.TrimSpace(record.Code),
		Name:        strings.TrimSpace(record.Name),
		VerifiedAt:  now.UTC().Truncate(time.Second).Format(time.RFC3339),
	}, nil
}
