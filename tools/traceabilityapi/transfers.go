package main

// Telling somebody that money was sent to their email address.
//
// pay.yamaletech moves the money itself: into a one-time account whose key is
// derived from a secret in a link, and, when the sender chose one, a code the
// sender passes on another way (clients/app/src/emailTransfer.ts). This
// endpoint only emails the link. It holds nothing and keeps nothing but a
// count, and it can be asked to send exactly one kind of message, so the
// platform's mailbox cannot be borrowed to send anything else:
//
//   - the link must be a claim link on pay.yamaletech.com, and nothing else;
//   - the sender is an account of the Yamale apps, signing the request;
//   - a sender may notify a handful of addresses a day, the platform a few
//     hundred.
//
// The link passes through this server on its way to the mailbox. Without a
// code it is the money; that is why the app offers the code, with which the
// link alone opens nothing.

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	notifyPerSenderDaily = 10
	notifyAllDaily       = 300
)

var claimLinkPattern = regexp.MustCompile(`^https://pay\.yamaletech\.com/app/#receive=[A-Za-z0-9_-]{43}(&code=1)?$`)

type transferNotice struct {
	To         string `json:"to"`
	SenderName string `json:"sender_name"`
	Amount     string `json:"amount"`
	Link       string `json:"link"`
	WithCode   bool   `json:"with_code"`
	Lang       string `json:"lang"`
}

func (n *transferNotice) check() error {
	addr, err := mail.ParseAddress(n.To)
	if err != nil || addr.Address != strings.TrimSpace(n.To) || strings.ContainsAny(n.To, "\r\n<>") {
		return fmt.Errorf("to is one email address")
	}
	if !claimLinkPattern.MatchString(n.Link) {
		return fmt.Errorf("link is a pay.yamaletech.com claim link")
	}
	if n.WithCode != strings.HasSuffix(n.Link, "&code=1") {
		return fmt.Errorf("with_code and the link disagree")
	}
	for name, v := range map[string]string{"sender_name": n.SenderName, "amount": n.Amount} {
		if v == "" || utf8.RuneCountInString(v) > 80 || strings.ContainsAny(v, "\r\n<>") {
			return fmt.Errorf("%s is one short line", name)
		}
	}
	return nil
}

type noticeRecord struct {
	Sender string `json:"sender"`
	At     int64  `json:"at"`
}

func (s *server) noticesPath() string { return filepath.Join(s.dir, "transfer-notices.json") }

