package main

// The commands. One per step of the enrolment, in the order the steps happen in,
// and each one refuses until the previous one's evidence is in the dossier.
//
// That is what "the tool enforces the bootstrap order" means here. Not a
// checklist and not a warning: `grants` will not compose a proposal for an office
// whose address has not been read back, `admit` will not compose one until the
// grant has been read back, and neither of them has a flag that says otherwise.
// The reason is that every one of these steps fails in a way that looks like a
// broken chain rather than like a missing step — a payments authority whose grant
// never executed refuses exactly as a payments authority that was never appointed
// does, and the operator goes looking at the chain.
//
//	ceremony country init    --config country.json
//	ceremony country groups  --dossier country-SN.json
//	ceremony country confirm --dossier ... --office "..." --tx ... --policy ... --members ...
//	ceremony country grants  --dossier ... --proposer <custodian> --invariants ... --alias-params ...
//	ceremony country verify  --dossier ... --office "..." --grants ... --jurisdiction ...
//	ceremony country seed    --dossier ... --proposer <custodian> --account <applicant>
//	ceremony country admit   --dossier ... --proposer <super user> --applicant <bank>
//	ceremony country validator --dossier ... --candidate <operator> --reconciliation r.json
//	ceremony country record  --dossier ... --config record.json

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

// countryUsage is printed for `ceremony country` with no subcommand.
const countryUsage = `ceremony country — enrol one country on a chain that is already running

  init      build the enrolment dossier from a config and the offices' ceremonies
  groups    the transactions that create the offices' M-of-N group accounts
  confirm   read an office's policy address back off the chain, and verify it
  grants    the foundation proposal that places the offices and grants their roles
  verify    read the grants and the placements back off the chain
  seed      place the first applicant institutions, which only the foundation can
  admit     the payments authority's proposal admitting one of them
  validator place an approved validator's operator account, checked against what
            it declared to x/validatorgov
  record    render the enrolment record for signature

Run "ceremony country <step> --help" for the flags.

The steps are in order and each one refuses until the previous one's evidence is
in the dossier. Nothing here talks to a chain: where a step needs to know what the
chain says, it reads the output of "blockchaind query ... -o json" and tells you
the exact command to produce it.
`

func runCountry(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, countryUsage)
		return errors.New("name a step")
	}
	switch args[0] {
	case "init":
		return runCountryInit(args[1:])
	case "groups":
		return runCountryGroups(args[1:])
	case "confirm":
		return runCountryConfirm(args[1:])
	case "grants":
		return runCountryGrants(args[1:])
	case "verify":
		return runCountryVerify(args[1:])
	case "seed":
		return runCountrySeed(args[1:])
	case "admit":
		return runCountryAdmit(args[1:])
	case "validator":
		return runCountryValidator(args[1:])
	case "record":
		return runCountryRecord(args[1:])
	case "help", "-h", "--help":
		fmt.Fprint(os.Stderr, countryUsage)
		return nil
	default:
		fmt.Fprint(os.Stderr, countryUsage)
		return fmt.Errorf("unknown step %q", args[0])
	}
}

// dossierPath is the flag every step but init takes.
func dossierFlag(flags *flag.FlagSet) *string {
	return flags.String("dossier", "", "the enrolment dossier written by `ceremony country init`")
}

func requireDossier(path string) (countryDossier, error) {
	if strings.TrimSpace(path) == "" {
		return countryDossier{}, errors.New("--dossier is required")
	}
	return readDossier(path)
}

// ---------------------------------------------------------------- init

