package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// trusting is a fetcher that trusts httptest's certificate (every TLS test
// server shares it), admits the given ports on loopback, and does not sleep.
func trusting(t *testing.T, srv *httptest.Server, ports ...uint16) (*fetcher, *[]time.Duration) {
	t.Helper()
	f := newFetcher("")
	if len(ports) > 0 {
		f.allow = func(a netip.AddrPort) bool {
			for _, p := range ports {
				if a.Port() == p {
					return true
				}
			}
			return false
		}
	}
	var mu sync.Mutex
	slept := &[]time.Duration{}
	f.sleep = func(_ context.Context, d time.Duration) error { mu.Lock(); *slept = append(*slept, d); mu.Unlock(); return nil }
	f.transport().TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	return f, slept
}

func portOf(t *testing.T, srv *httptest.Server) uint16 {
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	p, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return uint16(p)
}

func TestOnlyPublicAddressesArePublic(t *testing.T) {
	for _, a := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.188", "100.64.0.1", "100.68.207.17",
		"169.254.169.254", "0.0.0.0", "::1", "fe80::1", "fd00:ec2::254", "::ffff:127.0.0.1", "224.0.0.1"} {
		require.False(t, publicAddr(netip.MustParseAddr(a)), a)
	}
	for _, a := range []string{"23.227.38.65", "188.245.100.184", "2606:4700::6810:84e5"} {
		require.True(t, publicAddr(netip.MustParseAddr(a)), a)
	}
}

func TestHTTPIsRefusedWithoutAConnection(t *testing.T) {
	_, _, err := newFetcher("").get(context.Background(), "http://example.com/products.json", acceptPages)
	require.ErrorIs(t, err, errNotPublic)
}

func TestALoopbackServerIsRefusedAtDialTime(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("{}")) }))
	defer srv.Close()
	f, _ := trusting(t, srv) // default allow: public only
	_, _, err := f.get(context.Background(), srv.URL+"/products.json", acceptPages)
	require.ErrorIs(t, err, errNotPublic)
}

func TestARedirectToAPrivateAddressIsRefused(t *testing.T) {
	inner := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("secret")) }))
	defer inner.Close()
	outer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, inner.URL+"/admin", http.StatusFound)
	}))
	defer outer.Close()
	f, _ := trusting(t, outer, portOf(t, outer)) // the inner server's port is not admitted
	_, _, err := f.get(context.Background(), outer.URL+"/products.json", acceptPages)
	require.ErrorIs(t, err, errNotPublic)
}

func TestThreeRedirectsAndNoMore(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("stop") == strconv.Itoa(n) {
			w.Write([]byte("ok"))
			return
		}
		http.Redirect(w, r, srv.URL+"/p?stop="+r.URL.Query().Get("stop")+"&n="+strconv.Itoa(n+1), http.StatusFound)
	}))
	defer srv.Close()
	f, _ := trusting(t, srv, portOf(t, srv))
	body, _, err := f.get(context.Background(), srv.URL+"/p?stop=3&n=0", acceptPages)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))
	_, _, err = f.get(context.Background(), srv.URL+"/p?stop=4&n=0", acceptPages)
	require.ErrorIs(t, err, errTooManyRedirects)
}

func TestABodyOverFiveMegabytesIsRefused(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Write(bytes.Repeat([]byte{'x'}, 6<<20))
	}))
	defer srv.Close()
	f, _ := trusting(t, srv, portOf(t, srv))
	_, _, err := f.get(context.Background(), srv.URL+"/big", acceptPages)
	require.ErrorIs(t, err, errTooBig)
}

func TestASlowServerTimesOut(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(500 * time.Millisecond)
		w.Write([]byte("late"))
	}))
	defer srv.Close()
	f, _ := trusting(t, srv, portOf(t, srv))
	f.client.Timeout = 100 * time.Millisecond
	_, _, err := f.get(context.Background(), srv.URL+"/slow", acceptPages)
	require.ErrorIs(t, err, errUnreachable)
}

func TestA429IsWaitedOutThenGivenUpOn(t *testing.T) {
	hits := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			http.NotFound(w, r)
		case "/once":
			hits++
			if hits == 1 {
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Write([]byte("ok"))
		case "/long":
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer srv.Close()
	f, slept := trusting(t, srv, portOf(t, srv))
	body, _, err := f.get(context.Background(), srv.URL+"/once", acceptPages)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))
	require.Contains(t, *slept, 7*time.Second)
	_, _, err = f.get(context.Background(), srv.URL+"/long", acceptPages)
	require.ErrorIs(t, err, errRateLimited, "a wait over two minutes is not waited")
}

func TestRequestsToOneHostAreTwoSecondsApart(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()
	f, slept := trusting(t, srv, portOf(t, srv))
	for i := 0; i < 3; i++ {
		_, _, err := f.get(context.Background(), srv.URL+"/p", acceptPages)
		require.NoError(t, err)
	}
	// robots.txt, then three pages, each in the next two-second slot: 0, 2, 4, 6 s.
	var total time.Duration
	for _, d := range *slept {
		total += d
	}
	require.Greater(t, total, 5*time.Second)
}

func TestRobotsStopAFetch(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Write([]byte("User-agent: GPTBot\nDisallow: /\n"))
			return
		}
		w.Write([]byte("page"))
	}))
	defer srv.Close()
	f, _ := trusting(t, srv, portOf(t, srv))
	_, _, err := f.get(context.Background(), srv.URL+"/products.json", acceptPages)
	require.ErrorIs(t, err, errRobots)
}

func TestTheDevOriginIsTheOneException(t *testing.T) {
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("fixture")) }))
	defer dev.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("no")) }))
	defer other.Close()
	f := newFetcher(dev.URL)
	f.sleep = func(context.Context, time.Duration) error { return nil }
	body, _, err := f.get(context.Background(), dev.URL+"/products.json", acceptPages)
	require.NoError(t, err)
	require.Equal(t, "fixture", string(body))
	_, _, err = f.get(context.Background(), other.URL+"/products.json", acceptPages)
	require.ErrorIs(t, err, errNotPublic)
}

func TestAnOriginIsSchemeAndHostOnly(t *testing.T) {
	f := newFetcher("")
	got, err := f.origin("https://WWW.JewelOfAfrica.com/products")
	require.NoError(t, err)
	require.Equal(t, "https://www.jewelofafrica.com", got)
	got, err = f.origin("https://shop.example:443")
	require.NoError(t, err)
	require.Equal(t, "https://shop.example", got)
	for _, bad := range []string{"http://shop.example", "ftp://shop.example", "https://user:pw@shop.example", "not a url", ""} {
		_, err := f.origin(bad)
		require.True(t, errors.Is(err, errNotPublic), bad)
	}
}
