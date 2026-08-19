// Command bot periodically drives visible activity on a Money testnet
// node by shelling out to the blockchaind CLI with a weighted-random mix of
// actions read from config.yaml. It is intentionally simple: no direct
// Cosmos SDK client/signing code, just CLI invocations, so its behavior is
// exactly what an operator can reproduce by hand.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Action struct {
	Name   string   `yaml:"name"`
	Weight int      `yaml:"weight"`
	Args   []string `yaml:"args"`
}

type Config struct {
	Binary          string   `yaml:"binary"`
	ChainID         string   `yaml:"chain_id"`
	KeyringBackend  string   `yaml:"keyring_backend"`
	Home            string   `yaml:"home"`
	Node            string   `yaml:"node"`
	IntervalSeconds int      `yaml:"interval_seconds"`
	Accounts        []string `yaml:"accounts"`
	Actions         []Action `yaml:"actions"`
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to the bot's YAML config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	addrs, err := resolveAddresses(cfg)
	if err != nil {
		log.Fatalf("failed to resolve account addresses: %v", err)
	}
	for name, addr := range addrs {
		log.Printf("resolved account %s -> %s", name, addr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := time.Duration(cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("bot started: chain-id=%s interval=%s accounts=%v", cfg.ChainID, interval, cfg.Accounts)

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case <-ticker.C:
			runOneTick(ctx, cfg, addrs)
		}
	}
}

func loadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Binary == "" {
		cfg.Binary = "blockchaind"
	}
	if len(cfg.Accounts) < 1 {
		return cfg, fmt.Errorf("config must list at least 1 account")
	}
	if len(cfg.Actions) == 0 {
		return cfg, fmt.Errorf("config must list at least 1 action")
	}
	return cfg, nil
}

// resolveAddresses looks up each configured account's bech32 address once at
// startup via `blockchaind keys show <name> -a`.
//
// Deliberately passes only the home flag, never `--node`: reading the local
// keyring talks to no chain, and the command rejects the flag outright with
// `unknown flag: --node`. Passing it meant the bot could not start at all
// whenever a node was configured — which is every deployment that runs it
// anywhere but on a validator, the arrangement the runbook actually
// recommends.
func resolveAddresses(cfg Config) (map[string]string, error) {
	addrs := make(map[string]string, len(cfg.Accounts))
	for _, name := range cfg.Accounts {
		args := []string{"keys", "show", name, "-a", "--keyring-backend", cfg.KeyringBackend}
		args = append(args, homeFlag(cfg)...)

		out, err := exec.Command(cfg.Binary, args...).Output()
		if err != nil {
			var stderr string
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				stderr = strings.TrimSpace(string(exitErr.Stderr))
			}
			return nil, fmt.Errorf("resolving address for %s: %w: %s", name, err, stderr)
		}
		addrs[name] = strings.TrimSpace(string(out))
	}
	return addrs, nil
}

// homeFlag is what every invocation takes, online or not.
func homeFlag(cfg Config) []string {
	if cfg.Home == "" {
		return nil
	}
	return []string{"--home", cfg.Home}
}

// globalFlags adds the node as well, for the commands that reach the chain.
func globalFlags(cfg Config) []string {
	flags := homeFlag(cfg)
	if cfg.Node != "" {
		flags = append(flags, "--node", cfg.Node)
	}
	return flags
}

func runOneTick(ctx context.Context, cfg Config, addrs map[string]string) {
	action := pickWeightedAction(cfg.Actions)
	from := cfg.Accounts[rand.Intn(len(cfg.Accounts))]
	to := from
	if len(cfg.Accounts) > 1 {
		for to == from {
			to = cfg.Accounts[rand.Intn(len(cfg.Accounts))]
		}
	}

	replacer := strings.NewReplacer(
		"{from}", from,
		"{from_addr}", addrs[from],
		"{to}", to,
		"{to_addr}", addrs[to],
		"{chain_id}", cfg.ChainID,
		"{keyring_backend}", cfg.KeyringBackend,
		"{ts}", strconv.FormatInt(time.Now().Unix(), 10),
	)

	args := make([]string, 0, len(action.Args)+len(globalFlags(cfg)))
	for _, a := range action.Args {
		args = append(args, replacer.Replace(a))
	}
	args = append(args, globalFlags(cfg)...)

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(cmdCtx, cfg.Binary, args...).CombinedOutput()
	if err != nil {
		log.Printf("[%s] from=%s to=%s REJECTED: %v: %s", action.Name, from, to, err, strings.TrimSpace(string(out)))
		return
	}

	// The command exiting zero means the node accepted the transaction into the
	// mempool. It says nothing about whether the transaction did anything: a
	// swap below its slippage floor, a payment from an unapproved participant
	// and a spend over a treasury's limit all broadcast cleanly and then fail in
	// the block. Reporting that as success is how a bot ends up logging "ok"
	// for hours while the chain records nothing but failures — which is exactly
	// the reassuring, useless output an activity bot must not produce.
	hash := txHash(out)
	if hash == "" {
		log.Printf("[%s] from=%s to=%s broadcast, no tx hash in output: %s",
			action.Name, from, to, strings.TrimSpace(string(out)))
		return
	}

	code, rawLog := deliveredResult(ctx, cfg, hash)
	switch {
	case code == 0:
		log.Printf("[%s] from=%s to=%s ok: %s", action.Name, from, to, hash)
	case code < 0:
		log.Printf("[%s] from=%s to=%s broadcast %s, result unknown: %s", action.Name, from, to, hash, rawLog)
	default:
		log.Printf("[%s] from=%s to=%s FAILED in block: %s code=%d: %s", action.Name, from, to, hash, code, rawLog)
	}
}

// txHash pulls the hash out of the CLI's response, in either output format.
func txHash(out []byte) string {
	var jsonResp struct {
		TxHash string `json:"txhash"`
	}
	if err := json.Unmarshal(out, &jsonResp); err == nil && jsonResp.TxHash != "" {
		return jsonResp.TxHash
	}

	for _, line := range strings.Split(string(out), "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "txhash:"); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// deliveredResult asks the chain what the transaction actually did.
//
// Returns a negative code when the answer could not be obtained — usually
// because the transaction has not been included yet. That is reported as
// unknown rather than as either success or failure, because a bot that guesses
// in either direction is worse than one that says it does not know.
func deliveredResult(ctx context.Context, cfg Config, hash string) (int, string) {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return -1, "shutting down"
		case <-time.After(2 * time.Second):
		}

		args := []string{"query", "tx", hash, "--output", "json"}
		args = append(args, globalFlags(cfg)...)

		queryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		out, err := exec.CommandContext(queryCtx, cfg.Binary, args...).Output()
		cancel()
		if err != nil {
			continue // not indexed yet
		}

		var resp struct {
			Code   int    `json:"code"`
			RawLog string `json:"raw_log"`
		}
		if err := json.Unmarshal(out, &resp); err != nil {
			continue
		}
		return resp.Code, strings.TrimSpace(resp.RawLog)
	}
	return -1, "not included within 20s"
}

func pickWeightedAction(actions []Action) Action {
	total := 0
	for _, a := range actions {
		if a.Weight <= 0 {
			a.Weight = 1
		}
		total += a.Weight
	}
	r := rand.Intn(total)
	for _, a := range actions {
		w := a.Weight
		if w <= 0 {
			w = 1
		}
		if r < w {
			return a
		}
		r -= w
	}
	return actions[len(actions)-1]
}
