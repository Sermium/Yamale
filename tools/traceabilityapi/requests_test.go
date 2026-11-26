package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeEmails map[string]string

func (f fakeEmails) AddressOf(_ context.Context, email string) (string, error) {
	if a, ok := f[email]; ok {
		return a, nil
	}
	return "", errNoSuchAccount
}

type fakeAliases map[string]string

func (f fakeAliases) AliasAddress(_ context.Context, id string) (string, error) {
	if a, ok := f[id]; ok {
		return a, nil
	}
	return "", errNoSuchAccount
}

// fakeTxs: hash -> "from to denom amount" the transfer paid.
type fakeTxs map[string][4]string

func (f fakeTxs) Paid(_ context.Context, hash, from, to, denom, amount string) (bool, error) {
	p, ok := f[hash]
	return ok && p[0] == from && p[1] == to && p[2] == denom && p[3] == amount, nil
}

const payHash = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func requesting(t *testing.T) (*server, *fakeMailer) {
	s := testServer(t)
	m := &fakeMailer{}
	s.mailer = m
	sponsoredLedger(t, s, "awa")
	s.emails = fakeEmails{"kofi@example.test": addrOf(testKey("kofi"))}
	s.aliases = fakeAliases{"ZA9Q03CA3QQ": addrOf(testKey("kofi"))}
	s.txs = fakeTxs{payHash: {addrOf(testKey("kofi")), addrOf(testKey("awa")), "uusdc", "50000000"}}
	return s, m
}

func createRequest(t *testing.T, s *server, label string, in requestInput) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(in)
	w := serve(s, sign(t, request("POST", "/v1/requests", body), testKey(label), "request-create", sha256Hex(body), testNow.Unix()))
	var out map[string]any
	_ = json.NewDecoder(w.Body).Decode(&out)
	return w.Code, out
}

func updateRequest(t *testing.T, s *server, label, id, action, tx string) int {
	t.Helper()
	var body []byte
	if action == "paid" {
		body, _ = json.Marshal(map[string]string{"tx": tx})
	}
	return serve(s, sign(t, request("POST", "/v1/requests/"+id+"/"+action, body), testKey(label), "request-update", id+":"+action+":"+tx, testNow.Unix())).Code
}

func mine(t *testing.T, s *server, label string) (in, out []moneyRequest) {
	t.Helper()
	k := testKey(label)
	w := serve(s, sign(t, request("GET", "/v1/requests", nil), k, "requests-read", addrOf(k), testNow.Unix()))
	require.Equal(t, http.StatusOK, w.Code)
	var body struct{ Incoming, Outgoing []moneyRequest }
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	return body.Incoming, body.Outgoing
}

func ask() requestInput {
	return requestInput{To: "kofi@example.test", Amount: "50000000", Denom: "uusdc", Note: "Dîner", Lang: "fr", AmountText: "50 USDC"}
}

func TestAskingAnExistingAccountByEmailNotifiesIt(t *testing.T) {
	s, m := requesting(t)
	code, out := createRequest(t, s, "awa", ask())
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, addrOf(testKey("kofi")), out["to"])
	require.Equal(t, true, out["notified"])
	require.Contains(t, m.sent[0], "kofi@example.test|")
	require.Contains(t, m.sent[0], "50 USDC")

	in, _ := mine(t, s, "kofi")
	require.Len(t, in, 1)
	require.Equal(t, "open", in[0].Status)
	_, outgoing := mine(t, s, "awa")
	require.Len(t, outgoing, 1)
}

func TestAskingByIdentifierWorksWithDashes(t *testing.T) {
	s, _ := requesting(t)
	in := ask()
	in.To = "za-9q03-ca3q-q"
	code, _ := createRequest(t, s, "awa", in)
	require.Equal(t, http.StatusOK, code)
}

func TestNobodyWhoDoesNotExistCanBeAsked(t *testing.T) {
	s, m := requesting(t)
	in := ask()
	in.To = "nobody@example.test"
	code, _ := createRequest(t, s, "awa", in)
	require.Equal(t, http.StatusNotFound, code)
	in.To = "ZA-XXXX-XXXX-X"
	code, _ = createRequest(t, s, "awa", in)
	require.Equal(t, http.StatusNotFound, code)
	require.Empty(t, m.sent)
}

func TestOnlyMembersAskAndNeverThemselves(t *testing.T) {
	s, _ := requesting(t)
	code, _ := createRequest(t, s, "stranger", ask())
	require.Equal(t, http.StatusForbidden, code)
	s.emails = fakeEmails{"me@example.test": addrOf(testKey("awa"))}
	in := ask()
	in.To = "me@example.test"
	code, _ = createRequest(t, s, "awa", in)
	require.Equal(t, http.StatusBadRequest, code)
}

func TestARequestIsPaidOnlyByTheTransferThatPaysIt(t *testing.T) {
	s, _ := requesting(t)
	_, out := createRequest(t, s, "awa", ask())
	id := out["id"].(string)
	// Not by the person who asked, and not by a transfer that does not pay it.
	require.Equal(t, http.StatusForbidden, updateRequest(t, s, "awa", id, "paid", payHash))
	other := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	require.Equal(t, http.StatusConflict, updateRequest(t, s, "kofi", id, "paid", other))
	require.Equal(t, http.StatusOK, updateRequest(t, s, "kofi", id, "paid", payHash))
	in, _ := mine(t, s, "kofi")
	require.Equal(t, "paid", in[0].Status)
	// A second request cannot be paid with the same transfer.
	_, out2 := createRequest(t, s, "awa", ask())
	require.Equal(t, http.StatusConflict, updateRequest(t, s, "kofi", out2["id"].(string), "paid", payHash))
}

func TestDecliningAndCancellingBelongToEachSide(t *testing.T) {
	s, _ := requesting(t)
	_, out := createRequest(t, s, "awa", ask())
	id := out["id"].(string)
	require.Equal(t, http.StatusForbidden, updateRequest(t, s, "awa", id, "decline", ""))
	require.Equal(t, http.StatusForbidden, updateRequest(t, s, "kofi", id, "cancel", ""))
	require.Equal(t, http.StatusOK, updateRequest(t, s, "kofi", id, "decline", ""))
	require.Equal(t, http.StatusConflict, updateRequest(t, s, "awa", id, "cancel", ""))
}

func TestADailyAllowanceOfRequests(t *testing.T) {
	s, _ := requesting(t)
	for i := 0; i < requestsPerDay; i++ {
		code, _ := createRequest(t, s, "awa", ask())
		require.Equal(t, http.StatusOK, code)
	}
	code, _ := createRequest(t, s, "awa", ask())
	require.Equal(t, http.StatusTooManyRequests, code)
}
