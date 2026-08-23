package main

// The commands. One per step, in the order the steps happen in, and each one
// refuses until the previous one's evidence is in the dossier.
//
//	ceremony administrators init    --config administrators.json
//	ceremony administrators group   --dossier appointment-<name>.json
//	ceremony administrators confirm --dossier ... --tx ... --policy ... --members ...
//	ceremony administrators propose --dossier ... --alias-params ... --gov-account ... --deposit ...
//	ceremony administrators verify  --dossier ... --alias-params ...
//	ceremony administrators record  --dossier ... --config record.json
//
// The order is enforced rather than documented, because every one of these steps
// fails in a way that looks like a broken chain rather than like a missing step.
// A group that was never appointed refuses to correct a country in exactly the way
// a group whose proposal failed at execution does, and the operator goes looking
// at the chain.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	aliastypes "yamale/blockchain/x/alias/types"
)

const administratorsUsage = `ceremony administrators — appoint a foundation administrator on a running chain

  init      build the appointment dossier from a config and the group's ceremony
  group     the transaction that creates the M-of-N group account
  confirm   read the group's policy address back off the chain, and verify it
  propose   the GOVERNANCE proposal that appoints it
  verify    read x/alias's parameters back and check the appointment took effect
  record    render the appointment record for signature

Run "ceremony administrators <step> --help" for the flags.

A foundation administrator may correct the country recorded against ANY account on
the chain — which moves that account out from under the authority investigating it,
and retires and reissues its identifier — and may hold an identifier with no
country at all, carrying the reserved ZZ code.

It is appointed by an ordinary governance MsgUpdateParams on x/alias and by nothing
else. The foundation's own 3-of-5 cannot do it. So this ceremony ends at a
governance proposal rather than at a foundation vote.

The steps are in order and each one refuses until the previous one's evidence is in
the dossier. Nothing here talks to a chain: where a step needs to know what the
chain says, it reads the output of "blockchaind query ... -o json" and tells you the
exact command to produce it.
`

func runAdministrators(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, administratorsUsage)
		return errors.New("name a step")
	}
	switch args[0] {
	case "init":
		return runAdministratorsInit(args[1:])
	case "group":
		return runAdministratorsGroup(args[1:])
	case "confirm":
		return runAdministratorsConfirm(args[1:])
	case "propose":
		return runAdministratorsPropose(args[1:])
	case "verify":
		return runAdministratorsVerify(args[1:])
	case "record":
		return runAdministratorsRecord(args[1:])
	case "help", "-h", "--help":
		fmt.Fprint(os.Stderr, administratorsUsage)
		return nil
	default:
		fmt.Fprint(os.Stderr, administratorsUsage)
		return fmt.Errorf("unknown step %q", args[0])
	}
}

func administratorsDossierFlag(flags *flag.FlagSet) *string {
	return flags.String("dossier", "", "the appointment dossier written by `ceremony administrators init`")
}

func requireAdministratorsDossier(path string) (administratorsDossier, error) {
	if strings.TrimSpace(path) == "" {
		return administratorsDossier{}, errors.New("--dossier is required")
	}
	return readAdministratorsDossier(path)
}

// ---------------------------------------------------------------- init

func runAdministratorsInit(args []string) error {
	flags := flag.NewFlagSet("administrators init", flag.ExitOnError)
	configPath := flags.String("config", "", "the appointment config; see docs/guides/foundation-administrators.md")
	out := flags.String("out", ".", "directory for the dossier")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	var config administratorsConfig
	if err := readStrictJSON(*configPath, &config); err != nil {
		return err
	}
	dossier, err := administratorsDossierFor(config, time.Now())
	if err != nil {
		return err
	}

	path := filepath.Join(*out, fmt.Sprintf("appointment-%s.json", slug(dossier.Ceremony)))
	// Refused rather than overwritten. A dossier is the record of what has already
	// happened on a live chain — a group created, an appointment verified — and
	// rebuilding it from the config would silently discard all of it while
	// reporting success.
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf(
			"%s already exists. It carries what has already been done on the chain — a confirmed address, a "+
				"verified appointment — and rebuilding it from the config would throw that away. Move it aside "+
				"if you really mean to start again", path)
	}
	if err := writeAdministratorsDossier(path, dossier); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== appointing %s ===\n\n", dossier.Ceremony)
	c.printf("  chain              %s\n", dossier.ChainID)
	c.printf("  %d of %d, group fingerprint %s\n", dossier.Threshold, len(dossier.Members), dossier.GroupFingerprint)
	c.println()
	for _, member := range dossier.Members {
		c.printf("    %s  %s  %s\n", member.Fingerprint, member.Address, member.Name)
	}
	c.println()
	c.printf("Wrote %s.\n\n", path)
	c.println("No address for the group yet, and that is not an omission. An x/group policy")
	c.println("address is derived from the policy sequence number alone, so one computed now")
	c.println("would say nothing about who controls it — and on a live run of the country")
	c.println("ceremony a predicted address came out as the FOUNDATION'S OWN, because both")
	c.println("were sequence 1. A proposal carrying that would have appointed the foundation.")
	c.println()
	c.println("Create the group, then read the address back:")
	c.println()
	c.printf("  ceremony administrators group --dossier %s\n", path)
	return nil
}

