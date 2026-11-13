package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
)

// BalanceReader reads one account's balance of one denom.
type BalanceReader interface {
	Balance(ctx context.Context, address, denom string) (*big.Int, error)
}

func (c restChain) Balance(ctx context.Context, address, denom string) (*big.Int, error) {
	u := c.base + "/cosmos/bank/v1beta1/balances/" + url.PathEscape(address) + "/by_denom?denom=" + url.QueryEscape(denom)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the chain did not answer: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the chain answered %d for a balance", res.StatusCode)
	}
	var body struct {
		Balance struct {
			Amount string `json:"amount"`
		} `json:"balance"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("unreadable balance: %w", err)
	}
	n, ok := new(big.Int).SetString(body.Balance.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("unreadable amount %q", body.Balance.Amount)
	}
	return n, nil
}

var denomPattern = regexp.MustCompile(`^[a-z][a-z0-9/]{2,127}$`)

// splitCoin parses a coin such as "10000000uyml" into its amount and denom.
// It refuses an empty amount, a missing denom, a sign or any non-digit before
// the denom, and a denom outside the chain's pattern.
func splitCoin(s string) (*big.Int, string, error) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return nil, "", fmt.Errorf("coin %q has no amount", s)
	}
	denom := s[i:]
	if !denomPattern.MatchString(denom) {
		return nil, "", fmt.Errorf("coin %q has no valid denom", s)
	}
	n, ok := new(big.Int).SetString(s[:i], 10)
	if !ok {
		return nil, "", fmt.Errorf("coin %q has an unreadable amount", s)
	}
	return n, denom, nil
}

// handleSponsorHealth lets the monitor alert before the sponsor runs dry: a
// sponsor that cannot pay leaves every new owner unable to accept a jewel.
func (s *server) handleSponsorHealth(w http.ResponseWriter, r *http.Request) {
	if s.granter == nil {
		writeJSON(w, http.StatusOK, map[string]any{"sponsoring": false})
		return
	}
	have, err := s.balances.Balance(r.Context(), s.granter.Address(), s.sponsorDenom)
	if err != nil {
		chainUnreachable(w, err)
		return
	}
	body := map[string]any{"ok": have.Cmp(s.sponsorFloor) >= 0, "balance": have.String() + s.sponsorDenom,
		"floor": s.sponsorFloor.String() + s.sponsorDenom}
	if have.Cmp(s.sponsorFloor) < 0 {
		writeJSON(w, http.StatusServiceUnavailable, body)
		return
	}
	writeJSON(w, http.StatusOK, body)
}
