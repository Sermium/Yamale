package main

// Owners never hold coins. Each account is granted a fee allowance once,
// limited to the messages an owner sends, so an account cannot spend the
// platform's money on anything else.
//
// The grant is signed by the chain binary, as tools/faucet does it: a key in
// a keyring the binary already knows how to protect, rather than a second
// signing implementation in this process. This is the only key loyalty-api
// uses, and it can only grant fees from its own balance.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ownerMessages = []string{
	"/blockchain.jewel.v1.MsgOfferJewel", "/blockchain.jewel.v1.MsgCancelOffer",
	"/blockchain.jewel.v1.MsgAcceptOffer", "/blockchain.jewel.v1.MsgClaimOffer",
	"/blockchain.jewel.v1.MsgReportStolen", "/blockchain.jewel.v1.MsgClearStolen",
	"/blockchain.jewel.v1.MsgVetoReassignment", "/blockchain.jewel.v1.MsgCompleteReassignment",
}

var errAlreadyGranted = errors.New("this account already has a fee allowance from the sponsor")

type Granter interface {
	// Grant gives grantee an allowance limited to ownerMessages and returns
	// the transaction hash once a block has committed it.
	Grant(ctx context.Context, grantee string) (string, error)
	// Address is the account that pays.
	Address() string
}

type sponsorRecord struct {
	Address string `json:"address"`
	Tx      string `json:"tx"`
	At      int64  `json:"at"`
}

func (s *server) ledgerPath() string { return filepath.Join(s.dir, "sponsored.json") }

func (s *server) readLedger() ([]sponsorRecord, error) {
	raw, err := os.ReadFile(s.ledgerPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []sponsorRecord
	return recs, json.Unmarshal(raw, &recs)
}

func (s *server) handleSponsorInfo(w http.ResponseWriter, _ *http.Request) {
	if s.granter == nil {
		refuse(w, http.StatusServiceUnavailable, "fees are not sponsored here")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"granter": s.granter.Address()})
}

func (s *server) handleSponsor(w http.ResponseWriter, r *http.Request) {
	if s.granter == nil {
		refuse(w, http.StatusServiceUnavailable, "fees are not sponsored here")
		return
	}
	address, err := s.signer(r, "sponsor", func(a string) string { return a })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}

	// One at a time: the ledger check and the grant must not interleave, or
	// two quick requests both pass the "not yet sponsored" check.
	s.mu.Lock()
	defer s.mu.Unlock()

	ledger, err := s.readLedger()
	if err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	since := s.now().Add(-24 * time.Hour).Unix()
	today := 0
	for _, rec := range ledger {
		if rec.Address == address {
			writeJSON(w, http.StatusOK, map[string]any{"granted": false, "already": true, "granter": s.granter.Address()})
			return
		}
		if rec.At >= since {
			today++
		}
	}
	if today >= s.sponsorDaily {
		w.Header().Set("Retry-After", "3600")
		refuse(w, http.StatusTooManyRequests, "today's sponsorships are used up; try again in an hour")
		return
	}

	tx, err := s.granter.Grant(r.Context(), address)
	if err != nil && !errors.Is(err, errAlreadyGranted) {
		refuse(w, http.StatusBadGateway, "the grant did not go through: "+err.Error())
		return
	}
	ledger = append(ledger, sponsorRecord{Address: address, Tx: tx, At: s.now().Unix()})
	out, _ := json.MarshalIndent(ledger, "", " ")
	if err := writeAtomically(s.ledgerPath(), out); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"granted": true, "tx": tx, "granter": s.granter.Address()})
}

type cliGranter struct {
	bin, from, address, home, keyring, node, chainID, spendLimit, fees string
	lifetime                                                           time.Duration
	now                                                                func() time.Time
	run                                                                func(ctx context.Context, name string, args ...string) ([]byte, error)
	poll                                                               time.Duration
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func newCLIGranter(bin, from, home, keyring, node, chainID, spend, fees string, lifetime time.Duration) (*cliGranter, error) {
	g := &cliGranter{bin: bin, from: from, home: home, keyring: keyring, node: node, chainID: chainID,
		spendLimit: spend, fees: fees, lifetime: lifetime, now: time.Now, run: runCommand, poll: time.Second}
	args := []string{"keys", "show", from, "-a", "--keyring-backend", keyring}
	if home != "" {
		args = append(args, "--home", home)
	}
	out, err := g.run(context.Background(), bin, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	g.address = strings.TrimSpace(string(out))
	return g, nil
}

func (g *cliGranter) Address() string { return g.address }

func (g cliGranter) grantArgs(grantee string) []string {
	return []string{
		"tx", "feegrant", "grant", g.from, grantee,
		"--spend-limit", g.spendLimit,
		"--expiration", g.now().Add(g.lifetime).UTC().Format(time.RFC3339),
		"--allowed-messages", strings.Join(ownerMessages, ","),
		"--chain-id", g.chainID, "--keyring-backend", g.keyring, "--home", g.home, "--node", g.node,
		"--fees", g.fees, "--gas", "250000", "-y", "--output", "json",
	}
}

// Grant broadcasts, then waits for the block. A code of 0 from the broadcast
// is only CheckTx: the grant can still fail when the block runs it.
func (g cliGranter) Grant(ctx context.Context, grantee string) (string, error) {
	out, err := g.run(ctx, g.bin, g.grantArgs(grantee)...)
	var res struct {
		Code   int    `json:"code"`
		TxHash string `json:"txhash"`
		RawLog string `json:"raw_log"`
	}
	if jerr := json.Unmarshal(out, &res); jerr != nil {
		if err == nil {
			err = jerr
		}
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	if res.Code != 0 {
		if strings.Contains(res.RawLog, "fee allowance already exists") {
			return "", errAlreadyGranted
		}
		return "", errors.New(res.RawLog)
	}
	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(g.poll):
		}
		q, qerr := g.run(ctx, g.bin, "query", "tx", res.TxHash, "--node", g.node, "--output", "json")
		if qerr != nil {
			continue // not indexed yet
		}
		var done struct {
			Code   int    `json:"code"`
			RawLog string `json:"raw_log"`
		}
		if json.Unmarshal(q, &done) != nil {
			continue
		}
		if done.Code == 0 {
			return res.TxHash, nil
		}
		if strings.Contains(done.RawLog, "fee allowance already exists") {
			return res.TxHash, errAlreadyGranted
		}
		return "", errors.New(done.RawLog)
	}
	return "", fmt.Errorf("transaction %s was not committed in 30 polls", res.TxHash)
}
