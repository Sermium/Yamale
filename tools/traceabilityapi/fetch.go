package main

// The catalogue reader fetches other people's websites for a jeweler. Every
// request goes through this one fetcher, so its rules hold everywhere:
//
//   - https only, and only public addresses. The address is checked as it is
//     dialled, after DNS and after every redirect, so a name that resolves to
//     the tailnet (100.64/10), a metadata service or loopback is refused
//     however it was reached;
//   - at most three redirects, ten seconds and 5 MB a response;
//   - one request every two seconds to a host. Jewel of Africa's Shopify
//     answered 429 for over ten minutes after four requests in one second
//     (2026-11-20), so a polite pace is the difference between reading a
//     shop and being shut out of it;
//   - a 429 or 503 is waited out when it asks for two minutes or less, twice;
//   - robots.txt, per host, before anything else on that host (robots.go).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	fetchUserAgent = "YamaleCatalogue/1 (+https://traceability.yamaletech.com/about-import)"
	fetchTimeout   = 10 * time.Second
	fetchMaxBytes  = 5 << 20
	fetchRedirects = 3
	fetchInterval  = 2 * time.Second
	fetchMaxWait   = 2 * time.Minute
	fetchRetries   = 2
	robotsTTL      = time.Hour

	acceptPages  = "application/json, text/html;q=0.9, application/xml;q=0.8, */*;q=0.5"
	acceptImages = "image/webp, image/jpeg, image/png;q=0.9"
)

var (
	errNotPublic        = errors.New("only public https addresses can be imported")
	errTooBig           = errors.New("the site answered with more than 5 MB")
	errRateLimited      = errors.New("the site is limiting how fast it can be read")
	errUnreachable      = errors.New("the site could not be reached")
	errRobots           = errors.New("the site asks automated readers not to collect this page")
	errTooManyRedirects = errors.New("the site redirected more than three times")
)

type statusError struct{ code int }

func (e *statusError) Error() string { return fmt.Sprintf("the site answered %d", e.code) }

// pageFetcher is what the readers need; tests give them a map.
type pageFetcher interface {
	get(ctx context.Context, rawURL, accept string) ([]byte, http.Header, error)
}

type cachedRobots struct {
	r  robots
	at time.Time
}

type fetcher struct {
	client    *http.Client
	allow     func(netip.AddrPort) bool
	sleep     func(context.Context, time.Duration) error
	interval  time.Duration
	maxBytes  int64
	maxWait   time.Duration
	devOrigin string // "http://127.0.0.1:port" on yamale-local-1 only; "" elsewhere
	devAddr   netip.AddrPort

	mu     sync.Mutex
	next   map[string]time.Time
	robots map[string]cachedRobots
}

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"::/128", "::1/128", "64:ff9b::/96", "100::/64", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8",
	} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// publicAddr reports whether a is an address on the public internet.
func publicAddr(a netip.Addr) bool {
	a = a.Unmap().WithZone("") // Prefix.Contains is false for a zoned address
	if !a.IsValid() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func newFetcher(devOrigin string) *fetcher {
	f := &fetcher{
		allow:     func(a netip.AddrPort) bool { return publicAddr(a.Addr()) },
		sleep:     sleepCtx,
		interval:  fetchInterval,
		maxBytes:  fetchMaxBytes,
		maxWait:   fetchMaxWait,
		devOrigin: strings.TrimRight(devOrigin, "/"),
		next:      map[string]time.Time{},
		robots:    map[string]cachedRobots{},
	}
	if f.devOrigin != "" {
		if u, err := url.Parse(f.devOrigin); err == nil {
			f.devAddr, _ = netip.ParseAddrPort(u.Host)
		}
	}
	dialer := &net.Dialer{
		Timeout: fetchTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return errNotPublic
			}
			if f.devAddr.IsValid() && ap == f.devAddr {
				return nil
			}
			if !f.allow(ap) {
				return errNotPublic
			}
			return nil
		},
	}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   fetchTimeout,
		ResponseHeaderTimeout: fetchTimeout,
		MaxIdleConnsPerHost:   2,
	}
	f.client = &http.Client{
		Timeout:   fetchTimeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > fetchRedirects {
				return errTooManyRedirects
			}
			if !f.schemeOK(req.URL) {
				return errNotPublic
			}
			// A redirect is a new request: robots.txt and the host's pace apply to
			// it. A hop on robots.txt itself is exempt, or the check would recurse.
			if req.URL.Path != "/robots.txt" {
				rb, err := f.robotsFor(req.Context(), req.URL)
				if err != nil {
					return err
				}
				if !rb.allowed(req.URL.RequestURI()) {
					return errRobots
				}
			}
			return f.wait(req.Context(), req.URL.Host)
		},
	}
	return f
}