// POST /v1/transfers/notify, signed by the sender over the sha256 of the body.
func (s *server) handleTransferNotify(w http.ResponseWriter, r *http.Request) {
	if s.mailer == nil {
		refuse(w, http.StatusServiceUnavailable, "emails are not sent from here")
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<10))
	if err != nil {
		readFailed(w, err, "a notice is at most 2 KiB")
		return
	}
	sender, err := s.signer(r, "transfer-notify", func(string) string { return sha256Hex(body) })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	var n transferNotice
	if err := strictDecode(body, &n); err != nil {
		refuse(w, http.StatusBadRequest, `a notice is {"to", "sender_name", "amount", "link", "with_code", "lang"}`)
		return
	}
	if err := n.check(); err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	member, err := s.isMember(r, sender)
	if err != nil {
		chainUnreachable(w, err)
		return
	}
	if !member {
		refuse(w, http.StatusForbidden, "only an account of the Yamale apps sends money by email")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var sent []noticeRecord
	if raw, err := os.ReadFile(s.noticesPath()); err == nil {
		_ = json.Unmarshal(raw, &sent)
	}
	since := s.now().Add(-24 * time.Hour).Unix()
	mine, all := 0, 0
	kept := sent[:0]
	for _, rec := range sent {
		if rec.At < since {
			continue
		}
		kept = append(kept, rec)
		all++
		if rec.Sender == sender {
			mine++
		}
	}
	if mine >= notifyPerSenderDaily || all >= notifyAllDaily {
		w.Header().Set("Retry-After", "3600")
		refuse(w, http.StatusTooManyRequests, "today's emails are used up; the link can still be shared another way")
		return
	}
	subject, text := transferEmail(n)
	if err := s.mailer.Send(strings.TrimSpace(n.To), subject, text); err != nil {
		log.Printf("traceabilityapi: transfer notice from %s not sent: %v", sender, err)
		refuse(w, http.StatusBadGateway, "the email could not be sent; share the link another way")
		return
	}
	kept = append(kept, noticeRecord{Sender: sender, At: s.now().Unix()})
	out, _ := json.Marshal(kept)
	if err := writeAtomically(s.noticesPath(), out); err != nil {
		log.Printf("traceabilityapi: transfer notice count not saved: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

// transferEmail is the message, in the recipient's language when the sender's
// app gave one. Plain text: a link in it is a link the reader can read whole.
func transferEmail(n transferNotice) (string, string) {
	code := map[string]string{
		"fr": "\nPour l’ouvrir, il vous faudra aussi le code que %s vous communiquera autrement (téléphone, message).\n",
		"pt": "\nPara o abrir, precisa também do código que %s lhe vai dar de outra forma (telefone, mensagem).\n",
		"sw": "\nIli kuzifungua, utahitaji pia msimbo ambao %s atakupa kwa njia nyingine (simu, ujumbe).\n",
		"ar": "\nلفتحه، ستحتاج أيضًا إلى الرمز الذي سيعطيك إياه %s بطريقة أخرى (هاتف، رسالة).\n",
		"en": "\nTo open it you will also need the code %s gives you another way (a call, a message).\n",
	}
	texts := map[string][2]string{
		"fr": {"%s vous a envoyé %s",
			"Bonjour,\n\n%s vous a envoyé %s avec Yamale Pay.\n\nPour le recevoir, ouvrez ce lien, puis créez votre compte Yamale (ou connectez-vous) : l’argent arrive sur votre compte.\n\n%s\n%s\nCe lien vaut de l’argent : ne le transférez à personne. Si vous ne connaissez pas %s, ignorez ce message ; l’argent lui reviendra.\n\nYamale\n"},
		"pt": {"%s enviou-lhe %s",
			"Olá,\n\n%s enviou-lhe %s com o Yamale Pay.\n\nPara o receber, abra este link e crie a sua conta Yamale (ou entre): o dinheiro chega à sua conta.\n\n%s\n%s\nEste link vale dinheiro: não o reencaminhe a ninguém. Se não conhece %s, ignore esta mensagem; o dinheiro volta para essa pessoa.\n\nYamale\n"},
		"sw": {"%s amekutumia %s",
			"Habari,\n\n%s amekutumia %s kupitia Yamale Pay.\n\nIli kupokea, fungua kiungo hiki, kisha ufungue akaunti yako ya Yamale (au uingie): pesa zitafika kwenye akaunti yako.\n\n%s\n%s\nKiungo hiki kina thamani ya pesa: usikitume kwa mtu yeyote. Ikiwa humjui %s, puuza ujumbe huu; pesa zitamrudia.\n\nYamale\n"},
		"ar": {"أرسل إليك %s مبلغ %s",
			"مرحبًا،\n\nأرسل إليك %s مبلغ %s عبر Yamale Pay.\n\nلاستلامه، افتح هذا الرابط ثم أنشئ حسابك في Yamale (أو سجّل الدخول): تصل الأموال إلى حسابك.\n\n%s\n%s\nهذا الرابط يساوي مالًا: لا ترسله إلى أحد. إذا كنت لا تعرف %s، فتجاهل هذه الرسالة؛ ستعود إليه الأموال.\n\nYamale\n"},
		"en": {"%s sent you %s",
			"Hello,\n\n%s sent you %s with Yamale Pay.\n\nTo receive it, open this link, then create your Yamale account (or sign in): the money arrives in your account.\n\n%s\n%s\nThis link is worth money: do not forward it to anybody. If you do not know %s, ignore this message; the money goes back to them.\n\nYamale\n"},
	}
	lang := n.Lang
	if _, ok := texts[lang]; !ok {
		lang = "en"
	}
	t := texts[lang]
	codeLine := ""
	if n.WithCode {
		codeLine = fmt.Sprintf(code[lang], n.SenderName)
	}
	return fmt.Sprintf(t[0], n.SenderName, n.Amount),
		fmt.Sprintf(t[1], n.SenderName, n.Amount, n.Link, codeLine, n.SenderName)
}
