package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Jeweler is what the API needs to know about one, read from the chain's REST
// API, which encodes ids as strings and the status as its enum name.
type Jeweler struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Business string   `json:"business"`
	Staff    []string `json:"staff"`
	Status   string   `json:"status"`
}

func (j Jeweler) Active() bool { return j.Status == "JEWELER_ACTIVE" }

// Chain is the one thing the API asks the chain. It is asked on every write
// rather than cached, so removing a staff member takes effect at once.
type Chain interface {
	Jewelers(ctx context.Context) ([]Jeweler, error)
}

type restChain struct {
	base   string
	client *http.Client
}

func (c restChain) Jewelers(ctx context.Context) ([]Jeweler, error) {
	var all []Jeweler
	key := ""
	for page := 0; page < 100; page++ {
		u := c.base + "/yamale/blockchain/jewel/v1/jewelers?pagination.limit=200"
		if key != "" {
			u += "&pagination.key=" + url.QueryEscape(key)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		res, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("the chain did not answer: %w", err)
		}
		var body struct {
			Jewelers   []Jeweler `json:"jewelers"`
			Pagination struct {
				NextKey string `json:"next_key"`
			} `json:"pagination"`
		}
		err = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("the chain answered %d for the jewelers", res.StatusCode)
		}
		if err != nil {
			return nil, fmt.Errorf("unreadable jewelers: %w", err)
		}
		all = append(all, body.Jewelers...)
		if body.Pagination.NextKey == "" {
			return all, nil
		}
		key = body.Pagination.NextKey
	}
	return nil, errors.New("more than 100 pages of jewelers")
}

// jewelerOf is the active jeweler for which address is the business or staff.
func jewelerOf(js []Jeweler, address string) (Jeweler, bool) {
	for _, j := range js {
		if !j.Active() {
			continue
		}
		if j.Business == address {
			return j, true
		}
		for _, s := range j.Staff {
			if s == address {
				return j, true
			}
		}
	}
	return Jeweler{}, false
}

func jewelerByID(js []Jeweler, id string) (Jeweler, bool) {
	for _, j := range js {
		if j.ID == id {
			return j, true
		}
	}
	return Jeweler{}, false
}
