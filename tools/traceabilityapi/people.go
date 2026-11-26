package main

// People: the name behind an account, so the app can say "Awa Diop ·
// SN-K3M9-7QRT-B" instead of a code nobody recognises.
//
// Kept here and never on the chain: a public chain cannot forget, and a name
// and a date of birth are exactly what a person may later want removed.
// Only the account itself writes its record, signed with its key. The name is
// readable by any account of the app (any sponsored account, or a member of a
// jeweler's staff), which is what the owner decided on 2026-11-26; the date of
// birth is returned to the account alone. The country is not here: x/alias
// records it, and the identifier carries it.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cosmos/cosmos-sdk/types/bech32"
)

type person struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	BirthDate string `json:"birth_date,omitempty"`
}

var birthPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// checkName allows letters and marks in any script, spaces, and the
// apostrophes, hyphens and dots names carry; nothing that renders as markup.
func checkName(field, v string) error {
	n := utf8.RuneCountInString(v)
	if n == 0 || n > 60 {
		return errors.New(field + " is 1 to 60 characters")
	}
	for _, r := range v {
		if unicode.IsLetter(r) || unicode.IsMark(r) || r == ' ' || r == '-' || r == '\'' || r == '’' || r == '.' {
			continue
		}
		return errors.New(field + " holds a character a name does not")
	}
	return nil
}

func (p *person) check(now time.Time) error {
	p.FirstName, p.LastName = strings.TrimSpace(p.FirstName), strings.TrimSpace(p.LastName)
	if err := checkName("first_name", p.FirstName); err != nil {
		return err
	}
	if err := checkName("last_name", p.LastName); err != nil {
		return err
	}
	if p.BirthDate != "" {
		d, err := time.Parse("2006-01-02", p.BirthDate)
		if !birthPattern.MatchString(p.BirthDate) || err != nil {
			return errors.New("birth_date is written YYYY-MM-DD")
		}
		if d.Year() < 1900 || d.After(now) {
			return errors.New("birth_date is not a plausible date of birth")
		}
	}
	return nil
}

func validAddress(a string) bool {
	prefix, _, err := bech32.DecodeAndConvert(a)
	return err == nil && prefix == "yml"
}

func (s *server) personPath(address string) string {
	return filepath.Join(s.dir, "people", address+".json")
}

func (s *server) loadPerson(address string) (person, bool, error) {
	var p person
	raw, err := os.ReadFile(s.personPath(address))
	if errors.Is(err, os.ErrNotExist) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	return p, true, json.Unmarshal(raw, &p)
}

// PUT /v1/people/me, signed over "<address>.<sha256 of the body>".
func (s *server) handlePersonPut(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<10))
	if err != nil {
		readFailed(w, err, "a person is at most 1 KiB")
		return
	}
	sum := sha256.Sum256(body)
	address, err := s.signer(r, "person-save", func(a string) string { return a + "." + hex.EncodeToString(sum[:]) })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	var p person
	if err := strictDecode(body, &p); err != nil {
		refuse(w, http.StatusBadRequest, `a person is {"first_name", "last_name", "birth_date"}`)
		return
	}
	if err := p.check(s.now()); err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	out, _ := json.Marshal(p)
	if err := os.MkdirAll(filepath.Dir(s.personPath(address)), 0o700); err != nil {
		serverFault(w, err)
		return
	}
	if err := writeAtomically(s.personPath(address), out); err != nil {
		serverFault(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// GET /v1/people/me: the account's own record, date of birth included.
func (s *server) handlePersonMine(w http.ResponseWriter, r *http.Request) {
	address, err := s.signer(r, "person-read", func(a string) string { return a })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	p, found, err := s.loadPerson(address)
	if err != nil {
		serverFault(w, err)
		return
	}
	if !found {
		refuse(w, http.StatusNotFound, "no profile yet")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// isMember: the reader is an account of the app, sponsored by it or on a
// jeweler's staff. A key anybody can make is not enough to read names.
func (s *server) isMember(r *http.Request, address string) (bool, error) {
	ledger, err := s.readLedger()
	if err != nil {
		return false, err
	}
	for _, rec := range ledger {
		if rec.Address == address {
			return true, nil
		}
	}
	js, err := s.jewelers(r.Context())
	if err != nil {
		return false, err
	}
	_, ok := jewelerOf(js, address)
	return ok, nil
}

// GET /v1/people/{address}: the name behind an account, for a member of the
// app, signed over the address asked about. Never the date of birth.
func (s *server) handlePersonLookup(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("address")
	if !validAddress(target) {
		refuse(w, http.StatusBadRequest, "an account is named by its yml1… address")
		return
	}
	reader, err := s.signer(r, "person-lookup", func(string) string { return target })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	member, err := s.isMember(r, reader)
	if err != nil {
		chainUnreachable(w, err)
		return
	}
	if !member {
		refuse(w, http.StatusForbidden, "names are shown to the app's own accounts")
		return
	}
	p, found, err := s.loadPerson(target)
	if err != nil {
		serverFault(w, err)
		return
	}
	if !found {
		refuse(w, http.StatusNotFound, "no name for this account")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"first_name": p.FirstName, "last_name": p.LastName})
}
