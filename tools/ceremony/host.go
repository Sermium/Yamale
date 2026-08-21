package main

// The ceremony hosted for five custodians on five devices, anywhere.
//
// `ceremony serve` reaches exactly one machine, which is the right shape for the
// air-gapped ceremony and no use at all for a rehearsal with five people in five
// cities. This command issues one link per custodian and serves each of them a
// page that generates their key IN THEIR OWN BROWSER.
//
// That last part is the whole design, and it is worth being exact about why.
// This tool exists because the foundation account was once a single key on a
// single VM. If the coordinator generated the five phrases and sent them down the
// links, one machine would again have held every key to the account that receives
// every seized asset — strictly worse than five people in a room, which is the
// thing this replaces. So the coordinator serves code and relays public
// material, and there is no request in this file that could carry a phrase:
//
//   - Every request body is decoded with DisallowUnknownFields, so a body with a
//     phrase field in it is a 400 rather than a field this program ignores. A
//     modified page cannot post a phrase here even deliberately.
//   - No request type declared in hostsession.go has a field that could hold
//     seed material, and TestNoHostRouteAcceptsAPhrase drives every route in
//     hostRoutes with a phrase in the body to prove the first point holds for
//     all of them rather than for the ones somebody remembered.
//   - The custodian's name comes from the invite token, never from the body, so
//     a link cannot be redeemed for somebody else.
//
// What this arrangement costs, stated because the page says it too: the
// custodian is trusting the served code. A dishonest coordinator could serve a
// page that transmits the phrase, and no property of this binary would stop it.
// The mitigation is that the bundle is committed, unminified and hashed — the
// hash is printed at startup and shown in the page — so the code being run can
// be compared against a published value. The air-gapped binary needs no such
// argument, which is why it remains the stronger option and stays available.

import (
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"sort"
	"strings"
	"syscall"
	"time"
)

// hostedFiles is the built client bundle.
//
// Committed rather than gitignored build output, and embedded rather than served
// from disk, for two separate reasons. Committed, so `go build ./...` works on a
// clone with no JavaScript toolchain. Embedded, so a coordinator needs one binary
// and cannot accidentally serve a stale directory next to a fresh binary — which
// would make the bundle hash printed at startup a hash of something other than
// what the custodians are running.
//
//go:embed hosted
var hostedFiles embed.FS

func runHost(args []string) error {
	flags := flag.NewFlagSet("host", flag.ExitOnError)
	bind := flags.String("bind", "127.0.0.1", "address to listen on; only a loopback address is accepted")
	port := flags.Int("port", 8787, "port to listen on")
	prefix := flags.String("path", "/ceremony/", "URL path this ceremony is served under")
	publicURL := flags.String("public-url", "", "the URL custodians will reach, e.g. https://pay.example.com/ceremony/ — the invite links are built from it")
	out := flags.String("out", ".", "directory for the exported record and genesis fragment")
	if err := flags.Parse(args); err != nil {
		return err
	}

	mount, err := normalisePrefix(*prefix)
	if err != nil {
		return err
	}
	public, err := publicBase(*publicURL, *bind, *port, mount)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(*out, 0o700); err != nil {
		return fmt.Errorf("could not use %s as the output directory: %w", *out, err)
	}

	// listenLoopback rather than a bind of the operator's choosing, and it is the
	// same function `serve` uses. There is precedent in this project for why:
	// a signing service bound 0.0.0.0 while its credential gate sat on another
	// host, and accepted unauthenticated requests on every interface for days.
	// A hosted ceremony is meant to sit behind nginx with the existing
	// certificate, so loopback is not a limitation here — it is the whole
	// deployment.
	listener, err := listenLoopback(*bind, *port)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()

	bundle, err := hostedBundle()
	if err != nil {
		return err
	}

	coordinatorToken, err := newToken()
	if err != nil {
		return err
	}

	h := newHostSession(*out, public, bundle)
	server := &http.Server{
		Handler:           newHostServer(h, mount, coordinatorToken, public.Host),
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Println("=== hosted key ceremony ===")
	fmt.Println()
	fmt.Printf("Listening on %s, and on nothing else.\n", listener.Addr())
	fmt.Printf("Custodians reach it at %s\n", public.String())
	fmt.Println()
	fmt.Println("Put an nginx proxy in front of this. It expects to sit behind one:")
	fmt.Printf("  location %s { proxy_pass http://%s%s; proxy_set_header Host $host; }\n",
		mount, listener.Addr(), mount)
	fmt.Println()
	fmt.Println("Bundle SHA-256, which is what a custodian sees in the page and can compare")
	fmt.Println("against the value published in docs/guides/key-ceremony.md:")
	fmt.Println()
	fmt.Printf("  %s\n", bundle.Hash)
	fmt.Println()
	fmt.Println("Open the coordinator page. This link is the ceremony; anybody who has it can")
	fmt.Println("issue invites, so it is not a link to paste into a group chat:")
	fmt.Println()
	fmt.Printf("  %s?t=%s\n", public.String(), coordinatorToken)
	fmt.Println()
	fmt.Println("Ctrl-C ends the ceremony. Nothing here holds a phrase, so there is nothing to")
	fmt.Println("wipe on the way out — the phrases only ever existed in the custodians' browsers.")
	fmt.Println()

	done := make(chan string, 2)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		done <- "interrupted"
	}()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "ceremony:", err)
			done <- "the server stopped"
		}
	}()

	reason := <-done
	fmt.Printf("\nEnding the ceremony: %s.\n", reason)
	return server.Close()
}

