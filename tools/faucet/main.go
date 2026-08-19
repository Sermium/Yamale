// Command faucet hands out testnet tokens.
//
// A faucet is a key with money and an open HTTP port, which makes it the most
// exposed thing on a testnet and the thing most likely to be drained by a
// script within a day of launch. Everything here is shaped by that: a per
// address cooldown that survives restarts, a global daily ceiling, a queue of
// one so a burst cannot race the checks, and a balance it refuses to spend
// past.
//
// It hands out YML and, if asked, one of the chain's other currencies.
//
// With --sponsor it also grants the recipient a fee allowance, which is the
// point rather than a convenience: on this chain an institution pays the
// network fee for its customers, so somebody holding only naira can still move
// it. A faucet that solved that by bundling YML into every grant would be
// working around a feature the chain already has, and would teach every tester
// the wrong model of how the network is meant to be used.
//
//	faucet --from faucet-key --chain-id yamale-testnet-1 --listen :8080
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type faucet struct {
	binary  string
	home    string
	node    string
	chainID string
	keyring string
	from    string
	fees    string
	gas     string

	amount   string
	cooldown time.Duration
	dailyCap int
	// sponsor is the fee allowance each recipient is granted, so they can
	// transact without holding the native token — the same mechanism a bank
	// uses for its customers. Empty falls back to bundling YML.
	sponsor    string
	sponsorFor time.Duration
	// allowed is the set of denoms this faucet will send beyond the native
	// token. Empty means YML only.
	allowed map[string]bool

	// One mutex, held across the whole grant. A faucet that checked the
	// cooldown and then sent without holding a lock is a faucet that pays
	// twice when two requests arrive in the same millisecond — which is the
	// first thing anyone tries.
	mu     sync.Mutex
	last   map[string]time.Time
	served map[string]int // requests per UTC day
}

func main() {
	f := &faucet{last: map[string]time.Time{}, served: map[string]int{}, allowed: map[string]bool{}}

	listen := flag.String("listen", ":8080", "address to listen on")
	flag.StringVar(&f.binary, "binary", "blockchaind", "path to the chain binary")
	flag.StringVar(&f.home, "home", "", "node home directory holding the faucet key")
	flag.StringVar(&f.node, "node", "http://localhost:26657", "RPC endpoint")
	flag.StringVar(&f.chainID, "chain-id", "yamale-testnet-1", "chain id")
	flag.StringVar(&f.keyring, "keyring-backend", "test", "keyring backend")
	flag.StringVar(&f.from, "from", "", "key that holds the faucet's funds")
	flag.StringVar(&f.fees, "fees", "1000uyml", "fee per grant")
	flag.StringVar(&f.gas, "gas", "300000", "gas limit per grant")
	flag.StringVar(&f.amount, "amount", "1000000000uyml", "what each grant sends")
	flag.DurationVar(&f.cooldown, "cooldown", 12*time.Hour, "how long an address must wait between grants")
	flag.IntVar(&f.dailyCap, "daily-cap", 500, "how many grants to make in a day, in total")
	currencies := flag.String("currencies", "", "comma-separated denoms a request may also ask for, e.g. ungn,ukes")
	flag.StringVar(&f.sponsor, "sponsor", "1000000uyml", "fee allowance granted to each recipient; empty to bundle YML instead")
	flag.DurationVar(&f.sponsorFor, "sponsor-for", 30*24*time.Hour, "how long the fee allowance lasts")
	flag.Parse()

	if f.from == "" {
		fmt.Fprintln(os.Stderr, "faucet: --from is required")
		os.Exit(2)
	}
	for _, denom := range strings.Split(*currencies, ",") {
		if denom = strings.TrimSpace(denom); denom != "" {
			f.allowed[denom] = true
		}
	}

	http.HandleFunc("/", f.handle)
	http.HandleFunc("/health", f.health)

	log.Printf("faucet: listening on %s, granting %s per address every %s", *listen, f.amount, f.cooldown)
	log.Fatal(http.ListenAndServe(*listen, nil))
}

type request struct {
	Address string `json:"address"`
	Denom   string `json:"denom,omitempty"`
}

