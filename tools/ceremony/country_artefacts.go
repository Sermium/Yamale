package main

// What the country enrolment ceremony hands an operator: files to broadcast, and
// the exact commands to broadcast them with.
//
// Every message in here is built from the chain's own Go types and marshalled
// through the proto codec, never assembled as hand-written JSON. That is worth a
// sentence because the tempting alternative is a map literal with an "@type" in
// it, and this repository has a drift guard for exactly the reason that would be
// wrong: a field renamed in the proto breaks this build, where a map literal would
// keep producing a document the chain silently decodes into a message with a zero
// where the value used to be. The messages that matter here grant a country's
// enforcement authority.
//
// Nothing here signs anything, and that is the same decision clients/foundation
// took for the same reason. A tool that signed on a custodian's behalf would be
// asking them to approve an act they can only see through this tool's
// description of it; a tool that composes a document and prints the command puts
// the signature where the key is, and what the custodian approves is what their
// own node decodes.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/x/group"

	aliastypes "yamale/blockchain/x/alias/types"
	paymsgtypes "yamale/blockchain/x/paymsg/types"
)

// maxMetadataLen is x/group's cap on a proposal's metadata, title and summary.
//
// 255, matching MaxMetadataLen in app/app_config.go and MAX_METADATA_LEN in
// clients/foundation. Checked before composing rather than discovered on
// broadcast, because the rejection names a byte length and not the field that was
// too long — and by then three custodians have already been asked to vote.
const maxMetadataLen = 255

// enrolmentCodec is the codec every artefact is marshalled through.
//
// x/group for the proposal wrapper, x/alias for the grants and the jurisdiction
// records, x/paymsg for the participant admission.
//
// Three of the chain's type packages, which is more than this tool used to link
// and is worth justifying rather than shrugging at. The rule the program follows
// is that it must not link the node — a ceremony binary that carried a keeper or
// a store would defeat running it on a machine the node has never touched. A
// types package carries proto messages and pure functions: between them these
// three add exactly one package to what the tool already imported for x/group.
//
// What they buy is that every message this file composes is the chain's own type.
// The alternative is a map literal with an "@type" in it, and a field renamed in
// the proto would then keep producing a document the chain decodes into a message
// with a zero where the value used to be. The messages here grant a country's
// enforcement authority, so that is not a risk worth taking to avoid an import.
func enrolmentCodec() codec.Codec {
	registry := codectypes.NewInterfaceRegistry()
	group.RegisterInterfaces(registry)
	aliastypes.RegisterInterfaces(registry)
	paymsgtypes.RegisterInterfaces(registry)
	return codec.NewProtoCodec(registry)
}

// ---------------------------------------------------------------- phase one

// createGroupFiles is what one office's group needs to be created on chain.
type createGroupFiles struct {
	Office   string
	Members  []byte
	Policy   []byte
	Metadata string
	// Threshold and MemberCount are printed with the command so the operator can
	// see what they are about to create without opening the files.
	Threshold   int
	MemberCount int
}

// officeCreateFiles builds the two documents `blockchaind tx group
// create-group-with-policy` reads.
//
// The members and the policy come from assembleGroup — the same pure function the
// office's super users each ran in their own browser — so the group created on
// chain is the group whose fingerprint they read to each other. Recomputed here
// rather than copied out of the group file for the reason readAssembledGroup
// exists: a file that travelled is a file that can have been edited, and the
// member set is the one field that decides who ends up holding a country's
// authority.
//
// What this deliberately does NOT produce is an address. The transaction below
// creates the policy and the chain decides where it lands; `ceremony country
// confirm` reads it back. See the top of country.go for why predicting it would
// be handing a stranger a national authority.
func officeCreateFiles(office officeRecord, assembled assembled) (createGroupFiles, error) {
	if len(assembled.Members) == 0 || len(assembled.Policy) == 0 {
		return createGroupFiles{}, fmt.Errorf(
			"%s's ceremony produced no member or policy document", office.Name)
	}

	metadata, err := officeGroupMetadata(assembled)
	if err != nil {
		return createGroupFiles{}, fmt.Errorf("%s: %w", office.Name, err)
	}

	return createGroupFiles{
		Office:      office.Name,
		Members:     assembled.Members,
		Policy:      assembled.Policy,
		Metadata:    metadata,
		Threshold:   assembled.Params.Threshold,
		MemberCount: len(assembled.Custodians),
	}, nil
}

