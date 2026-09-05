package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
)

// The decision is tested in gate_test.go. This tests the wiring around it,
// which is where a filter breaks in production: the body has to survive being
// read for the decision and still reach the node, with a Content-Length that
// matches, or the upstream hangs waiting for bytes that are not coming.
func harness(t *testing.T) (*httptest.Server, *httptest.Server, *[]string) {
	t.Helper()

	var seen []string
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("the node could not read the forwarded body: %v", err)
		}
		seen = append(seen, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
	}))
	t.Cleanup(node.Close)

	target, err := url.Parse(node.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	front := httptest.NewServer(gate(proxy, log.New(io.Discard, "", 0)))
	t.Cleanup(front.Close)

	return front, node, &seen
}

func TestAnAllowedCallReachesTheNodeIntact(t *testing.T) {
	front, _, seen := harness(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"abci_query","params":{"path":"/cosmos.bank.v1beta1.Query/Params","data":""}}`
	res, err := http.Post(front.URL+"/", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", res.StatusCode)
	}
	if len(*seen) != 1 {
		t.Fatalf("the node saw %d requests, want 1", len(*seen))
	}
	// Byte-for-byte: the gate reads the body to decide and must hand on exactly
	// what it was given, params and all.
	if (*seen)[0] != body {
		t.Errorf("the node received\n  %s\nwant\n  %s", (*seen)[0], body)
	}
}

func TestARefusedCallNeverReachesTheNode(t *testing.T) {
	front, _, seen := harness(t)

	res, err := http.Post(front.URL+"/", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"net_info","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("got %d, want 403", res.StatusCode)
	}
	if len(*seen) != 0 {
		t.Fatalf("the node saw the refused call: %q", (*seen))
	}

	// The refusal has to be JSON-RPC, not HTML. Every client library in this
	// ecosystem reports an HTML body as a transport fault, which turns a
	// deliberate policy into an outage somebody spends an afternoon on.
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type is %q", ct)
	}
	var reply struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    string `json:"data"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
		t.Fatalf("the refusal is not JSON: %v", err)
	}
	if reply.Error.Code != -32601 {
		t.Errorf("error code %d, want -32601", reply.Error.Code)
	}
	if !strings.Contains(reply.Error.Data, "net_info") {
		t.Errorf("the refusal does not name the method: %q", reply.Error.Data)
	}
}

func TestAnOversizedBodyIsRefusedWithoutBeingHeld(t *testing.T) {
	front, _, seen := harness(t)

	huge := `{"jsonrpc":"2.0","id":1,"method":"abci_query","params":{"data":"` +
		strings.Repeat("a", maxBody) + `"}}`
	res, err := http.Post(front.URL+"/", "application/json", strings.NewReader(huge))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("got %d, want 403", res.StatusCode)
	}
	if len(*seen) != 0 {
		t.Fatal("an oversized body reached the node")
	}
}

func TestTheGetFormIsGatedToo(t *testing.T) {
	front, _, seen := harness(t)

	res, err := http.Get(front.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("GET /status got %d, want 200", res.StatusCode)
	}

	res, err = http.Get(front.URL + "/dump_consensus_state")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("GET /dump_consensus_state got %d, want 403", res.StatusCode)
	}

	if len(*seen) != 1 {
		t.Fatalf("the node saw %d requests, want 1", len(*seen))
	}
}
