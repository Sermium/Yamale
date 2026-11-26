package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeMailer struct{ sent []string }

func (f *fakeMailer) Send(to, subject, body string) error {
	f.sent = append(f.sent, to+"|"+subject+"|"+body)
	return nil
}

const testLink = "https://pay.yamaletech.com/app/#receive=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func notify(t *testing.T, s *server, label string, n transferNotice) int {
	t.Helper()
	body, _ := json.Marshal(n)
	return serve(s, sign(t, request("POST", "/v1/transfers/notify", body), testKey(label), "transfer-notify", sha256Hex(body), testNow.Unix())).Code
}

func notifying(t *testing.T) (*server, *fakeMailer) {
	s := testServer(t)
	m := &fakeMailer{}
	s.mailer = m
	sponsoredLedger(t, s, "sender")
	return s, m
}

func notice() transferNotice {
	return transferNotice{To: "kofi@example.test", SenderName: "Awa Diop", Amount: "50 USDC", Link: testLink, Lang: "fr"}
}

func TestAMemberNotifiesARecipient(t *testing.T) {
	s, m := notifying(t)
	require.Equal(t, http.StatusOK, notify(t, s, "sender", notice()))
	require.Len(t, m.sent, 1)
	require.True(t, strings.HasPrefix(m.sent[0], "kofi@example.test|Awa Diop vous a envoyé 50 USDC|"))
	require.Contains(t, m.sent[0], testLink)
	require.NotContains(t, m.sent[0], "code")
}

func TestTheCodeIsMentionedWhenThereIsOne(t *testing.T) {
	s, m := notifying(t)
	n := notice()
	n.Link += "&code=1"
	n.WithCode = true
	require.Equal(t, http.StatusOK, notify(t, s, "sender", n))
	require.Contains(t, m.sent[0], "le code que Awa Diop vous communiquera")
}

func TestOnlyAClaimLinkIsSent(t *testing.T) {
	s, m := notifying(t)
	for _, link := range []string{
		"https://evil.example/app/#receive=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"https://pay.yamaletech.com/app/#receive=short",
		testLink + "\nhttps://evil.example",
		"http://pay.yamaletech.com/app/#receive=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		n := notice()
		n.Link = link
		require.Equal(t, http.StatusBadRequest, notify(t, s, "sender", n), link)
	}
	bad := notice()
	bad.To = "a@b.c, d@e.f"
	require.Equal(t, http.StatusBadRequest, notify(t, s, "sender", bad))
	bad = notice()
	bad.SenderName = "Awa\r\nBcc: x@y.z"
	require.Equal(t, http.StatusBadRequest, notify(t, s, "sender", bad))
	require.Empty(t, m.sent)
}

func TestAKeyOutsideTheAppsSendsNothing(t *testing.T) {
	s, m := notifying(t)
	require.Equal(t, http.StatusForbidden, notify(t, s, "stranger", notice()))
	require.Empty(t, m.sent)
}

func TestASenderHasADailyAllowance(t *testing.T) {
	s, m := notifying(t)
	for i := 0; i < notifyPerSenderDaily; i++ {
		require.Equal(t, http.StatusOK, notify(t, s, "sender", notice()))
	}
	require.Equal(t, http.StatusTooManyRequests, notify(t, s, "sender", notice()))
	require.Len(t, m.sent, notifyPerSenderDaily)
}

func TestNoMailerNoEmail(t *testing.T) {
	s := testServer(t)
	sponsoredLedger(t, s, "sender")
	require.Equal(t, http.StatusServiceUnavailable, notify(t, s, "sender", notice()))
}
