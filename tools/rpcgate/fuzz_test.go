package main

import (
	"strings"
	"testing"
)

// Fuzzing the gate, because everything it reads is chosen by whoever is
// attacking it.
//
// permit() is the only thing standing between the public internet and the
// node's RPC, and its whole input — an HTTP method, a URL path and a request
// body — comes from an unauthenticated caller. A panic here is not a failed
// request, it is the public RPC falling over: the gate is a process, and a
// panic in the handler goroutine that is not recovered takes the process with
// it, which is the denial of service the gate exists to prevent.
//
// The properties asserted are the two that matter and neither depends on
// guessing what the fuzzer will produce:
//
//   - it never panics, whatever it is handed;
//   - it never permits a method that is not in the allow-list, which is the
//     security boundary itself.
//
// The second is the one worth having. A fuzzer that only checked for crashes
// would be satisfied by a gate that returned true for everything.
func FuzzPermit(f *testing.F) {
	// Seeds: the shapes a real caller sends, the shapes the audit found, and
	// the shapes that have historically broken JSON-RPC filters.
	f.Add("POST", "/", `{"jsonrpc":"2.0","id":1,"method":"status"}`)
	f.Add("POST", "/", `{"jsonrpc":"2.0","id":1,"method":"net_info","params":{}}`)
	f.Add("POST", "/", `[{"method":"status"},{"method":"dump_consensus_state"}]`)
	f.Add("POST", "/", `[]`)
	f.Add("POST", "/", ``)
	f.Add("POST", "/", `{"method":7}`)
	f.Add("POST", "/", `{"method":null}`)
	f.Add("POST", "/", `{"Method":"net_info"}`)                   // case differs from the tag
	f.Add("POST", "/", `{"method":"net_info"}`)                   // escaped n
	f.Add("POST", "/", `{"method":"status","method":"net_info"}`) // duplicate key
	f.Add("GET", "/status", ``)
	f.Add("GET", "/net_info", ``)
	f.Add("GET", "/", ``)
	f.Add("GET", "/../net_info", ``)
	f.Add("HEAD", "/abci_query", ``)
	f.Add("PUT", "/status", ``)
	f.Add("", "", ``)

	f.Fuzz(func(t *testing.T, method, path, body string) {
		ok, why := permit(method, path, []byte(body))

		if ok {
			// Whatever was permitted must be a method this gate was told to
			// allow, arriving by a form it understands. Anything else is the
			// boundary failing open, which is the only failure that matters.
			switch method {
			case "POST":
				methods, err := methodsOf([]byte(body))
				if err != nil {
					t.Fatalf("permitted a body that does not parse: %q", body)
				}
				if len(methods) == 0 {
					t.Fatalf("permitted a body naming no method: %q", body)
				}
				for _, m := range methods {
					if !allowed[m] {
						t.Fatalf("permitted %q, which is not in the allow-list (body %q)", m, body)
					}
				}
			case "GET", "HEAD":
				m := methodOfPath(path)
				if m == "" || !allowed[m] {
					t.Fatalf("permitted path %q resolving to %q", path, m)
				}
				if strings.Contains(m, "/") {
					t.Fatalf("permitted a multi-segment path as a method: %q", path)
				}
			default:
				t.Fatalf("permitted verb %q, which is neither POST, GET nor HEAD", method)
			}
			return
		}

		// A refusal must say something. An empty reason reaches the caller as a
		// JSON-RPC error with no data, and an operator debugging a refused
		// client then has nothing to go on.
		if strings.TrimSpace(why) == "" {
			t.Fatalf("refused %s %q body %q with no reason given", method, path, body)
		}
	})
}

// methodsOf is the parser, and it is handed raw bytes from the network before
// anything has decided they are JSON. It gets its own target because a parser
// that panics is a parser that takes the process down before permit() ever
// reaches a decision.
func FuzzMethodsOf(f *testing.F) {
	f.Add(`{"method":"status"}`)
	f.Add(`[{"method":"status"}]`)
	f.Add(`[[{"method":"status"}]]`)
	f.Add(`[{"method":{"nested":"object"}}]`)
	f.Add(`{"method":"` + strings.Repeat("a", 4096) + `"}`)
	f.Add(strings.Repeat("[", 512)) // deeply nested, unterminated
	f.Add(`{"method":"status"`)
	f.Add("\x00\x01\x02")

	f.Fuzz(func(t *testing.T, body string) {
		methods, err := methodsOf([]byte(body))
		if err != nil {
			if len(methods) != 0 {
				t.Fatalf("returned %d methods alongside an error", len(methods))
			}
			return
		}
		// A successful parse must name at least one method, or permit() would
		// loop over nothing and fall through to allowing the request.
		if len(methods) == 0 {
			t.Fatalf("parsed %q into no methods and no error", body)
		}
	})
}
