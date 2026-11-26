package main

// The card vault: a shop's ownership cards, readable by every member of its
// staff on any device, and by nobody else — this server included.
//
// A card's secret is what lets a customer claim the piece, so it was kept only
// in the browser that made the card. A second device, a colleague, or the
// shop's own account after a move found nothing and offered to make a new card,
// which would have cancelled the one already handed to the customer.
//
// Each card secret is sealed by the browser to each member's public key
// (secp256k1 ECDH, HKDF-SHA256, ChaCha20-Poly1305) and only the sealed boxes
// come here. A member's public key is learned from that member's own signed
// requests, or else read from its account on the chain, which holds the key of
// any account that has signed a transaction. The browser checks every key
// against the address it is listed for, so the server cannot slip in a key of
// its own; and a reader is handed only its own box. Whoever can open a card seals it for the members who
// lack it, the next time they open the cards page.
//
// What the server can do with the file is drop it, or serve a stale copy; it
// cannot read a card or forge one a member would accept, because the browser
// checks that a box opens to the secret of the card key recorded beside it.

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
)

const (
	vaultMaxCards = 20000
	// A box is a 33-byte ephemeral key, a 12-byte nonce and 32 bytes of secret
	// plus the 16-byte tag: 93 bytes, 124 in base64.
	vaultBoxLen = 124
)

var cardKeyPattern = regexp.MustCompile(`^0[23][0-9a-f]{64}$`)

type vaultCard struct {
	// CardKey is the card's public key, hex, as issued on the chain.
	CardKey string `json:"card_key"`
	// Boxes maps a member's address to the card secret sealed for it.
	Boxes map[string]string `json:"boxes"`
}

type vaultFile struct {
	// Keys maps an address to the public key (base64) it signed with.
	Keys  map[string]string    `json:"keys"`
	Cards map[string]vaultCard `json:"cards"`
}

// KeyReader reads the public key an account has signed with on the chain,
// base64; "" when the account has none yet.
type KeyReader interface {
	AccountKey(ctx context.Context, address string) (string, error)
}

