package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeGranter struct {
	calls []string
	err   error
}

func (f *fakeGranter) Grant(_ context.Context, grantee string) (string, error) {
	f.calls = append(f.calls, grantee)
	if f.err != nil {
		return "", f.err
	}
	return "TX" + grantee[len(grantee)-4:], nil
}

func (f *fakeGranter) Address() string { return "yml1sponsor" }

func sponsoring(t *testing.T, daily int) (*server, *fakeGranter) {
	s := testServer(t)
	g := &fakeGranter{}
	s.granter, s.sponsorDaily = g, daily
	return s, g
}

func askSponsor(t *testing.T, s *server, label string) *http.Response {
	k := testKey(label)
	return serve(s, sign(t, request("POST", "/v1/sponsor", nil), k, "sponsor", addrOf(k), testNow.Unix())).Result()
}

func TestAnAccountIsSponsoredOnceAndOnlyItself(t *testing.T) {
	s, g := sponsoring(t, 10)
	first := askSponsor(t, s, "owner")
	require.Equal(t, http.StatusOK, first.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(first.Body).Decode(&body))
	require.Equal(t, true, body["granted"])
	require.Equal(t, "yml1sponsor", body["granter"])

	again := askSponsor(t, s, "owner")
	require.NoError(t, json.NewDecoder(again.Body).Decode(&body))
	require.Equal(t, true, body["already"])
	require.Equal(t, []string{addrOf(testKey("owner"))}, g.calls)
}

func TestSponsorshipNeedsTheAccountsOwnSignature(t *testing.T) {
	s, g := sponsoring(t, 10)
	k := testKey("owner")
	// Signed over somebody else's address.
	w := serve(s, sign(t, request("POST", "/v1/sponsor", nil), k, "sponsor", addrOf(testKey("other")), testNow.Unix()))
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Empty(t, g.calls)
}

func TestTheDailyCeilingCountsOnlyTheLastDay(t *testing.T) {
	s, _ := sponsoring(t, 2)
	old, _ := json.Marshal([]sponsorRecord{{Address: "yml1old", At: testNow.Add(-25 * time.Hour).Unix()}})
	require.NoError(t, os.WriteFile(filepath.Join(s.dir, "sponsored.json"), old, 0o600))
	require.Equal(t, http.StatusOK, askSponsor(t, s, "a").StatusCode)
	require.Equal(t, http.StatusOK, askSponsor(t, s, "b").StatusCode)
	require.Equal(t, http.StatusTooManyRequests, askSponsor(t, s, "c").StatusCode)
}

func TestTheLedgerSurvivesARestart(t *testing.T) {
	s, _ := sponsoring(t, 10)
	require.Equal(t, http.StatusOK, askSponsor(t, s, "owner").StatusCode)
	restarted := &server{dir: s.dir, chainID: testChain, chain: s.chain, now: s.now, granter: &fakeGranter{}, sponsorDaily: 10}
	var body map[string]any
	require.NoError(t, json.NewDecoder(askSponsor(t, restarted, "owner").Body).Decode(&body))
	require.Equal(t, true, body["already"])
}

func TestAnExistingAllowanceCountsAsDone(t *testing.T) {
	s, g := sponsoring(t, 10)
	g.err = errAlreadyGranted
	require.Equal(t, http.StatusOK, askSponsor(t, s, "owner").StatusCode)
	g.err = errors.New("node down")
	require.Equal(t, http.StatusBadGateway, askSponsor(t, s, "another").StatusCode)
}

func TestSponsoringOffAnswers503(t *testing.T) {
	s := testServer(t)
	require.Equal(t, http.StatusServiceUnavailable, askSponsor(t, s, "owner").StatusCode)
	require.Equal(t, http.StatusServiceUnavailable, serve(s, request("GET", "/v1/sponsor", nil)).Code)
}

func TestTheGrantIsLimitedToOwnerMessages(t *testing.T) {
	g := cliGranter{
		bin: "blockchaind", from: "loyalty-sponsor", home: "/h", keyring: "test", node: "tcp://n:1",
		chainID: "c-1", spendLimit: "2000000uyml", fees: "0uyml", lifetime: time.Hour,
		now: func() time.Time { return testNow },
	}
	require.Equal(t, []string{
		"tx", "feegrant", "grant", "loyalty-sponsor", "yml1owner",
		"--spend-limit", "2000000uyml",
		"--expiration", testNow.Add(time.Hour).UTC().Format(time.RFC3339),
		"--allowed-messages", strings.Join(ownerMessages, ","),
		"--chain-id", "c-1", "--keyring-backend", "test", "--home", "/h", "--node", "tcp://n:1",
		"--fees", "0uyml", "--gas", "250000", "-y", "--output", "json",
	}, g.grantArgs("yml1owner"))
	require.Len(t, ownerMessages, 8)
}

func TestTheCLIGranterWaitsForTheBlock(t *testing.T) {
	script := func(outputs ...string) func(context.Context, string, ...string) ([]byte, error) {
		i := 0
		return func(context.Context, string, ...string) ([]byte, error) {
			out := outputs[i]
			i++
			if strings.HasPrefix(out, "ERR:") {
				return []byte(out[4:]), errors.New("exit 1")
			}
			return []byte(out), nil
		}
	}
	base := cliGranter{now: func() time.Time { return testNow }}

	ok := base
	ok.run = script(`{"code":0,"txhash":"ABC"}`, `ERR:tx (ABC) not found`, `{"code":0}`)
	hash, err := ok.Grant(context.Background(), "yml1owner")
	require.NoError(t, err)
	require.Equal(t, "ABC", hash)

	failed := base
	failed.run = script(`{"code":0,"txhash":"ABC"}`, `{"code":5,"raw_log":"insufficient funds"}`)
	_, err = failed.Grant(context.Background(), "yml1owner")
	require.ErrorContains(t, err, "insufficient funds")

	exists := base
	exists.run = script(`{"code":18,"txhash":"","raw_log":"fee allowance already exists"}`)
	_, err = exists.Grant(context.Background(), "yml1owner")
	require.ErrorIs(t, err, errAlreadyGranted)
}
