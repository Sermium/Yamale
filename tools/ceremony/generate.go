package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
)

// runKeyCeremony generates exactly one key, in front of the room.
//
// One per invocation, deliberately. A loop that produced all five custodians in
// a single run would have all five phrases pass through one process, so a crash
// dump or a scrollback would lose the whole group rather than one member of it —
// and the 3-of-5 exists precisely so that one loss is survivable.
func runKeyCeremony(args []string, r role) error {
	flags := flag.NewFlagSet(string(r), flag.ExitOnError)
	name := flags.String("name", "", "the custodian's or operator's name, as it will appear on the ceremony record")
	ceremony := flags.String("ceremony", "", "identifier for this ceremony, copied onto the record")
	out := flags.String("out", ".", "directory for the public record of this key")
	index := flags.Uint("index", 0, "HD account index")
	acknowledge := flags.String("network-acknowledged", "", "reason for proceeding with a network detected; recorded verbatim")
	armor := flags.String("armor", "", "ALSO write an encrypted keystore here — do not use this in a paper ceremony")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("--name is required: the ceremony record has to say whose key this is")
	}

	c := stdConsole()
	// Refused before anything is generated rather than before it is displayed,
	// so a redirected run costs nobody a destroyed key.
	if err := c.requireTerminal(); err != nil {
		return err
	}

	if r == roleValidator {
		printConsensusWarning(c)
	}

	answers, err := preflight(c, *acknowledge)
	if err != nil {
		return err
	}

	s, err := newSecret()
	// Zeroed on every path out of this function, including the error paths.
	// A failed ceremony is the one most likely to leave a phrase behind,
	// because it is the one where somebody is thinking about the error.
	defer s.zero()
	if err != nil {
		return err
	}

	c.clear()
	c.printf("=== %s: %s ===\n\n", r, *name)

	if err := verifyTranscription(c, s); err != nil {
		return err
	}

	priv, path, err := s.derive(uint32(*index))
	if err != nil {
		return err
	}
	defer zero(priv.Key)

	id, err := identityOf(*name, r, priv, path, time.Now())
	if err != nil {
		return err
	}
	id.Ceremony = *ceremony

	// The public record is written FIRST, and the keystore export afterwards.
	//
	// It used to be the other way round, with a fatal error on the export. That
	// meant a rejected passphrase at the very end discarded the whole ceremony:
	// the phrase had already been zeroed, the record was never written, and the
	// sheet the custodian had just filled in became a page of words belonging to
	// no key anybody could name. The most careful part of the process was thrown
	// away by the least important one.
	recordPath := filepath.Join(*out, fmt.Sprintf("%s-%s.json", r, slug(*name)))
	if err := writeIdentity(recordPath, id); err != nil {
		return err
	}

	// And the export cannot fail the run. If it goes wrong the key still exists,
	// on paper, and the keystore is reproducible from that paper — so this is a
	// step to retry, not a reason to generate a new key.
	if *armor != "" {
		if err := writeArmor(c, *armor, priv); err != nil {
			c.println()
			c.printf("The encrypted keystore was NOT written: %v", err)
			c.println()
			c.println()
			c.println("The key itself is fine and the sheet is good — the public record below was")
			c.println("written before this step. Produce the keystore from the phrase when you want it:")
			c.println()
			c.printf("  blockchaind keys add %s --recover --keyring-backend file", slug(*name))
			c.println()
			c.println()
		}
	}

	c.clear()
	c.println("=== public record ===")
	c.println()
	c.printf("  name         %s\n", id.Name)
	c.printf("  role         %s\n", id.Role)
	c.printf("  address      %s\n", id.Address)
	if id.Valoper != "" {
		c.printf("  valoper      %s\n", id.Valoper)
	}
	c.printf("  pubkey       %s\n", id.PubKey.Key)
	c.printf("  fingerprint  %s\n", id.Fingerprint)
	c.printf("  hd path      %s\n", id.HDPath)
	c.println()
	c.printf("Written to %s. Everything in that file is public.\n", recordPath)
	c.println()
	c.println("Now, before the room moves on:")
	c.printf("  1. Write the fingerprint %s on your sheet, next to the phrase.\n", id.Fingerprint)
	c.println("     It is what proves five years from now that this envelope holds this key.")
	c.println("  2. Read the fingerprint aloud. The scribe writes it on the ceremony record.")
	c.println("  3. Seal the sheet in your envelope and sign across the seal.")
	c.println()
	c.println("The phrase is gone from this machine as far as this program can make it. It")
	c.println("cannot be shown again. If you did not write it down, say so now.")

	_ = answers // carried into the record by `ceremony record`, which reads the JSON.
	return nil
}

