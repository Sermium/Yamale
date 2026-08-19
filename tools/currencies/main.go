// Command currencies seeds a genesis file with the chain's issuable currencies,
// and emits the same list in the other forms the repository needs.
//
// It exists because the alternative is forty-two governance proposals. Every
// currency needs an approved issuer before a single unit can be minted, and on
// a testnet that ceremony teaches nobody anything — it just delays the point,
// which is having real currencies to move around. On a real network these
// approvals are exactly the decisions that should be voted on, one at a time,
// which is why this writes genesis rather than offering a message that skips
// the vote.
//
//	go run ./tools/currencies --genesis ~/.blockchain/config/genesis.json --issuer yml1...
//	go run ./tools/currencies --emit-ts        # the SDK's display registry
//	go run ./tools/currencies --emit-denoms    # the oracle's accepted list
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	genesisPath := flag.String("genesis", "", "genesis.json to seed in place")
	issuer := flag.String("issuer", "", "address approved to issue every currency (the foundation, on testnet)")
	emitTS := flag.Bool("emit-ts", false, "print the SDK's KNOWN_DENOMS entries")
	emitDenoms := flag.Bool("emit-denoms", false, "print the oracle's accepted_denoms list")
	flag.Parse()

	switch {
	case *emitTS:
		fmt.Print(typeScript())
	case *emitDenoms:
		fmt.Println(strings.Join(acceptedDenoms(), ","))
	case *genesisPath != "":
		if *issuer == "" {
			fail("--issuer is required: every currency needs an address approved to issue it")
		}
		if err := seed(*genesisPath, *issuer); err != nil {
			fail(err.Error())
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// seed writes the currencies into a genesis file: the stablecoin module's
// approvals, the bank module's denom metadata, and the oracle's accepted list.
//
// All three, not one. The stablecoin entry is what lets the issuer mint; the
// bank metadata is what makes a wallet show "₦1,359.84" instead of
// "1359844414 ungn"; the oracle entry is what lets a rate be agreed for it. A
// currency with the first and not the others is mintable, unreadable and
// unpriced.
func seed(path, issuer string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var genesis map[string]any
	if err := json.Unmarshal(raw, &genesis); err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	appState, ok := genesis["app_state"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s has no app_state", path)
	}

	if err := seedStablecoin(appState, issuer); err != nil {
		return err
	}
	if err := seedBankMetadata(appState); err != nil {
		return err
	}
	if err := seedOracle(appState); err != nil {
		return err
	}

	out, err := json.MarshalIndent(genesis, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}

	fmt.Printf("seeded %d currencies into %s\n", len(AllCurrencies), path)
	fmt.Printf("  issuer:          %s\n", issuer)
	fmt.Printf("  oracle denoms:   %d (including the reference currencies)\n", len(acceptedDenoms()))
	fmt.Printf("  first, last:     %s … %s\n", AllCurrencies[0].Denom(), AllCurrencies[len(AllCurrencies)-1].Denom())
	return nil
}

func seedStablecoin(appState map[string]any, issuer string) error {
	stablecoin, ok := appState["stablecoin"].(map[string]any)
	if !ok {
		return fmt.Errorf("app_state has no stablecoin section; is this genesis from this chain?")
	}

	// Applications and approvals are written together and marked approved. The
	// module reads the approval when minting and the application when
	// describing the currency, so one without the other is a half-registered
	// currency that mints fine and cannot be explained.
	applications := make([]any, 0, len(AllCurrencies))
	approvals := make([]any, 0, len(AllCurrencies))

	existing := map[string]bool{}
	if prior, ok := stablecoin["approved_issuer_map"].([]any); ok {
		for _, entry := range prior {
			if m, ok := entry.(map[string]any); ok {
				if denom, ok := m["denom"].(string); ok {
					existing[denom] = true
				}
				approvals = append(approvals, entry)
			}
		}
	}
	if prior, ok := stablecoin["issuer_application_map"].([]any); ok {
		applications = append(applications, prior...)
	}

	for _, c := range AllCurrencies {
		if existing[c.Denom()] {
			// Re-running must not double-register. Genesis validation would
			// reject the duplicate, but only after the operator had already
			// distributed the file.
			continue
		}
		applications = append(applications, map[string]any{
			"denom":         c.Denom(),
			"status":        "approved",
			"creator":       issuer,
			"display_denom": c.Code,
			"exponent":      fmt.Sprintf("%d", ChainExponent),
			"name":          c.Name,
			"symbol":        c.Code,
			"description":   c.Description(),
		})
		approvals = append(approvals, map[string]any{
			"denom":  c.Denom(),
			"issuer": issuer,
		})
	}

	stablecoin["issuer_application_map"] = applications
	stablecoin["approved_issuer_map"] = approvals
	return nil
}

func seedBankMetadata(appState map[string]any) error {
	bank, ok := appState["bank"].(map[string]any)
	if !ok {
		return fmt.Errorf("app_state has no bank section")
	}

	metadata := make([]any, 0, len(AllCurrencies))
	seen := map[string]bool{}
	if prior, ok := bank["denom_metadata"].([]any); ok {
		for _, entry := range prior {
			if m, ok := entry.(map[string]any); ok {
				if base, ok := m["base"].(string); ok {
					seen[base] = true
				}
			}
			metadata = append(metadata, entry)
		}
	}

	for _, c := range AllCurrencies {
		if seen[c.Denom()] {
			continue
		}
		metadata = append(metadata, map[string]any{
			"description": c.Description(),
			"denom_units": []any{
				map[string]any{"denom": c.Denom(), "exponent": 0, "aliases": []any{}},
				map[string]any{"denom": c.Code, "exponent": ChainExponent, "aliases": []any{}},
			},
			"base":     c.Denom(),
			"display":  c.Code,
			"name":     c.Name,
			"symbol":   c.Code,
			"uri":      "",
			"uri_hash": "",
		})
	}

	bank["denom_metadata"] = metadata
	return nil
}

func seedOracle(appState map[string]any) error {
	oracle, ok := appState["oracle"].(map[string]any)
	if !ok {
		return fmt.Errorf("app_state has no oracle section")
	}
	params, ok := oracle["params"].(map[string]any)
	if !ok {
		return fmt.Errorf("oracle genesis has no params")
	}

	// A denom outside the accepted list is ignored rather than counted, so a
	// currency that is mintable but unaccepted would sit on the chain with no
	// price anybody could agree — and the lending and swap paths would treat it
	// as unknown rather than as worthless, which is the correct behaviour and a
	// confusing one to debug.
	params["accepted_denoms"] = toAny(acceptedDenoms())
	return nil
}

// acceptedDenoms is every denom the oracle should expect a rate for: the
// reference currencies the chain launched with, plus the African set.
func acceptedDenoms() []string {
	denoms := append([]string{}, ExistingDenoms...)
	for _, c := range AllCurrencies {
		denoms = append(denoms, c.Denom())
	}
	sort.Strings(denoms)
	return denoms
}

func toAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

// typeScript prints the entries for the SDK's display registry.
//
// Printed rather than written: the SDK file has hand-written entries above
// these, and a generator that owned the whole file would either drop them or
// have to understand them.
func typeScript() string {
	var b strings.Builder
	for _, c := range AllCurrencies {
		fmt.Fprintf(&b, "  %s: { base: '%s', symbol: '%s', exponent: %d, minorUnits: %d, name: %s },\n",
			c.Denom(), c.Denom(), c.Code, ChainExponent, c.Minor, quote(c.Name))
	}
	return b.String()
}

func quote(s string) string {
	if strings.Contains(s, "'") {
		return `"` + s + `"`
	}
	return "'" + s + "'"
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "currencies:", msg)
	os.Exit(1)
}
