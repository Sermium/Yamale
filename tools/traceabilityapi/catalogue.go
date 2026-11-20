package main

// A jeweler's catalogue: the designs read from its own website, drafted by
// Claude, and confirmed by a person before any piece of them is registered.
// It is kept here, off the chain, one file per jeweler. Nothing in it is on the
// register: a staff member registers pieces in the app, signing as today.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	jobIdle     = "idle"
	jobReading  = "reading"
	jobDrafting = "drafting"
	jobDone     = "done"
	jobFailed   = "failed"

	draftWorkers = 3
)

type importJob struct {
	State string `json:"state"`
	Done  int    `json:"done"`
	Of    int    `json:"of"`
	Error string `json:"error"`
}

type catalogue struct {
	Origin     string    `json:"origin"`
	ImportedAt string    `json:"importedAt"`
	Designs    []design  `json:"designs"`
	Job        importJob `json:"job"`
}

type usage struct{ In, Out int64 }

// drafter turns a design's own words into draft fields (draft.go).
type drafter interface {
	draft(ctx context.Context, title, description string) (fields, usage, error)
}

var (
	errDraftMalformed = errors.New("the draft did not fit the record's rules")
	errDraftRefused   = errors.New("the model declined to draft this text")
	errNoDesign       = &httpError{http.StatusNotFound, "", "no such design in this catalogue"}
	errDesignGone     = &httpError{http.StatusConflict, "gone", "this design is no longer on the website"}
)

// httpError carries the status and code a handler answers with.
type httpError struct {
	status int
	code   string
	msg    string
}

func (e *httpError) Error() string { return e.msg }

func refuseCode(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

func answer(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		refuseCode(w, he.status, he.code, he.msg)
		return
	}
	serverFault(w, err)
}

func (s *server) cataloguePath(id string) string {
	return filepath.Join(s.dir, "catalogue", id+".json")
}

// tidy makes every list a list, so the app never reads null.
func tidy(c *catalogue) {
	if c.Designs == nil {
		c.Designs = []design{}
	}
	for i := range c.Designs {
		d := &c.Designs[i]
		if d.Fields.Stones == nil {
			d.Fields.Stones = []stone{}
		}
		if d.Tags == nil {
			d.Tags = []string{}
		}
		if d.ImageURLs == nil {
			d.ImageURLs = []string{}
		}
		if d.Draft != nil && d.Draft.Stones == nil {
			d.Draft.Stones = []stone{}
		}
		if d.Confirmed != nil {
			if d.Confirmed.Fields.Stones == nil {
				d.Confirmed.Fields.Stones = []stone{}
			}
			if d.Confirmed.Photos == nil {
				d.Confirmed.Photos = []string{}
			}
		}
	}
	if c.Job.State == "" {
		c.Job.State = jobIdle
	}
}

// tidied is tidy for one design, for a response built before the save.
func tidied(d design) design {
	c := catalogue{Designs: []design{d}}
	tidy(&c)
	return c.Designs[0]
}

// loadCatalogue and saveCatalogue are called with catMu held.
func (s *server) loadCatalogue(id string) (catalogue, error) {
	var c catalogue
	b, err := os.ReadFile(s.cataloguePath(id))
	if errors.Is(err, os.ErrNotExist) {
		tidy(&c)
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	tidy(&c)
	return c, nil
}

// loadSettled loads a catalogue with catMu held. A job saved as reading or
// drafting while no import is running here was cut short by a restart: it is
// rewritten as failed ("interrupted") so the app stops waiting on it.
func (s *server) loadSettled(id string) (catalogue, error) {
	c, err := s.loadCatalogue(id)
	if err != nil {
		return c, err
	}
	if (c.Job.State == jobReading || c.Job.State == jobDrafting) && !s.importing[id] {
		c.Job = importJob{State: jobFailed, Error: "interrupted"}
		if err := s.saveCatalogue(id, c); err != nil {
			return c, err
		}
	}
	return c, nil
}

func (s *server) saveCatalogue(id string, c catalogue) error {
	tidy(&c)
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.cataloguePath(id)), 0o750); err != nil {
		return err
	}
	return writeAtomically(s.cataloguePath(id), b)
}