// officeGroupMetadata is the string the group carries on chain.
//
// Read out of the assembled documents rather than composed here, so there is one
// implementation of it and the value on chain is the value inside the fingerprint
// the office's super users compared. Composing it a second time in this file is
// how the two would drift, and the drift would be invisible: both strings would
// look like a description of the same office.
func officeGroupMetadata(assembled assembled) (string, error) {
	var msg struct {
		GroupMetadata string `json:"group_metadata"`
	}
	if err := json.Unmarshal(assembled.CreateMsg, &msg); err != nil {
		return "", fmt.Errorf("the ceremony's create-group message cannot be read: %w", err)
	}
	if strings.TrimSpace(msg.GroupMetadata) == "" {
		return "", fmt.Errorf("the ceremony's create-group message carries no group metadata")
	}
	if len(msg.GroupMetadata) > maxMetadataLen {
		return "", fmt.Errorf(
			"the group metadata is %d bytes and x/group accepts %d. Shorten the office's ceremony name: it is "+
				"the part of this string that a person chose",
			len(msg.GroupMetadata), maxMetadataLen)
	}
	return msg.GroupMetadata, nil
}

// ---------------------------------------------------------------- phase two

// proposalDocument is the JSON x/group's submit-proposal reads.
//
// Field names and shape taken from the same document clients/foundation composes,
// because a custodian may well be looking at both: the console shows them the
// foundation's proposals and this writes one of them.
type proposalDocument struct {
	GroupPolicyAddress string            `json:"group_policy_address"`
	Messages           []json.RawMessage `json:"messages"`
	Metadata           string            `json:"metadata"`
	Proposers          []string          `json:"proposers"`
	Title              string            `json:"title"`
	Summary            string            `json:"summary"`
}