// writeIdentity writes the public record, and refuses to overwrite one.
//
// Refusing matters more than it looks: two custodians given the same --name is
// an easy mistake in a room reading from a list, and silently overwriting would
// leave a group with four members and a fifth who believes they are in it.
func writeIdentity(path string, id identity) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists — refusing to overwrite it. Two people with the same name, or a re-run?", path)
	}

	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(name string) string {
	s := nonSlug.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(s, "-")
}

// writeArmor exports the key in the format `blockchaind keys import` reads.
//
// Present because a validator operator generating a key offline has to get it
// onto a node somehow, and retyping twenty-four words into a production server
// is worse than a file. It is not part of the custodian ceremony: a custodian
// with an encrypted export has a second copy of their key on a medium that
// leaves the room, which is the thing the paper is supposed to replace.
func writeArmor(c *console, path string, priv *secp256k1.PrivKey) error {
	c.println()
	c.println("--armor was passed. This writes an ENCRYPTED COPY OF THE KEY to disk.")
	c.println("In a paper ceremony that is a second copy of something that should have exactly")
	c.println("one, on a medium that leaves the room. Do not do it for a custodian key.")
	c.println()

	ok, err := c.confirm("Write the encrypted keystore anyway?")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("armor export declined; nothing was written")
	}

	passphrase, err := c.readPassphrase()
	if err != nil {
		return err
	}

	if err := writeArmorTo(path, priv, passphrase); err != nil {
		return err
	}

	c.printf("\nEncrypted keystore written to %s\n", path)
	c.printf("Import it with:  blockchaind keys import <name> %s\n", path)
	return nil
}

// writeArmorTo is the file half, split out so the passphrase prompt is not
// entangled with the encryption. Everything that decides *whether* to export is
// above; this only does it.
func writeArmorTo(path string, priv *secp256k1.PrivKey, passphrase string) error {
	armored := crypto.EncryptArmorPrivKey(priv, passphrase, string(hd.Secp256k1Type))
	// 0600: this file is the key, encrypted. Written with the care an SSH
	// private key gets, because it is the same kind of thing.
	return os.WriteFile(path, []byte(armored), 0o600)
}

// printConsensusWarning is shown before a validator operator key is generated.
//
// The two keys are easy to conflate — both belong to "the validator", both are
// generated once, both are catastrophic to lose — and the correct handling is
// opposite for each. The operator key should be generated here, on a machine
// with no network, and kept off the node. The consensus key must never be
// anywhere but the node.
func printConsensusWarning(c *console) {
	c.println("=== this generates an OPERATOR key, not a consensus key ===")
	c.println()
	c.println("A validator has two keys and they are handled in opposite ways.")
	c.println()
	c.println("  operator key   — moves the stake, changes the commission, signs governance.")
	c.println("                   Generated here, on a machine with no network, and kept off")
	c.println("                   the node entirely. This is that key.")
	c.println()
	c.println("  consensus key  — signs blocks, thousands of times a day. It is generated ON")
	c.println("                   THE NODE by `blockchaind init`, it lives in")
	c.println("                   config/priv_validator_key.json, and it must never be copied,")
	c.println("                   backed up to a second machine, or generated anywhere else.")
	c.println()
	c.println("Two copies of a consensus key signing at once is double-signing, which is")
	c.println("slashed and jailed by the chain — automatically, with no appeal and no")
	c.println("distinction between malice and a restored backup. A copy taken 'for safety' is")
	c.println("the usual cause.")
	c.println()
}