func (s *server) updateCatalogue(id string, change func(*catalogue) error) (catalogue, error) {
	s.catMu.Lock()
	defer s.catMu.Unlock()
	c, err := s.loadCatalogue(id)
	if err != nil {
		return c, err
	}
	if err := change(&c); err != nil {
		return c, err
	}
	return c, s.saveCatalogue(id, c)
}

// storedDesign reads the design a path names, for the signature's subject. A
// failed read is logged and answered generically: the error carries a path.
func (s *server) storedDesign(jeweler, id string) (design, error) {
	s.catMu.Lock()
	c, err := s.loadCatalogue(jeweler)
	s.catMu.Unlock()
	if err != nil {
		log.Printf("traceabilityapi: catalogue %s: %v", jeweler, err)
		return design{}, &httpError{http.StatusInternalServerError, "", "the server could not complete that; try again later"}
	}
	d := findDesign(&c, id)
	if d == nil {
		return design{}, errNoDesign
	}
	return *d, nil
}

func findDesign(c *catalogue, id string) *design {
	for i := range c.Designs {
		if c.Designs[i].ID == id {
			return &c.Designs[i]
		}
	}
	return nil
}

// catalogueSigner refuses strangers from the headers and the chain, reads at
// most limit bytes of body, and checks the signature over the subject that
// body and jeweler determine. On failure it has already answered.
func (s *server) catalogueSigner(w http.ResponseWriter, r *http.Request, limit int64, purpose string,
	subject func(j Jeweler, body []byte) (string, error)) (Jeweler, string, []byte, bool) {
	c, err := s.claimed(r)
	if err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return Jeweler{}, "", nil, false
	}
	js, err := s.jewelers(r.Context())
	if err != nil {
		chainUnreachable(w, err)
		return Jeweler{}, "", nil, false
	}
	j, ok := jewelerOf(js, c.address)
	if !ok {
		refuse(w, http.StatusForbidden, "only the staff of an active jeweler can use its catalogue")
		return Jeweler{}, "", nil, false
	}
	var body []byte
	if limit > 0 {
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
		if err != nil {
			readFailed(w, err, "that request is too large")
			return Jeweler{}, "", nil, false
		}
	}
	sub, err := subject(j, body)
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			answer(w, err)
		} else {
			refuse(w, http.StatusBadRequest, err.Error())
		}
		return Jeweler{}, "", nil, false
	}
	if err := s.verify(c, purpose, sub); err != nil {
		refuse(w, http.StatusUnauthorized, err.Error())
		return Jeweler{}, "", nil, false
	}
	return j, c.address, body, true
}

func (s *server) handleCatalogueImport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Origin string `json:"origin"`
	}
	j, _, _, ok := s.catalogueSigner(w, r, 1<<10, "catalogue-import", func(_ Jeweler, body []byte) (string, error) {
		if err := json.Unmarshal(body, &in); err != nil {
			return "", errors.New(`the body is {"origin": "https://…"}`)
		}
		return sha256Hex([]byte(in.Origin)), nil
	})
	if !ok {
		return
	}
	origin, err := s.fetch.origin(in.Origin)
	if err != nil || origin != in.Origin {
		refuseCode(w, http.StatusBadRequest, "refused", "only public https addresses can be imported, written as https://host")
		return
	}
	s.catMu.Lock()
	if s.importing == nil {
		s.importing = map[string]bool{}
	}
	if _, err := s.loadSettled(j.ID); err != nil {
		s.catMu.Unlock()
		serverFault(w, err)
		return
	}
	if s.importing[j.ID] {
		s.catMu.Unlock()
		refuseCode(w, http.StatusConflict, "busy", "an import is already running for this jeweler")
		return
	}
	s.importing[j.ID] = true
	c, err := s.loadCatalogue(j.ID)
	if err == nil {
		c.Job = importJob{State: jobReading}
		err = s.saveCatalogue(j.ID, c)
	}
	s.catMu.Unlock()
	if err != nil {
		s.endImport(j.ID)
		serverFault(w, err)
		return
	}
	go s.runImport(j, origin)
	writeJSON(w, http.StatusAccepted, c.Job)
}

func (s *server) endImport(id string) {
	s.catMu.Lock()
	delete(s.importing, id)
	s.catMu.Unlock()
}

