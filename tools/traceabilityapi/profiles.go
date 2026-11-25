package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// A jeweler's look on its own jewels' pages: a logo and two colours. It lives
// here rather than on the chain because it is presentation, and a jeweler
// should be able to change it without a transaction.

type profile struct {
	Logo   string `json:"logo"`
	Colour string `json:"colour"`
	Accent string `json:"accent"`
	// Contact is how the public reaches the shop, shown in the directory.
	// Absent in profiles set before the directory existed.
	Contact *contact `json:"contact,omitempty"`
}

var (
	colourPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	idPattern     = regexp.MustCompile(`^[1-9][0-9]*$`)
)

func (s *server) profilePath(id string) string {
	return filepath.Join(s.dir, "profiles", id+".json")
}

func (s *server) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		refuse(w, http.StatusBadRequest, "a jeweler id is a positive integer")
		return
	}
	body, err := os.ReadFile(s.profilePath(id))
	if err != nil {
		refuse(w, http.StatusNotFound, "this jeweler has not set a profile")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write(body)
}

func (s *server) handleProfilePut(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		refuse(w, http.StatusBadRequest, "a jeweler id is a positive integer")
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
	if err != nil {
		readFailed(w, err, "a profile is at most 8 KiB")
		return
	}
	sum := sha256.Sum256(body)
	address, err := s.signer(r, "profile", func(string) string { return id + "." + hex.EncodeToString(sum[:]) })
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return
	}
	var p profile
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		refuse(w, http.StatusBadRequest, "a profile is {logo, colour, accent, contact}: "+err.Error())
		return
	}
	if !colourPattern.MatchString(p.Colour) || !colourPattern.MatchString(p.Accent) {
		refuse(w, http.StatusBadRequest, "colours are written #rrggbb in lowercase hex")
		return
	}
	if p.Contact != nil {
		if err := p.Contact.check(); err != nil {
			refuse(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if p.Logo != "" {
		if !hashPattern.MatchString(p.Logo) {
			refuse(w, http.StatusBadRequest, "the logo is named by its fingerprint")
			return
		}
		if _, err := os.Stat(s.filePath(p.Logo)); err != nil {
			refuse(w, http.StatusBadRequest, "store the logo as a file before naming it here")
			return
		}
	}
	js, err := s.jewelers(r.Context())
	if err != nil {
		chainUnreachable(w, err)
		return
	}
	j, ok := jewelerByID(js, id)
	if !ok || j.Business != address {
		refuse(w, http.StatusForbidden, "only the jeweler's business account sets its profile")
		return
	}
	if !j.Active() {
		refuse(w, http.StatusForbidden, "a suspended jeweler cannot change its look")
		return
	}
	stored, _ := json.Marshal(p)
	if err := os.MkdirAll(filepath.Dir(s.profilePath(id)), 0o750); err != nil {
		serverFault(w, err)
		return
	}
	if err := writeAtomically(s.profilePath(id), stored); err != nil {
		serverFault(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
