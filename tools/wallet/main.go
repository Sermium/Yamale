// Command yamale-wallet creates and inspects Yamale keys, offline.
//
// It contains no network code at all. That is the point: the machine that
// generates a key that will hold money should not be the machine that talks to
// the internet, and a tool that *could* phone home is one somebody has to take
// on trust. There is nothing here to trust — it reads a mnemonic, derives an
// address, and stops.
//
// Derivation comes from the chain's own crypto packages rather than from a
// reimplementation. A wallet that derived addresses even slightly differently
// from the node would produce keys that look right and control nothing, and the
// person holding one would not find out until they tried to spend.
//
//	yamale-wallet new                      # a fresh key, printed once
//	yamale-wallet new --accounts 5         # a key and the first five accounts
//	yamale-wallet new --armor wallet.asc   # also write an importable keystore
//	yamale-wallet recover --index 3        # an address from an existing phrase
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cosmos/cosmos-sdk/crypto"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	bip39 "github.com/cosmos/go-bip39"
	"golang.org/x/term"
)

const (
	// Prefix and coinType are what make a key a Yamale key. Coin type 118 is
	// the Cosmos standard, which is why a Ledger works and why the same seed
	// phrase produces the same account in Keplr, Leap or Cosmostation.
	prefix   = "yml"
	coinType = 118

	// Twenty-four words. Twelve is legal and common, and this tool does not
	// offer it: the extra sixty-four bits cost nothing to write down once and
	// the choice is not one a person should have to reason about while looking
	// at a screen that is about to show them a key.
	entropyBits = 256
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "new":
		newKey(os.Args[2:])
	case "recover", "inspect":
		recover(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "yamale-wallet: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `yamale-wallet — create and inspect Yamale keys, offline

  new       generate a new key
  recover   derive addresses from a mnemonic you already have

Flags for both:
  --accounts N   show the first N accounts (default 1)
  --index N      show a single account by index
  --json         machine-readable output
  --armor FILE   write an encrypted keystore, importable with:
                 blockchaind keys import <name> FILE

This tool never connects to anything.
`)
}

type account struct {
	Index   uint32 `json:"index"`
	Path    string `json:"path"`
	Address string `json:"address"`
	Valoper string `json:"valoper"`
	PubKey  string `json:"pubkey"`
}

type output struct {
	Mnemonic string    `json:"mnemonic,omitempty"`
	Accounts []account `json:"accounts"`
}

func newKey(args []string) {
	flags := flag.NewFlagSet("new", flag.ExitOnError)
	accounts := flags.Uint("accounts", 1, "how many accounts to derive")
	index := flags.Int("index", -1, "derive a single account by index")
	asJSON := flags.Bool("json", false, "machine-readable output")
	armorPath := flags.String("armor", "", "write an encrypted keystore to this file")
	flags.Parse(args)

	entropy, err := bip39.NewEntropy(entropyBits)
	if err != nil {
		fail(err)
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		fail(err)
	}

	emit(mnemonic, true, *accounts, *index, *asJSON, *armorPath)
}

func recover(args []string) {
	flags := flag.NewFlagSet("recover", flag.ExitOnError)
	accounts := flags.Uint("accounts", 1, "how many accounts to derive")
	index := flags.Int("index", -1, "derive a single account by index")
	asJSON := flags.Bool("json", false, "machine-readable output")
	armorPath := flags.String("armor", "", "write an encrypted keystore to this file")
	flags.Parse(args)

	fmt.Fprint(os.Stderr, "Enter your recovery phrase: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		fail(err)
	}
	mnemonic := strings.Join(strings.Fields(line), " ")

	// Checked before deriving. An invalid phrase still derives *an* address —
	// BIP-39 will happily hash anything — so a tool that skipped this would
	// hand somebody a plausible-looking address for a key nobody can recover.
	if !bip39.IsMnemonicValid(mnemonic) {
		fmt.Fprintln(os.Stderr, "\nyamale-wallet: that is not a valid recovery phrase.")
		fmt.Fprintln(os.Stderr, "Check the word count and the spelling; a phrase with one wrong word")
		fmt.Fprintln(os.Stderr, "derives a different, empty account rather than failing.")
		os.Exit(1)
	}

	emit(mnemonic, false, *accounts, *index, *asJSON, *armorPath)
}

func emit(mnemonic string, isNew bool, count uint, index int, asJSON bool, armorPath string) {
	indexes := make([]uint32, 0, count)
	if index >= 0 {
		indexes = append(indexes, uint32(index))
	} else {
		for i := uint32(0); i < uint32(count); i++ {
			indexes = append(indexes, i)
		}
	}

	result := output{Accounts: make([]account, 0, len(indexes))}
	if isNew {
		result.Mnemonic = mnemonic
	}

	var first *secp256k1.PrivKey
	for _, i := range indexes {
		priv, acct, err := derive(mnemonic, i)
		if err != nil {
			fail(err)
		}
		if first == nil {
			first = priv
		}
		result.Accounts = append(result.Accounts, acct)
	}

	if armorPath != "" {
		if err := writeArmor(armorPath, first); err != nil {
			fail(err)
		}
	}

	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fail(err)
		}
		return
	}

	print(result, isNew, armorPath)
}

// derive turns a mnemonic and an account index into a key, using the same code
// path the node uses.
func derive(mnemonic string, index uint32) (*secp256k1.PrivKey, account, error) {
	path := hd.NewFundraiserParams(0, coinType, index).String()

	derived, err := hd.Secp256k1.Derive()(mnemonic, "", path)
	if err != nil {
		return nil, account{}, err
	}
	priv := hd.Secp256k1.Generate()(derived).(*secp256k1.PrivKey)
	addressBytes := priv.PubKey().Address().Bytes()

	address, err := bech32.ConvertAndEncode(prefix, addressBytes)
	if err != nil {
		return nil, account{}, err
	}
	valoper, err := bech32.ConvertAndEncode(prefix+"valoper", addressBytes)
	if err != nil {
		return nil, account{}, err
	}

	return priv, account{
		Index:   index,
		Path:    path,
		Address: address,
		Valoper: valoper,
		PubKey:  fmt.Sprintf("%X", priv.PubKey().Bytes()),
	}, nil
}

// writeArmor exports the key in the format `blockchaind keys import` reads, so
// a key generated on an offline machine can be carried to a node without ever
// retyping the phrase.
func writeArmor(path string, priv *secp256k1.PrivKey) error {
	passphrase, err := readPassphrase()
	if err != nil {
		return err
	}

	armored := crypto.EncryptArmorPrivKey(priv, passphrase, string(hd.Secp256k1Type))

	// 0600: this file is the key. Written with the same care as an SSH private
	// key, because it is the same kind of thing.
	if err := os.WriteFile(path, []byte(armored), 0o600); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\nEncrypted keystore written to %s\n", path)
	fmt.Fprintf(os.Stderr, "Import it with:  blockchaind keys import <name> %s\n", path)
	return nil
}

func readPassphrase() (string, error) {
	// Piped input reads a line instead of turning off echo, which is what the
	// node's own keyring does and what makes this tool scriptable. When there
	// is no terminal there is nothing to hide the typing from, and refusing to
	// run would only push people towards passing secrets on a command line
	// where the shell history keeps them.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}

	fmt.Fprint(os.Stderr, "Passphrase for the keystore: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}

	fmt.Fprint(os.Stderr, "Again: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}

	if string(first) != string(second) {
		return "", fmt.Errorf("the two passphrases do not match")
	}
	if len(first) < 8 {
		return "", fmt.Errorf("use at least 8 characters: this passphrase is the only thing between the file and the money")
	}
	return string(first), nil
}

func print(result output, isNew bool, armorPath string) {
	if result.Mnemonic != "" {
		fmt.Println("Recovery phrase — write it down now. It is shown once and cannot be recovered.")
		fmt.Println()
		// Numbered in rows, because a phrase transcribed out of order is a
		// phrase that recovers nothing, and people do transcribe out of order.
		words := strings.Fields(result.Mnemonic)
		for i, word := range words {
			fmt.Printf("%2d. %-12s", i+1, word)
			if (i+1)%4 == 0 {
				fmt.Println()
			}
		}
		if len(words)%4 != 0 {
			fmt.Println()
		}
		fmt.Println()
	}

	for _, a := range result.Accounts {
		if len(result.Accounts) > 1 {
			fmt.Printf("Account %d  (%s)\n", a.Index, a.Path)
		}
		fmt.Printf("  address  %s\n", a.Address)
		fmt.Printf("  valoper  %s\n", a.Valoper)
	}

	if isNew && armorPath == "" {
		fmt.Println()
		fmt.Println("Anyone with that phrase controls these accounts. There is no support desk")
		fmt.Println("that can restore it and nobody who can freeze it on your behalf.")
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "yamale-wallet:", err)
	os.Exit(1)
}
