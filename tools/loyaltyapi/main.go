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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type server struct {
	dir     string
	chainID string
	chain   Chain
	now     func() time.Time
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8095", "address to listen on")
	dir := flag.String("dir", "loyalty-data", "directory for files and profiles")
	rest := flag.String("rest", "http://127.0.0.1:1517", "the chain's REST API, read for jewelers and their staff")
	chainID := flag.String("chain-id", "yamale-local-1", "the chain id every signed request names")
	flag.Parse()

	for _, sub := range []string{"files", "profiles"} {
		if err := os.MkdirAll(filepath.Join(*dir, sub), 0o750); err != nil {
			log.Fatal(err)
		}
	}
	s := &server{
		dir:     *dir,
		chainID: *chainID,
		chain:   restChain{base: strings.TrimRight(*rest, "/"), client: &http.Client{Timeout: 10 * time.Second}},
		now:     time.Now,
	}
	srv := &http.Server{
		Addr: *listen, Handler: s.routes(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 2 * time.Minute,
	}
	log.Printf("loyaltyapi listening on %s for chain %s", *listen, *chainID)
	log.Fatal(srv.ListenAndServe())
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /v1/files", s.handleUpload)
	mux.HandleFunc("GET /v1/files/{hash}", s.handleFile)
	mux.HandleFunc("GET /v1/jewelers/{id}/profile", s.handleProfileGet)
	mux.HandleFunc("PUT /v1/jewelers/{id}/profile", s.handleProfilePut)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func refuse(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