func importErrorCode(err error) string {
	switch {
	case errors.Is(err, errUnreadable):
		return "unreadable"
	case errors.Is(err, errRobots):
		return "robots"
	case errors.Is(err, errRateLimited):
		return "rateLimited"
	case errors.Is(err, errNotPublic):
		return "refused"
	}
	return "unreachable"
}

// readWebsite tries Shopify's feed, then schema.org pages. A feed that is
// rate-limited falls back to the pages: on 2026-11-20 Jewel of Africa's
// sitemaps answered while its feed did not.
func (s *server) readWebsite(ctx context.Context, origin string) ([]sourceProduct, error) {
	products, shop, err := readShopify(ctx, s.fetch, origin, maxDesigns)
	if err == nil && shop {
		return products, nil
	}
	if err != nil && !errors.Is(err, errRateLimited) {
		return nil, err
	}
	pages, perr := readSchemaOrg(ctx, s.fetch, origin, maxDesigns)
	if perr != nil {
		return nil, perr
	}
	if len(pages) == 0 {
		if err != nil {
			return nil, err
		}
		return nil, errUnreadable
	}
	return pages, nil
}

func (s *server) setJob(id string, job importJob) {
	if _, err := s.updateCatalogue(id, func(c *catalogue) error { c.Job = job; return nil }); err != nil {
		log.Printf("traceabilityapi: catalogue %s: %v", id, err)
	}
}

func (s *server) runImport(j Jeweler, origin string) {
	defer s.endImport(j.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	read := s.readSite
	if read == nil {
		read = s.readWebsite
	}
	products, err := read(ctx, origin)
	if err != nil {
		log.Printf("traceabilityapi: catalogue %s: reading %s: %v", j.ID, origin, err)
		s.setJob(j.ID, importJob{State: jobFailed, Error: importErrorCode(err)})
		return
	}
	fresh := designsFrom(products)
	var pending []string
	if _, err := s.updateCatalogue(j.ID, func(c *catalogue) error {
		c.Origin, c.ImportedAt = origin, s.now().UTC().Format(time.RFC3339)
		c.Designs = mergeDesigns(c.Designs, fresh)
		for _, d := range c.Designs {
			if needsDraft(d) {
				pending = append(pending, d.ID)
			}
		}
		c.Job = importJob{State: jobDrafting, Of: len(pending)}
		return nil
	}); err != nil {
		log.Printf("traceabilityapi: catalogue %s: %v", j.ID, err)
		s.setJob(j.ID, importJob{State: jobFailed, Error: "unreachable"})
		return
	}
	s.draftAll(ctx, j, pending)
	s.setJob(j.ID, importJob{State: jobDone, Done: len(pending), Of: len(pending)})
}

func (s *server) draftAll(ctx context.Context, j Jeweler, ids []string) {
	work := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < draftWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range work {
				s.draftOne(ctx, j, id)
			}
		}()
	}
	for _, id := range ids {
		work <- id
	}
	close(work)
	wg.Wait()
}

func (s *server) draftOne(ctx context.Context, j Jeweler, id string) {
	s.catMu.Lock()
	c, err := s.loadCatalogue(j.ID)
	s.catMu.Unlock()
	if err != nil {
		return
	}
	d := findDesign(&c, id)
	if d == nil {
		return
	}
	title, text, key, hash := d.Title, d.Description, d.Key, d.SourceHash

	var got *fields
	note := ""
	switch {
	case s.drafter == nil:
		note = "unavailable"
	case !s.takeDraftAllowance(j.ID):
		note = "paused"
	default:
		f, use, err := s.drafter.draft(ctx, title, text)
		outcome := "ok"
		switch {
		case errors.Is(err, errDraftRefused):
			outcome = "refused"
		case errors.Is(err, errDraftMalformed):
			outcome = "malformed"
		case err != nil:
			outcome = "error"
			log.Printf("traceabilityapi: drafting %s/%s: %v", j.ID, key, err)
		}
		s.logDraft(j.ID, key, use, outcome)
		if err != nil {
			note = "unavailable"
		} else {
			got = &f
		}
	}
	if _, err := s.updateCatalogue(j.ID, func(c *catalogue) error {
		c.Job.Done++
		d := findDesign(c, id)
		if d == nil || d.SourceHash != hash {
			return nil
		}
		if got == nil {
			d.DraftNote = note
			return nil
		}
		d.Draft, d.DraftFor, d.DraftNote = got, hash, ""
		if d.Status == statusImported {
			d.Fields, d.Status = *got, statusDrafted
		}
		return nil
	}); err != nil {
		log.Printf("traceabilityapi: catalogue %s: %v", j.ID, err)
	}
}