func (c restChain) AccountKey(ctx context.Context, address string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/cosmos/auth/v1beta1/accounts/"+url.PathEscape(address), nil)
	if err != nil {
		return "", err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("the chain did not answer: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the chain answered %d for an account", res.StatusCode)
	}
	var body struct {
		Account struct {
			PubKey *struct {
				Type string `json:"@type"`
				Key  string `json:"key"`
			} `json:"pub_key"`
		} `json:"account"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("unreadable account: %w", err)
	}
	pk := body.Account.PubKey
	if pk == nil || pk.Type != "/cosmos.crypto.secp256k1.PubKey" {
		return "", nil
	}
	if raw, err := base64.StdEncoding.DecodeString(pk.Key); err != nil || len(raw) != 33 {
		return "", nil
	}
	return pk.Key, nil
}

func (s *server) vaultPath(jewelerID string) string {
	return filepath.Join(s.dir, "cardvault", jewelerID+".json")
}

func (s *server) loadVault(jewelerID string) (vaultFile, error) {
	v := vaultFile{Keys: map[string]string{}, Cards: map[string]vaultCard{}}
	raw, err := os.ReadFile(s.vaultPath(jewelerID))
	if errors.Is(err, os.ErrNotExist) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("card vault %s: %w", jewelerID, err)
	}
	if v.Keys == nil {
		v.Keys = map[string]string{}
	}
	if v.Cards == nil {
		v.Cards = map[string]vaultCard{}
	}
	return v, nil
}

func (s *server) storeVault(jewelerID string, v vaultFile) error {
	if err := os.MkdirAll(filepath.Dir(s.vaultPath(jewelerID)), 0o700); err != nil {
		return err
	}
	out, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeAtomically(s.vaultPath(jewelerID), out)
}

// members is the shop's business account and its staff, in that order.
func members(j Jeweler) []string {
	return append([]string{j.Business}, j.Staff...)
}

type vaultMember struct {
	Address string `json:"address"`
	Key     string `json:"key"`
}

type vaultEntry struct {
	CardKey string   `json:"card_key"`
	Box     string   `json:"box,omitempty"`
	Holders []string `json:"holders"`
}

// GET /v1/cardvault: the reader's own boxes, who holds each card, and the
// members whose keys are known. Reading also records the reader's key, which is
// how a member becomes someone the others can seal cards for.
func (s *server) handleVaultRead(w http.ResponseWriter, r *http.Request) {
	c, err := s.claimed(r)
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	j, me, _, ok := s.catalogueSigner(w, r, 0, "cardvault-read", func(j Jeweler, _ []byte) (string, error) { return j.ID, nil })
	if !ok {
		return
	}
	s.vaultMu.Lock()
	defer s.vaultMu.Unlock()
	v, err := s.loadVault(j.ID)
	if err != nil {
		serverFault(w, err)
		return
	}
	key := base64.StdEncoding.EncodeToString(c.pub)
	changed := v.Keys[me] != key
	v.Keys[me] = key
	// A member who has never opened the vault is still sealed for, if the
	// chain knows its key: the colleague holding the cards need not come back.
	if s.keys != nil {
		for _, m := range members(j) {
			if _, ok := v.Keys[m]; ok {
				continue
			}
			if k, err := s.keys.AccountKey(r.Context(), m); err == nil && k != "" {
				v.Keys[m] = k
				changed = true
			}
		}
	}
	if changed {
		if err := s.storeVault(j.ID, v); err != nil {
			serverFault(w, err)
			return
		}
	}
	out := struct {
		Members []vaultMember         `json:"members"`
		Cards   map[string]vaultEntry `json:"cards"`
	}{Members: []vaultMember{}, Cards: map[string]vaultEntry{}}
	for _, m := range members(j) {
		if k, ok := v.Keys[m]; ok {
			out.Members = append(out.Members, vaultMember{Address: m, Key: k})
		}
	}
	for id, card := range v.Cards {
		e := vaultEntry{CardKey: card.CardKey, Box: card.Boxes[me], Holders: []string{}}
		for addr := range card.Boxes {
			e.Holders = append(e.Holders, addr)
		}
		out.Cards[id] = e
	}
	writeJSON(w, http.StatusOK, out)
}

// PUT /v1/cardvault/{jewel}: boxes for one card. A card key other than the
// stored one replaces the entry (the card was issued anew, and the old boxes
// open a card that no longer works); the same key adds boxes for members who
// had none, and never overwrites one, so a member cannot spoil a colleague's.
func (s *server) handleVaultPut(w http.ResponseWriter, r *http.Request) {
	jewel := r.PathValue("jewel")
	if n, err := strconv.ParseUint(jewel, 10, 64); err != nil || n == 0 || strconv.FormatUint(n, 10) != jewel {
		refuse(w, http.StatusBadRequest, "a jewel is named by its id")
		return
	}
	j, _, body, ok := s.catalogueSigner(w, r, 64<<10, "cardvault-put", func(j Jeweler, body []byte) (string, error) {
		return sha256Hex([]byte(j.ID + ":" + jewel + ":" + string(body))), nil
	})
	if !ok {
		return
	}
	var in vaultCard
	if err := strictDecode(body, &in); err != nil {
		refuse(w, http.StatusBadRequest, `the body is {"card_key": "02…", "boxes": {"yml1…": "…"}}`)
		return
	}
	if !cardKeyPattern.MatchString(in.CardKey) {
		refuse(w, http.StatusBadRequest, "card_key is a compressed public key: 66 lower-case hex characters")
		return
	}
	if _, err := hex.DecodeString(in.CardKey); err != nil {
		refuse(w, http.StatusBadRequest, "card_key is not hex")
		return
	}
	allowed := map[string]bool{}
	for _, m := range members(j) {
		allowed[m] = true
	}
	for addr, box := range in.Boxes {
		if !allowed[addr] {
			refuse(w, http.StatusBadRequest, addr+" is not a member of "+j.Name)
			return
		}
		if raw, err := base64.StdEncoding.DecodeString(box); err != nil || len(box) != vaultBoxLen || len(raw) != 93 {
			refuse(w, http.StatusBadRequest, "a box is 93 bytes in base64")
			return
		}
	}

	s.vaultMu.Lock()
	defer s.vaultMu.Unlock()
	v, err := s.loadVault(j.ID)
	if err != nil {
		serverFault(w, err)
		return
	}
	cur, exists := v.Cards[jewel]
	if !exists && len(v.Cards) >= vaultMaxCards {
		refuse(w, http.StatusInsufficientStorage, "this shop's card vault is full")
		return
	}
	if !exists || cur.CardKey != in.CardKey {
		cur = vaultCard{CardKey: in.CardKey, Boxes: map[string]string{}}
	}
	added := 0
	for addr, box := range in.Boxes {
		if _, had := cur.Boxes[addr]; !had {
			cur.Boxes[addr] = box
			added++
		}
	}
	v.Cards[jewel] = cur
	if err := s.storeVault(j.ID, v); err != nil {
		serverFault(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "holders": len(cur.Boxes)})
}