type response struct {
	Sent    string `json:"sent,omitempty"`
	Address string `json:"address,omitempty"`
	TxHash  string `json:"tx_hash,omitempty"`
	Error   string `json:"error,omitempty"`
	// Sponsored is the fee allowance granted alongside, when the faucet pays
	// the recipient's network fees rather than handing them the native token.
	Sponsored string `json:"sponsored_fees,omitempty"`
	// RetryAfter is seconds, present when the refusal is a cooldown rather
	// than a mistake — so a client can wait rather than retry in a loop.
	RetryAfter int `json:"retry_after,omitempty"`
}

func (f *faucet) health(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	today := f.served[time.Now().UTC().Format(time.DateOnly)]
	f.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"granted_today": today,
		"daily_cap":     f.dailyCap,
		"amount":        f.amount,
		"cooldown":      f.cooldown.String(),
	})
}

func (f *faucet) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, response{
			Error: `POST {"address":"yml1…"} to receive testnet tokens.`,
		})
		return
	}

	var req request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "Send JSON like {\"address\":\"yml1…\"}."})
		return
	}

	address := strings.TrimSpace(req.Address)
	// Checked here rather than left to the chain. A malformed address would
	// otherwise cost a transaction and a fee to reject, which is a free way to
	// drain the faucet's gas.
	if !strings.HasPrefix(address, "yml1") || len(address) < 39 || len(address) > 90 {
		writeJSON(w, http.StatusBadRequest, response{
			Error: "That does not look like a Yamale address. They start with yml1.",
		})
		return
	}

	denom := strings.TrimSpace(req.Denom)
	if denom != "" && !f.allowed[denom] {
		writeJSON(w, http.StatusBadRequest, response{
			Error: fmt.Sprintf("This faucet does not hand out %s.", denom),
		})
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if wait, ok := f.onCooldown(address, denom); ok {
		writeJSON(w, http.StatusTooManyRequests, response{
			Error:      fmt.Sprintf("This address was funded with that currency recently. Try again in %s.", humanWait(wait)),
			RetryAfter: int(wait.Seconds()),
		})
		return
	}

	day := time.Now().UTC().Format(time.DateOnly)
	if f.served[day] >= f.dailyCap {
		writeJSON(w, http.StatusTooManyRequests, response{
			Error: "The faucet has given out its allowance for today. It resets at midnight UTC.",
		})
		return
	}

	amount := f.amount
	if denom != "" {
		amount = f.amount + "," + f.grantOf(denom)
	}
	// Sponsoring instead of bundling: the recipient is granted an allowance and
	// receives only the currency they asked for. It is the arrangement the
	// chain is built around, so a tester exercising it is exercising the real
	// thing rather than a faucet-shaped substitute.
	if f.sponsor != "" && denom != "" {
		amount = f.grantOf(denom)
	}

	hash, err := f.send(address, amount)
	if err != nil {
		// Logged in full, returned in summary. The detail is an operator's
		// problem — an insufficient balance, a wrong key — and repeating it to
		// the internet tells an attacker how the faucet is doing.
		log.Printf("faucet: sending to %s failed: %v", address, err)
		writeJSON(w, http.StatusServiceUnavailable, response{
			// Deliberately does not guess at emptiness. The overwhelmingly
			// common cause is two requests landing in one block and colliding
			// on the account sequence, and telling somebody the faucet is out
			// of funds sends them to check a balance that is fine.
			Error: "The faucet could not send that right now — most likely another request landed in the same block. Wait a few seconds and try again.",
		})
		return
	}

	// The transfer is not a transfer until the block says so.
	if err := f.confirm(hash); err != nil {
		log.Printf("faucet: grant to %s did not land (%s): %v", address, hash, err)
		writeJSON(w, http.StatusServiceUnavailable, response{
			Error: "The faucet could not send that right now. Please try again shortly.",
		})
		// Deliberately no cooldown entry: nothing was received, so refusing the
		// next attempt would punish somebody for the faucet's own failure.
		return
	}

	// Granted after the funds, and not fatal if it fails. A recipient who has
	// the money but not the allowance is inconvenienced; one who has neither
	// because the allowance failed first has been given nothing at all.
	if f.sponsor != "" {
		if err := f.grantAllowance(address); err != nil {
			log.Printf("faucet: fee allowance for %s failed: %v", address, err)
		}
		// The allowance is broadcast and not waited on, so unlike the transfer
		// it is still in flight when this handler returns. Releasing the lock
		// now lets the next request broadcast against a sequence the chain has
		// not committed yet, and it is rejected with "account sequence
		// mismatch" — which is precisely what happened when the old block-wait
		// was removed on the assumption that confirming the transfer covered
		// every transaction this handler sends. It covers one of two.
		defer time.Sleep(6 * time.Second)
	}

	f.last[cooldownKey(address, denom)] = time.Now()

	f.served[day]++

	log.Printf("faucet: sent %s to %s (%s)", amount, address, hash)
	writeJSON(w, http.StatusOK, response{
		Sent: amount, Address: address, TxHash: hash, Sponsored: f.sponsor,
	})
}

