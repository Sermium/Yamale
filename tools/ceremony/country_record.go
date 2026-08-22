package main

// The enrolment record: the document somebody signs to say a country was admitted
// and by whom.
//
// It is the sibling of the foundation ceremony's record and it answers the same
// question in a harder case. The foundation's record says "these five people hold
// the account every seized asset goes to, and here are their fingerprints" — one
// account, one group, one address to check. A country's record has to say who now
// holds the power to freeze accounts in Senegal, who granted it to them, and what
// on the chain a reader can check that against, three years from now, when the
// office has been reorganised and the people have moved on.
//
// So everything in it is either read out of the dossier — which is to say, out of
// the chain's own answers, verified — or typed by the scribe and clearly marked as
// having been typed. Nothing is inferred. In particular the addresses and the
// grants are not retyped: an enrolment record that carried a mistyped office
// address would be a document whose only purpose is to let somebody detect a
// mistyped office address.

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

// enrolmentRecordConfig is what the scribe fills in.
//
// Short, for the same reason recordConfig is short: everything that can be read
// from what the ceremony produced is read rather than asked for.
type enrolmentRecordConfig struct {
	Location     string        `json:"location"`
	StartedAt    string        `json:"started_at"`
	CompletedAt  string        `json:"completed_at"`
	Participants []participant `json:"participants"`
	// Custodians names the foundation custodians who voted for the enrolment.
	//
	// Typed rather than read, and this is the one place in the record where that
	// is a gap worth stating out loud. The dossier knows which foundation
	// approved the enrolment and does not know which three of its five custodians
	// voted — that fact is on the chain, in the proposal's votes, and this tool
	// has no way to read it. So the names here are a claim by whoever signs the
	// record, and the record says where to check it.
	Custodians []string `json:"custodians,omitempty"`
	// ProposalID is the foundation proposal that carried the enrolment, so a
	// reader can go and look at the votes themselves.
	ProposalID string   `json:"proposal_id,omitempty"`
	Notes      []string `json:"notes,omitempty"`
}

