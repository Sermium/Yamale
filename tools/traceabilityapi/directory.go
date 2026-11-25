package main

// The jewelers' directory: how a shop is found, reached, and what it makes.
//
// Contact details live in the profile the business account already signs, so
// only the business can change them. The portfolio is the catalogue's confirmed
// designs: what the shop has said, with its own signature, is its work and its
// to show. Drafts, unconfirmed imports and designs gone from the shop's site
// are never listed.

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type contact struct {
	About    string `json:"about,omitempty"`
	Address  string `json:"address,omitempty"`
	City     string `json:"city,omitempty"`
	Country  string `json:"country,omitempty"`
	Phone    string `json:"phone,omitempty"`
	WhatsApp string `json:"whatsapp,omitempty"`
	Email    string `json:"email,omitempty"`
	Website  string `json:"website,omitempty"`
	Hours    string `json:"hours,omitempty"`
}

var (
	phonePattern   = regexp.MustCompile(`^\+?[0-9][0-9 ().-]{3,29}$`)
	websitePattern = regexp.MustCompile(`^https://[^\s/]+\.[^\s]{1,190}$`)
	emailPattern   = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
)

// text refuses control characters other than a line break, so nothing in a
// shop's own words can rewrite the page around it, and caps the length in
// characters, not bytes, so Arabic and accented text get the same room.
func text(name, v string, max int, multiline bool) error {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > max {
		return errors.New(name + " is at most " + strconv.Itoa(max) + " characters")
	}
	for _, r := range v {
		if unicode.IsControl(r) && !(multiline && r == '\n') {
			return errors.New(name + " holds a control character")
		}
	}
	return nil
}

func (c contact) check() error {
	for _, f := range []struct {
		name, v   string
		max       int
		multiline bool
	}{
		{"about", c.About, 600, true}, {"address", c.Address, 200, true}, {"city", c.City, 80, false},
		{"hours", c.Hours, 200, true},
	} {
		if err := text(f.name, f.v, f.max, f.multiline); err != nil {
			return err
		}
	}
	switch {
	case c.Country != "" && !countryPattern.MatchString(c.Country):
		return errors.New("the country is its two-letter code, e.g. SN")
	case c.Phone != "" && !phonePattern.MatchString(c.Phone):
		return errors.New("the phone number holds digits, spaces and + ( ) . - only")
	case c.WhatsApp != "" && !phonePattern.MatchString(c.WhatsApp):
		return errors.New("the WhatsApp number holds digits, spaces and + ( ) . - only")
	case c.Email != "" && (len(c.Email) > 254 || !emailPattern.MatchString(c.Email)):
		return errors.New("the email address does not look like one")
	case c.Website != "" && !websitePattern.MatchString(c.Website):
		return errors.New("the website starts with https:// and has no spaces")
	}
	return nil
}

// portfolioDesign is what the public sees of one confirmed design: the words,
// materials and photos the shop confirmed, not the draft or the import.
type portfolioDesign struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Fields      fields   `json:"fields"`
	Photos      []string `json:"photos"`
	ProductURL  string   `json:"productUrl,omitempty"`
}

func portfolioOf(c catalogue) []portfolioDesign {
	out := []portfolioDesign{}
	for _, d := range c.Designs {
		if d.Confirmed == nil || d.Status == statusGone {
			continue
		}
		out = append(out, portfolioDesign{
			ID: d.ID, Title: d.Confirmed.Title, Description: d.Description, Type: d.Type,
			Fields: d.Confirmed.Fields, Photos: d.Confirmed.Photos, ProductURL: d.ProductURL,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title) })
	return out
}

// handlePortfolio lists a jeweler's confirmed designs to anyone. A suspended
// jeweler is not listed: the directory sends people to shops in good standing.
func (s *server) handlePortfolio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		refuse(w, http.StatusBadRequest, "a jeweler id is a positive integer")
		return
	}
	js, err := s.jewelers(r.Context())
	if err != nil {
		chainUnreachable(w, err)
		return
	}
	j, ok := jewelerByID(js, id)
	if !ok || !j.Active() {
		refuse(w, http.StatusNotFound, "no jeweler in good standing has this id")
		return
	}
	s.catMu.Lock()
	c, err := s.loadCatalogue(id)
	s.catMu.Unlock()
	if err != nil {
		serverFault(w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, map[string]any{"designs": portfolioOf(c)})
}
