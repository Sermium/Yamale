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
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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

	"yamale/blockchain/recovery"
	"yamale/blockchain/recovery/chain"

	_ "yamale/blockchain/app" // its init() seals the yml bech32 prefix
)

type server struct {
	store *Store
	index *BlindIndex
	// role is which of the three holdings this deployment keeps. Two
	// deployments of this binary — custodian and recovery — are what make an
	// enrolment possible without any one process holding two of the three.
	role       string
	recoveries *Recoveries
	notifier   Notifier
	started    time.Time
	// now is the server's clock, and it must be the SAME clock the recoveries
	// store stamps EligibleAt with.
	//
	// It was not. The store's clock is injectable and the handlers called
	// nowUTC() directly, so a test that moved the store's clock left the HTTP
	// layer reading wall time — and the recovery wiring test consequently
	// asserted the 72-hour notice period only while real time happened to be
	// behind the fixture's fixed date. It passed until 2026-09-04 and then
	// began reporting the approvals rule instead, silently, without anybody
	// changing the recovery code. A test that expires is not a test.
	//
	// Nil means wall clock, so production behaviour is unchanged.
	now func() time.Time
	// attempts slows password guessing per account. Its zero value works, so
	// every server has it, however it was built.
	attempts attemptThrottle
	// codes computes the emailed sign-in codes (signin.go); nil without
	// --signin-code-key-file, and then an emailed code cannot be answered.
	codes *signinCodes
	// mailer sends those codes and the new-sign-in notices. Custodian only.
	mailer Mailer
}

// clock is the server's time, defaulting to the wall clock.
func (s *server) clock() time.Time {
	if s.now == nil {
		return nowUTC()
	}
	return s.now()
}