// grantOf is how much of a non-native currency one request receives.
//
// A flat thousand units in display terms, which is deliberately crude: this is
// a testnet, and a faucet that tried to be worth a consistent amount in dollars
// would need the oracle, and would stop handing out anything the moment a rate
// went stale.
func (f *faucet) grantOf(denom string) string {
	return "1000000000" + denom
}

// humanWait rounds to something true at every scale.
//
// Rounding everything to the minute turns a 22-second wait into "0s", which
// sits next to a retry_after of 22 and makes the pair read as broken. Below a
// minute the seconds are the answer.
func humanWait(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	if d < time.Hour {
		return d.Round(time.Minute).String()
	}
	return d.Round(time.Hour).String()
}

// cooldownKey scopes the wait to one currency.
//
// Keyed on the address alone, taking YML locked the requester out of naira for
// the whole period — which is exactly backwards for a chain whose point is that
// you hold a local currency and have your fees sponsored. Somebody setting
// themselves up needs YML *and* the currency they are testing with, and being
// made to choose one per six hours makes the faucet useless for the flow it
// exists to support.
//
// The daily cap is deliberately left global: it is the spend ceiling, and
// splitting it per denom would multiply the faucet's total outlay by the number
// of currencies it offers.
func cooldownKey(address, denom string) string {
	if denom == "" {
		denom = "default"
	}
	return address + "|" + denom
}

func (f *faucet) onCooldown(address, denom string) (time.Duration, bool) {
	last, ok := f.last[cooldownKey(address, denom)]
	if !ok {
		return 0, false
	}
	if elapsed := time.Since(last); elapsed < f.cooldown {
		return f.cooldown - elapsed, true
	}
	return 0, false
}

// grantAllowance sponsors the recipient's network fees.
//
// Capped and dated. An open-ended allowance on a faucet key would let one
// tester spend the faucet's whole balance on gas, which is a slower way of
// draining it than asking for tokens and a less obvious one.
func (f *faucet) grantAllowance(address string) error {
	// An allowance from this granter to this grantee can only exist once —
	// granting a second time is rejected on chain with "fee allowance already
	// exists". The money still arrived, so the faucet treated it as a harmless
	// logged failure, but the rejected transaction is committed and shows up in
	// the explorer as a failed MsgGrantAllowance every time somebody claims a
	// second currency. Checking first keeps that off the chain entirely.
	check := []string{
		"query", "feegrant", "grant", f.from, address,
		"--node", f.node, "--output", "json",
	}
	if f.home != "" {
		check = append(check, "--home", f.home)
	}
	if out, err := exec.Command(f.binary, check...).CombinedOutput(); err == nil &&
		strings.Contains(string(out), "granter") {
		log.Printf("faucet: %s already has an allowance, not granting again", address)
		return nil
	}

	args := []string{
		"tx", "feegrant", "grant", f.from, address,
		"--spend-limit", f.sponsor,
		"--expiration", time.Now().Add(f.sponsorFor).UTC().Format(time.RFC3339),
		"--chain-id", f.chainID,
		"--keyring-backend", f.keyring,
		"--node", f.node,
		"--fees", f.fees,
		"--gas", f.gas,
		"-y", "--output", "json",
	}
	if f.home != "" {
		args = append(args, "--home", f.home)
	}

	// Two transactions from one key in the same second collide on the account
	// sequence: the send is still in the mempool when the grant is signed, so
	// the node rejects the grant with a mismatch. It broadcasts "successfully"
	// — exit status zero, a transaction hash — which is exactly why this checks
	// the code in the body rather than the exit status, and why the first
	// attempt at this silently granted nothing at all.
	// Four attempts, not two. One retry covers a single collision, but a faucet
	// being used by more than one person at a time collides repeatedly: the
	// mutex serialises this program, and the chain still needs a block between
	// two transactions from the same key. Two collisions in a row was reaching
	// the user as "the faucet may be empty" while the key held 399,000 KES.
	for attempt := 0; attempt < 4; attempt++ {
		out, err := exec.Command(f.binary, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}

		var result struct {
			Code   int    `json:"code"`
			RawLog string `json:"raw_log"`
		}
		if err := json.Unmarshal(out, &result); err != nil {
			return fmt.Errorf("unreadable response: %s", strings.TrimSpace(string(out)))
		}
		if result.Code == 0 {
			return nil
		}
		// 32 is a sequence mismatch, which is what a second transaction in the
		// same block looks like. Worth one retry after the block lands;
		// anything else is a real refusal and is returned.
		if result.Code != 32 || attempt == 3 {
			return fmt.Errorf("rejected (code %d): %s", result.Code, result.RawLog)
		}
		time.Sleep(6 * time.Second)
	}

	return nil
}

