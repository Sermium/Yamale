// Command loyaltyapi is the small backend of the jewel loyalty app.
//
// It does four things, and it never signs for anyone:
//
//   - it stores photos and record files under their SHA-256, so a record on
//     the chain can be fetched by its fingerprint and checked against it;
//   - it keeps each jeweler's logo and colours;
//   - it sponsors an account's fees once, through x/feegrant, for the owner
//     messages only (sponsor.go);
//   - it takes jeweler applications for the office to review.
//
// If it is down, photos stop and new accounts wait for their sponsorship. The
// register does not stop, because the app reads the chain directly.
//
//	loyaltyapi --listen 127.0.0.1:8095 --dir ./loyalty --rest http://127.0.0.1:1517 \
//	  --chain-id yamale-local-1
package main

import (
	"encoding/json"
	"flag"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type server struct {
	dir     string
	chainID string
	chain   Chain
	now     func() time.Time

	cacheMu  sync.Mutex
	cached   []Jeweler
	cachedAt time.Time

	slotsOnce sync.Once
	slots     chan struct{}
	slotWait  time.Duration // how long an upload waits for a slot; zero means 2s

	balances     BalanceReader // reads the sponsor's balance for /v1/health/sponsor
	sponsorFloor *big.Int      // below this balance the health check answers 503
	sponsorDenom string

	granter       Granter // nil: sponsoring is off
	sponsorDaily  int
	operatorToken string // empty: the application list is off
	mu            sync.Mutex
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8095", "address to listen on")
	dir := flag.String("dir", "loyalty-data", "directory for files and profiles")
	rest := flag.String("rest", "http://127.0.0.1:1517", "the chain's REST API, read for jewelers and their staff")
	chainID := flag.String("chain-id", "yamale-local-1", "the chain id every signed request names")
	bin := flag.String("bin", "blockchaind", "chain binary that signs the fee grants")
	from := flag.String("from", "", "key name of the sponsor account; empty turns sponsoring off")
	home := flag.String("home", "", "home of the chain binary, holding the keyring")
	keyring := flag.String("keyring-backend", "test", "keyring backend")
	node := flag.String("node", "tcp://127.0.0.1:26957", "CometBFT RPC the chain binary broadcasts to")
	spend := flag.String("sponsor", "2000000uyml", "fee allowance granted to each account")
	lifetime := flag.Duration("sponsor-for", 365*24*time.Hour, "how long an allowance lasts")
	fees := flag.String("fees", "0uyml", "fee the sponsor pays per grant")
	daily := flag.Int("sponsor-daily", 200, "most accounts sponsored in any 24 hours")
	floor := flag.String("sponsor-floor", "10000000uyml", "below this sponsor balance /v1/health/sponsor answers 503")
	tokenFile := flag.String("operator-token-file", "", "file holding the bearer token that lists applications; empty turns the list off")
	flag.Parse()

	for _, sub := range []string{"files", "profiles", "applications"} {
		if err := os.MkdirAll(filepath.Join(*dir, sub), 0o750); err != nil {
			log.Fatal(err)
		}
	}
	chain := restChain{base: strings.TrimRight(*rest, "/"), client: &http.Client{Timeout: 10 * time.Second}}
	floorAmount, floorDenom, err := splitCoin(*floor)
	if err != nil {
		log.Fatalf("sponsor floor: %v", err)
	}
	s := &server{
		dir:          *dir,
		chainID:      *chainID,
		chain:        chain,
		balances:     chain,
		sponsorFloor: floorAmount,
		sponsorDenom: floorDenom,
		now:          time.Now,
	}
	if *from != "" {
		g, err := newCLIGranter(*bin, *from, *home, *keyring, *node, *chainID, *spend, *fees, *lifetime)
		if err != nil {
			log.Fatalf("sponsor key %q: %v", *from, err)
		}
		s.granter = g
		s.sponsorDaily = *daily
		log.Printf("sponsoring fees from %s, at most %d accounts a day", g.Address(), *daily)
	}
	if *tokenFile != "" {
		raw, err := os.ReadFile(*tokenFile)
		if err != nil || len(strings.TrimSpace(string(raw))) < 32 {
			log.Fatalf("operator token file %s must hold at least 32 characters", *tokenFile)
		}
		s.operatorToken = strings.TrimSpace(string(raw))
	}
	srv := &http.Server{
		Addr: *listen, Handler: s.routes(),
		ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 2 * time.Minute,
	}
	log.Printf("loyaltyapi listening on %s for chain %s", *listen, *chainID)
	log.Fatal(srv.ListenAndServe())
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /v1/health/sponsor", s.handleSponsorHealth)
	mux.HandleFunc("POST /v1/files", s.handleUpload)
	mux.HandleFunc("GET /v1/files/{hash}", s.handleFile)
	mux.HandleFunc("GET /v1/jewelers/{id}/profile", s.handleProfileGet)
	mux.HandleFunc("PUT /v1/jewelers/{id}/profile", s.handleProfilePut)
	mux.HandleFunc("GET /v1/sponsor", s.handleSponsorInfo)
	mux.HandleFunc("POST /v1/sponsor", s.handleSponsor)
	mux.HandleFunc("POST /v1/applications", s.handleApply)
	mux.HandleFunc("GET /v1/applications", s.handleApplications)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// serverFault logs the detail where the operator reads it and tells the client
// only that the server failed: OS errors carry the server's own paths.
func serverFault(w http.ResponseWriter, err error) {
	log.Printf("loyaltyapi: %v", err)
	refuse(w, http.StatusInternalServerError, "the server could not complete that; try again later")
}

// chainUnreachable is serverFault's twin for a failed chain read: the REST
// error names the node's internal address, so the client hears only that the
// register could not be read.
func chainUnreachable(w http.ResponseWriter, err error) {
	log.Printf("loyaltyapi: chain read: %v", err)
	refuse(w, http.StatusBadGateway, "the register could not be read; try again later")
}

func refuse(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
