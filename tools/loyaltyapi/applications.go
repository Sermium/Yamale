package main

// A business applies to be admitted as a jeweler. It applies from the account
// that will become its business address, and signs the application with it,
// so the office admits the key it saw, and not one typed into a form.
//
// Applications are files in a spool, as tools/notify's notices are. The
// office reads them with the operator token and admits by an x/group proposal;
// nothing here admits anybody.

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type application struct {
	Business string `json:"business"`
	Name     string `json:"name"`
	Contact  string `json:"contact"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Country  string `json:"country"`
	Message  string `json:"message"`
}

type spooled struct {
	ReceivedAt  int64       `json:"received_at"`
	Application application `json:"application"`
}

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

func (a application) check() error {
	switch {
	case len(strings.TrimSpace(a.Name)) < 2 || len(a.Name) > 120:
		return fmt.Errorf("the business name must be 2 to 120 characters")
	case len(strings.TrimSpace(a.Contact)) < 2 || len(a.Contact) > 120:
		return fmt.Errorf("the contact's name must be 2 to 120 characters")
	case !strings.Contains(a.Email, "@") || len(a.Email) > 254:
		return fmt.Errorf("the email address does not look like one")
	case len(a.Phone) > 40:
		return fmt.Errorf("the phone number is too long")
	case !countryPattern.MatchString(a.Country):
		return fmt.Errorf("the country is its two-letter ISO code, in capitals")
	case len(a.Message) > 2000:
		return fmt.Errorf("the message is at most 2,000 characters")
	}
	return nil
}

// handleApply refuses strangers from the headers before it reads the body, as
// uploads do, then verifies the signature over the body's hash.
func (s *server) handleApply(w http.ResponseWriter, r *http.Request) {
	c, err := s.claimed(r)
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		readFailed(w, err, "an application is at most 16 KiB")
		return
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	if err := s.verify(c, "application", hash); err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	address := c.address
	var a application
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		refuse(w, http.StatusBadRequest, "unreadable application: "+err.Error())
		return
	}
	if err := a.check(); err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.Business != address {
		refuse(w, http.StatusForbidden, "apply from the account that will be the business's own")
		return
	}
	out, _ := json.MarshalIndent(spooled{ReceivedAt: s.now().Unix(), Application: a}, "", " ")
	name := fmt.Sprintf("%d-%s.json", s.now().Unix(), hash[:12])
	spool := filepath.Join(s.dir, "applications")
	if err := os.MkdirAll(spool, 0o750); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeAtomically(filepath.Join(spool, name), out); err != nil {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"received": true})
}

func (s *server) handleApplications(w http.ResponseWriter, r *http.Request) {
	if s.operatorToken == "" {
		refuse(w, http.StatusNotFound, "not available here")
		return
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.operatorToken)) != 1 {
		refuse(w, http.StatusUnauthorized, "the operator token is required")
		return
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, "applications"))
	if err != nil && !os.IsNotExist(err) {
		refuse(w, http.StatusInternalServerError, err.Error())
		return
	}
	list := []spooled{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, "applications", e.Name()))
		if err != nil {
			continue
		}
		var sp spooled
		if json.Unmarshal(raw, &sp) == nil {
			list = append(list, sp)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ReceivedAt > list[j].ReceivedAt })
	writeJSON(w, http.StatusOK, list)
}
