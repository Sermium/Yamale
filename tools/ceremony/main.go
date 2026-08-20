// Command ceremony walks a room through generating the keys that control this
// chain.
//
// It exists because of one account. The `foundation` account on the devnet —
// x/enforcement's recovery_destination, the address every seized asset is sent
// to — turned out to be a single secp256k1 key sitting unencrypted in a
// `keyring-test` backend on a cloud VM, created by a setup script, printed once
// to a terminal nobody was reading, and written down nowhere. Losing that VM
// would have lost every asset the chain had ever recovered, and anyone who
// could read its disk owned them.
//
// So this tool does the opposite of what that script did:
//
//   - The foundation is not a key. It is a 3-of-5 x/group policy account, so
//     authority is distributed on-chain and every signature is attributable,
//     and one custodian can be lost without migrating anything.
//   - A mnemonic is shown on a screen and never written anywhere. Not a temp
//     file, not a log, not an argument the shell would remember. See secret in
//     key.go for exactly what that does and does not guarantee.
//   - A transcription is verified before the ceremony moves on. A backup nobody
//     checked is not a backup, and one mis-copied word is the likeliest failure
//     in the whole process.
//   - The machine is checked, and what could not be checked is said out loud
//     rather than implied.
//
// Like tools/wallet, this program contains no network code: there is nothing
// here that could phone home, which is the only version of that claim worth
// making to somebody about to type a seed phrase in front of it.
//
//	ceremony preflight                       # check the machine, nothing else
//	ceremony custodian --name "A. Okafor" --role custodian
//	ceremony validator --name "Bank of X"    # an operator key, not a consensus key
//	ceremony consensus                       # refuses, and explains why
//	ceremony restore                         # the restore drill: paper back to an address
//	ceremony group --threshold 3 custodian-*.json
//	ceremony replace-custodian --outgoing old.json --incoming new.json custodian-*.json
//	ceremony record --config record.json
package main

import (
	"fmt"
	"os"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	// accountPrefix and coinType are what make a key a Yamale key, restated
	// here rather than imported from the app package because a ceremony tool
	// that linked the whole node would defeat the point of running it on a
	// machine the node has never touched. TestAddressConfigMatchesTheChain
	// imports the app package and fails if these two ever diverge from it — a
	// tool deriving addresses under the wrong prefix would hand five custodians
	// keys that look right and control nothing.
	accountPrefix = "yml"
	coinType      = 118

	// entropyBits is 256, so twenty-four words. Twelve is legal and this tool
	// does not offer it: the extra sixty-four bits cost one line on a sheet of
	// paper that will be written once and read almost never, and the choice is
	// not one a custodian should be reasoning about while a room waits.
	entropyBits = 256
)

func main() {
	configureAddresses()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(os.Args[2:])
	case "preflight":
		err = runPreflight(os.Args[2:])
	case "custodian":
		err = runKeyCeremony(os.Args[2:], roleCustodian)
	case "validator":
		err = runKeyCeremony(os.Args[2:], roleValidator)
	case "consensus":
		err = runConsensus(os.Args[2:])
	case "restore":
		err = runRestore(os.Args[2:])
	case "group":
		err = runGroup(os.Args[2:])
	case "replace-custodian":
		err = runReplace(os.Args[2:])
	case "address":
		err = runAddress(os.Args[2:])
	case "record":
		err = runRecord(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "ceremony: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "ceremony:", err)
		os.Exit(1)
	}
}

// configureAddresses points the SDK's global bech32 configuration at this
// chain.
//
// Guarded rather than unconditional, and not run from an init(): the app
// package sets the same values and seals the config in its own init(), so a
// test that imports both — which is the test proving these constants have not
// drifted — would panic here on a sealed config. Checking first makes the two
// orders equivalent.
func configureAddresses() {
	config := sdk.GetConfig()
	if config.GetBech32AccountAddrPrefix() == accountPrefix {
		return
	}
	config.SetCoinType(coinType)
	config.SetBech32PrefixForAccount(accountPrefix, accountPrefix+"pub")
	config.SetBech32PrefixForValidator(accountPrefix+"valoper", accountPrefix+"valoperpub")
	config.SetBech32PrefixForConsensusNode(accountPrefix+"valcons", accountPrefix+"valconspub")
}

func usage() {
	fmt.Fprint(os.Stderr, `ceremony — generate the keys that control this chain, in a room, on paper

  serve       run the ceremony as a local page, for a room that cannot read a
              terminal; --mode custodian gives one custodian their own instance
  preflight   check the machine and stop; run this before anything else
  custodian   generate one custodian's key for the foundation group
  validator   generate a validator's OPERATOR key (not its consensus key)
  consensus   explains why this tool will not generate a consensus key
  restore     derive an address from a written phrase — the restore drill
  group       assemble the M-of-N group from the custodians' public records
  replace-custodian
              swap a departing custodian for their replacement, in one proposal
  address     derive a group policy address for a sequence number
  record      render the ceremony record for signature

Run "ceremony <command> --help" for the flags.

Nothing this program writes contains a private key or a mnemonic unless you
pass --armor and a path, which you should not do in a paper ceremony.
`)
}
