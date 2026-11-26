package main

// Owners never hold coins. Each account is granted a fee allowance once,
// limited to the messages an owner sends, so an account cannot spend the
// platform's money on anything else.
//
// The grant is signed by the chain binary, as tools/faucet does it: a key in
// a keyring the binary already knows how to protect, rather than a second
// signing implementation in this process. This is the only key traceability-api
// uses, and it can only grant fees from its own balance.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	"/blockchain.jewel.v1.MsgFileClaim", "/blockchain.jewel.v1.MsgWithdrawClaim",
	"/blockchain.jewel.v1.MsgListJewel", "/blockchain.jewel.v1.MsgUnlistJewel",
	"/blockchain.jewel.v1.MsgBuyJewel",
}

// ownerList numbers the version of ownerMessages. An allowance is fixed when it
// is granted, so an account granted an older list cannot pay for a message
// added since: it is re-granted once, the old allowance revoked first.
//
//	1: offers, theft reports, reassignment (until 2026-11-25)
//	2: + filing and withdrawing a claim on an ownership card
//	3: + listing, unlisting and buying on the marketplace (2026-11-25)
const ownerList = 3

var errAlreadyGranted = errors.New("this account already has a fee allowance from the sponsor")

type Granter interface {
	// Grant gives grantee an allowance limited to ownerMessages and returns
	// the transaction hash once a block has committed it.
	Grant(ctx context.Context, grantee string) (string, error)
	// Regrant revokes grantee's allowance and grants the current list.
	Regrant(ctx context.Context, grantee string) (string, error)
	// Address is the account that pays.
	Address() string
}

type sponsorRecord struct {
	Address string `json:"address"`
	Tx      string `json:"tx"`
	At      int64  `json:"at"`
	// List is the ownerList version granted; 0 in records written before it
	// existed, which means version 1.
	List int `json:"list,omitempty"`
	// Welcome is the transaction that paid the devnet welcome, if one did.
	Welcome string `json:"welcome,omitempty"`
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
		serverFault(w, err)
		return
	}
	since := s.now().Add(-24 * time.Hour).Unix()
	today := 0
	for i, rec := range ledger {
		if rec.Address == address {
			if rec.List >= ownerList {
				writeJSON(w, http.StatusOK, map[string]any{"granted": false, "already": true, "granter": s.granter.Address()})
				return
			}
			s.regrant(w, r, ledger, i)
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
	if errors.Is(err, errAlreadyGranted) {
		// An allowance this ledger does not know of: its list is unknown, so it
		// is replaced rather than trusted.
		tx, err = s.granter.Regrant(r.Context(), address)
	}
	if err != nil {
		log.Printf("traceabilityapi: grant failed: %v", err)
		refuse(w, http.StatusBadGateway, "the grant did not go through; try again later")
		return
	}
	rec := sponsorRecord{Address: address, Tx: tx, At: s.now().Unix(), List: ownerList}
	// The welcome is a convenience: an account sponsored without it is still
	// sponsored, so a failed payment is logged rather than refused.
	if s.welcome != "" && s.sender != nil {
		if wtx, werr := s.sender.Send(r.Context(), address, s.welcome); werr != nil {
			log.Printf("traceabilityapi: welcome to %s failed: %v", address, werr)
		} else {
			rec.Welcome = wtx
		}
	}
	ledger = append(ledger, rec)
	out, _ := json.MarshalIndent(ledger, "", " ")
	if err := writeAtomically(s.ledgerPath(), out); err != nil {
		serverFault(w, err)
		return
	}
	body := map[string]any{"granted": true, "tx": tx, "granter": s.granter.Address()}
	if rec.Welcome != "" {
		body["welcome"] = rec.Welcome
	}
	writeJSON(w, http.StatusOK, body)
}

// regrant widens the allowance of the account at ledger[i]. It does not count
// toward the daily ceiling: that bounds new accounts, and each account is
// re-granted at most once per list version.
func (s *server) regrant(w http.ResponseWriter, r *http.Request, ledger []sponsorRecord, i int) {
	tx, err := s.granter.Regrant(r.Context(), ledger[i].Address)
	if err != nil {
		log.Printf("traceabilityapi: re-grant failed: %v", err)
		refuse(w, http.StatusBadGateway, "the grant did not go through; try again later")
		return
	}
	ledger[i].Tx, ledger[i].List = tx, ownerList
	out, _ := json.MarshalIndent(ledger, "", " ")
	if err := writeAtomically(s.ledgerPath(), out); err != nil {
		serverFault(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"granted": true, "regranted": true, "tx": tx, "granter": s.granter.Address()})
}

type cliGranter struct {
	bin, from, address, home, keyring, node, chainID, spendLimit, fees string
	lifetime                                                           time.Duration
	now                                                                func() time.Time
	run                                                                func(ctx context.Context, name string, args ...string) ([]byte, error)
	poll                                                               time.Duration
}

// runCommand returns the command's stdout alone, so a warning on stderr cannot
// corrupt the JSON the caller parses. Stderr rides along in the error.
func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			err = fmt.Errorf("%w: %s", err, msg)
		}
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
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
	args := []string{
		"tx", "feegrant", "grant", g.from, grantee,
		"--spend-limit", g.spendLimit,
		"--expiration", g.now().Add(g.lifetime).UTC().Format(time.RFC3339),
		"--allowed-messages", strings.Join(ownerMessages, ","),
		"--chain-id", g.chainID, "--keyring-backend", g.keyring,
	}
	// An empty --home would be taken literally by the SDK, so it is left out.
	if g.home != "" {
		args = append(args, "--home", g.home)
	}
	return append(args, "--node", g.node,
		"--fees", g.fees, "--gas", "250000", "-y", "--output", "json")
}

func (g cliGranter) revokeArgs(grantee string) []string {
	args := []string{"tx", "feegrant", "revoke", g.from, grantee,
		"--chain-id", g.chainID, "--keyring-backend", g.keyring}
	if g.home != "" {
		args = append(args, "--home", g.home)
	}
	return append(args, "--node", g.node,
		"--fees", g.fees, "--gas", "250000", "-y", "--output", "json")
}

// Regrant revokes, waits for the block, then grants: a grant in the same block
// as the revoke would be refused as already existing. A revoke that finds no
// allowance is not an error, since the end state is the same.
func (g cliGranter) Regrant(ctx context.Context, grantee string) (string, error) {
	if _, err := g.commit(ctx, g.revokeArgs(grantee)); err != nil && !strings.Contains(err.Error(), "not found") {
		return "", fmt.Errorf("revoking the old allowance: %w", err)
	}
	return g.Grant(ctx, grantee)
}

// Grant broadcasts, then waits for the block. A code of 0 from the broadcast
// is only CheckTx: the grant can still fail when the block runs it.
func (g cliGranter) Grant(ctx context.Context, grantee string) (string, error) {
	return g.commit(ctx, g.grantArgs(grantee))
}

// commit broadcasts one transaction and waits until a block has run it.
func (g cliGranter) commit(ctx context.Context, args []string) (string, error) {
	out, err := g.run(ctx, g.bin, args...)
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
