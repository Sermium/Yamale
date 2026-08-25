package main

// What the appointment ceremony hands an operator: a transaction to create the
// group, and the governance proposal that appoints it.
//
// Every message is built from the chain's own Go types and marshalled through the
// proto codec, never assembled as hand-written JSON — the same rule
// country_artefacts.go follows. A map literal with an "@type" in it would keep
// producing a decodable document after a field was renamed in the proto, with a
// zero where the value used to be, and the field that would go quietest is the
// scope: a grant decoding to an empty jurisdiction is a grant the chain refuses,
// but a grant decoding to a country nobody typed is a grant it accepts.
//
// Nothing here signs anything.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"

	aliastypes "yamale/blockchain/x/alias/types"
)

// administratorsCodec marshals the one message this ceremony proposes.
func administratorsCodec() codec.Codec {
	registry := codectypes.NewInterfaceRegistry()
	aliastypes.RegisterInterfaces(registry)
	return codec.NewProtoCodec(registry)
}

// govProposalDocument is the JSON `blockchaind tx gov submit-proposal` reads.
//
// A governance proposal, not an x/group one, and the difference is the whole point
// of this ceremony existing separately from the country enrolment. A grant at the
// chain-wide scope is refused from every signer but the governance module account;
// the foundation's 3-of-5 can grant a role inside a country and cannot grant this
// one. So the decision belongs to the voting set.
//
// `expedited` is deliberately absent rather than false. This is the appointment of
// the account that can move any customer on the chain out from under the authority
// investigating them, and the one thing an expedited proposal buys is less time
// for somebody to notice. If a chain's ordinary voting period is genuinely too
// long for an appointment that has waited for a key ceremony, the answer is to
// change the voting period, in public, rather than to shorten this one vote.
type govProposalDocument struct {
	Messages []json.RawMessage `json:"messages"`
	Metadata string            `json:"metadata"`
	Deposit  string            `json:"deposit"`
	Title    string            `json:"title"`
	Summary  string            `json:"summary"`
}

