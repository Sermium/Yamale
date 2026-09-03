// Command mpc drives threshold accounts from a terminal.
//
// The mpc package was written, tested and used by nothing — which is the same
// shape as the two dead functions found in x/tokenisation on 2026-08-27, and
// worth not repeating. This is its first caller.
//
// It is an operator's tool and a rehearsal, not the product. The product is a
// phone holding one share and a custodian holding another, exchanging messages
// over a network. Here every share is a file on one machine, which is exactly
// the arrangement the design exists to avoid — so the commands that need two
// shares say so out loud, and the file layout makes it obvious when somebody
// has put all three in one directory.
//
//	mpc keygen  --out DIR                          create an account, three shares
//	mpc address --share FILE                       the account a share belongs to
//	mpc sign    --digest B64 --share A --share B   a signature from any two
//	mpc reshare --share A --share B --out DIR      rotate; the address does not move
//	mpc pay     --shares DIR --to ADDR --amount X  a real payment on a real chain
//
// pay is the one that matters. Everything above it can be satisfied by a
// library that is subtly wrong; a transaction the chain accepts cannot.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"

	// Imported for its init(), which seals the yml bech32 prefix and the bond
	// denomination. Without it every address this tool printed would read
	// cosmos1... — correct bytes, and rejected by every Yamale interface.
	_ "yamale/blockchain/app"
	"yamale/blockchain/mpc"
	mpccosmos "yamale/blockchain/mpc/cosmos"
)

