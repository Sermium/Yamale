package main

// The ceremony as a local web page.
//
// The terminal version of this tool is the stronger one and it keeps working.
// This exists because of who is in the room. Five custodians who may be central
// bankers, judges or auditors are not going to read an eighty-column console, and
// "read word seventeen back to me" across a table needs type somebody can
// actually see from the other side of it. A ceremony people cannot follow is a
// ceremony they nod along to, and a custodian nodding along is a custodian who
// has not checked their sheet.
//
// Everything below is presentation over the functions the CLI already uses.
// There is no second derivation path, no second BIP-39 implementation and no
// second transcription check: newSecret, secret.derive, identityOf, pickPositions,
// wordMatches, buildGroup and renderRecord are the same code the terminal path
// runs. That is not tidiness. A web layer that derived its own addresses would
// eventually derive them slightly differently, and the failure — five custodians
// holding keys that look right and control nothing — would not surface until the
// first seizure.
//
// What a browser costs, and what is done about it:
//
//	Loopback only, and it refuses anything else. There is precedent in this
//	project: a signing service bound 0.0.0.0 while its credential gate lived on a
//	different host, and accepted unauthenticated signing requests on every
//	interface for days. So --bind is validated, the bound address is re-checked
//	after listen, every request's remote address is checked again per request, and
//	there is no fallback path that could turn a failure to bind loopback into a
//	success at binding something else.
//
//	A one-time token in the URL, printed to the terminal. It is not a password —
//	it dies with the process — it is what stops any other program on the machine
//	from driving the ceremony over loopback.
//
//	The Host header is checked. A page on any domain can resolve that domain to
//	127.0.0.1 and reach a loopback server; the token already stops it reading
//	anything, and refusing a foreign Host stops it trying.
//
//	No-store on every response, and the phrase is served once. See spendGrant.

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed ui.html
var uiFiles embed.FS

// runServe is the `ceremony serve` command.
func runServe(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	modeFlag := flags.String("mode", string(modeColocated), "co-located (one machine, everybody in the room) or custodian (this machine holds one custodian's key)")
	out := flags.String("out", ".", "directory for the public records and documents this ceremony produces")
	bind := flags.String("bind", "127.0.0.1", "address to listen on; only a loopback address is accepted")
	port := flags.Int("port", 0, "port to listen on; 0 picks a free one, which is what you want")
	browserPath := flags.String("browser", "", "path to the browser executable to launch; empty finds one")
	noBrowser := flags.Bool("no-browser", false, "print the URL instead of launching a browser — you then prepare the profile yourself")
	if err := flags.Parse(args); err != nil {
		return err
	}

	sessionMode := mode(*modeFlag)
	if sessionMode != modeColocated && sessionMode != modeCustodian {
		return fmt.Errorf("--mode must be %q or %q, not %q", modeColocated, modeCustodian, *modeFlag)
	}

	if err := os.MkdirAll(*out, 0o700); err != nil {
		return fmt.Errorf("could not use %s as the output directory: %w", *out, err)
	}

	listener, err := listenLoopback(*bind, *port)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()

	token, err := newToken()
	if err != nil {
		return err
	}

	s := newSession(sessionMode, *out)
	server := &http.Server{
		Handler: newCeremonyServer(s, token),
		// A ceremony step can take as long as a custodian needs to write
		// twenty-four words, so there is no read or write deadline. The
		// connection is loopback and the client is a browser this program
		// launched.
		ReadHeaderTimeout: 10 * time.Second,
	}

	url := fmt.Sprintf("http://%s/?t=%s", listener.Addr().String(), token)

	var browser *controlledBrowser
	if *noBrowser {
		printManualInstructions(url)
	} else {
		browser, err = launchBrowser(url, *browserPath, "")
		if err != nil {
			return err
		}
		fmt.Printf("Launched %s on a temporary profile at %s\n", browser.Name, browser.Profile)
		fmt.Println("That profile has no extensions, no autofill, no history and no session to restore,")
		fmt.Println("and it is deleted when this program exits.")
		fmt.Println()
		fmt.Printf("If the window did not open: %s\n", url)
	}

	fmt.Println()
	fmt.Printf("Serving the ceremony on %s. Nothing is listening on any other interface.\n", listener.Addr())
	fmt.Println("Close the browser window, or press Ctrl-C, to end the session and wipe the profile.")
	fmt.Println()

	// Three ways this ends, and all of them run the same teardown: the operator
	// closes the browser, the operator presses Ctrl-C, or the page finishes the
	// ceremony. Anything that only cleaned up on one of the three would leave a
	// profile directory behind on exactly the machine that is supposed to have
	// nothing left on it.
	done := make(chan string, 3)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		done <- "interrupted"
	}()

	if browser != nil {
		go func() {
			_ = browser.wait()
			if browser.exitedImmediately() {
				fmt.Println("The browser process exited immediately. Leaving the server up — open the URL above by hand.")
				return
			}
			done <- "the browser was closed"
		}()
	}

	go func() {
		<-s.finished
		// Long enough for the final screen's response, including its
		// Clear-Site-Data header, to reach the browser and be acted on.
		time.Sleep(3 * time.Second)
		done <- "the ceremony was completed"
	}()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "ceremony: serving stopped:", err)
			done <- "the server stopped"
		}
	}()

	reason := <-done
	fmt.Printf("\nEnding the session: %s.\n", reason)
	_ = server.Close()

	// Zeroed before the profile is removed, so an interrupt during a slow
	// filesystem delete does not leave a phrase in this process's memory while
	// it waits.
	s.wipe()

	if browser != nil {
		if err := browser.close(); err != nil {
			fmt.Fprintln(os.Stderr, "ceremony:", err)
			return err
		}
		fmt.Printf("Temporary browser profile %s removed.\n", browser.Profile)
	}
	fmt.Println("Every phrase this process held has been overwritten. Power the machine off.")
	return nil
}

