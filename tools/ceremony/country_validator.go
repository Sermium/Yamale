package main

// Placing a validator's operator account in a jurisdiction.
//
// This is the one place the enrolment touches the other registry that holds
// "which country does this belong to", and it is deliberately narrow.
//
// x/validatorgov collects a jurisdiction when a validator applies: the operator
// declares it, signs for it, and the concentration ceilings group by it. x/alias
// holds a different fact — the country an approved participant recorded after
// doing the know-your-customer work, or a foundation administrator corrected. One
// is a claim under the claimant's own key; the other is a finding by somebody who
// looked. They are reconciled and merged nowhere, because merging either direction
// destroys the thing that makes it worth having: overwriting the declaration
// throws away the signature that makes a false declaration an offence, and writing
// the registry from the declaration makes the registry a place where accounts
// place themselves.
//
// So this command does not copy one into the other. It places a validator's
// operator account the way any other account is placed — by the foundation, as an
// administrator, because a validator that banks nowhere has no participant to do
// it — and it refuses to place one in a country the validator did not declare.
//
// That refusal is the whole design. Placing a validator in a country it did not
// declare would MANUFACTURE a disagreement, from the enrolment, with the
// foundation's signature on it. If the two really do differ, that is a finding for
// a person: either the validator declared a country it does not answer to, or
// somebody is about to record the wrong one. Neither is a thing a ceremony should
// resolve by picking.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	aliastypes "yamale/blockchain/x/alias/types"
)

// The agreement states, as the reconciliation query names them.
//
// Compared as strings because that is what protobuf JSON writes for an enum, and
// the numeric form is accepted too because a gateway configured with
// `enums_as_ints` writes that instead. The zero value is refused rather than
// mapped: it is reserved as unspecified on the chain, so a row carrying it is a
// row this tool does not understand and must not act on.
const (
	agreementUnspecified = "JURISDICTION_AGREEMENT_UNSPECIFIED"
	agreementAgree       = "JURISDICTION_AGREEMENT_AGREE"
	agreementDisagree    = "JURISDICTION_AGREEMENT_DISAGREE"
	agreementUnrecorded  = "JURISDICTION_AGREEMENT_UNRECORDED"
)

// agreementNames maps the numeric form onto the names, for a gateway that renders
// enums as integers.
var agreementNames = map[int]string{
	0: agreementUnspecified,
	1: agreementAgree,
	2: agreementDisagree,
	3: agreementUnrecorded,
}

// reconciliationResponse is the part of `query validatorgov
// jurisdiction-reconciliation` this tool reads.
type reconciliationResponse struct {
	Records []struct {
		Candidate            string          `json:"candidate"`
		DeclaredJurisdiction string          `json:"declared_jurisdiction"`
		RecordedJurisdiction string          `json:"recorded_jurisdiction"`
		RecordedBy           string          `json:"recorded_by"`
		RecordedAtHeight     flexInt64       `json:"recorded_at_height"`
		Agreement            json.RawMessage `json:"agreement"`
	} `json:"records"`
	AgreeCount      flexUint64 `json:"agree_count"`
	DisagreeCount   flexUint64 `json:"disagree_count"`
	UnrecordedCount flexUint64 `json:"unrecorded_count"`
}

// agreementOf reads one row's finding, in either form the enum can arrive in.
func agreementOf(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		// Absent, which protobuf JSON does for the zero value. That is the
		// reserved unspecified state, and it is returned as such rather than
		// guessed at.
		return agreementUnspecified, nil
	}
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		return name, nil
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		name, known := agreementNames[number]
		if !known {
			return "", fmt.Errorf("%d is not an agreement state this tool knows", number)
		}
		return name, nil
	}
	return "", fmt.Errorf("%s is not a finding this tool can read", string(raw))
}

// validatorPlacement is what the reconciliation says about one candidate, once
// this tool has decided it is safe to act on.
type validatorPlacement struct {
	Candidate string
	Declared  string
	Recorded  string
	Agreement string
}