// appointmentProposal is the one act that gives this group the power.
//
// # The trap that used to be here, and why its absence is the point
//
// The appointment used to be a MsgUpdateParams setting
// alias.params.foundation_administrators. MsgUpdateParams carries a Params
// message, not a field mask, so setting it REPLACED THE WHOLE OBJECT: "appoint
// one administrator" was really "read the current parameters, add one address,
// and resubmit every parameter". The failure mode was not an error. It was a
// proposal that passed and quietly dropped the administrators already appointed,
// or reset payload_length to a default nobody voted for, and nothing on the chain
// caught it, because a shorter list is a perfectly valid list. Everything in this
// file existed to survive that: the current parameters were REQUIRED and read in
// full, every existing administrator was carried across, the resulting list was
// sorted so two orderings could not produce two objects, and the summary stated
// the count before and after because a list that had SHRUNK was the only visible
// evidence of a stale read.
//
// None of it is here any more, and that is the single best thing about the change.
// MsgGrantRole names ONE holder and is additive. It cannot drop an administrator
// it does not mention. It cannot re-parameterise the chain while reading as an
// appointment. A proposal composed from a view of the chain that went stale during
// the voting period is merely out of date, where before it was destructive.
//
// What is still read off the chain is who already holds the role, and for exactly
// one reason: the cap. See requireAppointableCount. Nothing about the message this
// function composes depends on it.
//
// # Why the grant records no required shape
//
// A required shape is the M-of-N the chain holds a grant's holder to for as long
// as the grant exists, and country_artefacts.go writes one onto every grant it
// composes. It can do that because a country's offices declare their minimum in
// the enrolment config, before the day, by the same person who writes down which
// offices exist — see officeMinimum, and the reason it is decided in advance: a
// requirement captured from the assembled group is no requirement at all, because
// it ratifies a one-of-one as readily as a three-of-five.
//
// This ceremony's config has no such field. The only numbers to hand are the
// dossier's own threshold and member count, and those are read out of the group
// file the custodians signed — which is precisely the captured-from-the-group
// number that argument refuses. Writing 3-of-5 into required_shape because the
// group that turned up was a 3-of-5 would put a requirement on the chain that
// nobody decided, and it would read on the signed record as though somebody had.
//
// So the grant is made with none, which is what every grant made before
// required_shape existed carries and what the chain reads as "no requirement"
// rather than as a requirement of zero. If administrators should be held to a
// shape, the place to add it is the appointment config, agreed before the
// ceremony; the grant can then be re-made to carry it, because GrantRole counts
// the cap excluding the holder so that a grant can be amended, and
// assertShapeNotReduced stops such an amendment ever lowering the bar.
func appointmentProposal(
	dossier administratorsDossier,
	current standingGrants,
	deposit string,
) ([]byte, error) {
	address, err := dossier.requireConfirmedGroup()
	if err != nil {
		return nil, err
	}
	if err := requireAccountAddress("group policy", address); err != nil {
		return nil, err
	}
	if err := current.validate(); err != nil {
		return nil, err
	}
	if err := requireAppointableCount(current.Administrators, address); err != nil {
		return nil, err
	}
	if strings.TrimSpace(deposit) == "" {
		return nil, fmt.Errorf(
			"a deposit is required. A governance proposal submitted with less than the minimum is accepted, " +
				"sits in the deposit period, and never enters a vote — which looks from the outside exactly " +
				"like a proposal nobody got round to voting on. Read the minimum with " +
				"`blockchaind query gov params deposit`")
	}

	msg := &aliastypes.MsgGrantRole{
		// The governance module account, and it is a required input rather than a
		// constant in this file. A tool with the authority compiled in would keep
		// composing confidently against a chain whose prefix or module name had
		// moved, and the proposal would pass its vote and then be refused at
		// execution by the authority check.
		Authority: current.Authority,
		Holder:    address,
		Role:      aliastypes.ROLE_FOUNDATION_ADMINISTRATOR,
		// aliastypes.ChainWide rather than the literal "*", so that the marker this
		// composes and the marker the keeper matches on are one value. They are the
		// same byte today; the day one of them moves, a literal here would compose a
		// proposal naming a scope ValidGrantScope refuses — after the vote. The test
		// pins the literal, which is the other half of the same arrangement: the
		// constant may not change its meaning without something failing.
		Jurisdiction: aliastypes.ChainWide,
	}

	// Checked against the chain's own validator before anybody votes, over the
	// record the grant would WRITE rather than over the message. RoleGrant.Validate
	// is the function both GrantRole and genesis validation run, so a proposal that
	// gets past this is one that fails at execution only for reasons this tool
	// cannot see.
	//
	// The version of this that stood here called Params.Validate() and a mutation
	// pass found it to be an EQUIVALENT MUTANT: every rule it enforced was already
	// enforced by the checks above, so deleting the line changed no test result.
	// The reasoning for keeping it survives the change of message intact. It is
	// here for the rule the chain adds NEXT, which this tool will not know about:
	// the alternative to calling the chain's own validator is a list of rules
	// maintained in two places, and the failure of that is a proposal this tool
	// blessed and the keeper rejected after a vote. No test can distinguish it
	// until such a rule exists, and pretending otherwise would mean writing a test
	// that asserts nothing.
	grant := aliastypes.RoleGrant{
		Holder:        msg.Holder,
		Role:          msg.Role,
		Jurisdiction:  msg.Jurisdiction,
		GrantedBy:     msg.Authority,
		RequiredShape: msg.RequiredShape,
	}
	if err := grant.Validate(); err != nil {
		return nil, fmt.Errorf(
			"the grant this proposal would make is one the chain refuses: %w.\n"+
				"That is RoleGrant.Validate(), the same function the keeper and genesis validation run — so this "+
				"would have passed a vote and then failed when it executed", err)
	}

	encoded, err := administratorsCodec().MarshalInterfaceJSON(msg)
	if err != nil {
		return nil, err
	}

	title := fmt.Sprintf("Appoint %s a foundation administrator", dossier.Ceremony)
	summary := appointmentSummary(dossier, address, current.Administrators)
	// Refused rather than truncated, unlike the summary, and the difference is who
	// chose the text. The summary is assembled by this tool, so this tool can
	// decide what to drop. The title is a sentence built around a name a person
	// picked, and silently shortening somebody's ceremony name in the field the
	// whole voting set reads first is not a decision to make on their behalf.
	if len(title) > maxMetadataLen {
		return nil, fmt.Errorf(
			"the proposal's title would be %d bytes and x/gov accepts %d. Shorten the ceremony name in the "+
				"dossier: it is the part of this string a person chose, and it is also recorded on chain inside "+
				"the group metadata, which has the same limit",
			len(title), maxMetadataLen)
	}

	return json.MarshalIndent(govProposalDocument{
		Messages: []json.RawMessage{encoded},
		Metadata: "",
		Deposit:  strings.TrimSpace(deposit),
		Title:    title,
		Summary:  summary,
	}, "", "  ")
}