// enrolmentProposal is the one act that makes a country operational.
//
// One proposal rather than one per office or one per grant, and that is the
// decision worth defending. Enrolling a country is a single decision the
// custodians take once; split across five proposals it becomes five votes that
// can individually pass, fail or time out, and the state in between is a country
// whose payments authority exists and whose enforcement authority does not — with
// money moving in a perimeter nobody can stop. x/group executes a proposal's
// messages together or not at all, so one proposal is the only shape in which
// "the country is enrolled" is a thing that either happened or did not.
//
// The cost of that choice is a longer document for three custodians to read, and
// it is mitigated the way clients/foundation mitigates it: the summary states
// every grant in words, so a custodian who reads only the summary has read the
// proposal.
//
// Two kinds of message, in this order:
//
//  1. MsgSetJurisdiction, placing each office's own group account in the country.
//     First, because an office nobody has placed is an office no authority's
//     perimeter contains — including its own — and several things downstream
//     refuse an account the chain cannot place. Signed by the foundation as a
//     foundation administrator, because no participant onboarded a group policy
//     and nobody may declare their own.
//  2. MsgGrantRole, one per office per role, scoped to the country and never to
//     the chain-wide marker. Signed by the foundation, which the constitution
//     pins; the chain refuses the chain-wide scope from it, and so does this.
func enrolmentProposal(dossier countryDossier, proposer string) ([]byte, error) {
	if err := requireEnrolmentCountry(dossier.Country); err != nil {
		return nil, err
	}
	if err := requireAccountAddress("proposer", proposer); err != nil {
		return nil, err
	}
	if err := requireProposerIsCustodian(dossier, proposer); err != nil {
		return nil, err
	}

	cdc := enrolmentCodec()
	messages := make([]json.RawMessage, 0, len(dossier.Offices)*3)
	var placed, granted []string

	for _, office := range dossier.Offices {
		address, err := requireConfirmed(office)
		if err != nil {
			return nil, err
		}

		placement := &aliastypes.MsgSetJurisdiction{
			Recorder: dossier.Foundation,
			Account:  address,
			Country:  dossier.Country,
		}
		encoded, err := cdc.MarshalInterfaceJSON(placement)
		if err != nil {
			return nil, err
		}
		messages = append(messages, encoded)
		placed = append(placed, fmt.Sprintf("%s (%s)", office.Name, address))

		roles, err := rolesOf(office.Roles)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", office.Name, err)
		}
		for _, role := range roles {
			grant := &aliastypes.MsgGrantRole{
				Authority:    dossier.Foundation,
				Holder:       address,
				Role:         role,
				Jurisdiction: dossier.Country,
			}
			// The scope this grant names is dossier.Country, and
			// requireEnrolmentCountry refused every unacceptable value of it at the
			// top of this function. A second check stood here and was removed: it
			// tested the same value against the same rule, and a rule enforced
			// twice in one function is a rule with two places to stop being
			// enforced.
			encoded, err := cdc.MarshalInterfaceJSON(grant)
			if err != nil {
				return nil, err
			}
			messages = append(messages, encoded)
			granted = append(granted, fmt.Sprintf("%s to %s", aliastypes.RoleName(role), office.Name))
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("this enrolment would propose nothing")
	}

	title := fmt.Sprintf("Enrol %s", dossier.Country)
	summary := enrolmentSummary(dossier, placed, granted)
	if err := requireMetadataLength("title", title); err != nil {
		return nil, err
	}
	if err := requireMetadataLength("summary", summary); err != nil {
		return nil, err
	}

	document := proposalDocument{
		GroupPolicyAddress: dossier.Foundation,
		Messages:           messages,
		Metadata:           "",
		Proposers:          []string{proposer},
		Title:              title,
		Summary:            summary,
	}
	return json.MarshalIndent(document, "", "  ")
}

// enrolmentSummary states the whole proposal in words.
//
// Every grant named, because the alternative is a custodian voting on a document
// whose effect they would have to decode from a list of Any-wrapped messages. The
// summary is the part they will actually read, so it has to be the part that is
// complete rather than the part that is short.
func enrolmentSummary(dossier countryDossier, placed, granted []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Admit %s. Places %d office account(s) in %s and grants: %s.",
		dossier.Country, len(placed), dossier.Country, strings.Join(granted, "; "))
	if len(dossier.Waivers) > 0 {
		rules := make([]string, 0, len(dossier.Waivers))
		for _, w := range dossier.Waivers {
			rules = append(rules, w.Rule)
		}
		fmt.Fprintf(&b, " Waived: %s.", strings.Join(rules, ", "))
	}
	summary := b.String()
	// Truncated rather than refused, because a country with many offices can
	// legitimately exceed 255 bytes and refusing would mean an enrolment that
	// cannot be proposed at all. The full list is in the dossier and on the
	// record; what is lost here is the tail of a sentence, and the marker says so.
	if len(summary) > maxMetadataLen {
		const marker = " [truncated; see the enrolment record]"
		summary = summary[:maxMetadataLen-len(marker)] + marker
	}
	return summary
}

func requireMetadataLength(field, value string) error {
	if len(value) <= maxMetadataLen {
		return nil
	}
	return fmt.Errorf(
		"the proposal's %s is %d bytes and x/group accepts %d", field, len(value), maxMetadataLen)
}

