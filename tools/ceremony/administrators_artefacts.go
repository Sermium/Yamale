package main

// What the appointment ceremony hands an operator: a transaction to create the
// group, and the governance proposal that appoints it.
//
// Every message is built from the chain's own Go types and marshalled through the
// proto codec, never assembled as hand-written JSON — the same rule
// country_artefacts.go follows, and here it earns its keep twice over. A map
// literal with an "@type" in it would keep producing a decodable document after a
// field was renamed in the proto, with a zero where the value used to be. The
// message this file composes replaces every parameter of x/alias at once.
//
// Nothing here signs anything.

import (
	"encoding/json"
	"fmt"
	"sort"
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
// of this ceremony existing separately from the country enrolment. x/alias's
// UpdateParams is authority-gated to the governance module account; the
// foundation's 3-of-5 cannot sign it. So the decision belongs to the voting set.
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
// # The trap it absorbs
//
// MsgUpdateParams carries a Params message, not a field mask, so setting it
// REPLACES THE WHOLE OBJECT. "Appoint one administrator" is really "read the
// current parameters, add one address, and resubmit every parameter", and the
// failure mode is not an error: it is a proposal that passes and quietly drops the
// administrators already appointed, or resets payload_length to a default nobody
// voted for. Nothing on the chain catches it. Params.Validate() bounds the length
// and refuses duplicates, and a shorter list than before is a perfectly valid
// list.
//
// So the current parameters are REQUIRED, read off the chain, and read in full.
// There is no path here that composes a proposal from a default:
//
//   - No --alias-params, no proposal.
//   - A payload_length that reads as zero is a refusal, not a default of eight.
//     Proto3 cannot tell a zero from a field nobody filled in, so a zero means
//     the value is unknown — and a proposal that guessed it would reset the
//     identifier length of a chain that had raised it, showing no change anywhere
//     a person would look.
//   - Every administrator already named is carried across, and the summary states
//     the count before and after so a voter can see whether the list shrank.
func appointmentProposal(
	dossier administratorsDossier,
	current aliasParams,
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
	if err := requireAppointableCount(current.FoundationAdministrators, address); err != nil {
		return nil, err
	}
	if strings.TrimSpace(deposit) == "" {
		return nil, fmt.Errorf(
			"a deposit is required. A governance proposal submitted with less than the minimum is accepted, " +
				"sits in the deposit period, and never enters a vote — which looks from the outside exactly " +
				"like a proposal nobody got round to voting on. Read the minimum with " +
				"`blockchaind query gov params deposit`")
	}

	// Sorted, so the resulting list depends on the SET of administrators rather
	// than on the order appointments happened to be proposed in. Two proposals
	// appointing the same two addresses in opposite orders should not produce two
	// different Params objects, and a list a reader has to sort in their head is a
	// list that hides a change.
	after := append([]string(nil), current.FoundationAdministrators...)
	after = append(after, address)
	sort.Strings(after)

	params := aliastypes.Params{
		PayloadLength:            current.PayloadLength,
		FoundationAdministrators: after,
	}
	// Checked against the chain's own validator before anybody votes. It is the
	// same function the keeper runs, so a proposal that gets past this is a
	// proposal that fails at execution only for reasons this tool cannot see.
	//
	// A mutation pass found this line to be an EQUIVALENT MUTANT: deleting it
	// changes no test result, because aliasParams.validate() and
	// requireAppointableCount above already enforce every rule Params.Validate()
	// enforces today — the bounds, the duplicates, the cap, the empty entry. That
	// is not a reason to delete it. It is here for the rule the chain adds NEXT,
	// which this tool will not know about: the alternative to calling the chain's
	// own validator is a list of rules maintained in two places, and the failure of
	// that is a proposal this tool blessed and the keeper rejected after a vote.
	// No test can distinguish it until such a rule exists, and pretending
	// otherwise would mean writing a test that asserts nothing.
	if err := params.Validate(); err != nil {
		return nil, fmt.Errorf(
			"the parameters this proposal would set are ones the chain refuses: %w.\n"+
				"That is Params.Validate(), the same function the keeper runs — so this would have passed a vote "+
				"and then failed when it executed", err)
	}

	msg := &aliastypes.MsgUpdateParams{
		// The governance module account, and it is a required input rather than a
		// constant in this file. A tool with the authority compiled in would keep
		// composing confidently against a chain whose prefix or module name had
		// moved, and the proposal would pass its vote and then be refused at
		// execution by the authority check.
		Authority: current.Authority,
		Params:    params,
	}
	encoded, err := administratorsCodec().MarshalInterfaceJSON(msg)
	if err != nil {
		return nil, err
	}

	title := fmt.Sprintf("Appoint %s a foundation administrator", dossier.Ceremony)
	summary := appointmentSummary(dossier, address, current.FoundationAdministrators, after)
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
// because the name of the parameter does not say. "foundation_administrators"
// reads like a list of people with logins; what it confers is the ability to move
// any account on the chain out from under the authority investigating it.
//
// The count before and after is in here on purpose. It is the one number that
// reveals the failure this whole design exists to prevent: a proposal composed
// from a stale read of the parameters shows a list that SHRANK, and the diff
// nobody looked at would not have said so.
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
func appointmentSummary(dossier administratorsDossier, address string, before, after []string) string {
	// The two things that must always be present. The power, because the parameter
	// name gives no hint of it; the counts, because a list that SHRANK is the only
	// visible evidence of a proposal composed from a stale read.
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
		"Appoints %s, a %d-of-%d x/group policy, a foundation administrator on x/alias, taking the list from "+
			"%d to %d. It may then correct the country recorded against ANY account — which moves that account "+
			"out from under the authority investigating it, and retires and reissues its identifier — and may "+
			"hold an identifier with no country at all, carrying the reserved %s code. payload_length is "+
			"unchanged by this proposal.",
		address, dossier.Threshold, len(dossier.Members), len(before), len(after),
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

// aliasParams is x/alias's parameters as this tool needs them, plus the authority.
//
// A type of its own rather than aliastypes.Params, because it carries one thing
// the chain's type does not: the authority the message has to name. Bundling them
// means there is one input to the composing function and one place that can be
// incomplete.
type aliasParams struct {
	PayloadLength            uint32
	FoundationAdministrators []string
	Authority                string
}

// validate refuses parameters this tool cannot safely resubmit.
//
// The payload_length rule is the one worth restating. It is not defaulted, and
// zero is refused rather than replaced with the chain's default of eight, because
// proto3 cannot tell a zero from a field nobody filled in — so a zero means the
// value is UNKNOWN. A proposal composed from a guess would reset the identifier
// length of a chain that had raised it, and the change would appear nowhere: the
// proposal would read as an appointment and would also silently be a
// re-parameterisation.
func (p aliasParams) validate() error {
	if p.PayloadLength == 0 {
		return fmt.Errorf(
			"payload_length came back as zero or absent, so this tool does not know x/alias's current " +
				"identifier length and will not guess it. Proto3 cannot tell a zero from a field nobody filled " +
				"in, and the chain refuses a zero anyway — so this is not the chain's real value. " +
				"MsgUpdateParams replaces EVERY parameter at once, so a proposal composed from a guess would " +
				"quietly re-parameterise the chain while reading as an appointment.\n" +
				"  blockchaind query alias params -o json > alias-params.json")
	}
	if p.PayloadLength < aliastypes.MinPayloadLen || p.PayloadLength > aliastypes.MaxPayloadLen {
		return fmt.Errorf(
			"payload_length reads as %d and the chain accepts %d to %d. A value the chain would refuse cannot "+
				"be resubmitted, and this tool will not quietly correct it: if the chain really holds that, "+
				"something is wrong that this proposal would hide",
			p.PayloadLength, aliastypes.MinPayloadLen, aliastypes.MaxPayloadLen)
	}
	if strings.TrimSpace(p.Authority) == "" {
		return fmt.Errorf(
			"the governance module account is required. x/alias refuses MsgUpdateParams from any other signer, " +
				"so a proposal naming the wrong one would pass its vote and then be refused at execution.\n" +
				"  blockchaind query auth module-account gov -o json > gov-account.json")
	}
	if err := requireAccountAddress("authority", strings.TrimSpace(p.Authority)); err != nil {
		return err
	}
	for _, administrator := range p.FoundationAdministrators {
		if strings.TrimSpace(administrator) == "" {
			return fmt.Errorf(
				"the administrator list read off the chain contains an empty entry, which the chain would not " +
					"have accepted. Do not resubmit it")
		}
	}
	return nil
}

// readAliasParams reads the parameters and the authority out of two queried files.
//
// Two files rather than one because they come from two queries, and it is worth
// naming what would happen with a default instead. `query auth module-account gov`
// is 401-gated behind some of this chain's REST front-ends, so the tempting
// shortcut is to fall back to the well-known derivation. This does not: it is one
// address, it is checked, and a tool that guessed it would compose a proposal that
// passed and then did nothing.
func readAliasParamsFiles(paramsPath, authorityPath string) (aliasParams, error) {
	var response aliasParamsResponse
	if err := readJSONFile(paramsPath, &response); err != nil {
		return aliasParams{}, err
	}

	var account moduleAccountResponse
	if err := readJSONFile(authorityPath, &account); err != nil {
		return aliasParams{}, err
	}
	address, name := account.address()
	if name != "gov" {
		return aliasParams{}, fmt.Errorf(
			"%s describes the module account %q, not \"gov\". x/alias's authority is the governance module "+
				"account and nothing else, so a proposal naming that address would pass its vote and then be "+
				"refused at execution", authorityPath, name)
	}

	return aliasParams{
		PayloadLength:            uint32(response.Params.PayloadLength),
		FoundationAdministrators: append([]string(nil), response.Params.FoundationAdministrators...),
		Authority:                address,
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