// normalisePrefix forces the mount path into the one shape the router and the
// page both assume: a leading and a trailing slash.
//
// Checked rather than tolerated, because "/ceremony" and "/ceremony/" produce
// different relative asset URLs in the browser, and the failure is a page that
// loads with no script and looks like a blank screen rather than an error.
func normalisePrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", errors.New("--path cannot be empty; use / to serve at the root")
	}
	cleaned := path.Clean("/" + strings.Trim(prefix, "/"))
	if cleaned == "/" {
		return "/", nil
	}
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("--path %q is not a path this will serve under", prefix)
	}
	return cleaned + "/", nil
}

// publicBase is the URL the invite links are built from.
//
// It has to be given explicitly when there is a proxy in front, because this
// program cannot see the name the custodian typed: the request arrives from
// nginx on loopback with whatever Host nginx chose to forward. A link built from
// the listener's own address would be http://127.0.0.1:8787/..., which works on
// the coordinator's machine and nowhere else — and a QR code of it would send
// five custodians to their own phones.
func publicBase(publicURL, bind string, port int, mount string) (*url.URL, error) {
	if strings.TrimSpace(publicURL) == "" {
		return &url.URL{Scheme: "http", Host: net.JoinHostPort(bind, fmt.Sprintf("%d", port)), Path: mount}, nil
	}
	parsed, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil {
		return nil, fmt.Errorf("--public-url %q is not a URL: %w", publicURL, err)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("--public-url %q has no host", publicURL)
	}
	if parsed.Scheme != "https" && !isLoopbackHost(parsed.Host) {
		// A phrase is not transmitted, so this is not about the phrase. It is
		// about the page: over plain HTTP anybody on the path can replace the
		// bundle with one that does transmit it, and the hash shown in the page
		// would be the hash of the substituted code.
		return nil, fmt.Errorf(
			"--public-url %q is not https. The custodian's browser generates the key from the code this URL "+
				"serves, so anything that can rewrite the response owns every key in the ceremony",
			publicURL)
	}
	if parsed.Path == "" {
		parsed.Path = mount
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	if parsed.Path != mount {
		return nil, fmt.Errorf(
			"--public-url path %q and --path %q disagree; the links would point somewhere this is not serving",
			parsed.Path, mount)
	}
	return parsed, nil
}

func isLoopbackHost(host string) bool {
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

// bundleInfo is the served client code and its digest.
type bundleInfo struct {
	Hash  string
	Files map[string][]byte
	// PerFile lets the page name which file it hashed. The combined hash covers
	// the whole set; a custodian comparing one script against a published digest
	// needs that file's own digest as well.
	PerFile map[string]string
}

// hostedBundle reads the embedded assets and hashes them.
//
// The combined hash is over the file names as well as the contents, length
// prefixed, so a rename or a file added with the same total bytes changes it. A
// digest that only covered concatenated contents would let a second script be
// smuggled in beside the first.
func hostedBundle() (*bundleInfo, error) {
	entries, err := fs.Sub(hostedFiles, "hosted")
	if err != nil {
		return nil, err
	}
	info := &bundleInfo{Files: map[string][]byte{}, PerFile: map[string]string{}}
	err = fs.WalkDir(entries, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(entries, name)
		if err != nil {
			return err
		}
		info.Files[name] = data
		sum := sha256.Sum256(data)
		info.PerFile[name] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(info.Files) == 0 {
		return nil, errors.New(
			"this binary has no client bundle embedded, so it cannot serve the ceremony. " +
				"Build it: cd clients && npm install && npm run build --workspace @yamale/ceremony")
	}

	names := make([]string, 0, len(info.Files))
	for name := range info.Files {
		names = append(names, name)
	}
	sort.Strings(names)

	combined := sha256.New()
	for _, name := range names {
		combined.Write(canonField(nil, name))
		combined.Write(canonBytes(nil, info.Files[name]))
	}
	info.Hash = hex.EncodeToString(combined.Sum(nil))
	return info, nil
}

// hostRoute is one endpoint.
//
// Declared as data rather than as a series of mux.HandleFunc calls so the tests
// can iterate the same list the server registers. TestNoHostRouteAcceptsAPhrase
// depends on that: a route somebody added and forgot to test would otherwise be
// exactly the route with the hole in it.
type hostRoute struct {
	Path string
	// Audience decides which credential opens it.
	Audience    audience
	AcceptsBody bool
	Handle      http.HandlerFunc
}

type audience int

const (
	// audiencePublic is the bundle and the bundle hash. Public because the page
	// has to load before anybody has been identified, and because the code and
	// its digest are meant to be published.
	audiencePublic audience = iota
	// audienceCoordinator is the ceremony's setup, the invites and the record.
	audienceCoordinator
	// audienceCustodian is opened by an invite token, which names exactly one
	// custodian.
	audienceCustodian
)

func newHostServer(h *hostSession, mount, coordinatorToken, publicHost string) http.Handler {
	mux := http.NewServeMux()
	for _, route := range h.routes() {
		route := route
		mux.Handle(mount+route.Path, hostGuard(h, route, coordinatorToken, publicHost))
	}

	// Each embedded file gets its own exact route rather than a directory
	// handler. There is no path to construct, so there is no path traversal to
	// get wrong, and a request for a file this binary does not carry is a 404
	// from the mux rather than something the filesystem decides.
	for name := range h.bundle.Files {
		if name == "index.html" {
			continue
		}
		mux.HandleFunc(mount+name, h.serveAsset(name))
	}

	// One page, one bundle, one hash. Which flow runs is decided by the query in
	// the link — ?i= is a custodian's invitation, ?t= is the coordinator — so
	// there is no second build to keep in step and no second digest to publish.
	mux.HandleFunc(mount, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != mount {
			http.NotFound(w, r)
			return
		}
		h.servePage(w)
	})
	if mount != "/" {
		// A coordinator who types the path without the trailing slash gets a
		// redirect rather than a 404, because the relative asset URLs in the
		// page depend on it and the resulting blank screen is impossible to
		// diagnose from the browser.
		mux.HandleFunc(strings.TrimSuffix(mount, "/"), func(w http.ResponseWriter, r *http.Request) {
			target := mount
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})
	}
	return mux
}

// hostGuard is the middleware every API response passes through.
func hostGuard(h *hostSession, route hostRoute, coordinatorToken, publicHost string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Cross-Origin-Opener-Policy", "same-origin")
		header.Set("Cross-Origin-Resource-Policy", "same-origin")
		header.Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), display-capture=(), serial=(), usb=(), hid=(), idle-detection=()")

		if !hostAllowed(r.Host, publicHost) {
			http.Error(w, "unexpected Host header", http.StatusForbidden)
			return
		}
		// Refused for API calls only. A document request genuinely is cross-site
		// when a custodian follows their link out of a message, and refusing
		// that would break the one journey this command exists for.
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" && r.Header.Get("Sec-Fetch-Dest") != "document" {
			http.Error(w, "cross-site requests are refused", http.StatusForbidden)
			return
		}
		// A route that takes no arguments takes no body either, and says so
		// before the handler runs. Without this a POST to a read-only route
		// would be answered happily with its body never read — so a modified
		// page could put a phrase in one and the refusal every other route
		// gives would not apply to it.
		if !route.AcceptsBody && r.Method != http.MethodGet {
			http.Error(w, "this step is a GET and takes no body", http.StatusMethodNotAllowed)
			return
		}

		switch route.Audience {
		case audiencePublic:
		case audienceCoordinator:
			if subtle.ConstantTimeCompare([]byte(presentedToken(r)), []byte(coordinatorToken)) != 1 {
				http.Error(w, "this is the coordinator's page and the link is missing its token.", http.StatusForbidden)
				return
			}
		case audienceCustodian:
			invite, err := h.inviteFor(presentedToken(r))
			if err != nil {
				fail(w, http.StatusForbidden, err)
				return
			}
			r = r.WithContext(withInvite(r.Context(), invite))
		}

		route.Handle(w, r)
	})
}

func presentedToken(r *http.Request) string {
	if header := r.Header.Get("X-Ceremony-Token"); header != "" {
		return header
	}
	return r.URL.Query().Get("t")
}

// hostAllowed checks the Host header against the name the ceremony is published
// under.
//
// It cannot be the loopback-only check `serve` uses, because a hosted ceremony
// arrives from nginx carrying the public name. So the accepted set is exactly
// that name plus loopback, and nothing else: a request whose Host is some other
// domain is a page on that domain talking to this server, which is a thing no
// legitimate request does.
func hostAllowed(host, publicHost string) bool {
	if isLoopbackHost(host) {
		return true
	}
	if publicHost == "" {
		return false
	}
	if strings.EqualFold(host, publicHost) {
		return true
	}
	// A proxy may drop the default port. Compared without it rather than
	// guessing which side carries it.
	name, _, err := net.SplitHostPort(publicHost)
	if err != nil {
		return false
	}
	return strings.EqualFold(host, name)
}
