package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// participant is somebody in the room who is not a custodian.
//
// The roles matter as much as the keys do. A ceremony run by one person with
// nobody watching produces exactly the same files as one run properly, and the
// only thing that distinguishes them afterwards is a record naming who was
// there and what each of them was responsible for.
type participant struct {
	Name string `json:"name"`
	Role string `json:"role"`
	// Organisation is what makes the observer's independence checkable. An
	// observer from the same team as the lead is a witness, not an observer.
	Organisation string `json:"organisation,omitempty"`
}

// recordConfig is what the scribe fills in. Everything else in the record comes
// from the files the ceremony produced, so the scribe cannot mistype an address
// or a fingerprint into the document that exists to detect mistyped addresses.
type recordConfig struct {
	Ceremony     string        `json:"ceremony"`
	ChainID      string        `json:"chain_id"`
	Location     string        `json:"location"`
	StartedAt    string        `json:"started_at"`
	CompletedAt  string        `json:"completed_at"`
	Participants []participant `json:"participants"`
	Threshold    int           `json:"threshold"`
	// CustodianFiles and GroupGenesisFile are paths, read rather than copied.
	CustodianFiles   []string `json:"custodian_files"`
	GroupGenesisFile string   `json:"group_genesis_file,omitempty"`
	PolicyAddress    string   `json:"policy_address"`
	// Office is set when this is a country office's ceremony rather than the
	// foundation's, and it changes what the record CLAIMS about the address
	// above.
	//
	// It has to, and this is not presentation. For the foundation the policy
	// address is a fact fixed by the genesis file, and the record says it is the
	// constitution's recovery destination. For an office it is a prediction from
	// a sequence number, and every sentence of that paragraph would be false —
	// including the one naming it as the place every seized asset on the chain is
	// sent. A record is a document somebody acts on.
	Office *officeParams `json:"office,omitempty"`
	BinaryHash       string   `json:"binary_hash,omitempty"`
	// Notes is where an exposure, an interruption, a destroyed key or a
	// regenerated one is written down. An empty list is a claim that nothing
	// happened, which is itself worth signing.
	Notes []string `json:"notes,omitempty"`
}

