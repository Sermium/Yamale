package main

// Asking another account of the apps for money (since 2026-11-26).
//
// A request names a person who already has an account, by their email or their
// identifier (ZA-K3M9-7QRT-B); one that does not exist is refused, never
// created. The person sees it in their app and pays it, or declines it. Paying
// is an ordinary transfer, and a request is marked paid only when the chain
// shows that transfer: from the person asked, to the person asking, the amount
// asked or more.
//
//	POST /v1/requests               create   (signed by the requester)
//	GET  /v1/requests               mine, both ways (signed by the account)
//	POST /v1/requests/{id}/paid     with the transfer's hash (signed by the payer)
//	POST /v1/requests/{id}/decline  (signed by the person asked)
//	POST /v1/requests/{id}/cancel   (signed by the requester)
//
// An email is resolved by the custodian (lookup.go there), the only service
// that knows which emails have an account; it is used to notify the person and
// is not kept. An identifier is resolved on the chain.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	requestsPerDay   = 20
	requestNoteLimit = 140
)

var (
	errNoSuchAccount   = errors.New("no account")
	identifierPattern  = regexp.MustCompile(`^[A-Z]{2}[0-9A-Z]{8,12}$`)
	amountPattern      = regexp.MustCompile(`^[1-9][0-9]{0,29}$`)
	requestIDPattern   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	txHashPattern      = regexp.MustCompile(`^[0-9A-F]{64}$`)
)

// EmailLookup is which account an email opens, asked of the custodian.
type EmailLookup interface {
	AddressOf(ctx context.Context, email string) (string, error)
}

// AliasReader resolves an identifier on the chain.
type AliasReader interface {
	AliasAddress(ctx context.Context, id string) (string, error)
}

// TxChecker says whether a committed transaction paid at least amount of denom
// from one account to another.
type TxChecker interface {
	Paid(ctx context.Context, hash, from, to, denom, amount string) (bool, error)
}

type moneyRequest struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	To       string `json:"to"`
	Amount   string `json:"amount"`
	Denom    string `json:"denom"`
	Note     string `json:"note"`
	Status   string `json:"status"` // open, paid, declined, cancelled
	Tx       string `json:"tx,omitempty"`
	Created  int64  `json:"created"`
	Updated  int64  `json:"updated"`
	FromName string `json:"from_name,omitempty"`
	ToName   string `json:"to_name,omitempty"`
}

func (s *server) requestsPath() string { return filepath.Join(s.dir, "requests.json") }