// listenLoopback binds a loopback address, or fails.
//
// There is no path through this function that ends in a socket on a routable
// interface. --bind is parsed and checked before listening, and the address the
// kernel actually gave us is checked afterwards — because a hostname, an empty
// string, or a future refactor that passed the flag through differently could all
// produce a wildcard bind from a value that looked fine going in.
//
// A tool holding twenty-four words on screen must not be one hostname resolution
// away from serving them to a network.
func listenLoopback(bind string, port int) (net.Listener, error) {
	ip := net.ParseIP(bind)
	if ip == nil {
		return nil, fmt.Errorf(
			"--bind %q is not an IP address. It has to be a loopback literal — 127.0.0.1 or ::1 — "+
				"because a hostname is resolved by something outside this program and could resolve anywhere",
			bind)
	}
	if !ip.IsLoopback() {
		return nil, fmt.Errorf(
			"refusing to listen on %s: it is not a loopback address.\n"+
				"This page shows recovery phrases. Serving it on an interface anything else can reach is\n"+
				"the mistake that put an unauthenticated signing endpoint on every interface of a host in\n"+
				"this project for days. There is no flag that allows it and no fallback that would do it\n"+
				"quietly",
			bind)
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port)))
	if err != nil {
		return nil, fmt.Errorf("could not listen on %s: %w", bind, err)
	}

	// Checked again on what was actually bound. Cheap, and the one thing that
	// catches a mistake made between the check above and the syscall.
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !addr.IP.IsLoopback() {
		_ = listener.Close()
		return nil, fmt.Errorf("the socket bound %s, which is not loopback; refusing to serve", listener.Addr())
	}
	return listener, nil
}

// newToken is the value that has to be in the URL.
//
// Two hundred and fifty-six bits from crypto/rand, base64url so it survives a
// query string. It authorises nothing beyond this process and is worthless the
// moment it exits — which is why it is allowed to sit in a URL. The browser
// profile that records that URL in its history is a directory this program
// created minutes ago and deletes on the way out.
func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func printManualInstructions(url string) {
	fmt.Println("--no-browser: this tool is NOT controlling the browser profile, so the properties it")
	fmt.Println("would otherwise guarantee are now yours to arrange:")
	fmt.Println()
	for i, step := range manualPreparation {
		fmt.Printf("  %d. %s\n", i+1, step)
	}
	fmt.Println()
	fmt.Println("Then open:")
	fmt.Println()
	fmt.Printf("    %s\n", url)
}