// requireEnrolmentCountry is the backstop every message-composing function runs
// before it writes a country into anything.
//
// The config validation already refuses "*" and the foundation's reserved code, so
// on every path a person can take this is unreachable. It is here because it
// guards the values whose mistake is unrecoverable — a grant of authority over
// every country, signed by the foundation, or a jurisdiction record naming
// nowhere — and the failure it guards against is a bug in this tool rather than an
// operator's input.
//
// It exists in this shape because a mutation pass found the grant path's copy of
// it untested and, in closing that, found that seedProposal and the validator
// placement had no copy at all: from a dossier whose country had somehow become
// "*", both composed a MsgSetJurisdiction naming it. The chain refuses that —
// MsgSetJurisdiction takes an assigned country and nothing else — so it was never
// exploitable, but a tool that composes a message the chain will reject is a tool
// that has asked three custodians to vote for nothing.
//
// One function rather than three copies, for the reason AssertScope is one
// function: a rule enforced in three places is a rule with three ways to be wrong.
func requireEnrolmentCountry(country string) error {
	switch {
	case country == aliastypes.ChainWide:
		return fmt.Errorf(
			"this enrolment's country is %q, the chain-wide scope. The foundation may admit a country and may "+
				"not manufacture authority over every country; the chain refuses it and so does this",
			aliastypes.ChainWide)
	case country == aliastypes.FoundationCountry:
		return fmt.Errorf(
			"this enrolment's country is %q, which marks the absence of a national perimeter rather than a "+
				"country. Nothing can be granted in it and no account can be recorded there",
			aliastypes.FoundationCountry)
	case !aliastypes.AssignedCountry(country):
		return fmt.Errorf(
			"this enrolment's country is %q, which is not an assigned ISO 3166-1 alpha-2 code. A dossier should "+
				"never reach this state, so treat it as a bug in the tool rather than as a config error",
			country)
	}
	return nil
}

// requireConfirmed is the refusal the two-phase design exists to make.
//
// No confirmed address, no grant. There is no flag that relaxes it, no fallback
// to a predicted address, and no path on which an office with an absent OnChain
// block produces a message naming anything at all.
func requireConfirmed(office officeRecord) (string, error) {
	if office.OnChain == nil {
		return "", fmt.Errorf(
			"%s's group has not been read back from the chain, so this tool does not know its address and will "+
				"not guess one.\n"+
				"An x/group policy address derives from the policy sequence number alone — not from the members, "+
				"the threshold or the admin — so an address computed offline commits to nothing about who "+
				"controls it. A grant naming a predicted address would go to whoever created that policy first, "+
				"and it would be a real grant of a national authority made by the foundation.\n"+
				"Create the group, then:\n"+
				"  ceremony country confirm --dossier <file> --office %q --tx tx.json --policy policy.json "+
				"--members members.json",
			office.Name, office.Name)
	}
	if strings.TrimSpace(office.OnChain.PolicyAddress) == "" {
		return "", fmt.Errorf(
			"%s has a confirmation record with no address in it, which should be impossible; do not proceed",
			office.Name)
	}
	return office.OnChain.PolicyAddress, nil
}

// requireProposerIsCustodian refuses a proposer who is not a member of the
// foundation group.
//
// x/group refuses it too, so this is not the check that stops it happening — it
// is the check that stops it happening after somebody has assembled a proposal,
// read it aloud on a call and asked two colleagues to vote. The dossier does not
// carry the foundation's membership, so what this can do is refuse the one case
// it can see: a proposer that is one of the country offices' super users, which
// is the plausible mistake rather than a hypothetical one.
func requireProposerIsCustodian(dossier countryDossier, proposer string) error {
	for _, office := range dossier.Offices {
		for _, member := range office.Members {
			if member.Address == strings.TrimSpace(proposer) {
				return fmt.Errorf(
					"%s is %s, a super user of %q. The enrolment proposal belongs to the FOUNDATION group — it is "+
						"the foundation that admits a country — so the proposer has to be one of its custodians. "+
						"A country office proposing its own authority would be an office granting itself a role",
					proposer, member.Name, office.Name)
			}
		}
	}
	if strings.TrimSpace(proposer) == strings.TrimSpace(dossier.Foundation) {
		return fmt.Errorf(
			"%s is the foundation's own policy address, not a custodian's key. A proposal is submitted by a "+
				"member of the group, who then votes on it along with two others", proposer)
	}
	return nil
}