func runCountryRecord(args []string) error {
	flags := flag.NewFlagSet("country record", flag.ExitOnError)
	dossierPath := dossierFlag(flags)
	configPath := flags.String("config", "", "JSON file describing the enrolment; see the country enrolment guide")
	out := flags.String("out", ".", "directory for the rendered record")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireDossier(*dossierPath)
	if err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	var config enrolmentRecordConfig
	if err := readStrictJSON(*configPath, &config); err != nil {
		return err
	}

	markdown, err := renderEnrolmentRecord(dossier, config)
	if err != nil {
		return err
	}

	markdownPath := filepath.Join(*out, fmt.Sprintf("enrolment-record-%s.md", dossier.Country))
	if err := os.WriteFile(filepath.Clean(markdownPath), []byte(markdown), 0o644); err != nil {
		return err
	}

	full := struct {
		enrolmentRecordConfig
		Enrolment countryDossier `json:"enrolment"`
	}{config, dossier}
	encoded, err := json.MarshalIndent(full, "", "  ")
	if err != nil {
		return err
	}
	jsonPath := filepath.Join(*out, fmt.Sprintf("enrolment-record-%s.json", dossier.Country))
	if err := os.WriteFile(filepath.Clean(jsonPath), append(encoded, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf("Wrote %s and %s.\n", markdownPath, jsonPath)
	fmt.Println("Print the Markdown. Everybody present signs the paper copy before leaving.")
	return nil
}

// renderEnrolmentRecord produces the document the room signs.
func renderEnrolmentRecord(dossier countryDossier, config enrolmentRecordConfig) (string, error) {
	// An enrolment nobody can check is an enrolment nobody can hold anyone to, so
	// a record is refused rather than rendered when the thing it would attest to
	// has not been verified against the chain.
	unconfirmed := []string{}
	ungranted := []string{}
	for _, office := range dossier.Offices {
		if office.OnChain == nil {
			unconfirmed = append(unconfirmed, office.Name)
			continue
		}
		roles, err := rolesOf(office.Roles)
		if err != nil {
			return "", err
		}
		for _, role := range roles {
			if !office.grantVerified(role, dossier.Country) {
				ungranted = append(ungranted, fmt.Sprintf("%s for %s", aliastypes.RoleName(role), office.Name))
			}
		}
	}
	if len(unconfirmed) > 0 {
		return "", fmt.Errorf(
			"%s has no confirmed group address, so this record would attest to an office that may not exist. "+
				"Confirm it first", strings.Join(unconfirmed, " and "))
	}
	if len(ungranted) > 0 {
		return "", fmt.Errorf(
			"these grants have not been read back off the chain: %s.\n"+
				"A record signed now would state that a country's authorities hold powers nobody has checked "+
				"they hold — and an accepted proposal that failed in execution looks exactly like this",
			strings.Join(ungranted, ", "))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Country enrolment record — %s\n\n", dossier.Country)
	fmt.Fprintf(&b, "**Enrolment:** %s  \n", dossier.Ceremony)
	fmt.Fprintf(&b, "**Chain:** `%s`  \n", dossier.ChainID)
	fmt.Fprintf(&b, "**Country:** `%s`  \n", dossier.Country)
	fmt.Fprintf(&b, "**Location:** %s  \n", config.Location)
	fmt.Fprintf(&b, "**Started:** %s  \n", config.StartedAt)
	fmt.Fprintf(&b, "**Completed:** %s  \n", config.CompletedAt)
	fmt.Fprintf(&b, "**Rendered:** %s\n\n", time.Now().UTC().Format(time.RFC3339))

	b.WriteString("Everything in this document is public. It contains no key material and no\n")
	b.WriteString("recovery phrase, and it is meant to be published.\n\n")

	b.WriteString("## What was admitted\n\n")
	fmt.Fprintf(&b, "`%s` now has authorities on this chain. Each of them is an `x/group` account:\n", dossier.Country)
	b.WriteString("a role held by a plain key is refused by the chain, so every power below needs\n")
	b.WriteString("several people inside one office to agree before it is used.\n\n")

	fmt.Fprintf(&b, "The foundation admitted it. That is `%s`, the account `x/constitution` pins as\n",
		dossier.Foundation)
	b.WriteString("`enforcement_recovery_destination` — the same account every seized asset on this\n")
	b.WriteString("chain is sent to, and an account whose identity cannot be changed by an ordinary\n")
	b.WriteString("governance vote. Admitting a country is a decision of its custodians rather than\n")
	b.WriteString("of the validator set, and what that gave up is a public voting period; what it\n")
	b.WriteString("bought is that enrolling a country is one decision instead of five proposals that\n")
	b.WriteString("can each fail separately.\n\n")

	if config.ProposalID != "" {
		fmt.Fprintf(&b, "The enrolment was foundation proposal **%s**. Its votes are on the chain:\n\n",
			config.ProposalID)
		fmt.Fprintf(&b, "```\nblockchaind query group votes-by-proposal %s\n```\n\n", config.ProposalID)
	}

	b.WriteString("## The offices\n\n")
	for _, office := range dossier.Offices {
		fmt.Fprintf(&b, "### %s\n\n", office.Name)
		fmt.Fprintf(&b, "**Group policy:** `%s`  \n", office.OnChain.PolicyAddress)
		fmt.Fprintf(&b, "**Group:** %d, created by `%s` at height %d  \n",
			office.OnChain.GroupID, office.OnChain.TxHash, office.OnChain.Height)
		fmt.Fprintf(&b, "**Rule:** %d of %d  \n", office.Threshold, len(office.Members))
		fmt.Fprintf(&b, "**Key ceremony:** `%s`, group fingerprint `%s`\n\n",
			office.CeremonyID, office.GroupFingerprint)

		b.WriteString("| Super user | Fingerprint | Address |\n| --- | --- | --- |\n")
		for _, member := range office.Members {
			fmt.Fprintf(&b, "| %s | `%s` | `%s` |\n", member.Name, member.Fingerprint, member.Address)
		}
		b.WriteString("\n")

		b.WriteString("| Role | Jurisdiction | Granted by | At height |\n| --- | --- | --- | --- |\n")
		for _, g := range office.Granted {
			fmt.Fprintf(&b, "| %s | `%s` | `%s` | %d |\n", g.Role, g.Jurisdiction, g.GrantedBy, g.GrantedAtHeight)
		}
		b.WriteString("\n")

		if office.Placed != nil {
			fmt.Fprintf(&b, "Its own account is recorded in `%s`, by `%s`.\n\n",
				office.Placed.Country, office.Placed.RecordedBy)
		}
	}

	b.WriteString("Each address above was read back off the chain and checked against the office\n")
	b.WriteString("that generated the keys: the same members, the same threshold, administering\n")
	b.WriteString("itself. None of them was derived from a group policy sequence number. That\n")
	b.WriteString("distinction is the whole of the enrolment procedure, because a policy address is\n")
	b.WriteString("derived from the sequence number alone and therefore commits to nothing about who\n")
	b.WriteString("controls it — an address that looks right is equally the address of these super\n")
	b.WriteString("users and of whoever created a group policy first.\n\n")

	if len(dossier.Seeded) > 0 {
		b.WriteString("## The first institutions\n\n")
		b.WriteString("These were placed in the country by the foundation, because the payments\n")
		b.WriteString("authority cannot admit an institution the chain has not placed and no approved\n")
		b.WriteString("participant existed yet to place one. Every account after these is placed by the\n")
		b.WriteString("participant that onboarded it.\n\n")
		b.WriteString("| Account | Country | Recorded by |\n| --- | --- | --- |\n")
		for _, seed := range dossier.Seeded {
			fmt.Fprintf(&b, "| `%s` | `%s` | `%s` |\n", seed.Account, seed.Country, seed.RecordedBy)
		}
		b.WriteString("\n")
	}

	if len(dossier.Admitted) > 0 {
		b.WriteString("## Admitted to the rail\n\n")
		b.WriteString("| Participant | Code | Name |\n| --- | --- | --- |\n")
		for _, admitted := range dossier.Admitted {
			fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", admitted.Participant, admitted.Code, admitted.Name)
		}
		b.WriteString("\n")
	}

	if len(dossier.Waivers) > 0 {
		b.WriteString("## Rules this enrolment proceeded without\n\n")
		for _, w := range dossier.Waivers {
			fmt.Fprintf(&b, "- **%s** — %s\n", w.Rule, w.Reason)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Who was present\n\n")
	b.WriteString("| Role | Name | Organisation |\n| --- | --- | --- |\n")
	for _, p := range config.Participants {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", p.Role, p.Name, p.Organisation)
	}
	b.WriteString("\n")

	if len(config.Custodians) > 0 {
		b.WriteString("The foundation custodians who voted for the enrolment, as stated by the people\n")
		b.WriteString("signing below:\n\n")
		for _, name := range config.Custodians {
			fmt.Fprintf(&b, "- %s\n", name)
		}
		b.WriteString("\n")
		b.WriteString("This is the one claim in this document that was typed rather than read from the\n")
		b.WriteString("chain. Which custodians voted is a fact on the chain and this tool cannot read\n")
		b.WriteString("it, so the names above are an assertion by the signatories and the proposal\n")
		b.WriteString("named earlier is where a reader checks them.\n\n")
	}

	notes := append([]string(nil), dossier.Notes...)
	notes = append(notes, config.Notes...)
	b.WriteString("## What happened\n\n")
	if len(notes) > 0 {
		for _, note := range notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
	} else {
		b.WriteString("Nothing was recorded as out of the ordinary. Signing below is a statement that\n")
		b.WriteString("this is true: that every office's keys were generated by its own super users on\n")
		b.WriteString("their own devices, that no phrase was seen by anybody else, and that no address\n")
		b.WriteString("in this document was written down from anywhere but the chain's own answer.\n")
	}
	b.WriteString("\n")

	b.WriteString("## Attestation\n\n")
	b.WriteString("By signing, each person below states that they were present for this enrolment,\n")
	b.WriteString("that each office's super users generated their own keys and nobody else saw a\n")
	b.WriteString("recovery phrase, that every group address in this record was read back off the\n")
	b.WriteString("chain and verified against the office's own membership rather than derived, and\n")
	b.WriteString("that every grant listed was confirmed to exist on the chain and to have been made\n")
	b.WriteString("by the foundation named above.\n\n")

	b.WriteString("| Role | Name | Signature | Date |\n| --- | --- | --- | --- |\n")
	for _, p := range config.Participants {
		fmt.Fprintf(&b, "| %s | %s | | |\n", p.Role, p.Name)
	}
	for _, office := range dossier.Offices {
		for _, member := range office.Members {
			fmt.Fprintf(&b, "| %s super user | %s | | |\n", office.Name, member.Name)
		}
	}
	b.WriteString("\n")

	b.WriteString("---\n\n")
	b.WriteString("**How to check this document against the chain.** Every claim in it is a query:\n\n")
	b.WriteString("```bash\n")
	fmt.Fprintf(&b, "blockchaind query constitution invariants          # the foundation is %s\n", dossier.Foundation)
	for _, office := range dossier.Offices {
		fmt.Fprintf(&b, "blockchaind query group group-members %d\n", office.OnChain.GroupID)
		fmt.Fprintf(&b, "blockchaind query alias role-grants %s\n", office.OnChain.PolicyAddress)
	}
	fmt.Fprintf(&b, "blockchaind query alias role-holders %s            # everyone who may act in %s\n",
		dossier.Country, dossier.Country)
	b.WriteString("blockchaind query alias chain-wide-grants          # and everyone no border bounds\n")
	b.WriteString("```\n")

	return b.String(), nil
}