func runCountryInit(args []string) error {
	flags := flag.NewFlagSet("country init", flag.ExitOnError)
	configPath := flags.String("config", "", "the enrolment config; see docs/guides/country-enrolment.md")
	out := flags.String("out", ".", "directory for the dossier")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	var config countryConfig
	if err := readStrictJSON(*configPath, &config); err != nil {
		return err
	}

	dossier, err := dossierFor(config, time.Now())
	if err != nil {
		return err
	}

	path := filepath.Join(*out, fmt.Sprintf("country-%s.json", dossier.Country))
	// Refused rather than overwritten. A dossier is the record of what has already
	// happened on a live chain — an office confirmed, a grant verified — and
	// rebuilding it from the config would silently discard all of it while
	// reporting success.
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf(
			"%s already exists. It carries what has already been done on the chain — confirmed addresses, "+
				"verified grants — and rebuilding it from the config would throw that away. Move it aside if you "+
				"really mean to start again", path)
	}
	if err := writeDossier(path, dossier); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== enrolling %s ===\n\n", dossier.Country)
	c.printf("  chain        %s\n", dossier.ChainID)
	c.printf("  foundation   %s\n", dossier.Foundation)
	c.println()
	for _, office := range dossier.Offices {
		c.printf("  %s\n", office.Name)
		c.printf("    %d of %d, group fingerprint %s\n", office.Threshold, len(office.Members), office.GroupFingerprint)
		c.printf("    roles: %s\n", strings.Join(office.Roles, ", "))
		for _, member := range office.Members {
			c.printf("      %s  %s  %s\n", member.Fingerprint, member.Address, member.Name)
		}
		c.println()
	}
	for _, w := range dossier.Waivers {
		c.printf("  WAIVED  %s: %s\n", w.Rule, w.Reason)
	}
	if len(dossier.Waivers) > 0 {
		c.println()
	}
	c.printf("Wrote %s.\n\n", path)
	c.println("No address for any office yet, and that is not an omission. An x/group policy")
	c.println("address is derived from the policy sequence number alone, so one computed now")
	c.println("would say nothing about who controls it. Create the groups, then read the")
	c.println("addresses back:")
	c.println()
	c.printf("  ceremony country groups --dossier %s\n", path)
	return nil
}

// ---------------------------------------------------------------- groups

func runCountryGroups(args []string) error {
	flags := flag.NewFlagSet("country groups", flag.ExitOnError)
	dossierPath := dossierFlag(flags)
	out := flags.String("out", ".", "directory for the member and policy documents")
	from := flags.String("from", "<your-key>", "the key that will broadcast; it keeps no authority over the group")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireDossier(*dossierPath)
	if err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s: create the office groups ===\n\n", dossier.Country)

	for _, office := range dossier.Offices {
		if office.OnChain != nil {
			c.printf("  %s is already on chain at %s — skipping.\n\n", office.Name, office.OnChain.PolicyAddress)
			continue
		}

		assembled, err := readAssembledGroup(office.GroupFile)
		if err != nil {
			return fmt.Errorf("%s: %w", office.Name, err)
		}
		files, err := officeCreateFiles(office, assembled)
		if err != nil {
			return err
		}

		slugged := slug(office.Name)
		membersPath := filepath.Join(*out, fmt.Sprintf("office-%s-members.json", slugged))
		policyPath := filepath.Join(*out, fmt.Sprintf("office-%s-policy.json", slugged))
		if err := os.WriteFile(filepath.Clean(membersPath), append(files.Members, '\n'), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Clean(policyPath), append(files.Policy, '\n'), 0o644); err != nil {
			return err
		}

		c.printf("  %s — %d of %d\n", office.Name, files.Threshold, files.MemberCount)
		c.printf("    %s\n", membersPath)
		c.printf("    %s\n", policyPath)
		c.println()
		c.printf("    blockchaind tx group create-group-with-policy %s \\\n", *from)
		c.printf("      %q \\\n", files.Metadata)
		c.printf("      %q \\\n", files.Metadata)
		c.printf("      %s %s \\\n", membersPath, policyPath)
		c.printf("      --group-policy-as-admin --chain-id %s\n", dossier.ChainID)
		c.println()
	}

	c.println("Any funded key can broadcast these. --group-policy-as-admin makes the policy")
	c.println("its own admin, so the account that signs keeps nothing: it cannot change the")
	c.println("membership, the threshold, or who administers the office afterwards. `confirm`")
	c.println("checks that it came out that way rather than assuming it.")
	c.println()
	c.println("Then, for each office, query the transaction back and confirm it. Note that a")
	c.println("broadcast reporting code 0 has been ACCEPTED and has not executed — the code")
	c.println("that matters is in the queried result:")
	c.println()
	c.println("  blockchaind query tx <hash> -o json > tx.json")
	c.println("  # the address is in the EventCreateGroupPolicy in that file")
	c.println("  blockchaind query group group-policy-info <address> -o json > policy.json")
	c.println("  blockchaind query group group-members <group id> -o json > members.json")
	c.printf("  ceremony country confirm --dossier %s --office \"<office>\" \\\n", *dossierPath)
	c.println("    --tx tx.json --policy policy.json --members members.json")
	return nil
}

// ---------------------------------------------------------------- confirm