// ---------------------------------------------------------------- the seed

// seedProposal places the first institutions in the country.
//
// This is the step the bootstrap order needs and that nothing in the design
// obviously announces, so it has a command of its own rather than being folded
// into the enrolment.
//
// x/paymsg's delegated approval path — the one that lets a national payments
// authority admit an institution without a governance vote — calls AssertScope on
// the applicant, and AssertScope refuses a target the chain cannot place before it
// looks at any grant. So the country's payments authority, holding a perfectly
// good grant, cannot admit the first bank in its own country: that bank has no
// jurisdiction record, and the only parties who may write one are the participant
// that onboarded it (there is none — it is the first) and a foundation
// administrator.
//
// Hence a seed. Once, per country, for the institutions that will be admitted
// first; everything after that is ordinary, because an approved participant can
// place the accounts it onboards.
func seedProposal(dossier countryDossier, proposer string, accounts []string) ([]byte, error) {
	if err := requireEnrolmentCountry(dossier.Country); err != nil {
		return nil, err
	}
	if err := requireAccountAddress("proposer", proposer); err != nil {
		return nil, err
	}
	if err := requireProposerIsCustodian(dossier, proposer); err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf(
			"name at least one applicant institution to place. Without one, the country's payments authority " +
				"cannot admit anybody: the delegated approval path refuses an applicant the chain cannot place, " +
				"and no participant exists yet to place the first one")
	}

	// Sorted and deduplicated, so the document depends on the set of accounts
	// rather than on the order they were typed, and so a repeated address is a
	// refusal rather than two identical messages in one proposal.
	sorted := append([]string(nil), accounts...)
	sort.Strings(sorted)
	cdc := enrolmentCodec()
	messages := make([]json.RawMessage, 0, len(sorted))
	seen := map[string]bool{}

	for _, account := range sorted {
		address := strings.TrimSpace(account)
		if err := requireAccountAddress("applicant", address); err != nil {
			return nil, err
		}
		if seen[address] {
			return nil, fmt.Errorf("%s is named twice", address)
		}
		seen[address] = true

		// An office is not an applicant. The enrolment proposal already places
		// every office, and placing one twice in a second proposal would look on
		// the record like two different facts about it.
		for _, office := range dossier.Offices {
			if office.OnChain != nil && office.OnChain.PolicyAddress == address {
				return nil, fmt.Errorf(
					"%s is %s's own group account, which the enrolment proposal already places. This step is for "+
						"the institutions the payments authority will admit", address, office.Name)
			}
		}

		encoded, err := cdc.MarshalInterfaceJSON(&aliastypes.MsgSetJurisdiction{
			Recorder: dossier.Foundation,
			Account:  address,
			Country:  dossier.Country,
		})
		if err != nil {
			return nil, err
		}
		messages = append(messages, encoded)
	}

	title := fmt.Sprintf("Place the first %s applicants", dossier.Country)
	summary := fmt.Sprintf(
		"Records %d applicant institution(s) in %s so that %s's payments authority can admit them. "+
			"Only a foundation administrator can do this for the first ones: the delegated approval path "+
			"refuses an applicant with no jurisdiction, and no participant exists yet to record one.",
		len(messages), dossier.Country, dossier.Country)
	if err := requireMetadataLength("title", title); err != nil {
		return nil, err
	}
	if len(summary) > maxMetadataLen {
		summary = summary[:maxMetadataLen]
	}

	return json.MarshalIndent(proposalDocument{
		GroupPolicyAddress: dossier.Foundation,
		Messages:           messages,
		Metadata:           "",
		Proposers:          []string{proposer},
		Title:              title,
		Summary:            summary,
	}, "", "  ")
}

// ---------------------------------------------------------------- phase three