// appointmentSummary states the whole proposal in words.
//
// What a voter is being asked to agree to, and specifically what the power IS,
// because the name of the role does not say. "Foundation administrator" reads like
// somebody with a login; what it confers is the ability to move any account on the
// chain out from under the authority investigating it.
//
// The count before and after is still here, and it is here for a different reason
// than it used to be. It used to be the one number that revealed a proposal
// composed from a stale read of the parameters, because such a proposal showed a
// list that had SHRUNK. A grant cannot shrink anything. What the count says now is
// how many accounts stand outside every national perimeter and how close this
// proposal takes the chain to the cap of eight — which is the fact the cap exists
// to keep visible, and which a voter reading one message about one holder has no
// other way to see.
// Assembled in priority order rather than written as one sentence and truncated,
// and that ordering is the whole of the function. x/group and x/gov cap a summary
// at 255 bytes; a foundation administrator's address is 62 of them and a ceremony
// name is however long somebody typed. Composed as one string, the first draft of
// this put the address and the ceremony name first — and a test found that the
// sentence saying WHAT THE POWER IS was the part that fell off the end.
//
// So the fixed text comes first, because it is the part a voter cannot work out
// for themselves, and the variable parts are appended only while they fit. What
// gets lost is a name and a reason that are both in the message, the dossier and
// the signed record; what survives is the description of the power and the count
// that reveals a shrinking list.
func appointmentSummary(dossier administratorsDossier, address string, before []string) string {
	// The two things that must always be present. The power, because the role's
	// name gives no hint of it; the counts, because one message about one holder
	// says nothing about how many accounts already stand outside every perimeter.
	//
	// The full description, because there is room for it. This was first written
	// against a 255-byte cap and had to have the sentence about the reserved code
	// cut out to fit — and the cap was wrong: both x/gov and x/group check the
	// summary against 40*MaxMetadataLen, so it is 10,200 bytes. See maxSummaryLen.
	//
	// The priority ordering below survives that correction rather than being made
	// redundant by it. The lead's reason is written by a person and nothing bounds
	// it, so something can still have to be dropped — and the thing dropped must
	// not be the description of the power.
	core := fmt.Sprintf(
		"Grants %s, a %d-of-%d x/group policy, %s at the %q scope, taking the number of accounts holding it "+
			"from %d to %d of a maximum of %d. It may then correct the country recorded against ANY account — "+
			"which moves that account out from under the authority investigating it, and retires and reissues "+
			"its identifier — and may hold an identifier with no country at all, carrying the reserved %s code. "+
			"This proposal changes nothing else: it names one holder and adds one grant, and no account that "+
			"holds the role today loses it.",
		address, dossier.Threshold, len(dossier.Members),
		aliastypes.RoleName(aliastypes.ROLE_FOUNDATION_ADMINISTRATOR), aliastypes.ChainWide,
		len(before), len(before)+1, aliastypes.MaxFoundationAdministrators,
		aliastypes.FoundationCountry)

	summary := core
	if reason := strings.TrimSpace(dossier.Reason); reason != "" {
		if candidate := summary + " " + reason; len(candidate) <= maxSummaryLen {
			summary = candidate
		} else {
			const marker = " […see the appointment record]"
			room := maxSummaryLen - len(summary) - 1 - len(marker)
			if room > 16 {
				summary = summary + " " + reason[:room] + marker
			}
		}
	}
	return summary
}