const usage = `mpc — threshold accounts, from a terminal

  keygen   --out DIR                            create an account and write its three shares
  address  --share FILE                         print the account a share belongs to
  sign     --digest BASE64 --share A --share B  produce a signature from any two shares
  reshare  --share A --share B --out DIR        rotate every share; the address does not move
  pay      --shares DIR --to ADDR --amount AMT  sign and broadcast a real payment
  enrol    --custodian URL --recovery URL       create an account against two live
           --email E --password P --out DIR     services, holding only this device's share
  sign-remote --share FILE --custodian URL      sign with this device's share and the
           --email E --password P --digest B64  custodian's, which never leaves its host

Three shares exist — device, custodian, recovery — and any two of them sign.
No single one can, which is the whole point: run "sign" with one share and it
refuses, and that refusal is the product.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = runKeygen(os.Args[2:])
	case "address":
		err = runAddress(os.Args[2:])
	case "sign":
		err = runSign(os.Args[2:])
	case "reshare":
		err = runReshare(os.Args[2:])
	case "pay":
		err = runPay(os.Args[2:])
	case "enrol":
		err = runEnrol(os.Args[2:])
	case "sign-remote":
		err = runSignRemote(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mpc:", err)
		os.Exit(1)
	}
}

// ------------------------------------------------------------------ keygen

func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", ".", "directory to write the three share files into")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "generating safe primes for three parties; this takes minutes")
	start := time.Now()
	pre := make(map[string]*keygen.LocalPreParams, len(mpc.Roles))
	type result struct {
		role string
		p    *keygen.LocalPreParams
		err  error
	}
	ch := make(chan result, len(mpc.Roles))
	for _, role := range mpc.Roles {
		go func(role string) {
			p, err := mpc.GeneratePreParams(mpc.KeygenTimeout)
			ch <- result{role, p, err}
		}(role)
	}
	for range mpc.Roles {
		r := <-ch
		if r.err != nil {
			return fmt.Errorf("%s: %w", r.role, r.err)
		}
		pre[r.role] = r.p
		fmt.Fprintf(os.Stderr, "  %s ready (%s)\n", r.role, time.Since(start).Round(time.Second))
	}

	fmt.Fprintln(os.Stderr, "running distributed key generation")
	shares, err := mpc.Keygen(pre)
	if err != nil {
		return err
	}

	for role, share := range shares {
		path := filepath.Join(*out, role+".share.json")
		raw, err := json.MarshalIndent(share, "", "  ")
		if err != nil {
			return err
		}
		// 0600, because this is one half of somebody's money. It is still a
		// file on a disk, which is why this command is a rehearsal: in
		// production the device share never exists anywhere but the device and
		// is sealed under the user's password before it touches storage.
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "  wrote %s\n", path)
	}

	pub, err := shares[mpc.RoleDevice].PublicKey()
	if err != nil {
		return err
	}
	addr, err := mpccosmos.Address(pub)
	if err != nil {
		return err
	}
	fmt.Println(addr)
	fmt.Fprintf(os.Stderr, "\naccount %s\ntook %s\n", addr, time.Since(start).Round(time.Second))
	fmt.Fprintln(os.Stderr,
		"\nthese three files are now the account. Any two sign; one signs nothing.\n"+
			"Keeping all three in this directory is convenient and is exactly what the\n"+
			"design forbids in production — separate them before this matters.")
	return nil
}

// ------------------------------------------------------------------ address

func runAddress(args []string) error {
	fs := flag.NewFlagSet("address", flag.ExitOnError)
	path := fs.String("share", "", "a share file; any one of the three will do")
	if err := fs.Parse(args); err != nil {
		return err
	}
	share, err := readShare(*path)
	if err != nil {
		return err
	}
	pub, err := share.PublicKey()
	if err != nil {
		return err
	}
	addr, err := mpccosmos.Address(pub)
	if err != nil {
		return err
	}
	// Any share answers, and they must all answer the same. A device that took
	// its address from the custodian's word for it is one that can be told to
	// show somebody else's balance.
	fmt.Println(addr)
	return nil
}

// ------------------------------------------------------------------ sign

type shareList []string

func (s *shareList) String() string     { return strings.Join(*s, ",") }
func (s *shareList) Set(v string) error { *s = append(*s, v); return nil }

func runSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	digestB64 := fs.String("digest", "", "32-byte digest to sign, base64")
	var paths shareList
	fs.Var(&paths, "share", "a share file; repeat for each signer")
	if err := fs.Parse(args); err != nil {
		return err
	}
	digest, err := base64.StdEncoding.DecodeString(*digestB64)
	if err != nil {
		return fmt.Errorf("--digest is not base64: %w", err)
	}
	shares, err := readShares(paths)
	if err != nil {
		return err
	}
	sig, pub, err := mpc.Sign(digest, shares)
	if err != nil {
		// The refusal for one share is the product, so it is printed as an
		// outcome rather than buried as a stack trace.
		return err
	}
	addr, err := mpccosmos.Address(pub)
	if err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(sig))
	fmt.Fprintf(os.Stderr, "signed for %s by %s\n", addr, strings.Join(rolesOf(shares), " + "))
	return nil
}

// ------------------------------------------------------------------ reshare

func runReshare(args []string) error {
	fs := flag.NewFlagSet("reshare", flag.ExitOnError)
	out := fs.String("out", "", "directory for the new shares")
	var paths shareList
	fs.Var(&paths, "share", "a surviving share file; repeat")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("--out is required: a reshare that wrote nowhere would retire an account")
	}
	old, err := readShares(paths)
	if err != nil {
		return err
	}
	before, err := addressOf(old)
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "resharing; the incoming committee needs its own safe primes")
	start := time.Now()
	fresh, err := mpc.Reshare(old, nil)
	if err != nil {
		return err
	}
	after, err := addressOf(fresh)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("the reshare moved the account from %s to %s", before, after)
	}

	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	for role, share := range fresh {
		raw, err := json.MarshalIndent(share, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*out, role+".share.json"), raw, 0o600); err != nil {
			return err
		}
	}
	fmt.Println(after)
	fmt.Fprintf(os.Stderr,
		"\nsame account, three new shares, %s\n"+
			"The old shares still sign until you delete them: retiring them is your\n"+
			"decision and the protocol will not make it for you.\n",
		time.Since(start).Round(time.Second))
	return nil
}

// ------------------------------------------------------------------ helpers

func readShare(path string) (mpc.Share, error) {
	if strings.TrimSpace(path) == "" {
		return mpc.Share{}, fmt.Errorf("--share is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return mpc.Share{}, err
	}
	var s mpc.Share
	if err := json.Unmarshal(raw, &s); err != nil {
		return mpc.Share{}, fmt.Errorf("%s: %w", path, err)
	}
	if s.Role == "" {
		return mpc.Share{}, fmt.Errorf("%s: the share names no role", path)
	}
	return s, nil
}

func readShares(paths []string) (map[string]mpc.Share, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no --share given")
	}
	out := make(map[string]mpc.Share, len(paths))
	for _, p := range paths {
		s, err := readShare(p)
		if err != nil {
			return nil, err
		}
		if _, seen := out[s.Role]; seen {
			return nil, fmt.Errorf("two shares both claim to be %s: one file counted twice is not two signers", s.Role)
		}
		out[s.Role] = s
	}
	return out, nil
}

func rolesOf(shares map[string]mpc.Share) []string {
	var roles []string
	for _, role := range mpc.Roles {
		if _, ok := shares[role]; ok {
			roles = append(roles, role)
		}
	}
	return roles
}

func addressOf(shares map[string]mpc.Share) (string, error) {
	for _, s := range shares {
		pub, err := s.PublicKey()
		if err != nil {
			return "", err
		}
		return mpccosmos.Address(pub)
	}
	return "", fmt.Errorf("no shares")
}

func sha256sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

var _ = context.Background
