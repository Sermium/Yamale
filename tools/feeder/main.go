// Command feeder reports exchange rates to the chain's oracle, once per voting
// round, on behalf of one validator.
//
// The oracle takes a stake-weighted median of what the validators report, so
// this program's job is narrow and its failure mode matters more than its
// success: it must report what it actually observed, and report nothing at all
// when it did not observe anything. A feeder that filled a gap with yesterday's
// number, or with a plausible guess, would be indistinguishable from a working
// one right up until the median it fed was wrong.
//
//	feeder --validator ymlvaloper1... --from feeder-key --source yahoo
//
// Run one per validator, and — deliberately — not all on the same source. A
// median of four reports drawn from one endpoint is one source counted four
// times.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type config struct {
	binary    string
	home      string
	node      string
	chainID   string
	keyring   string
	from      string
	validator string
	source    string
	fees      string
	gas       string
	// ymlUSD prices the native token, which no market quotes because it does
	// not trade yet. It is a stated fiction rather than a discovered price, and
	// naming it as a flag keeps it from being mistaken for one.
	ymlUSD   float64
	interval time.Duration
	once     bool
}

func main() {
	var c config
	flag.StringVar(&c.binary, "binary", "blockchaind", "path to the chain binary")
	flag.StringVar(&c.home, "home", "", "node home directory")
	flag.StringVar(&c.node, "node", "http://localhost:26657", "RPC endpoint")
	flag.StringVar(&c.chainID, "chain-id", "yamale-testnet-1", "chain id")
	flag.StringVar(&c.keyring, "keyring-backend", "test", "keyring backend")
	flag.StringVar(&c.from, "from", "", "key that signs the submission (the feeder key)")
	flag.StringVar(&c.validator, "validator", "", "validator operator address the rates are reported for")
	flag.StringVar(&c.source, "source", "yahoo", "price source: yahoo or erapi")
	flag.StringVar(&c.fees, "fees", "0uyml", "transaction fees")
	flag.StringVar(&c.gas, "gas", "600000", "gas limit")
	flag.Float64Var(&c.ymlUSD, "yml-usd", 0.10, "price of one YML in USD; a testnet convention, not a market")
	flag.DurationVar(&c.interval, "interval", 60*time.Second, "how often to report")
	flag.BoolVar(&c.once, "once", false, "report once and exit")
	flag.Parse()

	if c.from == "" || c.validator == "" {
		fmt.Fprintln(os.Stderr, "feeder: --from and --validator are both required")
		os.Exit(2)
	}

	source, err := newSource(c.source)
	if err != nil {
		fmt.Fprintln(os.Stderr, "feeder:", err)
		os.Exit(2)
	}

	fmt.Printf("feeder: reporting for %s from %s every %s\n", c.validator, source.Name(), c.interval)

	for {
		if err := c.round(source); err != nil {
			// Logged, not fatal. A feed that exits on a bad afternoon is a feed
			// nobody notices has stopped; a validator that stops reporting is
			// counted as a miss, which is visible on chain.
			fmt.Fprintln(os.Stderr, "feeder:", err)
		}
		if c.once {
			return
		}
		time.Sleep(c.interval)
	}
}

// round fetches, converts and submits one set of rates.
func (c config) round(source Source) error {
	denoms, err := c.acceptedDenoms()
	if err != nil {
		return fmt.Errorf("reading the oracle's accepted denoms: %w", err)
	}

	codes := make([]string, 0, len(denoms))
	for _, denom := range denoms {
		codes = append(codes, codeOf(denom))
	}

	rates, err := source.Fetch(codes)
	if err != nil {
		return fmt.Errorf("fetching from %s: %w", source.Name(), err)
	}
	derived := applyPegs(rates, codes)

	submissions, missing := c.convert(denoms, rates)
	if len(submissions) == 0 {
		return fmt.Errorf("%s returned nothing usable; reporting nothing this round", source.Name())
	}

	if err := c.submit(submissions); err != nil {
		return err
	}

	fmt.Printf("%s  reported %d rates via %s", time.Now().Format(time.RFC3339), len(submissions), source.Name())
	if len(derived) > 0 {
		fmt.Printf(" (%d from pegs)", len(derived))
	}
	if len(missing) > 0 {
		// Named, every round. A currency quietly absent from the report is a
		// currency the chain will eventually mark stale, and the operator
		// should hear it from the feeder first rather than from the oracle.
		fmt.Printf("; no price for %s", strings.Join(missing, ", "))
	}
	fmt.Println()
	return nil
}

