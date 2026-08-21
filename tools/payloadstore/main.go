// Command payloadstore serves the encrypted ISO 20022 payloads of the payments
// one participant instructed.
//
// The chain records a salted SHA-256 of each payload and nothing else. That is
// what makes this service possible and what makes it necessary: an append-only
// ledger has no deletion path, so any personal data written to it is beyond the
// reach of the NDPA, Ghana's DPA, POPIA and the GDPR forever. Moving the detail
// here puts it somewhere it can actually be destroyed, and the salt is what
// makes destroying it an erasure rather than a gesture — a four-character
// purpose code hashed without one is a lookup table, so the hash left on the
// chain would still reveal what the deleted payload said.
//
// It stores ciphertext it cannot read. Every envelope is sealed to the viewing
// keys of the payer, the payee, the regulator of the declared settlement
// jurisdiction and any live auditor; this process holds none of those private
// halves and gains nothing by being compromised beyond the traffic pattern.
//
// Retrieval is gated twice over. The caller proves possession of a viewing key
// registered on-chain, and the store then resolves from chain state whether
// that account is entitled to this particular payment. Neither check alone is
// enough: the first says who you are, the second says what that entitles you to.
//
//	payloadstore --participant yml1bankone --rest http://127.0.0.1:1417 \
//	  --listen 127.0.0.1:8090 --dir ./payloads --write-token <token>
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	cfg := config{}
	listen := flag.String("listen", "127.0.0.1:8090", "address to listen on")
	flag.StringVar(&cfg.participant, "participant", "", "the approved participant whose payments this store serves")
	flag.StringVar(&cfg.rest, "rest", "http://127.0.0.1:1417", "the chain's REST API, used to resolve entitlement")
	flag.StringVar(&cfg.dir, "dir", "payloads", "directory holding the stored envelopes")
	flag.StringVar(&cfg.writeToken, "write-token", "", "bearer token the participant's own back office presents to store and to erase")
	flag.DurationVar(&cfg.challengeTTL, "challenge-ttl", 2*time.Minute, "how long an issued challenge stays answerable")
	flag.Parse()

	if cfg.participant == "" {
		fmt.Fprintln(os.Stderr, "payloadstore: --participant is required; a store that does not know whose payments it holds cannot resolve entitlement for any of them")
		os.Exit(2)
	}
	// Refused rather than defaulted to open. A store started without a write
	// token would accept an envelope from anyone, and the envelope is what the
	// payee's client trusts to be the payload the payment recorded — so an
	// unauthenticated write is a way to replace one party's payment detail with
	// another's and have it look official.
	if cfg.writeToken == "" {
		fmt.Fprintln(os.Stderr, "payloadstore: --write-token is required; without it anyone could store or erase a payload")
		os.Exit(2)
	}

	store, err := newStore(cfg)
	if err != nil {
		log.Fatalf("payloadstore: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", store.health)
	mux.HandleFunc("/challenge", store.challenge)
	mux.HandleFunc("/payloads/", store.payload)

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("payloadstore: serving payloads for %s on %s from %s", cfg.participant, *listen, cfg.dir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("payloadstore: %v", err)
		}
	}()

	<-ctx.Done()
	// Drained rather than killed. A store cut off mid-write would leave a
	// partial envelope on disk, and a partial envelope is one that fails to
	// open — which a reader cannot tell from one that was tampered with.
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	log.Println("payloadstore: stopped")
}

type config struct {
	participant  string
	rest         string
	dir          string
	writeToken   string
	challengeTTL time.Duration
}

// response is the one body shape every endpoint answers with.
//
// Including the failures, and including erasure. A client has to be able to
// tell "this payload was destroyed" from "this host is unreachable" and from
// "you may not read this", because the three lead to completely different
// things being shown to a person — and a bare 404 collapses all three into the
// one that reads as a bug.
type response struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`

	// Envelope is the sealed payload, base64. Absent on every other answer.
	Envelope string `json:"envelope,omitempty"`

	// The challenge, when one is being issued.
	ChallengeID        string `json:"challenge_id,omitempty"`
	EphemeralPublicKey string `json:"ephemeral_public_key,omitempty"`
	WrappedNonce       string `json:"wrapped_nonce,omitempty"`
	KeyID              string `json:"key_id,omitempty"`
	ExpiresAt          string `json:"expires_at,omitempty"`
}

// Status values. Named rather than written inline at each call site, because
// the client switches on them and a typo in one branch is a client that reports
// an erased payload as a network fault.
const (
	statusOK = "ok"
	// statusUnavailable is the answer for a payload that is not here — whether
	// it was erased, was never stored, or belongs to a payment this store does
	// not serve. It is deliberately one status with a machine-readable reason
	// beside it: a client must render every one of them as detail being
	// unavailable, and must never render any of them as a payment with no
	// detail.
	statusUnavailable = "unavailable"
	statusDenied      = "denied"
	statusError       = "error"

	reasonErased        = "erased"
	reasonNeverStored   = "never_stored"
	reasonNoSuchPayment = "no_such_payment"
	reasonNotEntitled   = "not_entitled"
	reasonUnproven      = "unproven"
)

func writeJSON(w http.ResponseWriter, status int, body response) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *store) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, response{Status: statusOK, Detail: s.cfg.participant})
}

// bearer returns the presented write token, if any.
func bearer(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimPrefix(h, prefix)
}

// readJSON decodes a request body under a size cap.
//
// Capped because the body arrives from the network and the only thing standing
// between a small JSON object and this process's memory is the number here.
func readJSON(r *http.Request, out any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(out)
}

// unescape reverses the path escaping the SDK and the chain both apply when
// building a payload URL, so the store keys on the same strings the chain does.
func unescape(s string) (string, error) {
	return url.PathUnescape(s)
}