func runCountryConfirm(args []string) error {
	flags := flag.NewFlagSet("country confirm", flag.ExitOnError)
	dossierPath := dossierFlag(flags)
	officeName := flags.String("office", "", "which office's group to confirm")
	txPath := flags.String("tx", "", "`blockchaind query tx <hash> -o json` for the create-group transaction")
	policyPath := flags.String("policy", "", "`blockchaind query group group-policy-info <address> -o json`")
	membersPath := flags.String("members", "", "`blockchaind query group group-members <id> -o json`")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireDossier(*dossierPath)
	if err != nil {
		return err
	}
	for name, value := range map[string]string{
		"--office": *officeName, "--tx": *txPath, "--policy": *policyPath, "--members": *membersPath,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}

	office, err := dossier.office(*officeName)
	if err != nil {
		return err
	}

	tx, err := readTxResult(*txPath)
	if err != nil {
		return err
	}
	var policy policyInfo
	if err := readJSONFile(*policyPath, &policy); err != nil {
		return err
	}
	var members groupMembers
	if err := readJSONFile(*membersPath, &members); err != nil {
		return err
	}

	confirmed, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	if err != nil {
		return err
	}

	// Two offices cannot be the same account. Checked across the whole dossier
	// rather than per office, because the plausible mistake is confirming the same
	// transaction twice under two names — and the result would be one group holding
	// two offices' roles, which is the separation the roles describe collapsing
	// into one address with nothing on the record to show it.
	for i := range dossier.Offices {
		other := &dossier.Offices[i]
		if other.Name == office.Name || other.OnChain == nil {
			continue
		}
		if other.OnChain.PolicyAddress == confirmed.PolicyAddress {
			return fmt.Errorf(
				"%s is already confirmed at %s. Two offices at one address is one group holding both their "+
					"roles, and the separation between them would exist only on paper",
				other.Name, confirmed.PolicyAddress)
		}
	}

	// Re-confirming is allowed and idempotent; re-confirming to a DIFFERENT
	// address is not. The second is either a second group for one office — in
	// which case somebody has to say which one is the office — or evidence that
	// the wrong transaction was queried.
	if office.OnChain != nil && office.OnChain.PolicyAddress != confirmed.PolicyAddress {
		return fmt.Errorf(
			"%s is already confirmed at %s and this transaction created %s.\n"+
				"One of those two is not this office. Guessing which would put a country's authority on an "+
				"address nobody chose",
			office.Name, office.OnChain.PolicyAddress, confirmed.PolicyAddress)
	}

	office.OnChain = &confirmed
	if err := writeDossier(*dossierPath, dossier); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s confirmed ===\n\n", office.Name)
	c.printf("  policy address  %s\n", confirmed.PolicyAddress)
	c.printf("  group           %d\n", confirmed.GroupID)
	c.printf("  created by      %s at height %d\n", confirmed.TxHash, confirmed.Height)
	c.printf("  %d of %d, self-administered\n", office.Threshold, len(office.Members))
	c.println()
	c.println("That address was read out of the chain's answer and checked against this")
	c.println("office: the same members, the same threshold, administering itself. It was not")
	c.println("derived from a sequence number, which would have proved nothing about who")
	c.println("controls it.")
	c.println()

	pending := 0
	for _, o := range dossier.Offices {
		if o.OnChain == nil {
			pending++
			c.printf("  still to confirm: %s\n", o.Name)
		}
	}
	if pending == 0 {
		c.println("Every office is confirmed. The grants can be composed now:")
		c.println()
		c.println("  blockchaind query constitution invariants -o json > invariants.json")
		c.println("  blockchaind query alias params -o json > alias-params.json")
		c.printf("  ceremony country grants --dossier %s --proposer <custodian> \\\n", *dossierPath)
		c.println("    --invariants invariants.json --alias-params alias-params.json")
	}
	return nil
}

// ---------------------------------------------------------------- grants

