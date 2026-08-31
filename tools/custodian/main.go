// Command custodian holds one share of a consumer account and decides whether
// to co-sign with it.
//
// # What this is for
//
// A Yamale consumer key exists in three shares — device, custodian, recovery —
// and any two sign. The device's share lives on the customer's own hardware
// under their password. This service holds the second, and it is the reason the
// design works at all: the operator running it cannot move anybody's money,
// because one share signs nothing.
//
// # What it is actually protecting
//
// Not the share. The share alone is worthless, and saying so is not modesty —
// it is the property that makes the rest of this tractable.
//
// What it protects is the DECISION. A person's phone can be stolen, and the
// thief then holds a device share and needs exactly one thing to empty the
// account: a custodian willing to co-sign. Everything here is about refusing
// that — a password the thief does not have, a second factor above an amount,
// one signature at a time, sessions that expire, and a freeze an owner can
// trigger the moment they notice.
//
// So the endpoints are deliberately boring and the refusals are the product.
//
// # What it is not
//
// It is not the account service. There is no enrolment over HTTP yet, no
// recovery workflow, no notification, no second-factor enrolment: those are
// specified in docs/guides/accounts.md Part 4 and Part 5 and are the next
// piece. An account is enrolled here by an operator importing a share produced
// by the ceremony, which is honest for a service holding nobody's money yet and
// would not be acceptable once it does.
//
// # Deployment shape, and one thing that must be true
//
//	custodian --dir /var/lib/yamale/custodian \
//	          --sealing-key-file /etc/yamale/custodian.key \
//	          --pepper-file /etc/yamale/custodian.pepper \
//	          --listen 127.0.0.1:8099
//
// The sealing key and the pepper must not live in --dir. That is the whole
// point of both: a stolen directory yields ciphertext and a set of unguessable
// blind indexes, and neither can be attacked without a file that was never in
// it. The service refuses to start if either sits inside the store.
package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"yamale/blockchain/mpc"
	mpccosmos "yamale/blockchain/mpc/cosmos"

	_ "yamale/blockchain/app" // its init() seals the yml bech32 prefix
)

type server struct {
	store    *Store
	sessions *Sessions
	index    *BlindIndex
	// secondFactorAbove is the amount, in the smallest unit, at or above which
	// a signature needs more than a password. Zero means always.
	secondFactorAbove uint64
	started           time.Time
}