// admissionProposal is the payments authority admitting an institution.
//
// A proposal to the OFFICE's group, not the foundation's, and that is the whole
// point of having enrolled the country: the decision to license a payment service
// provider in Senegal is taken by Senegal's payments authority, M-of-N, and not by
// the foundation and not by a chain-wide governance vote.
//
// Refused unless three things have been read back off the chain first, in order,
// because the bootstrap order is what makes this work and getting it wrong looks
// like a broken chain rather than a missing step:
//
//  1. the office's group exists and is the office's;
//  2. it actually holds PAYMENTS_AUTHORITY in this country — verified from the
//     chain's own answer, not from the fact that a proposal was submitted;
//  3. the applicant has a jurisdiction record in this country, because the
//     delegated path refuses a target the chain cannot place.
func admissionProposal(dossier countryDossier, proposer, applicant string, approve bool) ([]byte, error) {
	office, err := dossier.paymentsOffice()
	if err != nil {
		return nil, err
	}
	address, err := requireConfirmed(*office)
	if err != nil {
		return nil, err
	}
	if err := requireAccountAddress("proposer", proposer); err != nil {
		return nil, err
	}
	if err := requireAccountAddress("applicant", applicant); err != nil {
		return nil, err
	}

	if !office.grantVerified(aliastypes.ROLE_PAYMENTS_AUTHORITY, dossier.Country) {
		return nil, fmt.Errorf(
			"%s has not been verified to hold %s in %s.\n"+
				"Submitting this proposal now would put it in front of the office's super users to vote on, and "+
				"the message would then be refused when it executed — after the vote, with the refusal in a "+
				"transaction log nobody is watching.\n"+
				"Verify the grant first:\n"+
				"  blockchaind query alias role-grants %s -o json > grants.json\n"+
				"  ceremony country verify --dossier <file> --office %q --grants grants.json",
			office.Name, aliastypes.RoleName(aliastypes.ROLE_PAYMENTS_AUTHORITY), dossier.Country,
			address, office.Name)
	}

	if !dossier.placed(applicant) {
		return nil, fmt.Errorf(
			"%s has no verified jurisdiction record in %s.\n"+
				"x/paymsg's delegated approval calls the perimeter check on the APPLICANT, and the perimeter "+
				"check refuses a target the chain cannot place before it looks at any grant — so %s's grant "+
				"does not help here. Somebody has to have recorded where %s is first, and for the first "+
				"institutions in a country that can only be the foundation:\n"+
				"  ceremony country seed --dossier <file> --proposer <custodian> --account %s",
			applicant, dossier.Country, office.Name, applicant, applicant)
	}

	cdc := enrolmentCodec()
	encoded, err := cdc.MarshalInterfaceJSON(&paymsgtypes.MsgApproveParticipant{
		Authority:   address,
		Participant: strings.TrimSpace(applicant),
		Approve:     approve,
	})
	if err != nil {
		return nil, err
	}

	verb := "Admit"
	if !approve {
		verb = "Reject"
	}
	title := fmt.Sprintf("%s %s", verb, applicant)
	summary := fmt.Sprintf(
		"%s %s to the payments rail in %s, decided by %s under the %s it holds there.",
		verb, applicant, dossier.Country, office.Name,
		aliastypes.RoleName(aliastypes.ROLE_PAYMENTS_AUTHORITY))
	if err := requireMetadataLength("title", title); err != nil {
		return nil, err
	}
	if len(summary) > maxMetadataLen {
		summary = summary[:maxMetadataLen]
	}

	return json.MarshalIndent(proposalDocument{
		GroupPolicyAddress: address,
		Messages:           []json.RawMessage{encoded},
		Metadata:           "",
		Proposers:          []string{strings.TrimSpace(proposer)},
		Title:              title,
		Summary:            summary,
	}, "", "  ")
}

// placed reports whether an account has a verified jurisdiction record in the
// dossier's country.
func (d *countryDossier) placed(account string) bool {
	address := strings.TrimSpace(account)
	for _, seed := range d.Seeded {
		if seed.Account == address && seed.Country == d.Country {
			return true
		}
	}
	for _, office := range d.Offices {
		if office.Placed != nil && office.Placed.Account == address && office.Placed.Country == d.Country {
			return true
		}
	}
	return false
}