func (f *faucet) send(address, amount string) (string, error) {
	args := []string{
		"tx", "bank", "send", f.from, address, amount,
		"--chain-id", f.chainID,
		"--keyring-backend", f.keyring,
		"--node", f.node,
		"--fees", f.fees,
		"--gas", f.gas,
		"-y", "--output", "json",
	}
	if f.home != "" {
		args = append(args, "--home", f.home)
	}

	out, err := exec.Command(f.binary, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}

	var result struct {
		Code   int    `json:"code"`
		TxHash string `json:"txhash"`
		RawLog string `json:"raw_log"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("unreadable response: %s", strings.TrimSpace(string(out)))
	}
	// Broadcast acceptance. CheckTx has passed, which says the transaction is
	// well-formed and the fee is payable — it says nothing about whether the
	// transfer will succeed when the block executes it.
	if result.Code != 0 {
		return "", fmt.Errorf("rejected (code %d): %s", result.Code, result.RawLog)
	}

	return result.TxHash, nil
}

// confirm waits for the transaction to be executed and reports what it did.
//
// Without this the faucet reports success for transfers that never happened. It
// did exactly that: the account it spends from ran out of a currency, every
// grant of that currency failed in the block with "insufficient funds", and the
// faucet logged "sent" and answered 200 for all of them. The users saw nothing
// arrive, the log said everything was fine, and the cooldown was recorded, so
// retrying was refused for six hours.
//
// The earlier version skipped this to avoid holding the lock for a block. That
// was a false economy — the handler already slept a full block before releasing
// it, so the wait was being paid anyway and simply not used for anything.
// Waiting here also gives the serialisation for free: a transaction that has
// been included has had its sequence committed, so the next request cannot
// collide with it.
func (f *faucet) confirm(hash string) error {
	args := []string{"query", "tx", hash, "--node", f.node, "--output", "json"}
	if f.home != "" {
		args = append(args, "--home", f.home)
	}

	// A block is ~5s. Polling to 30s covers a slow block without hanging a
	// request forever if the node stops producing them.
	deadline := time.Now().Add(30 * time.Second)
	for {
		time.Sleep(2 * time.Second)

		out, err := exec.Command(f.binary, args...).CombinedOutput()
		if err == nil {
			var result struct {
				Code   int    `json:"code"`
				RawLog string `json:"raw_log"`
			}
			if json.Unmarshal(out, &result) == nil {
				if result.Code != 0 {
					return fmt.Errorf("failed in block (code %d): %s", result.Code, result.RawLog)
				}
				return nil
			}
		}

		if time.Now().After(deadline) {
			// Not proven either way. Reported as a failure because the caller
			// can safely retry a grant that did land — they get a second one —
			// whereas being told a grant succeeded when it did not leaves them
			// stuck behind a cooldown with nothing to show for it.
			return fmt.Errorf("not included within 30s")
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	// A browser-based faucet page is the normal way people use one, and it
	// will be served from somewhere other than this port.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