func main() {
	dir := flag.String("dir", "", "directory holding the sealed account files")
	keyFile := flag.String("sealing-key-file", "", "file holding the 32-byte sealing key")
	pepperFile := flag.String("pepper-file", "", "file holding the blind-index pepper")
	listen := flag.String("listen", "127.0.0.1:8099", "address to serve on")
	sfAbove := flag.Uint64("second-factor-above", 0, "amount at or above which a second factor is required; 0 means always")
	importShare := flag.String("import", "", "enrol an account from a share file and exit")
	importEmail := flag.String("import-email", "", "the email to index the imported account under")
	importPassword := flag.String("import-password", "", "the password for the imported account")
	flag.Parse()

	if *dir == "" || *keyFile == "" || *pepperFile == "" {
		fmt.Fprintln(os.Stderr, "custodian: --dir, --sealing-key-file and --pepper-file are all required")
		os.Exit(2)
	}
	if err := outsideStore(*dir, *keyFile, "the sealing key"); err != nil {
		fmt.Fprintln(os.Stderr, "custodian:", err)
		os.Exit(2)
	}
	if err := outsideStore(*dir, *pepperFile, "the pepper"); err != nil {
		fmt.Fprintln(os.Stderr, "custodian:", err)
		os.Exit(2)
	}

	sealingKey, err := readSecret(*keyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "custodian: reading the sealing key:", err)
		os.Exit(1)
	}
	pepper, err := readSecret(*pepperFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "custodian: reading the pepper:", err)
		os.Exit(1)
	}

	store, err := NewStore(*dir, sealingKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "custodian:", err)
		os.Exit(1)
	}
	index, err := NewBlindIndex(string(pepper))
	if err != nil {
		fmt.Fprintln(os.Stderr, "custodian:", err)
		os.Exit(1)
	}

	s := &server{
		store: store, sessions: NewSessions(), index: index,
		secondFactorAbove: *sfAbove, started: time.Now(),
	}

	if *importShare != "" {
		if err := s.importAccount(*importShare, *importEmail, *importPassword); err != nil {
			fmt.Fprintln(os.Stderr, "custodian:", err)
			os.Exit(1)
		}
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/sign/start", s.handleStart)
	mux.HandleFunc("POST /v1/sign/message", s.handleMessage)
	mux.HandleFunc("POST /v1/sign/result", s.handleResult)
	mux.HandleFunc("POST /v1/freeze", s.handleFreeze)
	mux.HandleFunc("GET /v1/health", s.handleHealth)

	n, _ := store.Count()
	log.Printf("custodian on %s, %d accounts, second factor above %d", *listen, n, *sfAbove)
	log.Printf("this service holds ONE share per account and cannot sign with it alone")

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// outsideStore refuses a secret that lives in the directory it protects.
//
// Not pedantry. "Back up /var/lib/yamale/custodian" is the one instruction an
// operator will certainly follow, and if the sealing key is in there the backup
// is a plaintext copy of every share the service holds.
func outsideStore(dir, secret, what string) error {
	d, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	s, err := filepath.Abs(secret)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(d, s); err == nil && !strings.HasPrefix(rel, "..") {
		return fmt.Errorf(
			"%s is inside the account store (%s); anything that backs up the store would copy it, "+
				"and then the encryption at rest protects nothing", what, s)
	}
	return nil
}

func readSecret(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is readable by others (mode %o); chmod 600 it", path, info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Trimmed, because a trailing newline from an editor is the commonest way a
	// key that "looks right" is the wrong 33 bytes.
	return []byte(strings.TrimSpace(string(raw))), nil
}

// importAccount enrols an account from a share file produced by the ceremony.
func (s *server) importAccount(sharePath, email, password string) error {
	if email == "" || password == "" {
		return errors.New("--import needs --import-email and --import-password")
	}
	raw, err := os.ReadFile(sharePath)
	if err != nil {
		return err
	}
	var share mpc.Share
	if err := json.Unmarshal(raw, &share); err != nil {
		return fmt.Errorf("reading the share: %w", err)
	}
	if share.Role != mpc.RoleCustodian {
		return fmt.Errorf(
			"that is a %s share. A custodian must hold exactly the custodian's share: importing any "+
				"other would give this service two, and two shares sign", share.Role)
	}
	pub, err := share.PublicKey()
	if err != nil {
		return err
	}
	address, err := mpccosmos.Address(pub)
	if err != nil {
		return err
	}
	verifier, err := NewVerifier(password)
	if err != nil {
		return err
	}
	idx := s.index.Of(email)
	if _, err := s.store.Get(idx); err == nil {
		return errors.New("an account already exists for that email")
	}
	if err := s.store.Put(Account{
		Index: idx, Address: address, Password: verifier, Created: time.Now().UTC(),
	}, share); err != nil {
		return err
	}
	fmt.Println(address)
	fmt.Fprintf(os.Stderr, "enrolled %s\n", address)
	return nil
}

// ---------------------------------------------------------------- endpoints

type startRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// Digest is the 32 bytes to sign, base64. The custodian does not build the
	// transaction and does not parse it: it is handed a digest, and what that
	// digest commits to is the device's business.
	//
	// Worth being explicit that this is a LIMITATION and not a design win. A
	// custodian that cannot read what it signs cannot refuse a payment on its
	// merits — it can only refuse the signer. Making the amount visible so the
	// second-factor threshold applies to something real is the next piece.
	Digest string `json:"digest"`
	// Amount, for the second-factor rule. Asserted by the caller today, which
	// means a thief asserts zero. Named honestly rather than dressed up.
	Amount uint64 `json:"amount"`
	// SecondFactor is a placeholder until enrolment exists.
	SecondFactor string `json:"second_factor,omitempty"`
}

type startResponse struct {
	Session  string         `json:"session"`
	Address  string         `json:"address"`
	Outbound []mpc.Outbound `json:"outbound"`
}