func main() {
	dir := flag.String("dir", "", "directory holding the sealed account files")
	keyFile := flag.String("sealing-key-file", "", "file holding the 32-byte sealing key")
	pepperFile := flag.String("pepper-file", "", "file holding the blind-index pepper")
	listen := flag.String("listen", "127.0.0.1:8099", "address to serve on")
	role := flag.String("role", recovery.RoleCustodian, "which holding this deployment keeps: custodian or recovery")
	recoveryDir := flag.String("recovery-dir", "", "directory holding the recovery log; defaults to <dir>/recoveries")
	notifyCmd := flag.String("notify-command", "", "program run to notify an account holder; without it recovery refuses to start")
	signinKeyFile := flag.String("signin-code-key-file", "", "file holding the 32-byte key for emailed sign-in codes, the same at both services")
	smtpConfig := flag.String("smtp-config", "", "file configuring the SMTP submission that sends sign-in codes (custodian only)")
	importShare := flag.String("import", "", "enrol an account from a holding file and exit")
	importEmail := flag.String("import-email", "", "the email to index the imported account under")
	importPassword := flag.String("import-password", "", "the password for the imported account")
	importAddr := flag.String("import-address", "", "the account address the imported holding belongs to")
	flag.Parse()

	// Refused early and by name. A deployment started with --role device would
	// be one keeping the holding that is supposed to exist only on somebody's
	// phone, and the failure would be silent: enrolment would succeed and the
	// customer's own holding would be sitting on a server.
	if *role != recovery.RoleCustodian && *role != recovery.RoleRecovery {
		fmt.Fprintf(os.Stderr,
			"custodian: --role must be %q or %q, not %q. The device's holding belongs on the device "+
				"and a service keeping it defeats the point of splitting at all.\n",
			recovery.RoleCustodian, recovery.RoleRecovery, *role)
		os.Exit(2)
	}
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

	store, err := NewStore(*dir, *role, sealingKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "custodian:", err)
		os.Exit(1)
	}
	index, err := NewBlindIndex(string(pepper))
	if err != nil {
		fmt.Fprintln(os.Stderr, "custodian:", err)
		os.Exit(1)
	}

	// BESIDE the store, not inside it, and the default says so rather than
	// leaving it to configuration. The recovery log holds no key material and no
	// email — only blind indexes — so it does not want the sealing key, and
	// keeping it out of the store directory means an operator can hand the audit
	// trail to a regulator without handing over every share along with it.
	//
	// A sibling rather than somewhere unrelated so that one ReadWritePaths in
	// the unit still covers both.
	recDir := *recoveryDir
	if recDir == "" {
		recDir = strings.TrimRight(*dir, `/\`) + "-recoveries"
	}
	recoveries, err := NewRecoveries(recDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "custodian:", err)
		os.Exit(1)
	}

	var notifier Notifier = refusingNotifier{}
	if *notifyCmd != "" {
		notifier = newCommandNotifier(*notifyCmd)
	}

	s := &server{
		store: store, index: index, role: *role,
		recoveries: recoveries, notifier: notifier,
		started: time.Now(),
	}
	if *signinKeyFile != "" {
		if err := outsideStore(*dir, *signinKeyFile, "the sign-in code key"); err != nil {
			fmt.Fprintln(os.Stderr, "custodian:", err)
			os.Exit(2)
		}
		k, err := readSecret(*signinKeyFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "custodian: reading the sign-in code key:", err)
			os.Exit(1)
		}
		s.codes = newSigninCodes(k)
	}
	if *smtpConfig != "" {
		if *role != recovery.RoleCustodian {
			fmt.Fprintln(os.Stderr, "custodian: --smtp-config belongs on the custodian; the recovery service sends nothing")
			os.Exit(2)
		}
		m, err := loadSMTPConfig(*smtpConfig)
		if err != nil {
			fmt.Fprintln(os.Stderr, "custodian: reading the SMTP config:", err)
			os.Exit(1)
		}
		s.mailer = m
	}

	if *importShare != "" {
		if err := s.importAccount(*importShare, *importEmail, *importPassword, *importAddr); err != nil {
			fmt.Fprintln(os.Stderr, "custodian:", err)
			os.Exit(1)
		}
		return
	}

	n, _ := store.Count()
	log.Printf("custodian on %s as %s, %d accounts", *listen, *role, n)
	log.Printf("this service keeps ONE holding per account: a share of a ciphertext it cannot open")
	log.Printf("it does not co-sign anything; the device signs alone, and this is a recovery service")
	if *notifyCmd == "" {
		log.Printf("NO --notify-command: recovery will refuse to start, because a recovery " +
			"nobody was told about is worse than none")
	}

	// Two different kinds of handler live on this mux and they need two
	// different bounds, which Go's Server does not offer: ReadTimeout and
	// WriteTimeout are per-connection and global.
	//
	// Signing is milliseconds of arithmetic. Key generation is NOT: verifying
	// one round of another party's Paillier and range proofs takes tens of
	// seconds, and it takes them inside the handler. With a 30-second
	// WriteTimeout the server simply closed the connection mid-round, and the
	// client saw a bare "EOF" from its POST — no status, no message, and
	// nothing in this service's log, because the handler was still running and
	// had not failed. That is how the first live enrolment died.
	//
	// So the socket-level bound is generous enough for the slowest legitimate
	// handler, and the fast routes get their tight bound back individually
	// through TimeoutHandler, which answers 503 rather than dropping the
	// connection.
	fast := func(h http.HandlerFunc) http.Handler {
		return http.TimeoutHandler(h, 30*time.Second, `{"error":"this took too long"}`)
	}
	guarded := http.NewServeMux()
	// There are no /v1/sign routes any more. Under the architecture decision of
	// 2026-09-18 the device signs alone, so this service co-signs nothing and
	// there is no signing session for an endpoint to drive. They answer 410
	// with the reason rather than 404, because a cached client will hit them.
	guarded.Handle("POST /v1/sign/start", fast(handleEnrolGone))
	guarded.Handle("POST /v1/sign/message", fast(handleEnrolGone))
	guarded.Handle("POST /v1/sign/result", fast(handleEnrolGone))
	guarded.Handle("POST /v1/freeze", fast(s.handleFreeze))
	guarded.Handle("GET /v1/health", fast(s.handleHealth))
	guarded.Handle("POST /v1/recovery/initiate", fast(s.handleRecoveryInitiate))
	guarded.Handle("POST /v1/recovery/approve", fast(s.handleRecoveryApprove))
	guarded.Handle("POST /v1/recovery/complete", fast(s.handleRecoveryComplete))
	guarded.Handle("POST /v1/recovery/cancel", fast(s.handleRecoveryCancel))
	// The only endpoint that emits key material, and a customer one. See claim.go.
	guarded.Handle("POST /v1/recovery/claim", fast(s.handleRecoveryClaim))
	// Signing in on a new device: password and second factor release the
	// holding at once. Customer endpoints, like the claim. See signin.go.
	guarded.Handle("POST /v1/signin/start", fast(s.handleSigninStart))
	guarded.Handle("POST /v1/signin", fast(s.handleSignin))

	// Second-factor enrolment. Customer endpoints, like the claim: this is the
	// account holder's own half of a recovery and nobody can do it for them.
	guarded.Handle("POST /v1/factors", fast(s.handleFactors))
	guarded.HandleFunc("POST /v1/factors/totp/begin", s.handleTOTPBegin)
	guarded.HandleFunc("POST /v1/factors/totp/confirm", s.handleTOTPConfirm)
	guarded.Handle("GET /v1/recovery/statistics", fast(s.handleRecoveryStatistics))
	// Enrolment keeps the generous bound. It is one request now, but it still
	// pays for an Argon2id derivation at 64 MiB for the password verifier,
	// which is slow on purpose.
	guarded.HandleFunc("POST /v1/enrol", s.handleEnrol)
	guarded.Handle("POST /v1/enrol/start", fast(handleEnrolGone))
	guarded.Handle("POST /v1/enrol/message", fast(handleEnrolGone))
	guarded.Handle("POST /v1/enrol/finish", fast(handleEnrolGone))

	srv := &http.Server{
		Addr:              *listen,
		Handler:           guarded,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		// The slowest legitimate handler is now a single Argon2id derivation,
		// which is under a second. Left generous rather than tightened to
		// match, because the rate limit in front of this service is what bounds
		// the work, and a timeout tuned to the happy path is a timeout that
		// starts cutting off real requests on a loaded host.
		WriteTimeout: 1 * time.Minute,
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
	return parseSecret(path, raw)
}

// secretLen is the size of every secret this service reads: the sealing key is
// exactly this, and the pepper is at least this.
const secretLen = 32

// parseSecret accepts a secret in one of two forms that cannot be mistaken for
// each other.
//
// HEX, of at least 32 bytes, with any surrounding whitespace. This is the form
// to write — `openssl rand -hex 32` — and the only one where trimming is safe,
// because a hex digit is never whitespace.
//
// RAW, of exactly 32 bytes, used exactly as stored. Never trimmed. The version
// this replaced trimmed every file, so a random key whose first or last byte
// happened to be one of the six whitespace values — about one key in twenty —
// came back 31 bytes long and the service refused a key that had been
// generated correctly. The two services on devnet-2 hold raw keys, which is why
// raw is still read at all.
//
// They cannot collide: a raw secret is 32 bytes, and valid hex for 32 bytes is
// at least 64 characters. Anything else is refused, because the one case the
// old trimming was for — a typed secret plus an editor's newline — is
// indistinguishable from a random key that ends in a newline byte, and
// guessing is how the service came to refuse good keys.
func parseSecret(path string, raw []byte) ([]byte, error) {
	if text := strings.TrimSpace(string(raw)); len(text) >= 2*secretLen && len(text)%2 == 0 {
		if decoded, err := hex.DecodeString(text); err == nil {
			return decoded, nil
		}
	}
	if len(raw) == secretLen {
		return raw, nil
	}
	return nil, fmt.Errorf(
		"%s is %d bytes and is neither hex nor exactly %d raw bytes; "+
			"write it as hex with `openssl rand -hex 32 > %s`",
		path, len(raw), secretLen, path)
}

// importAccount enrols an account from a share file produced by the ceremony.
func (s *server) importAccount(holdingPath, email, password, importAddress string) error {
	if email == "" || password == "" {
		return errors.New("--import needs --import-email and --import-password")
	}
	raw, err := os.ReadFile(holdingPath)
	if err != nil {
		return err
	}
	var holding recovery.Holding
	if err := json.Unmarshal(raw, &holding); err != nil {
		return fmt.Errorf("reading the holding: %w", err)
	}
	if holding.Role != s.role {
		return fmt.Errorf(
			"that is a %s holding and this deployment is the %s. A service keeps exactly its own: "+
				"importing any other would give it two of the three",
			holding.Role, s.role)
	}
	// The address cannot be derived from a holding — it is a share of a
	// ciphertext, and deriving anything from it would need the password. So an
	// import takes the address alongside, and an operator importing the wrong
	// one produces an account that recovers to a key controlling a different
	// address. That is a real footgun and it is why this is an operator tool
	// rather than an endpoint.
	address := importAddress
	if address == "" {
		return errors.New("--import needs --import-address: an address cannot be derived from a holding")
	}
	if err := chain.CheckAddress(address); err != nil {
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
	}, holding); err != nil {
		return err
	}
	fmt.Println(address)
	fmt.Fprintf(os.Stderr, "enrolled %s\n", address)
	return nil
}

// ---------------------------------------------------------------- endpoints

// The signing endpoints are gone.
//
// They were the largest thing in this file: a three-message exchange that fed
// the custodian share into a threshold signing party, plus the rules about when
// it would run at all — one signature per account at a time, a two-minute
// session TTL, and a second factor demanded at the moment of signing rather
// than at login.
//
// The decision of 2026-09-18 removed the reason for all of it. A deal is
// between a buyer and a seller; the custodian appears only in a recovery. The
// device holds an ordinary key and signs with it, so there is no second party
// to a signature and nothing here to gate.
//
// Worth being clear about what was given up, because those rules were good ones
// and something real went with them: this service can no longer refuse a
// payment, and a frozen account can still spend. A freeze now blocks a
// RECOVERY, which is a narrower promise than the signing path made. Nothing in
// this codebase can stop a thief who has both the phone and the password —
// that is the shape of the architecture, not an oversight in it.

// handleFreeze marks an account as reported stolen.
//
// Password-authenticated, which is the right bar for this one action: somebody reporting a stolen
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
	body := map[string]any{
		"role":           s.role,
		"accounts":       n,
		"uptime_seconds": int(time.Since(s.started).Seconds()),
		"holds":          "one holding per account: a share of a ciphertext this service cannot open",
		"enrolment":      "ready",
	}

	// There is no pre-parameter pool to report and no open enrolments to count.
	// Both were real availability signals — an empty pool meant new accounts
	// had been silently refused for however long it had been empty — and both
	// disappeared with the protocol that needed them. Enrolment is now ready
	// whenever the process is running.

	// Whether recovery can run at all. A deployment with no notifier refuses to
	// start one, by design — and an operator should be able to see that here
	// rather than discover it the first time somebody needs a recovery.
	switch {
	case s.codes == nil:
		body["signin"] = "authenticator only: no --signin-code-key-file, so an emailed code cannot be checked"
	case s.role == recovery.RoleCustodian && s.mailer == nil:
		body["signin"] = "authenticator only: no --smtp-config, so no code can be emailed"
	default:
		body["signin"] = "ready"
	}
	if _, ok := s.notifier.(refusingNotifier); ok {
		body["recovery"] = "disabled: no notify command, and a recovery nobody is told about is worse than none"
	} else {
		body["recovery"] = "ready"
	}

	respond(w, body)
}

// authenticate resolves and checks an account, or writes the refusal.
//
// Every failure is the same refusal with the same status and the same wording.
// "No such account" and "wrong password" as distinct answers turn this endpoint
// into an oracle for which email addresses are enrolled.
func (s *server) authenticate(w http.ResponseWriter, email, password string) (Account, bool) {
	idx := s.index.Of(email)

	// Before anything is read. A guess made during the wait is refused
	// unchecked, and an address with no account waits the same as one with an
	// account, so neither the wait nor its absence says which it is.
	now := s.clock()
	if wait := s.attempts.wait(idx, now); wait > 0 {
		w.Header().Set("Retry-After", retryAfter(wait))
		refuse(w, http.StatusTooManyRequests,
			"too many wrong passwords for this account; try again in "+wait.Round(time.Second).String())
		return Account{}, false
	}

	account, err := s.store.Get(idx)
	if err != nil {
		// Still spend the time. Returning fast for an unknown account leaks
		// the same fact the wording was careful not to.
		_, _ = NewVerifier("a password that goes nowhere at all")
		s.attempts.failed(idx, now)
		refuse(w, http.StatusUnauthorized, "that email and password do not match an account")
		return Account{}, false
	}
	if !account.Password.Verify(password) {
		s.attempts.failed(idx, now)
		refuse(w, http.StatusUnauthorized, "that email and password do not match an account")
		return Account{}, false
	}
	s.attempts.succeeded(idx)
	if account.Frozen {
		refuse(w, http.StatusForbidden, "this account is frozen: "+account.FrozenReason)
		return Account{}, false
	}

	// The post-recovery freeze is deliberately NOT applied here.
	//
	// It used to be, and this was the right place then: it was checked on every
	// signature rather than at sign-in, so a session opened before the freeze
	// began could not outlive it.
	//
	// There are no signatures to check now. The freeze moved to the one action
	// this service still performs after a recovery — releasing the holding —
	// and handleRecoveryClaim enforces it there, with a reason and a
	// Retry-After. Applying it here as well would refuse the claim with 403 and
	// a message about payments being held, on a service that no longer has any
	// idea whether a payment is being made.
	//
	// The account-level Frozen flag above still applies, because a reported
	// theft should stop a recovery outright rather than delay it.

	return account, true
}

// newID is a random identifier for a recovery.
//
// It lived in session.go, which went with the signing sessions. Moved rather
// than reimplemented: recovery.go is its only remaining caller and an
// identifier generated two slightly different ways in one service is a thing
// somebody eventually has to reconcile.
//
// 18 bytes, so the base64 is 24 characters with no padding and the value is not
// guessable. A recovery identifier is handed out over HTTP and quoted back, and
// one an attacker can enumerate would let them cancel or approve somebody
// else's recovery.
func newID() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
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