// ---------------------------------------------------------------- group

func runAdministratorsGroup(args []string) error {
	flags := flag.NewFlagSet("administrators group", flag.ExitOnError)
	dossierPath := administratorsDossierFlag(flags)
	out := flags.String("out", ".", "directory for the member and policy documents")
	from := flags.String("from", "<your-key>", "the key that will broadcast; it keeps no authority over the group")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireAdministratorsDossier(*dossierPath)
	if err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s: create the group ===\n\n", dossier.Ceremony)

	if dossier.OnChain != nil {
		c.printf("  Already on chain at %s — nothing to do.\n", dossier.OnChain.PolicyAddress)
		return nil
	}

	assembled, err := readAssembledGroup(dossier.GroupFile)
	if err != nil {
		return err
	}
	// Re-checked here and not only at init, because the group file is what decides
	// who holds the power and a file that travelled is a file that can have been
	// swapped. The dossier records a fingerprint; this is the check that the file
	// still matches the ceremony the dossier was built from.
	if err := requireAdministratorsCeremony(assembled.Params, dossier.Ceremony); err != nil {
		return err
	}
	if assembled.Fingerprint != dossier.GroupFingerprint {
		return fmt.Errorf(
			"%s now has group fingerprint %s and the dossier recorded %s.\n"+
				"That is a different group. The fingerprint covers the member set, the threshold and the "+
				"metadata, so this file is not the one the custodians read aloud to each other",
			dossier.GroupFile, assembled.Fingerprint, dossier.GroupFingerprint)
	}
	if len(assembled.Members) == 0 || len(assembled.Policy) == 0 {
		return fmt.Errorf("%s produced no member or policy document", dossier.GroupFile)
	}

	metadata, err := officeGroupMetadata(assembled)
	if err != nil {
		return err
	}

	membersPath := filepath.Join(*out, fmt.Sprintf("administrators-%s-members.json", slug(dossier.Ceremony)))
	policyPath := filepath.Join(*out, fmt.Sprintf("administrators-%s-policy.json", slug(dossier.Ceremony)))
	if err := os.WriteFile(filepath.Clean(membersPath), append(assembled.Members, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Clean(policyPath), append(assembled.Policy, '\n'), 0o644); err != nil {
		return err
	}

	c.printf("  %d of %d\n", dossier.Threshold, len(dossier.Members))
	c.printf("    %s\n", membersPath)
	c.printf("    %s\n", policyPath)
	c.println()
	c.printf("    blockchaind tx group create-group-with-policy %s \\\n", *from)
	c.printf("      %q \\\n", metadata)
	c.printf("      %q \\\n", metadata)
	c.printf("      %s %s \\\n", membersPath, policyPath)
	c.printf("      --group-policy-as-admin --chain-id %s\n", dossier.ChainID)
	c.println()
	c.println("Any funded key can broadcast this. --group-policy-as-admin makes the policy its")
	c.println("own admin, so the account that signs keeps nothing: it cannot change the")
	c.println("membership, the threshold, or who administers the group afterwards. `confirm`")
	c.println("checks that it came out that way rather than assuming it.")
	c.println()
	c.println("Then query the transaction back and confirm it. Note that a broadcast reporting")
	c.println("code 0 has been ACCEPTED and has not executed — the code that matters is in the")
	c.println("queried result:")
	c.println()
	c.println("  blockchaind query tx <hash> -o json > tx.json")
	c.println("  # the address is in the EventCreateGroupPolicy in that file")
	c.println("  blockchaind query group group-policy-info <address> -o json > policy.json")
	c.println("  blockchaind query group group-members <group id> -o json > members.json")
	c.printf("  ceremony administrators confirm --dossier %s \\\n", *dossierPath)
	c.println("    --tx tx.json --policy policy.json --members members.json")
	return nil
}

// ---------------------------------------------------------------- confirm

func runAdministratorsConfirm(args []string) error {
	flags := flag.NewFlagSet("administrators confirm", flag.ExitOnError)
	dossierPath := administratorsDossierFlag(flags)
	txPath := flags.String("tx", "", "`blockchaind query tx <hash> -o json` for the create-group transaction")
	policyPath := flags.String("policy", "", "`blockchaind query group group-policy-info <address> -o json`")
	membersPath := flags.String("members", "", "`blockchaind query group group-members <id> -o json`")
	foundation := flags.String("foundation", "",
		"the foundation's policy address, refused as this group's; from "+
			"`blockchaind query constitution invariants -o json`")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireAdministratorsDossier(*dossierPath)
	if err != nil {
		return err
	}
	for name, value := range map[string]string{
		"--tx": *txPath, "--policy": *policyPath, "--members": *membersPath,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
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

	// The foundation's own address is refused as this group's, and this is the
	// specific hazard the two-phase design exists for: on a live run of the
	// country ceremony a predicted address came out as the foundation's, both
	// being policy sequence 1. Optional because the foundation address is not
	// always to hand, and named in the refusal when it is not given, because a
	// check somebody skipped silently is not a check.
	var forbidden []forbiddenAddress
	if trimmed := strings.TrimSpace(*foundation); trimmed != "" {
		forbidden = append(forbidden, forbiddenAddress{
			Address: trimmed,
			Reason: "A group confirmed there would make the appointment proposal appoint the FOUNDATION as a " +
				"foundation administrator — which would pass, execute, and read as correct, while the group " +
				"this ceremony was held for held nothing",
		})
	}

	confirmed, err := confirmGroup(dossier.attested(), forbidden, tx, policy, members, time.Now())
	if err != nil {
		return err
	}

	// Re-confirming is idempotent; re-confirming to a DIFFERENT address is not.
	// The second is either a second group for this ceremony — in which case
	// somebody has to say which one it is — or evidence that the wrong
	// transaction was queried.
	if dossier.OnChain != nil && dossier.OnChain.PolicyAddress != confirmed.PolicyAddress {
		return fmt.Errorf(
			"this dossier is already confirmed at %s and this transaction created %s.\n"+
				"One of those two is not this group. Guessing which would propose appointing an address nobody "+
				"chose", dossier.OnChain.PolicyAddress, confirmed.PolicyAddress)
	}

	dossier.OnChain = &confirmed
	if err := writeAdministratorsDossier(*dossierPath, dossier); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s confirmed ===\n\n", dossier.Ceremony)
	c.printf("  policy address  %s\n", confirmed.PolicyAddress)
	c.printf("  group           %d\n", confirmed.GroupID)
	c.printf("  created by      %s at height %d\n", confirmed.TxHash, confirmed.Height)
	c.printf("  %d of %d, self-administered\n", dossier.Threshold, len(dossier.Members))
	c.println()
	c.println("That address was read out of the chain's answer and checked against this")
	c.println("ceremony: the same members, the same threshold, administering itself. It was not")
	c.println("derived from a sequence number, which would have proved nothing about who")
	c.println("controls it.")
	if strings.TrimSpace(*foundation) == "" {
		c.println()
		c.println("NOTE: --foundation was not given, so this did not check that the address is")
		c.println("not the foundation's own. That is the one substitution that has actually")
		c.println("happened here. Compare it yourself before proposing:")
		c.println("  blockchaind query constitution invariants -o json")
	}
	c.println()
	c.println("Now read x/alias's current parameters and the governance module account. Both")
	c.println("are required: MsgUpdateParams replaces EVERY parameter at once, so a proposal")
	c.println("composed without the current values would silently reset the ones it did not")
	c.println("know:")
	c.println()
	c.println("  blockchaind query alias params -o json > alias-params.json")
	c.println("  blockchaind query auth module-account gov -o json > gov-account.json")
	c.println("  blockchaind query gov params deposit -o json   # for --deposit")
	c.printf("  ceremony administrators propose --dossier %s \\\n", *dossierPath)
	c.println("    --alias-params alias-params.json --gov-account gov-account.json \\")
	c.println("    --deposit 1000000uyml")
	return nil
}

// ---------------------------------------------------------------- propose

func runAdministratorsPropose(args []string) error {
	flags := flag.NewFlagSet("administrators propose", flag.ExitOnError)
	dossierPath := administratorsDossierFlag(flags)
	aliasParamsPath := flags.String("alias-params", "", "`blockchaind query alias params -o json`")
	govAccountPath := flags.String("gov-account", "", "`blockchaind query auth module-account gov -o json`")
	deposit := flags.String("deposit", "", "the proposal deposit, e.g. 1000000uyml; see `query gov params deposit`")
	out := flags.String("out", ".", "directory for the proposal")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireAdministratorsDossier(*dossierPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*aliasParamsPath) == "" {
		return errors.New(
			"--alias-params is required, and it is the whole point of this step.\n" +
				"MsgUpdateParams carries a Params message, not a field mask, so setting it REPLACES THE WHOLE " +
				"OBJECT. Appointing one administrator means reading the current parameters, adding one address, " +
				"and resubmitting everything — and a proposal composed without them would drop the " +
				"administrators already appointed, or reset payload_length, and pass anyway. Nothing on the " +
				"chain catches that: a shorter list than before is a perfectly valid list.\n" +
				"  blockchaind query alias params -o json > alias-params.json")
	}
	if strings.TrimSpace(*govAccountPath) == "" {
		return errors.New(
			"--gov-account is required. x/alias's UpdateParams is authority-gated to the governance module " +
				"account and to nothing else, and this tool will not compile that address in: one that had " +
				"gone stale would produce a proposal that passed its vote and was then refused at execution.\n" +
				"  blockchaind query auth module-account gov -o json > gov-account.json")
	}

	current, err := readAliasParamsFiles(*aliasParamsPath, *govAccountPath)
	if err != nil {
		return err
	}
	document, err := appointmentProposal(dossier, current, *deposit)
	if err != nil {
		return err
	}

	address, err := dossier.requireConfirmedGroup()
	if err != nil {
		return err
	}
	path := filepath.Join(*out, fmt.Sprintf("appoint-%s-proposal.json", slug(dossier.Ceremony)))
	if err := os.WriteFile(filepath.Clean(path), append(document, '\n'), 0o644); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s: the appointment proposal ===\n\n", dossier.Ceremony)
	c.printf("  appointing   %s\n", address)
	c.printf("  authority    %s\n", current.Authority)
	c.println()
	// The before and after, in full, for the same reason the governance console
	// shows it: this message replaces the whole object, so what is NOT changing is
	// information.
	c.println("  x/alias parameters, whole object, before and after:")
	c.printf("    payload_length             %d  ->  %d\n", current.PayloadLength, current.PayloadLength)
	c.printf("    foundation_administrators  %d  ->  %d\n",
		len(current.FoundationAdministrators), len(current.FoundationAdministrators)+1)
	for _, existing := range current.FoundationAdministrators {
		c.printf("      kept   %s\n", existing)
	}
	c.printf("      ADDED  %s\n", address)
	c.println()
	c.printf("Wrote %s.\n\n", path)
	c.println("Check that list against what you expect. Every address in it is one this")
	c.println("proposal RE-SUBMITS, and any that is missing is one it removes — silently, and")
	c.println("with a valid signature, because a shorter list is a valid list. If the count")
	c.println("above is lower than you believe it should be, the parameters were read before")
	c.println("somebody else's proposal landed: re-read them and compose this again.")
	c.println()
	c.printf("  blockchaind tx gov submit-proposal %s \\\n", path)
	c.printf("    --from <your-key> --chain-id %s \\\n", dossier.ChainID)
	c.println("    --gas 600000 --fees 20000uyml")
	c.println()
	c.println("The gas is explicit because the 200,000 default runs out part-way through a")
	c.println("proposal that carries a message, and fails with code 11 — which reads like a")
	c.println("rejected proposal rather than an unfunded transaction.")
	c.println()
	c.println("Then: a broadcast reporting code 0 has been ACCEPTED and has not executed. The")
	c.println("proposal id is in the queried transaction's events and nowhere in the")
	c.println("broadcast's own output:")
	c.println()
	c.println("  blockchaind query tx <hash> -o json")
	c.printf("  blockchaind tx gov vote <id> yes --from <key> --chain-id %s\n", dossier.ChainID)
	c.println()
	c.println("And when the voting period ends, verify it. A proposal can PASS and still fail")
	c.println("when it executes, which leaves the parameters exactly as they were:")
	c.println()
	c.println("  blockchaind query alias params -o json > alias-params.json")
	c.printf("  ceremony administrators verify --dossier %s --alias-params alias-params.json\n", *dossierPath)
	return nil
}

// ---------------------------------------------------------------- verify

func runAdministratorsVerify(args []string) error {
	flags := flag.NewFlagSet("administrators verify", flag.ExitOnError)
	dossierPath := administratorsDossierFlag(flags)
	aliasParamsPath := flags.String("alias-params", "", "`blockchaind query alias params -o json`")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireAdministratorsDossier(*dossierPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*aliasParamsPath) == "" {
		return errors.New("--alias-params is required: this step exists to read the chain's answer, not to assume it")
	}

	verified, err := verifyAppointment(dossier, *aliasParamsPath, time.Now())
	if err != nil {
		return err
	}
	dossier.Appointed = &verified
	if err := writeAdministratorsDossier(*dossierPath, dossier); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("=== %s is a foundation administrator ===\n\n", dossier.Ceremony)
	c.printf("  %s\n", verified.PolicyAddress)
	c.printf("  read back at %s\n", verified.VerifiedAt)
	c.println()
	c.printf("  alias.params.payload_length             %d\n", verified.PayloadLength)
	c.printf("  alias.params.foundation_administrators  %d of %d\n",
		len(verified.Administrators), aliastypes.MaxFoundationAdministrators)
	for _, administrator := range verified.Administrators {
		marker := "  "
		if administrator == verified.PolicyAddress {
			marker = "->"
		}
		c.printf("    %s %s\n", marker, administrator)
	}
	c.println()
	c.println("The whole list is recorded, not just this group. That is deliberate: the list is")
	c.println("what a carelessly composed MsgUpdateParams destroys, and if it is shorter than")
	c.println("it was before this proposal, the evidence is here and nowhere else.")
	c.println()
	c.printf("This group can now correct the country recorded against any account, %d of %d:\n",
		dossier.Threshold, len(dossier.Members))
	c.println()
	c.println("  # composed as a group proposal, because the administrator is the GROUP")
	c.printf("  #   message: /blockchain.alias.v1.MsgSetJurisdiction\n")
	c.printf("  #   recorder: %s\n", verified.PolicyAddress)
	c.println("  blockchaind tx group submit-proposal correction.json --from <a-custodian> \\")
	c.printf("    --chain-id %s\n", dossier.ChainID)
	c.println()
	c.printf("Then render the record:\n")
	c.printf("  ceremony administrators record --dossier %s --config record.json\n", *dossierPath)
	return nil
}

// ---------------------------------------------------------------- record

func runAdministratorsRecord(args []string) error {
	flags := flag.NewFlagSet("administrators record", flag.ExitOnError)
	dossierPath := administratorsDossierFlag(flags)
	configPath := flags.String("config", "", "the record config: location, times, participants")
	out := flags.String("out", ".", "directory for the rendered record")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dossier, err := requireAdministratorsDossier(*dossierPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*configPath) == "" {
		return errors.New("--config is required")
	}

	var config recordConfig
	if err := readStrictJSON(*configPath, &config); err != nil {
		return err
	}

	// Taken from the dossier rather than from the config, so the scribe cannot
	// mistype the values into the document that exists to detect mistyped values.
	config.Ceremony = dossier.Ceremony
	config.ChainID = dossier.ChainID
	config.Threshold = dossier.Threshold
	config.Administrators = true
	config.Office = nil
	if dossier.OnChain != nil {
		config.PolicyAddress = dossier.OnChain.PolicyAddress
	}

	custodians := make([]identity, 0, len(dossier.Members))
	for _, member := range dossier.Members {
		custodians = append(custodians, identity{
			Name:        member.Name,
			Address:     member.Address,
			Fingerprint: member.Fingerprint,
		})
	}

	rendered, err := renderRecord(config, custodians)
	if err != nil {
		return err
	}
	path := filepath.Join(*out, fmt.Sprintf("appointment-record-%s.md", slug(dossier.Ceremony)))
	if err := os.WriteFile(filepath.Clean(path), []byte(rendered), 0o644); err != nil {
		return err
	}

	c := stdConsole()
	c.printf("Wrote %s.\n\n", path)
	if dossier.Appointed == nil {
		c.println("NOTE: this appointment has not been verified against the chain, so the record")
		c.println("describes a group that may hold nothing. Run `verify` first if the proposal has")
		c.println("passed.")
	}
	c.println("Print it, read it in the room, and have everybody sign it on paper.")
	return nil
}