func (s *server) loadRequests() ([]moneyRequest, error) {
	raw, err := os.ReadFile(s.requestsPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []moneyRequest
	return out, json.Unmarshal(raw, &out)
}

func (s *server) storeRequests(list []moneyRequest) error {
	out, _ := json.Marshal(list)
	return writeAtomically(s.requestsPath(), out)
}

// nameOf is the name an account gave in its profile, if any.
func (s *server) nameOf(address string) string {
	p, found, err := s.loadPerson(address)
	if err != nil || !found {
		return ""
	}
	return strings.TrimSpace(p.FirstName + " " + p.LastName)
}

type requestInput struct {
	To     string `json:"to"`
	Amount string `json:"amount"`
	Denom  string `json:"denom"`
	Note   string `json:"note"`
	Lang   string `json:"lang"`
	// AmountText is how the requester's app writes the amount, for the email.
	AmountText string `json:"amount_text"`
}

// POST /v1/requests, signed by the requester over the sha256 of the body.
func (s *server) handleRequestCreate(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<10))
	if err != nil {
		readFailed(w, err, "a request is at most 2 KiB")
		return
	}
	from, err := s.signer(r, "request-create", func(string) string { return sha256Hex(body) })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	var in requestInput
	if err := strictDecode(body, &in); err != nil {
		refuse(w, http.StatusBadRequest, `a request is {"to", "amount", "denom", "note", "lang", "amount_text"}`)
		return
	}
	in.To, in.Note = strings.TrimSpace(in.To), strings.TrimSpace(in.Note)
	if !amountPattern.MatchString(in.Amount) || !denomPattern.MatchString(in.Denom) {
		refuse(w, http.StatusBadRequest, "the amount is a positive whole number of base units, in a currency")
		return
	}
	if utf8.RuneCountInString(in.Note) > requestNoteLimit || strings.ContainsAny(in.Note, "\r\n<>") {
		refuse(w, http.StatusBadRequest, "the note is one short line")
		return
	}
	if utf8.RuneCountInString(in.AmountText) > 40 || strings.ContainsAny(in.AmountText, "\r\n<>") {
		refuse(w, http.StatusBadRequest, "amount_text is one short line")
		return
	}
	member, err := s.isMember(r, from)
	if err != nil {
		chainUnreachable(w, err)
		return
	}
	if !member {
		refuse(w, http.StatusForbidden, "only an account of the Yamale apps asks for money")
		return
	}

	// Who is asked: an existing account, by email or by identifier.
	var to, email string
	switch {
	case strings.Contains(in.To, "@"):
		if s.emails == nil {
			refuse(w, http.StatusServiceUnavailable, "asking by email is not available here; use the identifier")
			return
		}
		email = in.To
		to, err = s.emails.AddressOf(r.Context(), email)
	default:
		id := strings.ToUpper(strings.ReplaceAll(in.To, "-", ""))
		if !identifierPattern.MatchString(id) || s.aliases == nil {
			refuse(w, http.StatusBadRequest, "ask by email or by identifier (ZA-K3M9-7QRT-B)")
			return
		}
		to, err = s.aliases.AliasAddress(r.Context(), id)
	}
	if errors.Is(err, errNoSuchAccount) {
		refuse(w, http.StatusNotFound, "nobody has a Yamale account with that; a request goes only to an existing account")
		return
	}
	if err != nil {
		chainUnreachable(w, err)
		return
	}
	if to == from {
		refuse(w, http.StatusBadRequest, "a request is to somebody else")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadRequests()
	if err != nil {
		serverFault(w, err)
		return
	}
	since := s.now().Add(-24 * time.Hour).Unix()
	today := 0
	for _, q := range list {
		if q.From == from && q.Created >= since {
			today++
		}
	}
	if today >= requestsPerDay {
		w.Header().Set("Retry-After", "3600")
		refuse(w, http.StatusTooManyRequests, "today's requests are used up")
		return
	}
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		serverFault(w, err)
		return
	}
	now := s.now().Unix()
	q := moneyRequest{ID: hex.EncodeToString(idBytes), From: from, To: to, Amount: in.Amount, Denom: in.Denom,
		Note: in.Note, Status: "open", Created: now, Updated: now}
	list = append(list, q)
	if err := s.storeRequests(list); err != nil {
		serverFault(w, err)
		return
	}
	notified := false
	if email != "" && s.mailer != nil {
		subject, text := requestEmail(in.Lang, s.nameOf(from), firstNonEmpty(in.AmountText, in.Amount+" "+in.Denom), in.Note)
		if err := s.mailer.Send(email, subject, text); err != nil {
			log.Printf("traceabilityapi: request %s not emailed: %v", q.ID, err)
		} else {
			notified = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": q.ID, "to": to, "to_name": s.nameOf(to), "notified": notified})
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// GET /v1/requests, signed by the account over its own address.
func (s *server) handleRequestsMine(w http.ResponseWriter, r *http.Request) {
	me, err := s.signer(r, "requests-read", func(a string) string { return a })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	s.mu.Lock()
	list, err := s.loadRequests()
	s.mu.Unlock()
	if err != nil {
		serverFault(w, err)
		return
	}
	incoming, outgoing := []moneyRequest{}, []moneyRequest{}
	for i := len(list) - 1; i >= 0; i-- {
		q := list[i]
		switch me {
		case q.To:
			q.FromName = s.nameOf(q.From)
			incoming = append(incoming, q)
		case q.From:
			q.ToName = s.nameOf(q.To)
			outgoing = append(outgoing, q)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"incoming": incoming, "outgoing": outgoing})
}

// POST /v1/requests/{id}/{action}: paid, decline or cancel.
func (s *server) handleRequestUpdate(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	if !requestIDPattern.MatchString(id) || (action != "paid" && action != "decline" && action != "cancel") {
		refuse(w, http.StatusNotFound, "no such request")
		return
	}
	var in struct {
		Tx string `json:"tx"`
	}
	if action == "paid" {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512)).Decode(&in); err != nil || !txHashPattern.MatchString(strings.ToUpper(in.Tx)) {
			refuse(w, http.StatusBadRequest, `paid takes {"tx": "<the transfer's hash>"}`)
			return
		}
		in.Tx = strings.ToUpper(in.Tx)
	}
	who, err := s.signer(r, "request-update", func(string) string { return id + ":" + action + ":" + in.Tx })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadRequests()
	if err != nil {
		serverFault(w, err)
		return
	}
	i := -1
	for k := range list {
		if list[k].ID == id {
			i = k
		}
	}
	if i < 0 {
		refuse(w, http.StatusNotFound, "no such request")
		return
	}
	q := &list[i]
	if q.Status != "open" {
		refuse(w, http.StatusConflict, "this request is already "+q.Status)
		return
	}
	switch action {
	case "cancel":
		if who != q.From {
			refuse(w, http.StatusForbidden, "only the person asking cancels a request")
			return
		}
		q.Status = "cancelled"
	case "decline":
		if who != q.To {
			refuse(w, http.StatusForbidden, "only the person asked declines a request")
			return
		}
		q.Status = "declined"
	case "paid":
		if who != q.To {
			refuse(w, http.StatusForbidden, "only the person asked pays a request")
			return
		}
		if s.txs == nil {
			refuse(w, http.StatusServiceUnavailable, "payments cannot be checked here")
			return
		}
		for _, other := range list {
			if other.Tx == in.Tx {
				refuse(w, http.StatusConflict, "that transfer already paid a request")
				return
			}
		}
		ok, err := s.txs.Paid(r.Context(), in.Tx, q.To, q.From, q.Denom, q.Amount)
		if err != nil {
			chainUnreachable(w, err)
			return
		}
		if !ok {
			refuse(w, http.StatusConflict, "that transfer does not pay this request")
			return
		}
		q.Status, q.Tx = "paid", in.Tx
	}
	q.Updated = s.now().Unix()
	if err := s.storeRequests(list); err != nil {
		serverFault(w, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

/* ------------------------------------------------ the chain and the custodian */

func (c restChain) getJSON(ctx context.Context, path string, into any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return 0, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("the chain did not answer: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res.StatusCode, nil
	}
	return res.StatusCode, json.NewDecoder(res.Body).Decode(into)
}

func (c restChain) AliasAddress(ctx context.Context, id string) (string, error) {
	var body struct {
		Alias struct {
			Address string `json:"address"`
		} `json:"alias"`
	}
	status, err := c.getJSON(ctx, "/yamale/blockchain/alias/v1/alias/"+url.PathEscape(id), &body)
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound || (status == http.StatusOK && body.Alias.Address == "") {
		return "", errNoSuchAccount
	}
	if status != http.StatusOK {
		// The alias module answers an unknown identifier with an error status too.
		return "", errNoSuchAccount
	}
	return body.Alias.Address, nil
}

func (c restChain) Paid(ctx context.Context, hash, from, to, denom, amount string) (bool, error) {
	var body struct {
		TxResponse struct {
			Code int `json:"code"`
			Tx   struct {
				Body struct {
					Messages []struct {
						Type   string `json:"@type"`
						From   string `json:"from_address"`
						To     string `json:"to_address"`
						Amount []struct {
							Denom  string `json:"denom"`
							Amount string `json:"amount"`
						} `json:"amount"`
					} `json:"messages"`
				} `json:"body"`
			} `json:"tx"`
		} `json:"tx_response"`
	}
	status, err := c.getJSON(ctx, "/cosmos/tx/v1beta1/txs/"+url.PathEscape(hash), &body)
	if err != nil {
		return false, err
	}
	if status != http.StatusOK || body.TxResponse.Code != 0 {
		return false, nil
	}
	want, _ := new(big.Int).SetString(amount, 10)
	paid := new(big.Int)
	for _, m := range body.TxResponse.Tx.Body.Messages {
		if m.Type != "/cosmos.bank.v1beta1.MsgSend" || m.From != from || m.To != to {
			continue
		}
		for _, c := range m.Amount {
			if c.Denom != denom {
				continue
			}
			if n, ok := new(big.Int).SetString(c.Amount, 10); ok {
				paid.Add(paid, n)
			}
		}
	}
	return want != nil && paid.Cmp(want) >= 0, nil
}

type custodianLookup struct {
	url, token string
	client     *http.Client
}

func (c custodianLookup) AddressOf(ctx context.Context, email string) (string, error) {
	raw, _ := json.Marshal(map[string]string{"email": email})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/v1/internal/address", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Yamale-Lookup-Token", c.token)
	res, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("the custodian did not answer: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return "", errNoSuchAccount
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the custodian answered %d", res.StatusCode)
	}
	var body struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil || body.Address == "" {
		return "", fmt.Errorf("unreadable custodian answer")
	}
	return body.Address, nil
}

func requestEmail(lang, from, amount, note string) (string, string) {
	if from == "" {
		from = map[string]string{"fr": "Quelqu’un", "pt": "Alguém", "sw": "Mtu", "ar": "شخص ما"}[lang]
		if from == "" {
			from = "Somebody"
		}
	}
	noteLine := ""
	if note != "" {
		noteLine = "\n« " + note + " »\n"
	}
	texts := map[string][2]string{
		"fr": {"%s vous demande %s", "Bonjour,\n\n%s vous demande %s sur Yamale Pay.\n%s\nOuvrez Yamale Pay pour payer ou refuser :\nhttps://pay.yamaletech.com/app/\n\nYamale\n"},
		"pt": {"%s pede-lhe %s", "Olá,\n\n%s pede-lhe %s no Yamale Pay.\n%s\nAbra o Yamale Pay para pagar ou recusar:\nhttps://pay.yamaletech.com/app/\n\nYamale\n"},
		"sw": {"%s anakuomba %s", "Habari,\n\n%s anakuomba %s kwenye Yamale Pay.\n%s\nFungua Yamale Pay ili ulipe au ukatae:\nhttps://pay.yamaletech.com/app/\n\nYamale\n"},
		"ar": {"%s يطلب منك %s", "مرحبًا،\n\n%s يطلب منك %s على Yamale Pay.\n%s\nافتح Yamale Pay للدفع أو الرفض:\nhttps://pay.yamaletech.com/app/\n\nYamale\n"},
		"en": {"%s asks you for %s", "Hello,\n\n%s asks you for %s on Yamale Pay.\n%s\nOpen Yamale Pay to pay or decline:\nhttps://pay.yamaletech.com/app/\n\nYamale\n"},
	}
	t, ok := texts[lang]
	if !ok {
		t = texts["en"]
	}
	return fmt.Sprintf(t[0], from, amount), fmt.Sprintf(t[1], from, amount, noteLine)
}