func runCountryGrants(args []string) error {
	flags := flag.NewFlagSet("country grants", flag.ExitOnError)
	dossierPath := dossierFlag(flags)
	proposer := flags.String("proposer", "", "the foundation custodian submitting the proposal")
	invariants := flags.String("invariants", "", "`blockchaind query constitution invariants -o json`")
	aliasParams := flags.String("alias-params", "", "`blockchaind query alias params -o json`")
	out := flags.String("out", ".", "directory for the proposal")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireDossier(*dossierPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*proposer) == "" {
		return errors.New("--proposer is required: a group proposal is submitted by one of the group's members")
	}
	if strings.TrimSpace(*invariants) == "" {
		return errors.New(
			"--invariants is required. The account that may admit a country is the one x/constitution pins, and " +
				"a proposal built for any other address would be voted through by three custodians and then " +
				"refused by the chain")
	}
	if strings.TrimSpace(*aliasParams) == "" {
		return errors.New(
			"--alias-params is required. Placing an office's own account needs the foundation to be a foundation " +
				"administrator in x/alias's parameters, which is a different mechanism from the constitutional " +
				"invariant and has to be checked separately")
	}

	threshold, err := requireFoundation(dossier, *invariants)
	if err != nil {
		return err
	}
	if err := requireFoundationAdministrator(dossier, *aliasParams); err != nil {
		return err
	}

	document, err := enrolmentProposal(dossier, *proposer)
	if err != nil {
		return err
	}

	path := filepath.Join(*out, fmt.Sprintf("enrol-%s-proposal.json", dossier.Country))
	if err := os.WriteFile(filepath.Clean(path), append(document, '\n'), 0o644); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s: the enrolment proposal ===\n\n", dossier.Country)
	for _, office := range dossier.Offices {
		c.printf("  %s\n", office.Name)
		c.printf("    %s\n", office.OnChain.PolicyAddress)
		c.printf("    placed in %s, then granted %s\n", dossier.Country, strings.Join(office.Roles, ", "))
	}
	c.println()
	c.printf("Wrote %s.\n\n", path)
	c.println("One proposal, not one per office. x/group executes a proposal's messages")
	c.println("together or not at all, so this is the only shape in which \"the country is")
	c.println("enrolled\" either happened or did not. Split up, the state in between is a")
	c.println("country with a payments authority and no enforcement authority.")
	c.println()
	c.printf("  blockchaind tx group submit-proposal %s --from <custodian> --chain-id %s\n", path, dossier.ChainID)
	c.println()
	// The number the constitution says, not a number this tool chose. An operator
	// told to collect three votes on a chain whose invariant says four collects
	// three and then watches an exec do nothing.
	c.printf("Then %d of the foundation's custodians vote yes and one of them executes.\n", threshold)
	c.println()
	c.println("Note what `tx group exec` does before that threshold is reached: it returns")
	c.println("code 0 and DOES NOTHING. The only signal is the EventExec attribute in the")
	c.println("transaction, which reads PROPOSAL_EXECUTOR_RESULT_NOT_RUN rather than SUCCESS.")
	c.println("So do not read an exec's exit status as evidence that the country is enrolled.")
	c.println()
	c.println("Read the proposal on your own node before voting — not this tool's summary of it:")
	c.println()
	c.printf("  blockchaind query group proposals-by-group-policy %s -o json\n", dossier.Foundation)
	c.println()
	c.println("When it has executed, verify it landed. An accepted proposal that failed in")
	c.println("execution leaves the grants absent and reports nothing anybody is watching:")
	c.println()
	for _, office := range dossier.Offices {
		c.printf("  blockchaind query alias role-grants %s -o json > grants-%s.json\n",
			office.OnChain.PolicyAddress, slug(office.Name))
	}
	return nil
}

// ---------------------------------------------------------------- verify

