// Package main is the JSON-RPC method gate that sits in front of CometBFT.
//
// # Why this exists rather than an nginx rule
//
// The deny list in yamale-api.conf is an nginx location regex, so it matches on
// the URL PATH. CometBFT accepts the same calls two ways:
//
//	GET  /api/rpc/net_info                    -> 403, the regex sees it
//	POST /api/rpc/  {"method":"net_info"}     -> answered in full
//
// and the second form is the one every CosmJS client uses. So the gate that was
// written to keep dump_consensus_state, unconfirmed_txs, broadcast_tx_commit
// and net_info off the public internet has never blocked any of them. Verified
// against the live funnel on 2026-09-05: all four answer over POST.
//
// nginx cannot read a request body without njs or Lua, and a path allow-list on
// abci_query has exactly the same hole. So the filter has to be a process that
// parses JSON-RPC, and this is it: one allow-list, applied to the method
// whichever way it arrives, in front of a node that is otherwise unchanged.
//
// It is deliberately small and deliberately fails closed. Anything it cannot
// parse, cannot classify, or has not been told about is refused.
package main

import (
	"encoding/json"
	"errors"
	"strings"
)

// allowed is every JSON-RPC method the public may reach.
//
// An allow-list, not a deny list. The deny list this replaces was written
// against the methods somebody had thought of, which is why adding abci_query
// to it would have closed one of the two forms it arrives in and nothing else.
// A method absent from here is refused, so a CometBFT upgrade that adds one
// cannot quietly widen the surface.
//
// The four the old list named are absent, and so is everything else that dumps
// unbounded state, exposes the mempool, holds a connection open, or reconfigures
// the node.
var allowed = map[string]bool{
	// What a client must read to build and sign a transaction.
	"abci_info":  true,
	"abci_query": true,
	"status":     true,
	"health":     true,

	// Blocks and history. The consoles read these directly.
	"block":            true,
	"block_by_hash":    true,
	"block_results":    true,
	"blockchain":       true,
	"commit":           true,
	"header":           true,
	"header_by_hash":   true,
	"consensus_params": true,
	"validators":       true,
	"genesis_chunked":  true,

	// Transactions: submitting one, and finding it afterwards.
	//
	// broadcast_tx_sync returns on mempool acceptance and is what every
	// application here already uses. broadcast_tx_commit is NOT here: it holds
	// the connection until the transaction commits, so a handful of callers
	// exhaust the RPC connection pool while looking like ordinary clients.
	"broadcast_tx_sync":  true,
	"broadcast_tx_async": true,
	"tx":                 true,
	"tx_search":          true,
	"block_search":       true,
}

// maxBody bounds what will be read and parsed before a decision is made.
//
// A JSON-RPC envelope carrying a signed transaction is a few kilobytes; a
// megabyte is generous. The bound matters because the body is buffered in
// memory to be parsed, so without one the gate is itself the denial of service
// it was put there to prevent.
const maxBody = 1 << 20

var (
	// errUnparseable covers a body that is not JSON-RPC at all.
	errUnparseable = errors.New("the request body is not a JSON-RPC call")
	// errEmptyBatch is its own case because an empty array parses fine and
	// names no method, so a naive loop over it would permit everything.
	errEmptyBatch = errors.New("the request is an empty JSON-RPC batch")
)

// envelope is the part of a JSON-RPC request this gate reads. Everything else
// is passed through untouched.
type envelope struct {
	Method string `json:"method"`
}

// methodsOf returns every method named by a request body, single or batched.
//
// Batches are the reason this returns a slice. CometBFT accepts an array of
// calls in one POST, and a filter that read only the first element would let
// any denied method through by putting an allowed one in front of it.
func methodsOf(body []byte) ([]string, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, errUnparseable
	}

	if strings.HasPrefix(trimmed, "[") {
		var batch []envelope
		if err := json.Unmarshal([]byte(trimmed), &batch); err != nil {
			return nil, errUnparseable
		}
		if len(batch) == 0 {
			return nil, errEmptyBatch
		}
		methods := make([]string, 0, len(batch))
		for _, call := range batch {
			methods = append(methods, call.Method)
		}
		return methods, nil
	}

	var single envelope
	if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
		return nil, errUnparseable
	}
	return []string{single.Method}, nil
}

// methodOfPath reads the method from a GET-form URL, e.g. /status or
// /abci_query?path=...
//
// The path form is already refused by the nginx regex for the four methods it
// knows about. It is checked here as well because a filter that trusts a rule
// somewhere else to still be correct is how the original hole was made.
func methodOfPath(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return ""
	}
	if i := strings.IndexByte(trimmed, '/'); i >= 0 {
		// CometBFT has no nested RPC paths, so anything with a second segment
		// is not a method call and will not be classified as one.
		return ""
	}
	return trimmed
}

// permit reports whether a request may reach the node, and why not when it may
// not.
//
// Every path through this function that is not an explicit allow is a refusal.
// That is the whole design: the previous gate enumerated what was dangerous,
// and was therefore wrong about everything nobody had thought of yet.
func permit(method string, path string, body []byte) (bool, string) {
	if method == "POST" {
		if len(body) > maxBody {
			return false, "request body too large"
		}
		methods, err := methodsOf(body)
		if err != nil {
			return false, err.Error()
		}
		for _, m := range methods {
			if m == "" {
				return false, "the request names no method"
			}
			if !allowed[m] {
				return false, m + " is not available on this endpoint"
			}
		}
		return true, ""
	}

	if method == "GET" || method == "HEAD" {
		m := methodOfPath(path)
		if m == "" {
			// The bare root is CometBFT's HTML method index. Harmless, but it
			// is not a method call and nothing here needs it.
			return false, "no method named"
		}
		if !allowed[m] {
			return false, m + " is not available on this endpoint"
		}
		return true, ""
	}

	// OPTIONS, PUT, DELETE and anything else. CometBFT answers none of them
	// usefully and a client here needs none of them.
	return false, method + " is not accepted on this endpoint"
}
