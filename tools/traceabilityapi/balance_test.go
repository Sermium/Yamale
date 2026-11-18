package main

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeBalance struct {
	amount *big.Int
	err    error
}

func (f fakeBalance) Balance(context.Context, string, string) (*big.Int, error) {
	return f.amount, f.err
}

type fixedGranter struct{}

func (fixedGranter) Address() string                               { return "yml1sponsor" }
func (fixedGranter) Grant(context.Context, string) (string, error) { return "", nil }

func sponsorHealth(t *testing.T, s *server) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/health/sponsor", nil))
	return w
}

func TestSponsorHealthWhenSponsoringIsOff(t *testing.T) {
	s := testServer(t)
	w := sponsorHealth(t, s)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"sponsoring":false}`, w.Body.String())
}

func TestSponsorHealthAnswers503BelowTheFloor(t *testing.T) {
	s := testServer(t)
	s.granter = fixedGranter{}
	s.sponsorFloor, s.sponsorDenom = big.NewInt(10_000_000), "uyml"
	s.balances = fakeBalance{amount: big.NewInt(9_999_999)}
	require.Equal(t, http.StatusServiceUnavailable, sponsorHealth(t, s).Code)
	s.balances = fakeBalance{amount: big.NewInt(10_000_000)}
	require.Equal(t, http.StatusOK, sponsorHealth(t, s).Code)
}

func TestSponsorHealthHidesTheChainError(t *testing.T) {
	s := testServer(t)
	s.granter = fixedGranter{}
	s.sponsorFloor, s.sponsorDenom = big.NewInt(1), "uyml"
	s.balances = fakeBalance{err: errors.New(`Get "http://10.0.0.7:1317/cosmos/bank": down`)}
	w := sponsorHealth(t, s)
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.NotContains(t, w.Body.String(), "10.0.0.7")
}

func TestSplitCoin(t *testing.T) {
	amount, denom, err := splitCoin("10000000uyml")
	require.NoError(t, err)
	require.Equal(t, "10000000", amount.String())
	require.Equal(t, "uyml", denom)

	for _, bad := range []string{"", "uyml", "10", "-5uyml", "5 uyml", "5U", "5x"} {
		_, _, err := splitCoin(bad)
		require.Error(t, err, "%q must be refused", bad)
	}
}
