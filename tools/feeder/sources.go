package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Where prices come from.
//
// Deliberately more than one, and the choice is a flag rather than a constant.
// The oracle takes a stake-weighted median across validators, and a median of
// four reports drawn from the same endpoint is one source counted four times —
// the threshold looks like agreement and measures nothing. Operators are meant
// to run different sources on purpose.
//
// Each source answers one question: how many units of this currency does one US
// dollar buy? Everything else — inverting to the chain's convention, filling in
// pegs — happens above.

// Source fetches USD-based rates for the codes it is asked about.
type Source interface {
	// Name is recorded alongside the submission so a disagreement between
	// validators can be diagnosed rather than argued about.
	Name() string
	// Fetch returns units-per-USD, keyed by ISO code. A code the source does
	// not know is absent rather than zero: a missing rate must never look like
	// a currency that has become worthless.
	Fetch(codes []string) (map[string]float64, error)
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// ---------------------------------------------------------------- er-api

// erAPI is exchangerate-api's free tier. No key, every African currency, but
// updated once a day — so a rate it reports as current may be a day old
// upstream while the chain considers it fresh. Fine for a testnet, and stated
// plainly rather than glossed over.
type erAPI struct{}

func (erAPI) Name() string { return "open.er-api.com" }

func (erAPI) Fetch(_ []string) (map[string]float64, error) {
	resp, err := httpClient.Get("https://open.er-api.com/v6/latest/USD")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Result != "success" {
		return nil, fmt.Errorf("er-api returned %q", body.Result)
	}
	return body.Rates, nil
}

// ---------------------------------------------------------------- yahoo

// yahoo reads the currency market rather than a published table: intraday, no
// key, one request per pair. It is an unofficial endpoint with no guarantee
// behind it, which is an argument for running it alongside another source, not
// instead of one.
type yahoo struct{}

func (yahoo) Name() string { return "finance.yahoo.com" }

func (y yahoo) Fetch(codes []string) (map[string]float64, error) {
	rates := make(map[string]float64, len(codes))

	for _, code := range codes {
		if code == "USD" {
			rates[code] = 1
			continue
		}
		rate, err := y.one(code)
		if err != nil {
			// One thin pair must not cost us the other forty. The currency is
			// left absent, and the caller decides whether it can proceed.
			continue
		}
		rates[code] = rate
		// Courtesy pacing. An unofficial endpoint hit forty times in a burst is
		// an endpoint that starts refusing.
		time.Sleep(120 * time.Millisecond)
	}

	if len(rates) == 0 {
		return nil, fmt.Errorf("yahoo returned nothing for any of the %d codes", len(codes))
	}
	return rates, nil
}

func (yahoo) one(code string) (float64, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s=X?interval=1d&range=1d", code)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "yamale-feeder/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return 0, fmt.Errorf("%s: HTTP %d", code, resp.StatusCode)
	}

	var body struct {
		Chart struct {
			Result []struct {
				Meta struct {
					RegularMarketPrice float64 `json:"regularMarketPrice"`
				} `json:"meta"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}
	if len(body.Chart.Result) == 0 || body.Chart.Result[0].Meta.RegularMarketPrice <= 0 {
		return 0, fmt.Errorf("%s: no price", code)
	}
	return body.Chart.Result[0].Meta.RegularMarketPrice, nil
}

// ---------------------------------------------------------------- pegs

// Pegs are currencies whose rate is a fact rather than a quote.
//
// The CFA francs are fixed to the euro by treaty; the loti, the Namibian dollar
// and the lilangeni are one-for-one with the rand. Deriving these is more
// robust than quoting them: the pairs are thinly traded, so a market feed for
// XOF/USD is a euro rate with noise added, and the noise is what would end up
// in a median.
type peg struct {
	// Against is the ISO code this currency is fixed to.
	Against string
	// PerUnit is how many of this currency one unit of Against buys.
	PerUnit float64
}

var pegs = map[string]peg{
	"XOF": {"EUR", 655.957},
	"XAF": {"EUR", 655.957},
	"KMF": {"EUR", 491.96775},
	"CVE": {"EUR", 110.265},
	"LSL": {"ZAR", 1},
	"NAD": {"ZAR", 1},
	"SZL": {"ZAR", 1},
	"ERN": {"USD", 15},
}

// applyPegs fills in the fixed currencies from whatever they are fixed to.
//
// It overwrites a fetched value rather than only filling gaps. A source that
// reports XOF at 570 when the euro rate implies 568.4 is not more current, it
// is wrong, and letting it through would put the error into the chain's median
// where it is much harder to see.
func applyPegs(rates map[string]float64, wanted []string) []string {
	var derived []string

	for _, code := range wanted {
		p, ok := pegs[code]
		if !ok {
			continue
		}
		anchor, ok := rates[p.Against]
		if !ok {
			continue
		}
		rates[code] = anchor * p.PerUnit
		derived = append(derived, code)
	}

	return derived
}

// codeOf turns a base denom into the ISO code the sources use: uyml → YML.
func codeOf(denom string) string {
	return strings.ToUpper(strings.TrimPrefix(denom, "u"))
}

func newSource(name string) (Source, error) {
	switch strings.ToLower(name) {
	case "erapi", "er-api":
		return erAPI{}, nil
	case "yahoo", "market":
		return yahoo{}, nil
	default:
		return nil, fmt.Errorf("unknown source %q; try erapi or yahoo", name)
	}
}