// runConsensus exists only to refuse.
//
// An operator who reaches for `ceremony consensus` is about to generate a
// block-signing key on the wrong machine, and the useful thing to hand them is
// not a usage error — it is the reason, and the command they actually wanted.
func runConsensus(_ []string) error {
	c := stdConsole()
	printConsensusWarning(c)
	c.println("This tool will not generate a consensus key. There is no flag for it.")
	c.println()
	c.println("Generate it on the node that will use it:")
	c.println()
	c.println("    blockchaind init <moniker> --chain-id <chain> --default-denom uyml")
	c.println()
	c.println("That writes config/priv_validator_key.json. Read the public half with")
	c.println("`blockchaind comet show-validator` and carry THAT — the pubkey — to wherever")
	c.println("your create-validator transaction is built. The private half stays where it")
	c.println("was made, for the whole life of the validator.")
	return fmt.Errorf("no consensus key was generated, which is the correct outcome")
}

// runRestore is the restore drill, and it is the only reason this tool ever
// reads a phrase in.
//
// It derives an address and a fingerprint from a phrase somebody has typed back
// in off paper, and prints them so the room can compare against the public
// record made minutes earlier. Nothing is stored: this is a comparison, not an
// import. The drill is in the runbook because a ceremony that never reads a
// sheet back has verified the transcription against the screen and never
// against the paper — and the paper is the thing that has to work in five
// years.
func runRestore(args []string) error {
	flags := flag.NewFlagSet("restore", flag.ExitOnError)
	expect := flags.String("expect", "", "the address this phrase should produce; compared and reported")
	index := flags.Uint("index", 0, "HD account index")
	if err := flags.Parse(args); err != nil {
		return err
	}

	c := stdConsole()
	if err := c.requireTerminal(); err != nil {
		return err
	}

	c.println("Restore drill. Type the phrase from the sheet, on one line, separated by spaces.")
	c.println("Nothing is written to this machine. When the drill is over, this machine is wiped.")
	c.println()

	line, err := c.readLine("phrase: ")
	if err != nil {
		return err
	}

	s, err := secretFromInput(line)
	defer s.zero()
	if err != nil {
		return err
	}

	c.clear()

	priv, path, err := s.derive(uint32(*index))
	if err != nil {
		return err
	}
	defer zero(priv.Key)

	id, err := identityOf("restore drill", roleCustodian, priv, path, time.Now())
	if err != nil {
		return err
	}

	c.println("=== restored ===")
	c.println()
	c.printf("  address      %s\n", id.Address)
	c.printf("  fingerprint  %s\n", id.Fingerprint)
	c.println()

	if strings.TrimSpace(*expect) == "" {
		c.println("Compare both against the custodian's public record. If either differs, the")
		c.println("sheet is wrong: destroy the key, destroy the sheet, and generate a new one.")
		return nil
	}

	if strings.TrimSpace(*expect) != id.Address {
		return fmt.Errorf(
			"THE SHEET IS WRONG.\n"+
				"  expected  %s\n"+
				"  restored  %s\n"+
				"Destroy this key and this sheet and generate a new one. Do not try to work out\n"+
				"which word is wrong and correct it: a sheet that has been edited after the fact\n"+
				"is a sheet nobody can trust, and the key it protects is a key nobody has proven",
			strings.TrimSpace(*expect), id.Address)
	}

	c.println("Matches the expected address. The paper works.")
	c.println()
	c.println("Now destroy this instance: this machine is wiped before it leaves the room, and")
	c.println("the phrase you just typed exists on it until that happens.")
	return nil
}