// newCeremonyServer wires the routes.
//
// Split from runServe so the tests drive the real handlers over a real loopback
// socket instead of calling the state machine directly. The properties that
// matter here — a phrase served once, a foreign Host refused, no-store on
// everything — are properties of the HTTP layer, and a test that bypassed it
// would be testing something else.
func newCeremonyServer(s *session, token string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handlePage)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/preflight", s.handlePreflight)
	mux.HandleFunc("/api/params", s.handleParams)
	mux.HandleFunc("/api/custodian/begin", s.handleBegin)
	mux.HandleFunc("/api/custodian/phrase", s.handlePhrase)
	mux.HandleFunc("/api/custodian/written", s.handleWritten)
	mux.HandleFunc("/api/custodian/verify", s.handleVerify)
	mux.HandleFunc("/api/custodian/commit", s.handleCommit)
	mux.HandleFunc("/api/custodian/abandon", s.handleAbandon)
	mux.HandleFunc("/api/restore", s.handleRestore)
	mux.HandleFunc("/api/submission", s.handleSubmission)
	mux.HandleFunc("/api/assemble", s.handleAssemble)
	mux.HandleFunc("/api/genesis", s.handleGenesis)
	mux.HandleFunc("/api/attest", s.handleAttest)
	mux.HandleFunc("/api/attestation", s.handleAttestation)
	mux.HandleFunc("/api/record", s.handleRecord)
	mux.HandleFunc("/api/complete", s.handleComplete)

	return guard(token, mux)
}

// loopbackOnly reports whether a request arrived over loopback.
func loopbackOnly(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// allowedHost reports whether the Host header names this machine.
//
// A page served from any domain can point that domain at 127.0.0.1 and then talk
// to a loopback server as same-origin — the browser sees one origin, the server
// sees a local connection, and both are satisfied. The token already stops that
// page reading anything, since it cannot guess two hundred and fifty-six bits.
// This is the second lock: a request whose Host is somebody's domain name is not
// a request from the browser this program launched.
func allowedHost(host string) bool {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	name = strings.TrimSuffix(strings.TrimPrefix(name, "["), "]")
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

// guard is the middleware every response passes through.
func guard(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()

		// no-store rather than no-cache. no-cache permits a stored copy that is
		// revalidated; no-store is the one that says do not write this to disk.
		// It is on every response because the interesting responses — the phrase,
		// the verification challenge — are indistinguishable from the rest to
		// whatever is caching.
		header.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		header.Set("Pragma", "no-cache")
		header.Set("Expires", "0")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Cross-Origin-Opener-Policy", "same-origin")
		header.Set("Cross-Origin-Resource-Policy", "same-origin")
		header.Set("Cross-Origin-Embedder-Policy", "require-corp")
		header.Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), display-capture=(), clipboard-read=(), serial=(), usb=(), hid=(), idle-detection=()")

		if !loopbackOnly(r.RemoteAddr) {
			// Loud, on stderr, naming the address. A connection from off-machine
			// to this server means either the bind check has been defeated or
			// something is forwarding, and both are worth stopping the ceremony
			// over.
			fmt.Fprintf(os.Stderr, "ceremony: REFUSED a request from %s — this server is loopback only\n", r.RemoteAddr)
			http.Error(w, "this server answers loopback only", http.StatusForbidden)
			return
		}
		if !allowedHost(r.Host) {
			http.Error(w, "unexpected Host header", http.StatusForbidden)
			return
		}
		// A cross-site fetch cannot read the response without the token anyway,
		// but a browser that tells us the request is cross-site is telling us
		// something no legitimate request would say.
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			http.Error(w, "cross-site requests are refused", http.StatusForbidden)
			return
		}

		presented := r.URL.Query().Get("t")
		if presented == "" {
			presented = r.Header.Get("X-Ceremony-Token")
		}
		if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			http.Error(w, "the one-time token in the URL is missing or wrong. Use the link this tool printed.", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