func (s *server) handleStart(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if !decode(w, r, &req) {
		return
	}
	digest, err := base64.StdEncoding.DecodeString(req.Digest)
	if err != nil || len(digest) != 32 {
		refuse(w, http.StatusBadRequest, "the digest must be 32 bytes, base64")
		return
	}

	account, ok := s.authenticate(w, req.Email, req.Password)
	if !ok {
		return
	}
	if SecondFactorRequired(req.Amount, s.secondFactorAbove) && req.SecondFactor == "" {
		// Refused rather than waved through, even though enrolment does not
		// exist yet. A service that skips a check because the feature behind it
		// is unfinished is a service that ships with the check missing.
		refuse(w, http.StatusForbidden,
			"this amount needs a second factor, and second-factor enrolment is not built yet")
		return
	}

	share, err := s.store.Share(account)
	if err != nil {
		log.Printf("unsealing a share for %s: %v", account.Index, err)
		refuse(w, http.StatusInternalServerError, "this account's share cannot be opened")
		return
	}
	sess, out, err := s.sessions.Start(account.Index, digest, share)
	if errors.Is(err, ErrBusy) {
		refuse(w, http.StatusConflict, ErrBusy.Error())
		return
	}
	if err != nil {
		log.Printf("starting a session: %v", err)
		refuse(w, http.StatusInternalServerError, "the session could not be started")
		return
	}
	respond(w, startResponse{Session: sess.ID, Address: account.Address, Outbound: out})
}

type messageRequest struct {
	Session string       `json:"session"`
	Message mpc.Outbound `json:"message"`
}

func (s *server) handleMessage(w http.ResponseWriter, r *http.Request) {
	var req messageRequest
	if !decode(w, r, &req) {
		return
	}
	out, err := s.sessions.Handle(req.Session, req.Message)
	if err != nil {
		sessionError(w, err)
		return
	}
	respond(w, map[string]any{"outbound": out})
}

func (s *server) handleResult(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Session string `json:"session"`
	}
	if !decode(w, r, &req) {
		return
	}
	sig, done, err := s.sessions.Signature(req.Session)
	if err != nil {
		sessionError(w, err)
		return
	}
	if !done {
		respond(w, map[string]any{"pending": true})
		return
	}
	s.sessions.Close(req.Session)
	respond(w, map[string]any{"signature": base64.StdEncoding.EncodeToString(sig)})
}

// handleFreeze stops an account co-signing.
//
// Password-authenticated, which is the right bar for this one action and a
// deliberately different judgement from signing: somebody reporting a stolen
// phone is often distressed, sometimes on a borrowed device, and rarely able to
// produce a second factor that was on the phone. Making a freeze hard to
// request protects nobody — the cost of a false freeze is an inconvenience, and
// the cost of a slow one is the account.
func (s *server) handleFreeze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Reason   string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	account, ok := s.authenticate(w, req.Email, req.Password)
	if !ok {
		return
	}
	if err := s.store.Freeze(account.Index, req.Reason); err != nil {
		refuse(w, http.StatusInternalServerError, "the freeze did not take")
		return
	}
	log.Printf("account frozen: %s", account.Address)
	respond(w, map[string]any{"frozen": true})
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	n, _ := s.store.Count()
	respond(w, map[string]any{
		"accounts":       n,
		"open_sessions":  s.sessions.Open(),
		"uptime_seconds": int(time.Since(s.started).Seconds()),
		"holds":          "one share per account, which signs nothing alone",
	})
}

// authenticate resolves and checks an account, or writes the refusal.
//
// Every failure is the same refusal with the same status and the same wording.
// "No such account" and "wrong password" as distinct answers turn this endpoint
// into an oracle for which email addresses are enrolled.
func (s *server) authenticate(w http.ResponseWriter, email, password string) (Account, bool) {
	idx := s.index.Of(email)
	account, err := s.store.Get(idx)
	if err != nil {
		// Still spend the time. Returning fast for an unknown account leaks
		// the same fact the wording was careful not to.
		_, _ = NewVerifier("a password that goes nowhere at all")
		refuse(w, http.StatusUnauthorized, "that email and password do not match an account")
		return Account{}, false
	}
	if !account.Password.Verify(password) {
		refuse(w, http.StatusUnauthorized, "that email and password do not match an account")
		return Account{}, false
	}
	if account.Frozen {
		refuse(w, http.StatusForbidden, "this account is frozen: "+account.FrozenReason)
		return Account{}, false
	}
	return account, true
}

func sessionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoSession), errors.Is(err, ErrSessionOld):
		refuse(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrFinished):
		refuse(w, http.StatusConflict, err.Error())
	default:
		refuse(w, http.StatusBadRequest, err.Error())
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		refuse(w, http.StatusBadRequest, "the request does not decode: "+err.Error())
		return false
	}
	return true
}

func respond(w http.ResponseWriter, body any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func refuse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
