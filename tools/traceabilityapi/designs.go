package main

// A design is one product of a jeweler's website: what the site says about it,
// Claude's draft of its materials, and a person's confirmation. A design is
// never on the register; a piece of it is, once registered from the app.

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	statusImported  = "imported"
	statusDrafted   = "drafted"
	statusChanged   = "changed"
	statusConfirmed = "confirmed"
	statusGone      = "gone"

	maxDescription = 5000
	maxTitle       = 120
	maxImages      = 12
)

type stone struct {
	Kind  string `json:"kind"`
	Carat string `json:"carat"`
	Count int    `json:"count"`
}

type fields struct {
	Metal       string  `json:"metal"`
	WeightGrams string  `json:"weightGrams"`
	Stones      []stone `json:"stones"`
}

type money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type confirmation struct {
	Title      string   `json:"title"`
	Fields     fields   `json:"fields"`
	Photos     []string `json:"photos"`
	SourceHash string   `json:"sourceHash"`
	By         string   `json:"by"`
	At         string   `json:"at"`
}

type design struct {
	ID          string        `json:"id"`
	Key         string        `json:"key"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Type        string        `json:"type"`
	Tags        []string      `json:"tags"`
	ProductURL  string        `json:"productUrl"`
	ListPrice   *money        `json:"listPrice"`
	ImageURLs   []string      `json:"imageUrls"`
	Draft       *fields       `json:"draft"`
	DraftFor    string        `json:"draftFor"`
	DraftNote   string        `json:"draftNote"`
	Fields      fields        `json:"fields"`
	SourceHash  string        `json:"sourceHash"`
	Status      string        `json:"status"`
	GoneFrom    string        `json:"goneFrom,omitempty"`
	Confirmed   *confirmation `json:"confirmed"`
}

var (
	decimalPattern  = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
	designIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func designID(key string) string { return sha256Hex([]byte(key))[:16] }

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func designKey(p sourceProduct) string {
	if sku := strings.TrimSpace(p.SKU); sku != "" && utf8.RuneCountInString(sku) <= 80 {
		return sku
	}
	if h := handleOf(p); h != "" {
		return "handle:" + h
	}
	return "url:" + p.Path
}

var productPathPattern = regexp.MustCompile(`^/products/([^/?#]+)/?$`)

// handleOf is the product's handle: its own, or the one its /products/<handle>
// path carries (the schema.org fallback reads pages, which give no handle), so a
// product keys the same whether the feed or the page read it.
func handleOf(p sourceProduct) string {
	if p.Handle != "" {
		return p.Handle
	}
	if m := productPathPattern.FindStringSubmatch(p.Path); m != nil {
		return m[1]
	}
	return ""
}

// designsFrom builds the designs a site describes, before any merge.
func designsFrom(ps []sourceProduct) []design {
	seen := map[string]bool{}
	var out []design
	for _, p := range ps {
		key := designKey(p)
		if seen[key] {
			if h := handleOf(p); h != "" {
				key = "handle:" + h
			} else {
				key = "url:" + p.Path
			}
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		d := design{
			ID: designID(key), Key: key,
			Title:       clip(strings.TrimSpace(p.Title), maxTitle),
			Description: clip(plainText(p.DescriptionHTML), maxDescription),
			Type:        p.Type, Tags: p.Tags,
			Fields: fields{Stones: []stone{}},
		}
		if d.Tags == nil {
			d.Tags = []string{}
		}
		if strings.HasPrefix(p.URL, "https://") && len(p.URL) <= 500 {
			d.ProductURL = p.URL
		}
		if decimalPattern.MatchString(p.Price) && len(p.Price) <= 20 && currencyPattern.MatchString(p.Currency) {
			d.ListPrice = &money{Amount: p.Price, Currency: p.Currency}
		}
		dedupe := map[string]bool{}
		d.ImageURLs = []string{}
		for _, im := range p.Images {
			if !dedupe[im] && len(d.ImageURLs) < maxImages {
				dedupe[im] = true
				d.ImageURLs = append(d.ImageURLs, im)
			}
		}
		price := ""
		if d.ListPrice != nil {
			price = d.ListPrice.Amount + " " + d.ListPrice.Currency
		}
		d.SourceHash = sha256Hex([]byte(d.Title + "\n" + d.Description + "\n" + price + "\n" + strings.Join(d.ImageURLs, "\n")))
		out = append(out, d)
	}
	return out
}

// needsDraft: Claude has not drafted this text yet, and someone still needs it.
func needsDraft(d design) bool {
	return (d.Status == statusImported || d.Status == statusChanged) && d.DraftFor != d.SourceHash
}

// mergeDesigns applies a refresh. A new key is imported; a confirmed design
// whose site text changed becomes changed and keeps its confirmed fields; an
// unconfirmed one whose text changed starts again; a key the site no longer
// lists is gone, and comes back as what it was if it reappears unchanged.
func mergeDesigns(old, fresh []design) []design {
	byKey := map[string]design{}
	for _, d := range old {
		byKey[d.Key] = d
	}
	seen := map[string]bool{}
	out := make([]design, 0, len(fresh)+len(old))
	for _, f := range fresh {
		seen[f.Key] = true
		o, had := byKey[f.Key]
		if !had {
			f.Status = statusImported
			out = append(out, f)
			continue
		}
		f.Draft, f.DraftFor, f.DraftNote, f.Fields, f.Confirmed = o.Draft, o.DraftFor, o.DraftNote, o.Fields, o.Confirmed
		prior := o.Status
		if prior == statusGone {
			prior = o.GoneFrom
		}
		switch {
		case o.SourceHash == f.SourceHash:
			f.Status = prior
		case o.Confirmed != nil && o.Confirmed.SourceHash == f.SourceHash:
			f.Status = statusConfirmed
		case o.Confirmed != nil:
			f.Status = statusChanged
		default:
			f.Status, f.Draft, f.DraftFor, f.DraftNote, f.Fields = statusImported, nil, "", "", fields{Stones: []stone{}}
		}
		out = append(out, f)
	}
	for _, o := range old {
		if seen[o.Key] {
			continue
		}
		if o.Status != statusGone {
			o.GoneFrom, o.Status = o.Status, statusGone
		}
		out = append(out, o)
	}
	return out
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// fieldsProblem is the first thing wrong with a design's title and fields, or
// "". complete adds what confirming needs. It mirrors designComplete in
// clients/traceability/src/atelier/draft.ts; change both together.
func fieldsProblem(title string, f fields, complete bool) string {
	switch {
	case runes(title) > maxTitle:
		return "the title is at most 120 characters"
	case runes(f.Metal) > 80:
		return "the metal is at most 80 characters"
	case runes(f.WeightGrams) > 12 || (f.WeightGrams != "" && !decimalPattern.MatchString(f.WeightGrams)):
		return "the weight is a number of grams, like 7.5"
	case len(f.Stones) > 50:
		return "a design lists at most 50 kinds of stone"
	}
	for _, s := range f.Stones {
		switch {
		case runes(s.Kind) > 60 || runes(s.Carat) > 12:
			return "a stone's kind is at most 60 characters and its carats at most 12"
		case s.Carat != "" && !decimalPattern.MatchString(s.Carat):
			return "carats are a number, like 0.25"
		case s.Count < 0 || s.Count > 9999:
			return "a stone count is 1 to 9999"
		}
	}
	if !complete {
		return ""
	}
	if strings.TrimSpace(title) == "" {
		return "a confirmed design has a title"
	}
	if strings.TrimSpace(f.Metal) == "" {
		return "a confirmed design names its metal"
	}
	// Carats and count may stay empty and 0: the website often does not say, and
	// the record then reads "not stated" rather than a number nobody gave.
	for _, s := range f.Stones {
		if strings.TrimSpace(s.Kind) == "" {
			return "every stone of a confirmed design is named"
		}
	}
	return ""
}