func (f *fetcher) transport() *http.Transport { return f.client.Transport.(*http.Transport) }

// schemeOK: https, or exactly the devnet's fixture origin.
func (f *fetcher) schemeOK(u *url.URL) bool {
	if u.Scheme == "https" && u.Host != "" {
		return true
	}
	return f.devOrigin != "" && u.Scheme+"://"+u.Host == f.devOrigin
}

// origin reduces raw to scheme://host as a browser's URL.origin writes it, or
// refuses it.
func (f *fetcher) origin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || !f.schemeOK(u) {
		return "", errNotPublic
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if p := u.Port(); p != "" && !(u.Scheme == "https" && p == "443") && !(u.Scheme == "http" && p == "80") {
		host += ":" + p
	}
	return u.Scheme + "://" + host, nil
}

// get fetches rawURL if the host's robots.txt allows it.
func (f *fetcher) get(ctx context.Context, rawURL, accept string) ([]byte, http.Header, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !f.schemeOK(u) {
		return nil, nil, errNotPublic
	}
	rb, err := f.robotsFor(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	if !rb.allowed(u.RequestURI()) {
		return nil, nil, errRobots
	}
	return f.raw(ctx, u.String(), accept)
}

func (f *fetcher) robotsFor(ctx context.Context, u *url.URL) (robots, error) {
	key := u.Scheme + "://" + u.Host
	f.mu.Lock()
	c, ok := f.robots[key]
	f.mu.Unlock()
	if ok && time.Since(c.at) < robotsTTL {
		return c.r, nil
	}
	body, _, err := f.raw(ctx, key+"/robots.txt", "text/plain")
	var se *statusError
	switch {
	case err == nil:
		c = cachedRobots{r: parseRobots(body), at: time.Now()}
	case errors.As(err, &se) && se.code >= 400 && se.code < 500:
		c = cachedRobots{at: time.Now()} // no robots.txt: nothing is asked of us
	default:
		return robots{}, err
	}
	f.mu.Lock()
	f.robots[key] = c
	f.mu.Unlock()
	return c.r, nil
}

// wait takes the host's next slot and sleeps until it.
func (f *fetcher) wait(ctx context.Context, host string) error {
	f.mu.Lock()
	now := time.Now()
	at := f.next[host]
	if at.Before(now) {
		at = now
	}
	f.next[host] = at.Add(f.interval)
	f.mu.Unlock()
	return f.sleep(ctx, time.Until(at))
}

func retryAfter(v string) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return time.Minute
}

// raw is one paced request, without robots.txt.
func (f *fetcher) raw(ctx context.Context, rawURL, accept string) ([]byte, http.Header, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !f.schemeOK(u) {
		return nil, nil, errNotPublic
	}
	for attempt := 0; ; attempt++ {
		if err := f.wait(ctx, u.Host); err != nil {
			return nil, nil, fmt.Errorf("%w: %v", errUnreachable, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, nil, errNotPublic
		}
		req.Header.Set("User-Agent", fetchUserAgent)
		req.Header.Set("Accept", accept)
		res, err := f.client.Do(req)
		if err != nil {
			switch {
			case errors.Is(err, errNotPublic):
				return nil, nil, errNotPublic
			case errors.Is(err, errTooManyRedirects):
				return nil, nil, errTooManyRedirects
			case errors.Is(err, errRobots):
				return nil, nil, errRobots
			}
			return nil, nil, fmt.Errorf("%w: %v", errUnreachable, err)
		}
		if res.StatusCode == http.StatusTooManyRequests || res.StatusCode == http.StatusServiceUnavailable {
			wait := retryAfter(res.Header.Get("Retry-After"))
			res.Body.Close()
			if attempt >= fetchRetries || wait > f.maxWait {
				return nil, nil, errRateLimited
			}
			if err := f.sleep(ctx, wait); err != nil {
				return nil, nil, errRateLimited
			}
			continue
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, f.maxBytes+1))
		res.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", errUnreachable, err)
		}
		if int64(len(body)) > f.maxBytes {
			return nil, nil, errTooBig
		}
		if res.StatusCode != http.StatusOK {
			return nil, res.Header, &statusError{res.StatusCode}
		}
		return body, res.Header, nil
	}
}