// findValidator locates one candidate in a reconciliation response and refuses
// every state in which placing it would be wrong.
//
// Five refusals, and the third is the one this command exists for:
//
//  1. the response describes no validators at all — an empty answer is what a
//     chain with no approved validators returns and also what a query pointed at
//     the wrong node returns, so it is reported rather than treated as "nothing to
//     do";
//  2. the candidate is not an approved validator. An application is not an
//     approval, and placing a candidate nobody admitted would put a jurisdiction
//     record against an account the validator set never accepted;
//  3. the validator declared a DIFFERENT country from this enrolment's. Refused,
//     because placing it here would manufacture the disagreement the
//     reconciliation query exists to reveal, with the foundation's signature on
//     it;
//  4. the finding is the reserved unspecified state, which a correct chain never
//     produces;
//  5. it is already recorded and agrees, in which case there is nothing to do and
//     saying so is more useful than composing a proposal that changes nothing.
func findValidator(response reconciliationResponse, candidate, country string) (validatorPlacement, error) {
	if len(response.Records) == 0 {
		return validatorPlacement{}, errors.New(
			"that reconciliation names no validators. On a chain with approved validators it lists every one of " +
				"them, agreements included, so an empty answer is either a chain that has admitted nobody through " +
				"x/validatorgov or a query that went somewhere else")
	}

	wanted := strings.TrimSpace(candidate)
	for _, row := range response.Records {
		if strings.TrimSpace(row.Candidate) != wanted {
			continue
		}

		finding, err := agreementOf(row.Agreement)
		if err != nil {
			return validatorPlacement{}, fmt.Errorf("%s: %w", wanted, err)
		}

		declared := aliastypes.NormaliseCountry(row.DeclaredJurisdiction)
		if declared != country {
			return validatorPlacement{}, fmt.Errorf(
				"%s declared %s and this enrolment is for %s.\n"+
					"Placing it in %s would create a disagreement between the two registries rather than resolve "+
					"one, and it would do so from the enrolment, with the foundation's signature on it. The "+
					"declaration is signed by the operator; if it names the wrong country then that is what has to "+
					"be corrected, through x/validatorgov, by whoever signed it",
				wanted, declared, country, country)
		}

		switch finding {
		case agreementAgree:
			return validatorPlacement{}, fmt.Errorf(
				"%s is already recorded in %s, by %s, and it agrees with the declaration. There is nothing to place",
				wanted, aliastypes.NormaliseCountry(row.RecordedJurisdiction), row.RecordedBy)
		case agreementDisagree:
			// Not a refusal. A validator recorded in the wrong country is exactly
			// what wants correcting, and a foundation administrator is who may
			// correct a jurisdiction that is already recorded. But it is said out
			// loud, because this is no longer placing an unplaced account — it is
			// overwriting somebody's finding, and that belongs on the record.
			return validatorPlacement{
				Candidate: wanted, Declared: declared,
				Recorded: aliastypes.NormaliseCountry(row.RecordedJurisdiction), Agreement: finding,
			}, nil
		case agreementUnrecorded:
			return validatorPlacement{Candidate: wanted, Declared: declared, Agreement: finding}, nil
		default:
			return validatorPlacement{}, fmt.Errorf(
				"the reconciliation reports %s for %s, which is the reserved unspecified state. A correct chain "+
					"never produces it, so this is not an answer to act on", finding, wanted)
		}
	}

	names := make([]string, 0, len(response.Records))
	for _, row := range response.Records {
		names = append(names, row.Candidate)
	}
	return validatorPlacement{}, fmt.Errorf(
		"%s is not an approved validator on this chain. The reconciliation lists: %s.\n"+
			"An application is not an approval, and a jurisdiction record against an account the validator set "+
			"never admitted would place something nobody has accepted",
		wanted, strings.Join(names, ", "))
}

// validatorPlacementProposal is the foundation proposal that places one
// validator's operator account.
//
// A foundation proposal, and not because a validator is special: it is the same
// reason the first applicant institutions need one. A validator's operator account
// is nobody's payment customer, so no approved participant acts for it, and
// MsgSetJurisdiction accepts only that participant, a foundation administrator or
// governance. Nobody may declare their own.
func validatorPlacementProposal(dossier countryDossier, proposer string, placement validatorPlacement) ([]byte, error) {
	if err := requireEnrolmentCountry(dossier.Country); err != nil {
		return nil, err
	}
	if err := requireAccountAddress("proposer", proposer); err != nil {
		return nil, err
	}
	if err := requireProposerIsCustodian(dossier, proposer); err != nil {
		return nil, err
	}
	if err := requireAccountAddress("candidate", placement.Candidate); err != nil {
		return nil, err
	}

	cdc := enrolmentCodec()
	encoded, err := cdc.MarshalInterfaceJSON(&aliastypes.MsgSetJurisdiction{
		Recorder: dossier.Foundation,
		Account:  placement.Candidate,
		Country:  dossier.Country,
	})
	if err != nil {
		return nil, err
	}

	title := fmt.Sprintf("Place validator %s in %s", short(placement.Candidate), dossier.Country)
	summary := fmt.Sprintf(
		"Records the operator account of an approved validator in %s, which is the country it declared. "+
			"Its declaration stays where it is; this is the other registry, written by an administrator because "+
			"a validator banks nowhere and no participant acts for it.", dossier.Country)
	if placement.Agreement == agreementDisagree {
		summary = fmt.Sprintf(
			"CORRECTS the recorded country of an approved validator from %s to %s, which is the country it "+
				"declared. This overwrites an existing finding, so read the reconciliation before voting.",
			placement.Recorded, dossier.Country)
	}
	if err := requireMetadataLength("title", title); err != nil {
		return nil, err
	}
	if len(summary) > maxSummaryLen {
		summary = summary[:maxSummaryLen]
	}

	return json.MarshalIndent(proposalDocument{
		GroupPolicyAddress: dossier.Foundation,
		Messages:           []json.RawMessage{encoded},
		Metadata:           "",
		Proposers:          []string{strings.TrimSpace(proposer)},
		Title:              title,
		Summary:            summary,
	}, "", "  ")
}