// takeDraftAllowance counts one draft against the jeweler's month, or refuses.
func (s *server) takeDraftAllowance(id string) bool {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()
	path := filepath.Join(s.dir, "catalogue", id+".usage.json")
	month := s.now().UTC().Format("2006-01")
	var u struct {
		Month   string `json:"month"`
		Drafted int    `json:"drafted"`
	}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &u)
	}
	if u.Month != month {
		u.Month, u.Drafted = month, 0
	}
	if u.Drafted >= s.draftMonthly {
		return false
	}
	u.Drafted++
	b, _ := json.Marshal(u)
	if err := writeAtomically(path, b); err != nil {
		log.Printf("traceabilityapi: draft allowance %s: %v", id, err)
		return false
	}
	return true
}

// logDraft appends one line per call: who, which design, which model, the
// tokens and the outcome. Never the text, and never the key.
func (s *server) logDraft(jeweler, key string, use usage, outcome string) {
	line, _ := json.Marshal(map[string]any{
		"at": s.now().UTC().Format(time.RFC3339), "jeweler": jeweler, "key": key, "model": s.draftModel,
		"inputTokens": use.In, "outputTokens": use.Out, "outcome": outcome,
	})
	s.logMu.Lock()
	defer s.logMu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.dir, "catalogue", "drafting.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		log.Printf("traceabilityapi: drafting log: %v", err)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

func (s *server) handleCatalogue(w http.ResponseWriter, r *http.Request) {
	j, _, _, ok := s.catalogueSigner(w, r, 0, "catalogue-read", func(j Jeweler, _ []byte) (string, error) { return j.ID, nil })
	if !ok {
		return
	}
	s.catMu.Lock()
	c, err := s.loadSettled(j.ID)
	s.catMu.Unlock()
	if err != nil {
		serverFault(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func strictDecode(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *server) handleDesignSave(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !designIDPattern.MatchString(id) {
		refuse(w, http.StatusBadRequest, "a design is named by 16 hex characters")
		return
	}
	j, _, body, ok := s.catalogueSigner(w, r, 16<<10, "design-save", func(j Jeweler, body []byte) (string, error) {
		d, err := s.storedDesign(j.ID, id)
		if err != nil {
			return "", err
		}
		return sha256Hex(append([]byte(d.Key+":"), body...)), nil
	})
	if !ok {
		return
	}
	var in struct {
		Title  string `json:"title"`
		Fields fields `json:"fields"`
	}
	if err := strictDecode(body, &in); err != nil {
		refuse(w, http.StatusBadRequest, "a design is saved as {title, fields}: "+err.Error())
		return
	}
	if p := fieldsProblem(in.Title, in.Fields, false); p != "" {
		refuse(w, http.StatusBadRequest, p)
		return
	}
	if in.Fields.Stones == nil {
		in.Fields.Stones = []stone{}
	}
	var saved design
	if _, err := s.updateCatalogue(j.ID, func(c *catalogue) error {
		d := findDesign(c, id)
		if d == nil {
			return errNoDesign
		}
		if d.Status == statusGone {
			return errDesignGone
		}
		d.Title, d.Fields = in.Title, in.Fields
		if d.Status != statusChanged {
			d.Status = statusDrafted
		}
		saved = *d
		return nil
	}); err != nil {
		answer(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tidied(saved))
}

type confirmBody struct {
	SourceHash      string   `json:"sourceHash"`
	Title           string   `json:"title"`
	Fields          fields   `json:"fields"`
	Photos          []string `json:"photos"`
	RightsConfirmed bool     `json:"rightsConfirmed"`
}

func (s *server) storedImage(hash string) bool {
	if !hashPattern.MatchString(hash) {
		return false
	}
	t, err := os.ReadFile(s.filePath(hash) + ".type")
	return err == nil && strings.HasPrefix(string(t), "image/")
}

func (s *server) handleDesignConfirm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !designIDPattern.MatchString(id) {
		refuse(w, http.StatusBadRequest, "a design is named by 16 hex characters")
		return
	}
	var in confirmBody
	j, address, _, ok := s.catalogueSigner(w, r, 32<<10, "design-confirm", func(j Jeweler, body []byte) (string, error) {
		if err := strictDecode(body, &in); err != nil {
			return "", errors.New("a confirmation is {sourceHash, title, fields, photos, rightsConfirmed}: " + err.Error())
		}
		d, err := s.storedDesign(j.ID, id)
		if err != nil {
			return "", err
		}
		return sha256Hex(append([]byte(d.Key+":"+in.SourceHash+":"), body...)), nil
	})
	if !ok {
		return
	}
	if !in.RightsConfirmed {
		refuse(w, http.StatusBadRequest, "confirm that these photos and texts are yours to use")
		return
	}
	if p := fieldsProblem(in.Title, in.Fields, true); p != "" {
		refuse(w, http.StatusBadRequest, p)
		return
	}
	if in.Fields.Stones == nil {
		in.Fields.Stones = []stone{}
	}
	if len(in.Photos) < 1 || len(in.Photos) > maxImages {
		refuse(w, http.StatusBadRequest, "a design has 1 to 12 photos")
		return
	}
	for _, h := range in.Photos {
		if !s.storedImage(h) {
			refuse(w, http.StatusBadRequest, "store every photo as a file before naming it here")
			return
		}
	}
	var saved design
	if _, err := s.updateCatalogue(j.ID, func(c *catalogue) error {
		d := findDesign(c, id)
		if d == nil {
			return errNoDesign
		}
		if d.Status == statusGone {
			return errDesignGone
		}
		if d.SourceHash != in.SourceHash {
			return &httpError{http.StatusConflict, "stale", "the website changed this design since it was checked"}
		}
		d.Title, d.Fields, d.Status, d.GoneFrom = in.Title, in.Fields, statusConfirmed, ""
		d.Confirmed = &confirmation{Title: in.Title, Fields: in.Fields, Photos: in.Photos, SourceHash: in.SourceHash,
			By: address, At: s.now().UTC().Format(time.RFC3339)}
		saved = *d
		return nil
	}); err != nil {
		answer(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tidied(saved))
}

// handleDesignImage fetches one of a design's own images, as listed when the
// site was read, for the app to re-encode and store. It does not wait out a
// rate limit for long: a person is looking at the screen.
func (s *server) handleDesignImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := strconv.Atoi(r.PathValue("n"))
	if !designIDPattern.MatchString(id) || err != nil || n < 0 {
		refuse(w, http.StatusBadRequest, "an image is /designs/<id>/images/<n>")
		return
	}
	j, _, _, ok := s.catalogueSigner(w, r, 0, "catalogue-read", func(j Jeweler, _ []byte) (string, error) { return j.ID, nil })
	if !ok {
		return
	}
	s.catMu.Lock()
	c, err := s.loadCatalogue(j.ID)
	s.catMu.Unlock()
	if err != nil {
		serverFault(w, err)
		return
	}
	d := findDesign(&c, id)
	if d == nil || n >= len(d.ImageURLs) {
		refuse(w, http.StatusNotFound, "no such image")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	body, _, err := s.fetch.get(ctx, d.ImageURLs[n], acceptImages)
	switch {
	case errors.Is(err, errRateLimited):
		w.Header().Set("Retry-After", "60")
		refuseCode(w, http.StatusServiceUnavailable, "rateLimited", "the website is limiting how fast it can be read; try again in a minute")
		return
	case errors.Is(err, errRobots):
		refuseCode(w, http.StatusBadGateway, "robots", err.Error())
		return
	case err != nil:
		refuseCode(w, http.StatusBadGateway, "unreachable", err.Error())
		return
	}
	ctype := http.DetectContentType(body)
	switch ctype {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
	default:
		refuseCode(w, http.StatusBadGateway, "notImage", "the website did not answer with an image")
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}