type rate struct {
	Denom string `json:"denom"`
	Rate  string `json:"rate"`
}

// convert turns units-per-USD into the chain's convention: how much one display
// unit of the denom is worth in the quote currency.
//
// That is the inverse, and getting it backwards is the mistake this whole
// program exists to avoid — it would price the naira at 1,359 dollars and every
// number downstream would be wrong by six orders of magnitude while looking
// entirely plausible.
func (c config) convert(denoms []string, rates map[string]float64) ([]rate, []string) {
	out := make([]rate, 0, len(denoms))
	var missing []string

	for _, denom := range denoms {
		code := codeOf(denom)

		var usd float64
		switch {
		case denom == "uyml":
			usd = c.ymlUSD
		case code == "USD":
			usd = 1
		default:
			perUSD, ok := rates[code]
			if !ok || perUSD <= 0 {
				missing = append(missing, code)
				continue
			}
			usd = 1 / perUSD
		}

		// Eighteen places because the small currencies need them: one Guinean
		// franc is about 0.000115 USD, and rounding that to six would throw
		// away a tenth of its value.
		out = append(out, rate{Denom: denom, Rate: fmt.Sprintf("%.18f", usd)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Denom < out[j].Denom })
	sort.Strings(missing)
	return out, missing
}

func (c config) submit(rates []rate) error {
	args := []string{"tx", "oracle", "submit-rates", c.validator}

	// One --rates flag per entry, each a single JSON object. autocli binds a
	// repeated message field as a repeatable flag, not as a flag taking a JSON
	// array — passing the array fails with "unexpected token [", which reads
	// like a malformed payload rather than the wrong shape entirely.
	for _, r := range rates {
		entry, err := json.Marshal(r)
		if err != nil {
			return err
		}
		args = append(args, "--rates", string(entry))
	}

	args = append(args,
		"--from", c.from,
		"--chain-id", c.chainID,
		"--keyring-backend", c.keyring,
		"--node", c.node,
		"--fees", c.fees,
		"--gas", c.gas,
		"-y", "--output", "json",
	)
	if c.home != "" {
		args = append(args, "--home", c.home)
	}

	out, err := exec.Command(c.binary, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("submitting: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// The broadcast result is only that the node accepted it into the mempool.
	// A submission from an unauthorised feeder, or for a denom the oracle does
	// not accept, broadcasts cleanly and fails in the block — so a feeder that
	// reported success here would be reporting that it sent something, not that
	// it worked.
	var result struct {
		Code   int    `json:"code"`
		RawLog string `json:"raw_log"`
	}
	if err := json.Unmarshal(out, &result); err == nil && result.Code != 0 {
		return fmt.Errorf("rejected at broadcast (code %d): %s", result.Code, result.RawLog)
	}
	return nil
}

// acceptedDenoms asks the chain what it wants prices for, rather than carrying
// its own list. The oracle ignores a denom outside its accepted set, so a
// hard-coded list here would drift into submitting rates that are silently
// discarded.
func (c config) acceptedDenoms() ([]string, error) {
	args := []string{"query", "oracle", "params", "--node", c.node, "--output", "json"}
	if c.home != "" {
		args = append(args, "--home", c.home)
	}

	out, err := exec.Command(c.binary, args...).Output()
	if err != nil {
		return nil, err
	}

	var params struct {
		Params struct {
			AcceptedDenoms []string `json:"accepted_denoms"`
		} `json:"params"`
	}
	if err := json.Unmarshal(out, &params); err != nil {
		return nil, err
	}
	if len(params.Params.AcceptedDenoms) == 0 {
		return nil, fmt.Errorf("the oracle accepts no denoms")
	}
	return params.Params.AcceptedDenoms, nil
}
