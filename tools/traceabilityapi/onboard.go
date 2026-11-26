package main

// Onboarding, so that an account can hold an x/alias identifier under the
// module's own rules. An identifier needs a recorded country, and the first
// recording must come from the approved participant that registered the
// account as its customer. On devnet the sponsor account is that participant
// (YAMALE-ONBOARD, approved by governance), and the country is the one the
// person chose at signup.
//
// That is why this is refused on any chain that does not name itself a devnet
// or a local chain: there nobody checked who the person is, and the country
// is only their word. On mainnet the participant is whoever performs the
// identity check, and the country comes from that check.
//
// The account then claims its identifier itself (MsgRegisterAlias, sponsored),
// in the same transaction as it confirms the participant.

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Onboarder registers an account as the participant's customer and records its
// country. Both return the committed transaction hash.
type Onboarder interface {
	RegisterCustomer(ctx context.Context, customer string) (string, error)
	SetJurisdiction(ctx context.Context, account, country string) (string, error)
}

type onboardRecord struct {
	Address  string `json:"address"`
	Country  string `json:"country"`
	Customer string `json:"customer_tx"`
	Recorded string `json:"jurisdiction_tx"`
	At       int64  `json:"at"`
}

func (s *server) onboardPath() string { return filepath.Join(s.dir, "onboarded.json") }

func (s *server) readOnboarded() ([]onboardRecord, error) {
	raw, err := os.ReadFile(s.onboardPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []onboardRecord
	return out, json.Unmarshal(raw, &out)
}

// POST /v1/onboard {"country": "SN"}, signed by the account over
// "<address>:<country>".
func (s *server) handleOnboard(w http.ResponseWriter, r *http.Request) {
	if s.onboarder == nil {
		refuse(w, http.StatusServiceUnavailable, "accounts are not onboarded here")
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	var in struct {
		Country string `json:"country"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&in); err != nil {
		refuse(w, http.StatusBadRequest, `the body is {"country": "SN"}`)
		return
	}
	country := strings.ToUpper(strings.TrimSpace(in.Country))
	if !countryPattern.MatchString(country) {
		refuse(w, http.StatusBadRequest, "country is a two-letter ISO 3166 code")
		return
	}
	address, err := s.signer(r, "onboard", func(a string) string { return a + ":" + country })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	ledger, err := s.readOnboarded()
	if err != nil {
		serverFault(w, err)
		return
	}
	since := s.now().Add(-24 * time.Hour).Unix()
	today := 0
	for _, rec := range ledger {
		if rec.Address == address {
			writeJSON(w, http.StatusOK, map[string]any{"already": true, "country": rec.Country, "participant": s.granter.Address()})
			return
		}
		if rec.At >= since {
			today++
		}
	}
	if s.sponsorDaily > 0 && today >= s.sponsorDaily {
		w.Header().Set("Retry-After", "3600")
		refuse(w, http.StatusTooManyRequests, "today's onboardings are used up; try again in an hour")
		return
	}

	customerTx, err := s.onboarder.RegisterCustomer(r.Context(), address)
	if err != nil {
		log.Printf("traceabilityapi: registering %s as a customer failed: %v", address, err)
		refuse(w, http.StatusBadGateway, "the registration did not go through; try again later")
		return
	}
	recordTx, err := s.onboarder.SetJurisdiction(r.Context(), address, country)
	if err != nil && !strings.Contains(err.Error(), "is recorded in") {
		log.Printf("traceabilityapi: recording %s for %s failed: %v", country, address, err)
		refuse(w, http.StatusBadGateway, "the country could not be recorded; try again later")
		return
	}
	ledger = append(ledger, onboardRecord{Address: address, Country: country, Customer: customerTx, Recorded: recordTx, At: s.now().Unix()})
	out, _ := json.MarshalIndent(ledger, "", " ")
	if err := writeAtomically(s.onboardPath(), out); err != nil {
		serverFault(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"onboarded": true, "country": country, "participant": s.granter.Address()})
}

func (g cliGranter) txArgs(parts ...string) []string {
	args := append([]string{"tx"}, parts...)
	args = append(args, "--from", g.from, "--chain-id", g.chainID, "--keyring-backend", g.keyring)
	if g.home != "" {
		args = append(args, "--home", g.home)
	}
	return append(args, "--node", g.node, "--fees", g.fees, "--gas", "250000", "-y", "--output", "json")
}

// RegisterCustomer claims customer as the sponsor's, as an approved participant.
func (g cliGranter) RegisterCustomer(ctx context.Context, customer string) (string, error) {
	return g.commit(ctx, g.txArgs("paymsg", "register-customer", customer, "true"))
}

// SetJurisdiction records the account's country, as the participant that registered it.
func (g cliGranter) SetJurisdiction(ctx context.Context, account, country string) (string, error) {
	return g.commit(ctx, g.txArgs("alias", "set-jurisdiction", account, country))
}
