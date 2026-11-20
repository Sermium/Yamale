package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTheShopsRobotsAllowWhatTheImportReads(t *testing.T) {
	raw, err := os.ReadFile("testdata/catalogue/shopify-robots.txt")
	require.NoError(t, err)
	r := parseRobots(raw)
	for _, p := range []string{"/products.json?limit=250&page=1", "/meta.json", "/sitemap.xml", "/products/emerald-parrot-pendant-071", "/robots.txt"} {
		require.True(t, r.allowed(p), p)
	}
	for _, p := range []string{"/admin", "/checkout", "/cart/add", "/collections/rings?sort_by=price", "/x?oseid=1"} {
		require.False(t, r.allowed(p), p)
	}
}

func TestAnAICrawlerGroupBindsUs(t *testing.T) {
	r := parseRobots([]byte("User-agent: *\nAllow: /\n\nUser-agent: GPTBot\nDisallow: /\n"))
	require.False(t, r.allowed("/products.json"))
	r = parseRobots([]byte("User-agent: Googlebot\nDisallow: /\n\nUser-agent: *\nDisallow:\n"))
	require.True(t, r.allowed("/products.json"), "a group for another crawler, and an empty Disallow, bind nobody here")
	r = parseRobots([]byte("User-agent: ClaudeBot\nUser-agent: CCBot\nDisallow: /products\n"))
	require.False(t, r.allowed("/products.json"))
	require.True(t, r.allowed("/meta.json"))
}

func TestTheLongestRuleWinsAndAllowWinsATie(t *testing.T) {
	r := parseRobots([]byte("User-agent: *\nDisallow: /products\nAllow: /products/\nDisallow: /*.json$\n"))
	require.True(t, r.allowed("/products/ring"))
	require.False(t, r.allowed("/products"))
	require.False(t, r.allowed("/meta.json"))
	require.True(t, r.allowed("/meta.json?x=1"), "$ anchors the end")
	r = parseRobots([]byte("User-agent: *\nDisallow: /a\nAllow: /a\n"))
	require.True(t, r.allowed("/a"))
}

func TestCommentsAndCaseAreIgnored(t *testing.T) {
	r := parseRobots([]byte("# header\nUSER-AGENT: *   # everyone\nDISALLOW: /private # no\n"))
	require.False(t, r.allowed("/private/x"))
	require.True(t, r.allowed("/public"))
}

func TestABOMDoesNotHideTheFirstLine(t *testing.T) {
	r := parseRobots([]byte("\xEF\xBB\xBFUser-agent: *\nDisallow: /"))
	require.False(t, r.allowed("/x"))
}