// runRecord renders the ceremony record.
//
// Markdown and JSON from the same source. The Markdown is what the room signs —
// on paper, with pens, because a record everybody signs in a shared document
// afterwards is a record signed by whoever had the link. The JSON is what
// tooling reads, so nobody has to parse the signed copy.
func runRecord(args []string) error {
	flags := flag.NewFlagSet("record", flag.ExitOnError)
	configPath := flags.String("config", "", "JSON file describing the ceremony; see the key ceremony guide")
	out := flags.String("out", ".", "directory for the rendered record")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}

	data, err := os.ReadFile(*configPath)
	if err != nil {
		return err
	}
	var config recordConfig
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("%s: %w", *configPath, err)
	}

	custodians, err := readIdentities(config.CustodianFiles)
	if err != nil {
		return err
	}

	markdown, err := renderRecord(config, custodians)
	if err != nil {
		return err
	}

	markdownPath := filepath.Join(*out, "ceremony-record.md")
	if err := os.WriteFile(markdownPath, []byte(markdown), 0o644); err != nil {
		return err
	}

	full := struct {
		recordConfig
		Custodians []identity `json:"custodians"`
	}{config, custodians}
	encoded, err := json.MarshalIndent(full, "", "  ")
	if err != nil {
		return err
	}
	jsonPath := filepath.Join(*out, "ceremony-record.json")
	if err := os.WriteFile(jsonPath, append(encoded, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf("Wrote %s and %s.\n", markdownPath, jsonPath)
	fmt.Println("Print the Markdown. Every participant signs the paper copy before leaving.")
	return nil
}

// renderRecord produces the document the room signs.
func renderRecord(config recordConfig, custodians []identity) (string, error) {
	if config.Threshold < 2 || config.Threshold >= len(custodians) {
		return "", fmt.Errorf(
			"threshold %d over %d custodians is not a group that both works and survives a loss",
			config.Threshold, len(custodians))
	}
	// Required for the foundation, where the address IS the record: a genesis file
	// names it, the constitution pins it, and a record without it attests to
	// nothing checkable.
	//
	// Not required for a country office, because at the moment its keys are made
	// the office genuinely has no address — the chain has not created its group
	// yet. Demanding one here would have forced the record to print a prediction
	// in the place a person reads an address, which is the whole failure the
	// enrolment's two phases exist to avoid.
	if config.Office == nil && strings.TrimSpace(config.PolicyAddress) == "" {
		return "", fmt.Errorf("policy_address is required: it is the whole reason this record exists")
	}
	if strings.TrimSpace(config.ChainID) == "" {
		return "", fmt.Errorf("chain_id is required: an address is only meaningful against a named chain")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Key ceremony record — %s\n\n", config.Ceremony)
	fmt.Fprintf(&b, "**Chain:** `%s`  \n", config.ChainID)
	fmt.Fprintf(&b, "**Location:** %s  \n", config.Location)
	fmt.Fprintf(&b, "**Started:** %s  \n", config.StartedAt)
	fmt.Fprintf(&b, "**Completed:** %s  \n", config.CompletedAt)
	fmt.Fprintf(&b, "**Rendered:** %s\n\n", time.Now().UTC().Format(time.RFC3339))

	if config.BinaryHash != "" {
		fmt.Fprintf(&b, "**Ceremony tool:** `sha256:%s`\n\n", config.BinaryHash)
	}

	b.WriteString("Everything in this document is public. It contains no key material and no\n")
	b.WriteString("recovery phrase, and it is meant to be published.\n\n")

	if office := config.Office; office != nil {
		// A country office, whose record must not claim to be the foundation's.
		//
		// The paragraph below used to be printed for every ceremony, and for an
		// office every sentence of it was false: it named the address as the
		// constitution's recovery destination and as the place every seized asset
		// on the chain is sent. A signed record saying that about a national
		// payments office is not a cosmetic problem — it is a document somebody
		// would act on.
		fmt.Fprintf(&b, "## %s\n\n", config.Ceremony)
		fmt.Fprintf(&b, "A **%d-of-%d** `x/group` policy for a country office in `%s`. Any %d of the %d\n",
			config.Threshold, len(custodians), office.Country, config.Threshold, len(custodians))
		b.WriteString("super users below can act for it; no smaller number can, and every signature is\n")
		b.WriteString("attributable on chain to the person who made it.\n\n")
		if len(office.Roles) > 0 {
			fmt.Fprintf(&b, "It is intended to hold **%s**, in `%s` and nowhere else.\n\n",
				strings.Join(office.Roles, "**, **"), office.Country)
		}
		b.WriteString("**This office has no address yet.** An `x/group` policy address is derived from\n")
		b.WriteString("the group policy sequence number alone — not from these members, not from the\n")
		b.WriteString("threshold — so it cannot be known until the group has been created on the chain,\n")
		b.WriteString("and an address computed in advance would commit to nothing about who controls\n")
		b.WriteString("it. The enrolment reads it back and checks it against the membership above; see\n")
		b.WriteString("the country enrolment guide. Any address printed elsewhere in this ceremony's\n")
		b.WriteString("output is a prediction and is not this office.\n\n")
		fmt.Fprintf(&b, "What this record fixes is **which %d keys** the office is, and their\n", len(custodians))
		b.WriteString("fingerprints. That is what the enrolment checks the chain's answer against.\n\n")
	} else {
		b.WriteString("## The foundation account\n\n")
		fmt.Fprintf(&b, "A **%d-of-%d** `x/group` policy. Any %d of the %d custodians below can move what\n",
			config.Threshold, len(custodians), config.Threshold, len(custodians))
		b.WriteString("this account holds; no smaller number can, and every signature is attributable\n")
		b.WriteString("on chain to the custodian who made it.\n\n")
		fmt.Fprintf(&b, "```\n%s\n```\n\n", config.PolicyAddress)
		b.WriteString("This address is `enforcement_recovery_destination` in the constitution and\n")
		b.WriteString("`recovery_destination` in `x/enforcement`'s parameters. It is where every asset\n")
		b.WriteString("this chain ever seizes is sent.\n\n")
	}

	b.WriteString("## Custodians\n\n")
	b.WriteString("| # | Custodian | Fingerprint | Address |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for i, custodian := range custodians {
		fmt.Fprintf(&b, "| %d | %s | `%s` | `%s` |\n", i+1, custodian.Name, custodian.Fingerprint, custodian.Address)
	}
	b.WriteString("\n")
	b.WriteString("The fingerprint is a digest of the public key. Each custodian wrote their own on\n")
	b.WriteString("their own sheet: an envelope opened years from now either recovers to a key whose\n")
	b.WriteString("fingerprint matches the row above or it does not, and that check needs no network\n")
	b.WriteString("and nobody's word.\n\n")

	b.WriteString("## Who was present\n\n")
	b.WriteString("| Role | Name | Organisation |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, p := range config.Participants {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", p.Role, p.Name, p.Organisation)
	}
	b.WriteString("\n")

	if len(config.Notes) > 0 {
		b.WriteString("## What happened\n\n")
		for _, note := range config.Notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("## What happened\n\n")
		b.WriteString("Nothing was recorded as out of the ordinary: no phrase was exposed, no key was\n")
		b.WriteString("destroyed and regenerated, and nobody entered the room who was not named above.\n")
		b.WriteString("Signing below is a statement that this is true.\n\n")
	}

	b.WriteString("## Attestation\n\n")
	b.WriteString("By signing, each person below states that they were present for the whole\n")
	b.WriteString("ceremony, that the machine was prepared as the runbook requires, that each\n")
	b.WriteString("custodian's transcription was read back and verified, that the restore drill\n")
	b.WriteString("reproduced a custodian's address from paper, and that no recovery phrase was\n")
	b.WriteString("photographed, copied, or written anywhere but on its custodian's own sheet.\n\n")

	b.WriteString("| Role | Name | Signature | Date |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, p := range config.Participants {
		fmt.Fprintf(&b, "| %s | %s | | |\n", p.Role, p.Name)
	}
	for _, custodian := range custodians {
		fmt.Fprintf(&b, "| custodian | %s | | |\n", custodian.Name)
	}
	b.WriteString("\n")

	return b.String(), nil
}
