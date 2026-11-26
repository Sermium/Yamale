package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeSender struct {
	sent []string
	err  error
}

func (f *fakeSender) Send(_ context.Context, to, coins string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.sent = append(f.sent, to+" "+coins)
	return "WELCOME" + to[len(to)-4:], nil
}

func TestANewAccountIsWelcomedOnce(t *testing.T) {
	s, _ := sponsoring(t, 10)
	f := &fakeSender{}
	s.welcome, s.sender = "10000000000uusdc", f

	var body map[string]any
	require.NoError(t, json.NewDecoder(askSponsor(t, s, "owner").Body).Decode(&body))
	owner := addrOf(testKey("owner"))
	require.Equal(t, "WELCOME"+owner[len(owner)-4:], body["welcome"])

	// Asking again is "already sponsored", and pays nothing more.
	require.Equal(t, http.StatusOK, askSponsor(t, s, "owner").StatusCode)
	require.Equal(t, []string{owner + " 10000000000uusdc"}, f.sent)

	ledger, err := s.readLedger()
	require.NoError(t, err)
	require.Equal(t, "WELCOME"+owner[len(owner)-4:], ledger[0].Welcome)
}

func TestAFailedWelcomeStillSponsors(t *testing.T) {
	s, g := sponsoring(t, 10)
	s.welcome, s.sender = "10000000000uusdc", &fakeSender{err: errors.New("insufficient funds")}

	resp := askSponsor(t, s, "owner")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, true, body["granted"])
	require.NotContains(t, body, "welcome")
	require.Len(t, g.calls, 1)
}

func TestNoWelcomeWithoutTheSetting(t *testing.T) {
	s, _ := sponsoring(t, 10)
	f := &fakeSender{}
	s.sender = f
	require.Equal(t, http.StatusOK, askSponsor(t, s, "owner").StatusCode)
	require.Empty(t, f.sent)
}

func TestTheWelcomeIsForTestChainsOnly(t *testing.T) {
	require.True(t, welcomeAllowed("yamale-devnet-2"))
	require.True(t, welcomeAllowed("yamale-local-1"))
	require.False(t, welcomeAllowed("yamale-1"))
	require.False(t, welcomeAllowed("yamale-mainnet"))
}
