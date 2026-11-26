package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeOnboarder struct {
	customers []string
	recorded  []string
	recordErr error
}

func (f *fakeOnboarder) RegisterCustomer(_ context.Context, customer string) (string, error) {
	f.customers = append(f.customers, customer)
	return "CUST", nil
}

func (f *fakeOnboarder) SetJurisdiction(_ context.Context, account, country string) (string, error) {
	if f.recordErr != nil {
		return "", f.recordErr
	}
	f.recorded = append(f.recorded, account+" "+country)
	return "JUR", nil
}

func onboarding(t *testing.T) (*server, *fakeOnboarder) {
	s, _ := sponsoring(t, 10)
	o := &fakeOnboarder{}
	s.onboarder = o
	return s, o
}

func askOnboard(t *testing.T, s *server, label, country, signedCountry string) *http.Response {
	k := testKey(label)
	body, _ := json.Marshal(map[string]string{"country": country})
	return serve(s, sign(t, request("POST", "/v1/onboard", body), k, "onboard", addrOf(k)+":"+signedCountry, testNow.Unix())).Result()
}

func TestAnAccountIsOnboardedOnceInTheCountryItChose(t *testing.T) {
	s, o := onboarding(t)
	owner := addrOf(testKey("owner"))
	require.Equal(t, http.StatusOK, askOnboard(t, s, "owner", "sn", "SN").StatusCode)
	require.Equal(t, []string{owner}, o.customers)
	require.Equal(t, []string{owner + " SN"}, o.recorded)

	var body map[string]any
	require.NoError(t, json.NewDecoder(askOnboard(t, s, "owner", "SN", "SN").Body).Decode(&body))
	require.Equal(t, true, body["already"])
	require.Len(t, o.customers, 1)
}

func TestTheCountryIsPartOfTheSignature(t *testing.T) {
	s, o := onboarding(t)
	// Signed for Senegal, sent as Kenya.
	require.Equal(t, http.StatusUnauthorized, askOnboard(t, s, "owner", "KE", "SN").StatusCode)
	require.Empty(t, o.customers)
}

func TestACountryIsTwoLetters(t *testing.T) {
	s, o := onboarding(t)
	require.Equal(t, http.StatusBadRequest, askOnboard(t, s, "owner", "SEN", "SEN").StatusCode)
	require.Equal(t, http.StatusBadRequest, askOnboard(t, s, "owner", "", "").StatusCode)
	require.Empty(t, o.customers)
}

func TestAnAlreadyRecordedCountryIsNotAFailure(t *testing.T) {
	s, o := onboarding(t)
	o.recordErr = errors.New("jurisdiction already set: yml1… is recorded in SN")
	require.Equal(t, http.StatusOK, askOnboard(t, s, "owner", "SN", "SN").StatusCode)
}

func TestAFailedRecordingIsReportedAndNotLedgered(t *testing.T) {
	s, o := onboarding(t)
	o.recordErr = errors.New("not an approved participant")
	require.Equal(t, http.StatusBadGateway, askOnboard(t, s, "owner", "SN", "SN").StatusCode)
	ledger, err := s.readOnboarded()
	require.NoError(t, err)
	require.Empty(t, ledger)
}

func TestOnboardingIsOffWithoutTheSetting(t *testing.T) {
	s, _ := sponsoring(t, 10)
	require.Equal(t, http.StatusServiceUnavailable, askOnboard(t, s, "owner", "SN", "SN").StatusCode)
}