func runCountryVerify(args []string) error {
	flags := flag.NewFlagSet("country verify", flag.ExitOnError)
	dossierPath := dossierFlag(flags)
	officeName := flags.String("office", "", "which office to verify")
	grantsPath := flags.String("grants", "", "`blockchaind query alias role-grants <address> -o json`")
	jurisdictionPath := flags.String("jurisdiction", "",
		"`blockchaind query alias jurisdiction <address> -o json`")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireDossier(*dossierPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*officeName) == "" {
		return errors.New("--office is required")
	}
	office, err := dossier.office(*officeName)
	if err != nil {
		return err
	}
	address, err := requireConfirmed(*office)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*grantsPath) == "" && strings.TrimSpace(*jurisdictionPath) == "" {
		return fmt.Errorf(
			"give --grants, --jurisdiction, or both:\n"+
				"  blockchaind query alias role-grants %s -o json > grants.json\n"+
				"  blockchaind query alias jurisdiction %s -o json > jurisdiction.json",
			address, address)
	}

	c := stdConsole()
	c.printf("=== %s ===\n\n", office.Name)
	c.printf("  %s\n\n", address)

	now := time.Now()
	if strings.TrimSpace(*grantsPath) != "" {
		verified, extra, err := verifyGrants(office, dossier.Country, dossier.Foundation, *grantsPath, now)
		if err != nil {
			return err
		}
		office.Granted = verified
		for _, g := range verified {
			c.printf("  GRANTED   %s in %s, by the foundation, at height %d\n",
				g.Role, g.Jurisdiction, g.GrantedAtHeight)
		}
		// Reported, not refused. An office may legitimately hold something
		// governance granted it; what a reader of the enrolment record needs is to
		// see that the office holds more than the record describes.
		for _, other := range extra {
			c.printf("  ALSO      %s — not part of this enrolment\n", other)
		}
		c.println()
	}

	if strings.TrimSpace(*jurisdictionPath) != "" {
		placed, err := verifyPlacement(address, dossier.Country, *jurisdictionPath, now)
		if err != nil {
			return err
		}
		office.Placed = &placed
		c.printf("  PLACED    in %s, recorded by %s\n\n", placed.Country, placed.RecordedBy)
	}

	if err := writeDossier(*dossierPath, dossier); err != nil {
		return err
	}

	if office.holdsRole(aliastypes.ROLE_PAYMENTS_AUTHORITY) &&
		office.grantVerified(aliastypes.ROLE_PAYMENTS_AUTHORITY, dossier.Country) {
		c.println("This office can now admit institutions in its country — once they have been")
		c.println("placed. The first ones cannot place themselves and no participant exists yet")
		c.println("to place them, so the foundation does it:")
		c.println()
		c.printf("  ceremony country seed --dossier %s --proposer <custodian> --account <applicant>\n", *dossierPath)
	}
	return nil
}

// ---------------------------------------------------------------- seed

