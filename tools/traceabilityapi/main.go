// Command traceabilityapi is the small backend of the jewel traceability app.
//
// It does five things, and it never signs for anyone:
//
//   - it stores photos and record files under their SHA-256, so a record on
//     the chain can be fetched by its fingerprint and checked against it;
//   - it keeps each jeweler's logo and colours;
//   - it sponsors an account's fees once, through x/feegrant, for the owner
//     messages only (sponsor.go);
//   - it takes jeweler applications for the office to review;
//   - it reads a jeweler's own website into a catalogue of designs, and has
//     Claude draft their materials for a person to confirm (catalogue.go).
//
// If it is down, photos stop and new accounts wait for their sponsorship. The
// register does not stop, because the app reads the chain directly.
//
//	traceabilityapi --listen 127.0.0.1:8095 --dir ./traceability --rest http://127.0.0.1:1517 \
//	  --chain-id yamale-local-1
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"math/big"
	"net/http"
	"net/url"
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
	welcome       string  // devnet only: coins each newly sponsored account receives; empty is none
	sender        Sender  // pays the welcome
	onboarder     Onboarder // devnet only: registers accounts and records their country; nil is off
	sponsorDaily  int
	operatorToken string // empty: the application list is off
	mu            sync.Mutex

	// The catalogue (catalogue.go). readSite is nil in production: it reads
	// the website through fetch. drafter is nil when no Anthropic key is set.
	fetch        *fetcher
	readSite     func(ctx context.Context, origin string) ([]sourceProduct, error)
	drafter      drafter
	draftModel   string
	draftMonthly int
	catMu        sync.Mutex
	vaultMu      sync.Mutex // the card vault files
	keys         KeyReader  // nil: members are known only by their own requests
	importing    map[string]bool
	usageMu      sync.Mutex
	logMu        sync.Mutex
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8095", "address to listen on")
	dir := flag.String("dir", "traceability-data", "directory for files and profiles")
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
	onboard := flag.Bool("onboard", false, "devnet only: the sponsor is an approved participant and records the country each account chose")
	welcome := flag.String("welcome", "", "devnet only: coins sent to each newly sponsored account, e.g. 10000000000uusdc; empty sends none")
	floor := flag.String("sponsor-floor", "10000000uyml", "below this sponsor balance /v1/health/sponsor answers 503")
	tokenFile := flag.String("operator-token-file", "", "file holding the bearer token that lists applications; empty turns the list off")
	keyFile := flag.String("anthropic-key-file", "", "file holding the Anthropic API key that drafts catalogue designs; empty turns drafting off")
	draftModel := flag.String("draft-model", "claude-opus-5-5", "model that drafts catalogue designs")
	draftMonthly := flag.Int("draft-monthly-designs", 1000, "most designs drafted for one jeweler in a calendar month")
	anthropicURL := flag.String("anthropic-base-url", "", "Anthropic API base URL; empty is the public API (a local stub on a devnet)")
	devOrigin := flag.String("catalogue-dev-origin", "", "on yamale-local-1 only: one http://127.0.0.1:<port> origin the catalogue may read")
	flag.Parse()

	for _, sub := range []string{"files", "profiles", "applications", "catalogue"} {
		if err := os.MkdirAll(filepath.Join(*dir, sub), 0o750); err != nil {
			log.Fatal(err)
		}
	}
	if *devOrigin != "" {
		u, err := url.Parse(*devOrigin)
		if *chainID != "yamale-local-1" || err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" {
			log.Fatalf("--catalogue-dev-origin is http://127.0.0.1:<port>, on yamale-local-1 only")
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
		keys:         chain,
		sponsorFloor: floorAmount,
		sponsorDenom: floorDenom,
		now:          time.Now,
		fetch:        newFetcher(*devOrigin),
		draftModel:   *draftModel,
		draftMonthly: *draftMonthly,
	}
	if *from != "" {
		g, err := newCLIGranter(*bin, *from, *home, *keyring, *node, *chainID, *spend, *fees, *lifetime)
		if err != nil {
			log.Fatalf("sponsor key %q: %v", *from, err)
		}
		s.granter = g
		s.sponsorDaily = *daily
		log.Printf("sponsoring fees from %s, at most %d accounts a day", g.Address(), *daily)
		if *welcome != "" {
			if !welcomeAllowed(*chainID) {
				log.Fatalf("--welcome is for devnets only, and %s does not name itself one", *chainID)
			}
			s.welcome, s.sender = *welcome, g
			log.Printf("each newly sponsored account receives %s", *welcome)
		}
		if *onboard {
			if !welcomeAllowed(*chainID) {
				log.Fatalf("--onboard records a country nobody checked, so it is for devnets only, and %s does not name itself one", *chainID)
			}
			s.onboarder = g
			log.Printf("onboarding accounts as participant %s", g.Address())
		}
	}
	if *tokenFile != "" {
		raw, err := os.ReadFile(*tokenFile)
		if err != nil || len(strings.TrimSpace(string(raw))) < 32 {
			log.Fatalf("operator token file %s must hold at least 32 characters", *tokenFile)
		}
		s.operatorToken = strings.TrimSpace(string(raw))
	}
	if *keyFile != "" {
		raw, err := os.ReadFile(*keyFile)
		if err != nil || strings.TrimSpace(string(raw)) == "" {
			// Never the contents, and never fatal: the photos, uploads and sponsorship must keep running.
			log.Printf("catalogue drafting off: the key file is empty or unreadable")
		} else {
			s.drafter = newClaudeDrafter(strings.TrimSpace(string(raw)), *draftModel, *anthropicURL, 90*time.Second, 2)
			log.Printf("catalogue drafting on, with %s, at most %d designs a jeweler a month", *draftModel, *draftMonthly)
		}
	} else {
		log.Printf("catalogue drafting off: no --anthropic-key-file")
	}
	srv := &http.Server{
		Addr: *listen, Handler: s.routes(),
		ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 2 * time.Minute,
	}
	log.Printf("traceabilityapi listening on %s for chain %s", *listen, *chainID)
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
	mux.HandleFunc("GET /v1/jewelers/{id}/portfolio", s.handlePortfolio)
	mux.HandleFunc("GET /v1/sponsor", s.handleSponsorInfo)
	mux.HandleFunc("POST /v1/sponsor", s.handleSponsor)
	mux.HandleFunc("POST /v1/onboard", s.handleOnboard)
	mux.HandleFunc("POST /v1/applications", s.handleApply)
	mux.HandleFunc("GET /v1/applications", s.handleApplications)
	mux.HandleFunc("POST /v1/catalogue/import", s.handleCatalogueImport)
	mux.HandleFunc("GET /v1/catalogue", s.handleCatalogue)
	mux.HandleFunc("GET /v1/cardvault", s.handleVaultRead)
	mux.HandleFunc("PUT /v1/cardvault/{jewel}", s.handleVaultPut)
	mux.HandleFunc("PUT /v1/catalogue/designs/{id}", s.handleDesignSave)
	mux.HandleFunc("POST /v1/catalogue/designs/{id}/confirm", s.handleDesignConfirm)
	mux.HandleFunc("GET /v1/catalogue/designs/{id}/images/{n}", s.handleDesignImage)
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
	log.Printf("traceabilityapi: %v", err)
	refuse(w, http.StatusInternalServerError, "the server could not complete that; try again later")
}

// chainUnreachable is serverFault's twin for a failed chain read: the REST
// error names the node's internal address, so the client hears only that the
// register could not be read.
func chainUnreachable(w http.ResponseWriter, err error) {
	log.Printf("traceabilityapi: chain read: %v", err)
	refuse(w, http.StatusBadGateway, "the register could not be read; try again later")
}

func refuse(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
