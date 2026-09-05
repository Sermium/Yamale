package main

import (
	"strings"
	"testing"
)

// The four the audit named, verified refused in the form they actually arrive
// in. Every one of these answered in full on the live funnel on 2026-09-05,
// because the nginx deny list matches on the URL path and CometBFT takes the
// method in the POST body.
func TestTheMethodsTheDenyListNeverBlocked(t *testing.T) {
	for _, method := range []string{
		"net_info",
		"dump_consensus_state",
		"consensus_state",
		"unconfirmed_txs",
		"num_unconfirmed_txs",
		"broadcast_tx_commit",
		"dial_seeds",
		"dial_peers",
		"unsafe_flush_mempool",
	} {
		body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{}}`
		if ok, _ := permit("POST", "/", []byte(body)); ok {
			t.Errorf("POST %s was permitted", method)
		}
		if ok, _ := permit("GET", "/"+method, nil); ok {
			t.Errorf("GET %s was permitted", method)
		}
	}
}

// And the ones the consoles actually use still work, or this is an outage
// rather than a gate. These four are the paths found in clients/: abci_query,
// block, status and tx_search.
func TestWhatTheConsolesNeedStillPasses(t *testing.T) {
	for _, method := range []string{
		"abci_query", "block", "status", "tx_search",
		"abci_info", "health", "broadcast_tx_sync", "tx", "validators",
	} {
		body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{}}`
		if ok, why := permit("POST", "/", []byte(body)); !ok {
			t.Errorf("POST %s was refused: %s", method, why)
		}
		if ok, why := permit("GET", "/"+method, nil); !ok {
			t.Errorf("GET %s was refused: %s", method, why)
		}
	}
}

// A batch is the obvious way round a filter that reads only the first call.
func TestABatchIsJudgedByEveryCallInIt(t *testing.T) {
	good := `[{"jsonrpc":"2.0","id":1,"method":"status"},
	          {"jsonrpc":"2.0","id":2,"method":"abci_query"}]`
	if ok, why := permit("POST", "/", []byte(good)); !ok {
		t.Fatalf("an all-allowed batch was refused: %s", why)
	}

	// An allowed method in front of a denied one, which is exactly what a
	// filter that stopped at the first element would wave through.
	smuggled := `[{"jsonrpc":"2.0","id":1,"method":"status"},
	              {"jsonrpc":"2.0","id":2,"method":"dump_consensus_state"}]`
	ok, why := permit("POST", "/", []byte(smuggled))
	if ok {
		t.Fatal("a batch smuggled dump_consensus_state past the gate")
	}
	if !strings.Contains(why, "dump_consensus_state") {
		t.Errorf("the refusal should name the offending method, got %q", why)
	}

	// An empty array parses cleanly and names no method at all, so a loop over
	// it succeeds vacuously.
	if ok, _ := permit("POST", "/", []byte(`[]`)); ok {
		t.Error("an empty batch was permitted")
	}
}

// Anything unparseable, unnamed or unknown is refused. The previous gate
// enumerated what was dangerous and was therefore wrong about everything
// nobody had thought of; this one is wrong only in the safe direction.
func TestAnythingItCannotClassifyIsRefused(t *testing.T) {
	cases := map[string]string{
		"empty body":         ``,
		"whitespace":         `   `,
		"not json":           `net_info`,
		"truncated json":     `{"jsonrpc":"2.0","method":`,
		"no method key":      `{"jsonrpc":"2.0","id":1,"params":{}}`,
		"empty method":       `{"jsonrpc":"2.0","id":1,"method":""}`,
		"unknown method":     `{"jsonrpc":"2.0","id":1,"method":"some_future_method"}`,
		"method is a number": `{"jsonrpc":"2.0","id":1,"method":7}`,
		"nested array":       `[[{"method":"status"}]]`,
	}
	for name, body := range cases {
		if ok, _ := permit("POST", "/", []byte(body)); ok {
			t.Errorf("%s was permitted", name)
		}
	}

	if ok, _ := permit("POST", "/", make([]byte, maxBody+1)); ok {
		t.Error("an oversized body was permitted")
	}
}

// The GET form must not be talked into a method by a path that is not one.
func TestThePathFormIsNotACharacterGame(t *testing.T) {
	for _, path := range []string{
		"/",
		"",
		"/net_info/extra",
		"/../net_info",
		"/NET_INFO",
		"/status/",
	} {
		ok, _ := permit("GET", path, nil)
		if path == "/status/" {
			// A trailing slash is the same method, and refusing it would be a
			// gate that breaks a correct client.
			if !ok {
				t.Errorf("GET %q was refused", path)
			}
			continue
		}
		if ok {
			t.Errorf("GET %q was permitted", path)
		}
	}
}

// Methods this gate does not accept at all.
func TestOtherVerbsAreRefused(t *testing.T) {
	for _, verb := range []string{"PUT", "DELETE", "PATCH", "OPTIONS", "TRACE"} {
		if ok, _ := permit(verb, "/status", nil); ok {
			t.Errorf("%s was permitted", verb)
		}
	}
}

// The allow-list is the security boundary, so a method being in it is a
// decision somebody made rather than something that drifted in.
func TestTheAllowListHoldsNothingDangerous(t *testing.T) {
	for _, forbidden := range []string{
		"net_info", "dump_consensus_state", "consensus_state",
		"unconfirmed_txs", "num_unconfirmed_txs", "broadcast_tx_commit",
		"dial_seeds", "dial_peers", "unsafe_flush_mempool", "check_tx",
	} {
		if allowed[forbidden] {
			t.Errorf("%s is in the allow-list", forbidden)
		}
	}
}