func runCountrySeed(args []string) error {
	flags := flag.NewFlagSet("country seed", flag.ExitOnError)
	dossierPath := dossierFlag(flags)
	proposer := flags.String("proposer", "", "the foundation custodian submitting the proposal")
	out := flags.String("out", ".", "directory for the proposal")
	verified := flags.String("verified", "",
		"`blockchaind query alias jurisdiction <account> -o json`, to record that a seed landed")
	account := flags.String("account", "", "an applicant institution to place; repeatable as a comma-separated list")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireDossier(*dossierPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*account) == "" {
		return errors.New("--account is required")
	}
	accounts := splitList(*account)

	// The verification branch, when a seed has already been proposed and executed.
	if strings.TrimSpace(*verified) != "" {
		if len(accounts) != 1 {
			return errors.New("--verified takes one --account at a time; the query response is about one account")
		}
		placed, err := verifyPlacement(accounts[0], dossier.Country, *verified, time.Now())
		if err != nil {
			return err
		}
		if strings.TrimSpace(placed.RecordedBy) != strings.TrimSpace(dossier.Foundation) {
			// Not a refusal. Somebody else placing the applicant is a perfectly
			// good outcome — an approved participant onboarding it, say — and the
			// only thing that matters downstream is that it is placed. But the
			// record has to say who, because a seed the foundation is credited
			// with and did not make is a claim on the enrolment record that is
			// simply false.
			stdConsole().printf(
				"  note: %s was placed by %s, not by the foundation. Recording it as it stands.\n",
				placed.Account, placed.RecordedBy)
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
		c.println("The payments authority can admit it now:")
		c.println()
		c.printf("  ceremony country admit --dossier %s --proposer <super user> --applicant %s\n",
			*dossierPath, placed.Account)
		return nil
	}

	if strings.TrimSpace(*proposer) == "" {
		return errors.New("--proposer is required: a group proposal is submitted by one of the group's members")
	}

	document, err := seedProposal(dossier, *proposer, accounts)
	if err != nil {
		return err
	}
	path := filepath.Join(*out, fmt.Sprintf("seed-%s-proposal.json", dossier.Country))
	if err := os.WriteFile(filepath.Clean(path), append(document, '\n'), 0o644); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s: place the first applicants ===\n\n", dossier.Country)
	for _, a := range accounts {
		c.printf("  %s\n", a)
	}
	c.println()
	c.printf("Wrote %s.\n\n", path)
	c.println("This step exists because of an ordering nothing announces. x/paymsg's delegated")
	c.println("approval calls the perimeter check on the APPLICANT, and the perimeter check")
	c.println("refuses a target the chain cannot place before it consults any grant. So the")
	c.println("country's payments authority, holding a perfectly good grant, cannot admit the")
	c.println("first institution in its own country: nobody has recorded where it is, it may")
	c.println("not declare its own, and there is no approved participant yet to do it.")
	c.println()
	c.println("A foundation administrator can, and that is the only reason this is a foundation")
	c.println("proposal. Every account after the first ones is placed by the participant that")
	c.println("onboarded it, which is where the record belongs.")
	c.println()
	c.printf("  blockchaind tx group submit-proposal %s --from <custodian> --chain-id %s\n", path, dossier.ChainID)
	c.println()
	c.println("Then, once it has executed:")
	c.println()
	for _, a := range accounts {
		c.printf("  blockchaind query alias jurisdiction %s -o json > placed.json\n", a)
		c.printf("  ceremony country seed --dossier %s --account %s --verified placed.json\n", *dossierPath, a)
	}
	return nil
}

// ---------------------------------------------------------------- admit

func runCountryAdmit(args []string) error {
	flags := flag.NewFlagSet("country admit", flag.ExitOnError)
	dossierPath := dossierFlag(flags)
	proposer := flags.String("proposer", "", "the office super user submitting the proposal")
	applicant := flags.String("applicant", "", "the institution being admitted")
	reject := flags.Bool("reject", false, "reject the application instead of approving it")
	out := flags.String("out", ".", "directory for the proposal")
	verified := flags.String("verified", "",
		"`blockchaind query paymsg get-approved-participant <address> -o json`, to record that it landed")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireDossier(*dossierPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*applicant) == "" {
		return errors.New("--applicant is required")
	}

	if strings.TrimSpace(*verified) != "" {
		admitted, err := verifyAdmission(*applicant, *verified, time.Now())
		if err != nil {
			return err
		}
		replaced := false
		for i := range dossier.Admitted {
			if dossier.Admitted[i].Participant == admitted.Participant {
				dossier.Admitted[i] = admitted
				replaced = true
			}
		}
		if !replaced {
			dossier.Admitted = append(dossier.Admitted, admitted)
		}
		if err := writeDossier(*dossierPath, dossier); err != nil {
			return err
		}
		c := stdConsole()
		c.printf("  ADMITTED  %s as %s (%s)\n\n", admitted.Participant, admitted.Code, admitted.Name)
		c.println("It can now register its own customers and record where they are. That is the")
		c.println("last step of the bootstrap and the first ordinary one: from here the")
		c.println("jurisdiction of an account is recorded by the institution that did its KYC,")
		c.println("which is the only party that knows the answer.")
		return nil
	}

	if strings.TrimSpace(*proposer) == "" {
		return errors.New("--proposer is required: a group proposal is submitted by one of the group's members")
	}

	document, err := admissionProposal(dossier, *proposer, *applicant, !*reject)
	if err != nil {
		return err
	}
	office, err := dossier.paymentsOffice()
	if err != nil {
		return err
	}
	path := filepath.Join(*out, fmt.Sprintf("admit-%s-proposal.json", slug(*applicant)))
	if err := os.WriteFile(filepath.Clean(path), append(document, '\n'), 0o644); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s: %s ===\n\n", office.Name, *applicant)
	c.printf("Wrote %s.\n\n", path)
	c.printf("This is %s's proposal, to its own group — not the foundation's. That is what\n", office.Name)
	c.println("enrolling the country bought: licensing a payment service provider in")
	c.printf("%s is decided by %s of %s's super users.\n",
		dossier.Country, ordinal(office.Threshold), office.Name)
	c.println()
	c.printf("  blockchaind tx group submit-proposal %s --from <super user> --chain-id %s\n", path, dossier.ChainID)
	c.println()
	c.printf("  blockchaind query paymsg get-approved-participant %s -o json > admitted.json\n", *applicant)
	c.printf("  ceremony country admit --dossier %s --applicant %s --verified admitted.json\n",
		*dossierPath, *applicant)
	return nil
}

// ---------------------------------------------------------------- helpers

// splitList reads a comma-separated flag value.
func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func ordinal(n int) string {
	switch n {
	case 1:
		return "one"
	case 2:
		return "two"
	case 3:
		return "three"
	case 4:
		return "four"
	case 5:
		return "five"
	default:
		return fmt.Sprintf("%d", n)
	}
}

// readStrictJSON loads a config, refusing any field it does not know.
//
// Strict here, unlike the query responses in country_evidence.go, and the
// asymmetry is deliberate: a config is written by a person and a misspelled field
// is a value they believe they set. A "roles" that was typed "role" would
// otherwise produce an office with no roles and a successful-looking run.
func readStrictJSON(path string, into any) error {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
