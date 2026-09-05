package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

// rpcgate is a reverse proxy that answers only the JSON-RPC methods in
// allowed, and passes everything it permits through to CometBFT unchanged.
//
// Deployment is one line of nginx: point /api/rpc/ at this instead of at
// 26657. The node's own listener stays on 127.0.0.1 and is not otherwise
// touched, so a failure here takes the public RPC down and nothing else —
// which is what the deploy check is for.
func main() {
	listen := flag.String("listen", "127.0.0.1:26659", "address to serve the gated RPC on")
	upstream := flag.String("upstream", "http://127.0.0.1:26657", "the CometBFT RPC to forward to")
	flag.Parse()

	target, err := url.Parse(*upstream)
	if err != nil {
		log.Fatalf("upstream %q is not a URL: %v", *upstream, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		// The node being down is not the same as a method being refused, and a
		// client that cannot tell them apart will report the wrong thing to
		// whoever is on the other end of it.
		log.Printf("upstream: %v", err)
		writeRefusal(w, http.StatusBadGateway, "the node is not answering")
	}

	logger := log.New(os.Stderr, "", log.LstdFlags|log.LUTC)

	srv := &http.Server{
		Addr:    *listen,
		Handler: gate(proxy, logger),
		// Generous, because a signed transaction from a slow mobile connection
		// is an ordinary request, and mean on the header phase, which is where
		// a slowloris lives.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	logger.Printf("rpcgate listening on %s, forwarding to %s", *listen, target)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal(err)
	}
}

// gate reads the body, decides, and either forwards or refuses.
//
// The body has to be buffered because the decision depends on it and the proxy
// needs it afterwards. maxBody bounds that, so the gate cannot be made to hold
// arbitrary memory by the thing it exists to protect against.
func gate(next http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil {
			var err error
			body, err = io.ReadAll(io.LimitReader(r.Body, maxBody+1))
			r.Body.Close()
			if err != nil {
				writeRefusal(w, http.StatusBadRequest, "the request body could not be read")
				return
			}
		}

		if ok, why := permit(r.Method, r.URL.Path, body); !ok {
			logger.Printf("refused %s %s from %s: %s", r.Method, r.URL.Path, clientOf(r), why)
			writeRefusal(w, http.StatusForbidden, why)
			return
		}

		// Restored for the proxy, with the length reset: ContentLength survives
		// from the original request and a mismatch makes the upstream hang
		// waiting for bytes that are not coming.
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		next.ServeHTTP(w, r)
	})
}

// writeRefusal answers in JSON-RPC's own error shape.
//
// A refusal that comes back as HTML is indistinguishable from a proxy fault to
// every client library in this ecosystem, which is how a deliberate policy gets
// diagnosed as an outage. The code is JSON-RPC's "method not found".
func writeRefusal(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      nil,
		"error": map[string]any{
			"code":    -32601,
			"message": "method not available on this endpoint",
			"data":    reason,
		},
	})
}

// clientOf reports who asked, preferring what nginx forwarded over the socket's
// own peer, which is always the proxy.
func clientOf(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	return r.RemoteAddr
}