// standingGrants is what the chain already says about the role, plus the
// authority a proposal has to name.
//
// It is what aliasParams used to be, reshaped by the message changing. A proposal
// no longer needs to know anything about x/alias's PARAMETERS — it carries none of
// them — so the parameter fields are gone and what remains is the set of accounts
// already holding the role. One type rather than two arguments, for the reason the
// old one gave: one input to the composing function is one place that can be
// incomplete.
type standingGrants struct {
	// Administrators is every account already holding
	// ROLE_FOUNDATION_ADMINISTRATOR at the chain-wide scope.
	//
	// Read for the cap and for the summary's count, and for nothing else. The
	// message this proposal carries does not depend on it, which is the whole
	// difference between this and the parameter list it replaced: a stale value
	// here makes the count in the summary wrong, where a stale parameter list made
	// the proposal itself destructive.
	Administrators []string

	// Authority is the governance module account.
	Authority string
}

// validate refuses a view of the chain this tool cannot compose against.
func (s standingGrants) validate() error {
	if strings.TrimSpace(s.Authority) == "" {
		return fmt.Errorf(
			"the governance module account is required. A grant at the chain-wide scope is refused from every " +
				"signer but governance — assertMayGrant refuses it before it has even read the constitution — " +
				"so a proposal naming any other authority would pass its vote and then be refused at execution.\n" +
				"  blockchaind query auth module-account gov -o json > gov-account.json")
	}
	if err := requireAccountAddress("authority", strings.TrimSpace(s.Authority)); err != nil {
		return err
	}
	for _, administrator := range s.Administrators {
		if strings.TrimSpace(administrator) == "" {
			return fmt.Errorf(
				"the chain-wide grants read off the chain include one with an empty holder, which the chain " +
					"would not have written. This is not an answer to count a cap against")
		}
	}
	return nil
}

// readStandingGrants reads the holders and the authority out of two queried files.
//
// Two files rather than one because they come from two queries, and it is worth
// naming what would happen with a default instead. `query auth module-account gov`
// is 401-gated behind some of this chain's REST front-ends, so the tempting
// shortcut is to fall back to the well-known derivation. This does not: it is one
// address, it is checked, and a tool that guessed it would compose a proposal that
// passed and then did nothing.
//
// The grants file is `query alias chain-wide-grants`, which takes no argument, and
// not `role-grants <holder>`, which takes one and renders the same shape. See
// chainWideGrantsOf: the scope is checked on every record so the second file's
// country grants cannot be counted as chain-wide ones, but a role-grants file for
// an account that holds nothing still reads as a chain with no administrators, and
// nothing here can tell those apart.
func readStandingGrants(grantsPath, authorityPath string) (standingGrants, error) {
	held, err := chainWideGrantsOf(grantsPath, aliastypes.ROLE_FOUNDATION_ADMINISTRATOR)
	if err != nil {
		return standingGrants{}, err
	}

	var account moduleAccountResponse
	if err := readJSONFile(authorityPath, &account); err != nil {
		return standingGrants{}, err
	}
	address, name := account.address()
	if name != "gov" {
		return standingGrants{}, fmt.Errorf(
			"%s describes the module account %q, not \"gov\". A chain-wide grant is governance's and nobody "+
				"else's, so a proposal naming that address would pass its vote and then be refused at execution",
			authorityPath, name)
	}

	holders := make([]string, 0, len(held))
	for _, grant := range held {
		holders = append(holders, grant.Holder)
	}
	return standingGrants{
		Administrators: holders,
		Authority:      address,
	}, nil
}

// moduleAccountResponse is the part of `query auth module-account gov` this reads.
//
// Both shapes, because the CLI wraps it in {"account":{"type":…,"value":{…}}} and
// the REST gateway renders it flatter. Checking both rather than one, because the
// failure of guessing is a tool that silently reads an empty address and then
// refuses with a message about the authority being missing when it is right there
// in the file.
type moduleAccountResponse struct {
	Account struct {
		Value struct {
			Address string `json:"address"`
			Name    string `json:"name"`
		} `json:"value"`
		BaseAccount struct {
			Address string `json:"address"`
		} `json:"base_account"`
		Name string `json:"name"`
	} `json:"account"`
}

func (m moduleAccountResponse) address() (string, string) {
	if addr := strings.TrimSpace(m.Account.Value.Address); addr != "" {
		return addr, strings.TrimSpace(m.Account.Value.Name)
	}
	return strings.TrimSpace(m.Account.BaseAccount.Address), strings.TrimSpace(m.Account.Name)
}