// short abbreviates an address for a title that x/group caps at 255 bytes.
func short(address string) string {
	if len(address) <= 16 {
		return address
	}
	return address[:10] + "…" + address[len(address)-6:]
}

// runCountryValidator is `ceremony country validator`.
func runCountryValidator(args []string) error {
	flags := flag.NewFlagSet("country validator", flag.ExitOnError)
	dossierPath := dossierFlag(flags)
	proposer := flags.String("proposer", "", "the foundation custodian submitting the proposal")
	candidate := flags.String("candidate", "", "the validator's operator account, in its account form")
	reconciliation := flags.String("reconciliation", "",
		"`blockchaind query validatorgov jurisdiction-reconciliation -o json`")
	verified := flags.String("verified", "",
		"`blockchaind query alias jurisdiction <candidate> -o json`, to record that it landed")
	out := flags.String("out", ".", "directory for the proposal")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireDossier(*dossierPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*candidate) == "" {
		return errors.New("--candidate is required: the validator's operator account, in its yml1… account form")
	}

	if strings.TrimSpace(*verified) != "" {
		placed, err := verifyPlacement(*candidate, dossier.Country, *verified, time.Now())
		if err != nil {
			return err
		}
		replaced := false
		for i := range dossier.Seeded {
			if dossier.Seeded[i].Account == placed.Account {
				dossier.Seeded[i] = placed
				replaced = true
			}
		}
		if !replaced {
			dossier.Seeded = append(dossier.Seeded, placed)
		}
		if err := writeDossier(*dossierPath, dossier); err != nil {
			return err
		}
		c := stdConsole()
		c.printf("  PLACED    %s in %s, recorded by %s\n\n", placed.Account, placed.Country, placed.RecordedBy)
		c.println("The two registries now agree about this validator. Check that from the other end,")
		c.println("which is the side a supervisor reads:")
		c.println()
		c.println("  blockchaind query validatorgov jurisdiction-reconciliation")
		return nil
	}

	if strings.TrimSpace(*reconciliation) == "" {
		return fmt.Errorf(
			"--reconciliation is required. This command will not place a validator without reading what the two " +
				"registries currently say about it, because placing one in a country it did not declare would " +
				"manufacture the disagreement that query exists to reveal:\n" +
				"  blockchaind query validatorgov jurisdiction-reconciliation -o json > reconciliation.json")
	}
	if strings.TrimSpace(*proposer) == "" {
		return errors.New("--proposer is required: a group proposal is submitted by one of the group's members")
	}

	var response reconciliationResponse
	if err := readJSONFile(*reconciliation, &response); err != nil {
		return err
	}
	placement, err := findValidator(response, *candidate, dossier.Country)
	if err != nil {
		return err
	}

	document, err := validatorPlacementProposal(dossier, *proposer, placement)
	if err != nil {
		return err
	}
	path := filepath.Join(*out, fmt.Sprintf("validator-%s-proposal.json", slug(placement.Candidate)))
	if err := os.WriteFile(filepath.Clean(path), append(document, '\n'), 0o644); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s: place a validator ===\n\n", dossier.Country)
	c.printf("  candidate  %s\n", placement.Candidate)
	c.printf("  declared   %s   (by the operator, under its own key)\n", placement.Declared)
	switch placement.Agreement {
	case agreementUnrecorded:
		c.printf("  recorded   nothing yet\n")
	case agreementDisagree:
		c.printf("  recorded   %s   <-- this proposal CORRECTS it\n", placement.Recorded)
	}
	c.println()
	c.printf("Wrote %s.\n\n", path)
	c.println("Two registries, still two. This writes the x/alias record and leaves the")
	c.println("validator's declaration exactly where it is: the declaration is a claim signed")
	c.println("by the operator, and this is a finding by an administrator. Nothing here copies")
	c.println("one into the other, and the reconciliation query is what shows them side by side.")
	c.println()
	if placement.Agreement == agreementDisagree {
		c.println("Note that this OVERWRITES a country somebody already recorded. Read the")
		c.println("reconciliation and find out who recorded it and why before three custodians")
		c.println("vote — a validator recorded in the wrong country and a validator declaring the")
		c.println("wrong one look identical from here.")
		c.println()
	}
	c.printf("  blockchaind tx group submit-proposal %s --from <custodian> --chain-id %s\n", path, dossier.ChainID)
	c.println()
	c.printf("  blockchaind query alias jurisdiction %s -o json > placed.json\n", placement.Candidate)
	c.printf("  ceremony country validator --dossier %s --candidate %s --verified placed.json\n",
		*dossierPath, placement.Candidate)
	return nil
}
