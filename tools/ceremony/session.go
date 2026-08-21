package main

// The state a served ceremony holds, and the handlers over it.
//
// The session lives in this process, never in the browser. That is not a
// preference about architecture: the page runs in a profile this program deletes
// on exit, so anything the page stored would be gone anyway — and anything it
// stored that *survived* would be the one thing a ceremony must not leave
// behind. So the page is a view, `/api/state` is how it recovers after a reload,
// and no phrase, key or progress marker is ever written to any browser storage.
//
// One decision here is worth stating because it differs from the terminal path
// and is stronger, not weaker. The terminal generates, verifies, derives, writes
// the public record and zeroes the key inside one function. A served ceremony
// spans requests, so holding the derived private key between them would mean
// five signing keys sitting in this process's memory for the length of the
// ceremony. It does not. The key is derived at commit, used once to sign the
// submission, and zeroed immediately. When a custodian later attests to the
// assembled group they type their phrase again from their own sheet, and this
// re-derives from that.
//
// Which means the attestation step is a second transcription check, performed
// against the paper the custodian is actually going to store, after the paper
// has been folded and handled. That is the check that matters most and the
// terminal path never had it.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type mode string

const (
	// modeColocated is everybody in one room at one machine, taking turns.
	modeColocated mode = "co-located"
	// modeCustodian is one custodian on their own machine, which is the mode
	// where no single machine ever sees more than one phrase.
	modeCustodian mode = "custodian"
)

// inflight is the one custodian currently mid-generation.
//
// At most one at a time, in either mode. In a room, two phrases on screen at
// once is how two custodians end up writing down each other's words.
type inflight struct {
	name  string
	index uint32

	secret *secret
	// granted records that the phrase has been served. It is never reset:
	// see handlePhrase.
	granted   bool
	written   bool
	positions []int
	verified  bool
}

func (f *inflight) discard() {
	if f == nil {
		return
	}
	if f.secret != nil {
		f.secret.zero()
		f.secret = nil
	}
}

type session struct {
	mu sync.Mutex

	mode      mode
	out       string
	startedAt time.Time

	binaryHash     string
	network        []networkFinding
	virtualisation []vmFinding
	answers        []checklistAnswer
	acknowledged   string
	// rehearsal is set when the room accepted that a virtualised machine makes
	// this practice rather than a ceremony whose keys may hold value.
	rehearsal     bool
	preflightDone bool

	params    ceremonyParams
	paramsSet bool

	current   *inflight
	committed []identity
	// submissions carries this instance's own custodian plus every one imported
	// from elsewhere. In co-located mode all five are generated here.
	submissions []submission

	assembledDoc *assembled
	attestations []signedAttestation

	// drilled records which addresses have had their paper proven by a restore.
	drilled  map[string]bool
	notes    []string
	complete bool

	// finished is closed once, when the room says the ceremony is over, so
	// runServe can tear down the browser profile without the operator having to
	// return to a terminal they may not have.
	finished  chan struct{}
	finishing sync.Once
}

// wipe overwrites every phrase this process still holds.
//
// Called on the way out of runServe, on every exit path including an interrupt.
// The failed and interrupted ceremonies are the ones most likely to leave a
// phrase behind, because they are the ones where somebody is thinking about the
// problem rather than about the key.
func (s *session) wipe() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current.discard()
	s.current = nil
}

func newSession(m mode, out string) *session {
	s := &session{
		mode:      m,
		out:       out,
		startedAt: time.Now(),
		drilled:   map[string]bool{},
		finished:  make(chan struct{}),
	}
	// Gathered once at start rather than per request: the answers cannot change
	// while the machine sits in a locked room, and re-scanning would invite a
	// page to poll for a different answer until it got a convenient one.
	if hash, err := binaryHash(); err == nil {
		s.binaryHash = hash
	}
	s.network = scanNetwork()
	s.virtualisation = detectVirtualisation()
	return s
}

// ---------------------------------------------------------------- transport

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// fail sends the error as text the page shows verbatim.
//
// Verbatim because these messages are the ceremony's voice at the moment
// something is wrong, and a page that rewrote them into "an error occurred"
// would be hiding the sentence somebody needs to act on.
func fail(w http.ResponseWriter, code int, err error) {
	reply(w, code, map[string]string{"error": err.Error()})
}

func posted(w http.ResponseWriter, r *http.Request, into any) bool {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, errors.New("this step is a POST"))
		return false
	}
	if into == nil {
		return true
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(into); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("could not read the request: %w", err))
		return false
	}
	return true
}

func (s *session) handlePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page, err := uiFiles.ReadFile("ui.html")
	if err != nil {
		http.Error(w, "the interface is missing from this binary", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A page that cannot reach the network cannot leak what it renders, and the
	// ceremony has no reason to load anything. 'unsafe-inline' is present
	// because the page is one embedded file with its script and style in it;
	// there is no origin it could fetch them from.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; "+
			"img-src data:; connect-src 'self'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'")
	_, _ = w.Write(page)
}

// ---------------------------------------------------------------- state

type stateView struct {
	Mode        mode           `json:"mode"`
	Out         string         `json:"out"`
	StartedAt   string         `json:"started_at"`
	PreflightOK bool           `json:"preflight_done"`
	Rehearsal   bool           `json:"rehearsal"`
	ParamsSet   bool           `json:"params_set"`
	Params      ceremonyParams `json:"params"`
	Fingerprint string         `json:"params_fingerprint"`
	Current     *currentView   `json:"current"`
	Committed   []identity     `json:"committed"`
	Submissions int            `json:"submissions"`
	// Missing is prose for the operator in both directions — it says so when
	// nothing is missing. Ready is the machine-readable answer, because a page
	// that inferred readiness from an empty string would wait forever.
	Missing      string              `json:"missing"`
	Ready        bool                `json:"ready"`
	Assembled    *assembled          `json:"assembled"`
	Attestations []signedAttestation `json:"attestations"`
	Drilled      []string            `json:"drilled"`
	Notes        []string            `json:"notes"`
	Complete     bool                `json:"complete"`
}

type currentView struct {
	Name      string `json:"name"`
	Granted   bool   `json:"phrase_served"`
	Written   bool   `json:"written"`
	Verified  bool   `json:"verified"`
	Positions []int  `json:"positions"`
	Words     int    `json:"word_count"`
}

func (s *session) view() stateView {
	v := stateView{
		Mode:         s.mode,
		Out:          s.out,
		StartedAt:    s.startedAt.UTC().Format(time.RFC3339),
		PreflightOK:  s.preflightDone,
		Rehearsal:    s.rehearsal,
		ParamsSet:    s.paramsSet,
		Params:       s.params,
		Committed:    s.committed,
		Submissions:  len(s.submissions),
		Assembled:    s.assembledDoc,
		Attestations: s.attestations,
		Notes:        s.notes,
		Complete:     s.complete,
	}
	if s.paramsSet {
		v.Fingerprint = s.params.fingerprint()
		v.Missing = missingFrom(s.params, s.submissions)
		v.Ready = s.ready()
	}
	if s.current != nil {
		v.Current = &currentView{
			Name:      s.current.name,
			Granted:   s.current.granted,
			Written:   s.current.written,
			Verified:  s.current.verified,
			Positions: s.current.positions,
		}
		if s.current.secret != nil {
			v.Current.Words = s.current.secret.wordCount()
		}
		if v.Current.Positions == nil {
			v.Current.Positions = []int{}
		}
	}
	for address := range s.drilled {
		v.Drilled = append(v.Drilled, address)
	}
	// A nil slice marshals as null, not [], and every caller would otherwise
	// need the same guard before touching .length. The API says "none" with an
	// empty list rather than with an absence.
	if v.Committed == nil {
		v.Committed = []identity{}
	}
	if v.Attestations == nil {
		v.Attestations = []signedAttestation{}
	}
	if v.Drilled == nil {
		v.Drilled = []string{}
	}
	if v.Notes == nil {
		v.Notes = []string{}
	}
	if v.Params.Custodians == nil {
		v.Params.Custodians = []string{}
	}
	return v
}

func (s *session) handleState(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reply(w, http.StatusOK, s.view())
}

// ---------------------------------------------------------------- pre-flight

type findingView struct {
	What   string `json:"what"`
	Detail string `json:"detail"`
	Strong bool   `json:"strong"`
}

func (s *session) handlePreflight(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.Method == http.MethodGet {
		items := make([]map[string]string, 0, len(checklist))
		for _, item := range checklist {
			items = append(items, map[string]string{"id": item.id, "question": item.question})
		}
		net := make([]findingView, 0, len(s.network))
		for _, f := range s.network {
			net = append(net, findingView{What: f.what, Detail: f.detail})
		}
		vms := make([]findingView, 0, len(s.virtualisation))
		for _, f := range s.virtualisation {
			vms = append(vms, findingView{What: f.What, Detail: f.Detail, Strong: f.Strong})
		}
		reply(w, http.StatusOK, map[string]any{
			"binary_hash":        s.binaryHash,
			"checklist":          items,
			"network":            net,
			"virtualisation":     vms,
			"virtual":            isVirtual(s.virtualisation),
			"manual_preparation": manualPreparation,
			// Named so the page can say what was NOT checked rather than
			// implying the absence of findings is a guarantee.
			"unverifiable": []string{
				"whether a radio will associate in ninety seconds",
				"whether a phone is pointed at the screen",
				"whether this browser honours the request to keep nothing",
				"whether swap or hibernation is genuinely off",
				"whether anyone is recording the session",
				"whether the published hash of this binary is the one you compared against",
			},
		})
		return
	}

	var body struct {
		Answers         []checklistAnswer `json:"answers"`
		Acknowledgement string            `json:"acknowledgement"`
		Rehearsal       bool              `json:"rehearsal"`
	}
	if !posted(w, r, &body) {
		return
	}

	for _, a := range body.Answers {
		if !a.Confirmed {
			fail(w, http.StatusBadRequest, fmt.Errorf(
				"the room has not confirmed: %s. Nothing is generated until every item is true — "+
					"an unconfirmed item is a reason to stop, not a box to leave empty", a.Question))
			return
		}
	}
	if len(body.Answers) != len(checklist) {
		fail(w, http.StatusBadRequest, fmt.Errorf(
			"%d of %d pre-flight items answered", len(body.Answers), len(checklist)))
		return
	}
	if len(s.network) > 0 && strings.TrimSpace(body.Acknowledgement) == "" {
		fail(w, http.StatusBadRequest, errors.New(
			"a network was detected. Say why you are proceeding anyway — it goes on the record verbatim"))
		return
	}
	// A guest can have its memory written to the host's disk by somebody the
	// room cannot see, so keys generated here cannot be treated as production
	// keys. The ceremony is not blocked, because rehearsing the process on a VM
	// is exactly the right thing to do — but it must be named as a rehearsal in
	// the record, and every attestation carries the flag.
	if isVirtual(s.virtualisation) && !body.Rehearsal {
		fail(w, http.StatusBadRequest, errors.New(
			"this machine looks like a virtual guest, so a hypervisor operator could snapshot its memory. "+
				"Keys generated here must not hold value. Confirm this is a rehearsal to continue, "+
				"or move to bare metal"))
		return
	}

	s.answers = body.Answers
	s.acknowledged = strings.TrimSpace(body.Acknowledgement)
	s.rehearsal = body.Rehearsal || isVirtual(s.virtualisation)
	s.preflightDone = true
	reply(w, http.StatusOK, s.view())
}

// ---------------------------------------------------------------- parameters

func (s *session) handleParams(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.Method == http.MethodGet {
		reply(w, http.StatusOK, map[string]any{
			"params":      s.params,
			"set":         s.paramsSet,
			"fingerprint": s.paramsFingerprint(),
		})
		return
	}

	var p ceremonyParams
	if !posted(w, r, &p) {
		return
	}
	if len(s.submissions) > 0 {
		fail(w, http.StatusConflict, errors.New(
			"the parameters cannot change once a submission exists: every fingerprint already "+
				"read aloud covers them, and changing them silently invalidates all of it"))
		return
	}
	if strings.TrimSpace(p.ID) == "" {
		id, err := newCeremonyID()
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		p.ID = id
	}
	if err := p.validate(); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	s.params = p
	s.paramsSet = true
	reply(w, http.StatusOK, map[string]any{
		"params":      s.params,
		"set":         true,
		"fingerprint": s.params.fingerprint(),
	})
}

// ready reports that every name on the roster has a verified submission.
//
// Counted against the roster rather than read out of missingFrom, which returns
// prose in both directions.
func (s *session) ready() bool {
	if !s.paramsSet || len(s.params.Custodians) == 0 {
		return false
	}
	present := map[string]bool{}
	for _, sub := range s.submissions {
		present[strings.ToLower(strings.TrimSpace(sub.Identity.Name))] = true
	}
	for _, name := range s.params.Custodians {
		if !present[strings.ToLower(strings.TrimSpace(name))] {
			return false
		}
	}
	return true
}

func (s *session) paramsFingerprint() string {
	if !s.paramsSet {
		return ""
	}
	return s.params.fingerprint()
}

// ---------------------------------------------------------------- generation

func (s *session) handleBegin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Index uint32 `json:"index"`
	}
	if !posted(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.preflightDone {
		fail(w, http.StatusConflict, errors.New("the pre-flight has not been confirmed"))
		return
	}
	if !s.paramsSet {
		fail(w, http.StatusConflict, errors.New("the ceremony parameters have not been agreed"))
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		fail(w, http.StatusBadRequest, errors.New("the record has to say whose key this is"))
		return
	}
	if !onRoster(s.params.Custodians, name) {
		fail(w, http.StatusBadRequest, fmt.Errorf(
			"%q is not on this ceremony's roster. A name that does not match the agreed list is "+
				"either a typo or a different ceremony", name))
		return
	}
	for _, id := range s.committed {
		if strings.EqualFold(id.Name, name) {
			fail(w, http.StatusConflict, fmt.Errorf(
				"%s already has a key from this ceremony. Generating a second would leave the "+
					"group holding whichever one was written last", name))
			return
		}
	}
	// One at a time. Two phrases on screen in one room is how two custodians
	// write down each other's words.
	if s.current != nil {
		fail(w, http.StatusConflict, fmt.Errorf(
			"%s is still mid-generation. Finish or abandon that key first", s.current.name))
		return
	}

	secret, err := newSecret()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	s.current = &inflight{name: name, index: body.Index, secret: secret}
	reply(w, http.StatusOK, s.view())
}

// handlePhrase serves the phrase, once.
//
// The grant is spent on the first read and never reset, so a reload, a back
// button, a duplicated tab or a curious second look all get 410 rather than the
// words. That is deliberate and it is the difference between a phrase that
// existed on screen for one controlled moment and one that can be summoned back
// at any point in the next hour — which is how a phrase ends up in a screenshot
// nobody meant to keep, or on a screen the room has stopped watching.
//
// A custodian who genuinely did not write it down does not get a second look.
// They abandon the key and generate another, which costs a minute and is the
// only answer that leaves the ceremony honest.
func (s *session) handlePhrase(w http.ResponseWriter, r *http.Request) {
	if !posted(w, r, nil) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current == nil || s.current.secret == nil {
		fail(w, http.StatusConflict, errors.New("no key is being generated"))
		return
	}
	if s.current.granted {
		fail(w, http.StatusGone, errors.New(
			"the phrase has already been shown and cannot be shown again. If it was not written "+
				"down, abandon this key and generate another — that is the only answer that leaves "+
				"the ceremony honest"))
		return
	}
	s.current.granted = true

	count := s.current.secret.wordCount()
	words := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		words = append(words, string(s.current.secret.word(i)))
	}
	reply(w, http.StatusOK, map[string]any{"words": words})
}

func (s *session) handleWritten(w http.ResponseWriter, r *http.Request) {
	if !posted(w, r, nil) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current == nil || s.current.secret == nil {
		fail(w, http.StatusConflict, errors.New("no key is being generated"))
		return
	}
	if !s.current.granted {
		fail(w, http.StatusConflict, errors.New("the phrase has not been shown yet"))
		return
	}
	positions, err := pickPositions(s.current.secret.wordCount(), 4)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	s.current.written = true
	s.current.positions = positions
	reply(w, http.StatusOK, map[string]any{"positions": positions})
}

func (s *session) handleVerify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Answers map[string]string `json:"answers"`
	}
	if !posted(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current == nil || s.current.secret == nil {
		fail(w, http.StatusConflict, errors.New("no key is being generated"))
		return
	}
	if !s.current.written {
		fail(w, http.StatusConflict, errors.New("the transcription check has not been started"))
		return
	}

	// Every position is reported, not just the first that disagreed. A custodian
	// told one word is wrong will fix it and discover the second on the next
	// pass; the whole list is what lets a sheet be corrected in one go.
	var wrong []int
	for _, position := range s.current.positions {
		if !wordMatches(s.current.secret, position, body.Answers[fmt.Sprint(position)]) {
			wrong = append(wrong, position)
		}
	}
	if len(wrong) > 0 {
		reply(w, http.StatusOK, map[string]any{"wrong": wrong, "verified": false})
		return
	}
	s.current.verified = true
	reply(w, http.StatusOK, map[string]any{"wrong": []int{}, "verified": true})
}

func (s *session) handleCommit(w http.ResponseWriter, r *http.Request) {
	if !posted(w, r, nil) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current == nil || s.current.secret == nil {
		fail(w, http.StatusConflict, errors.New("no key is being generated"))
		return
	}
	if !s.current.verified {
		fail(w, http.StatusConflict, errors.New(
			"the transcription has not been verified. A sheet nobody checked is not a backup"))
		return
	}

	f := s.current
	priv, path, err := f.secret.derive(f.index)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	// Derived, used, gone. The private key does not outlive this handler: the
	// attestation later re-derives from the custodian's own paper instead, which
	// is a second check on the sheet rather than a key held in memory for the
	// length of a ceremony.
	defer zero(priv.Key)

	id, err := identityOf(f.name, roleCustodian, priv, path, time.Now())
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	id.Ceremony = s.params.Name

	sub, err := signSubmission(s.params.ID, id, priv)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	recordPath := filepath.Join(s.out, fmt.Sprintf("%s-%s.json", roleCustodian, slug(f.name)))
	if err := writeIdentity(recordPath, id); err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	subPath := filepath.Join(s.out, fmt.Sprintf("submission-%s.json", slug(f.name)))
	if err := writeJSONFile(subPath, sub); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	f.discard()
	s.current = nil
	s.committed = append(s.committed, id)
	s.submissions = append(s.submissions, sub)

	reply(w, http.StatusOK, map[string]any{
		"identity":        id,
		"record_path":     recordPath,
		"submission_path": subPath,
		"state":           s.view(),
	})
}

// handleAbandon destroys an in-flight key.
//
// The exposure path. Somebody photographs the screen, a stranger walks in, a
// custodian says they lost track of word nine — the answer is always this, and
// it is one button rather than a decision, because in the room there will be
// pressure to call it probably fine and nobody wants to be the person who
// restarted a ceremony with five senior people in it.
func (s *session) handleAbandon(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	if !posted(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current == nil {
		fail(w, http.StatusConflict, errors.New("no key is being generated"))
		return
	}
	name := s.current.name
	s.current.discard()
	s.current = nil

	note := fmt.Sprintf("key for %s abandoned and destroyed", name)
	if reason := strings.TrimSpace(body.Reason); reason != "" {
		note += ": " + reason
	}
	// On the record rather than forgotten. An abandoned key is exactly the kind
	// of thing a reader years later needs to see, and a ceremony record with no
	// notes is a claim that nothing happened.
	s.notes = append(s.notes, note)
	reply(w, http.StatusOK, s.view())
}

// ---------------------------------------------------------------- restore drill

func (s *session) handleRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phrase string `json:"phrase"`
		Expect string `json:"expect"`
		Index  uint32 `json:"index"`
	}
	if !posted(w, r, &body) {
		return
	}

	secret, err := secretFromInput(body.Phrase)
	if secret != nil {
		defer secret.zero()
	}
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	priv, path, err := secret.derive(body.Index)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	defer zero(priv.Key)

	id, err := identityOf("restore drill", roleCustodian, priv, path, time.Now())
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	expect := strings.TrimSpace(body.Expect)
	matches := expect == "" || expect == id.Address
	if matches && expect != "" {
		s.mu.Lock()
		s.drilled[id.Address] = true
		s.mu.Unlock()
	}
	reply(w, http.StatusOK, map[string]any{
		"address":     id.Address,
		"fingerprint": id.Fingerprint,
		"matches":     matches,
		// Said here as well as in the runbook, because this is the moment the
		// temptation exists. A sheet edited after the fact is a sheet nobody can
		// trust, and the key it protects is a key nobody has proven.
		"advice": "If either value differs from the custodian's public record, destroy the key and " +
			"the sheet and generate a new one. Do not work out which word is wrong and correct it.",
	})
}

// ---------------------------------------------------------------- distributed

func (s *session) handleSubmission(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.Method == http.MethodGet {
		reply(w, http.StatusOK, map[string]any{
			"submissions": s.submissions,
			"missing":     missingFrom(s.params, s.submissions),
			"expected":    s.params.Custodians,
		})
		return
	}

	var sub submission
	if !posted(w, r, &sub) {
		return
	}
	if !s.paramsSet {
		fail(w, http.StatusConflict, errors.New("the ceremony parameters have not been agreed"))
		return
	}
	// Verified rather than trusted. Without the possession signature this is
	// "here is an address", and anyone can send one of those.
	id, err := verifySubmission(s.params, sub)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	for _, existing := range s.submissions {
		if strings.EqualFold(existing.Identity.Name, id.Name) {
			if existing.Identity.Address == id.Address {
				reply(w, http.StatusOK, s.view())
				return
			}
			fail(w, http.StatusConflict, fmt.Errorf(
				"a different key is already recorded for %s. One of the two is not the key that "+
					"custodian holds, and guessing which would put a stranger in the group", id.Name))
			return
		}
	}
	s.submissions = append(s.submissions, sub)
	// Any change to the set changes the computed bytes, so an assembly made
	// before this one is no longer the thing anybody read aloud.
	s.assembledDoc = nil
	reply(w, http.StatusOK, s.view())
}

// handleAssemble computes the group. It does not decide it.
//
// Every instance runs this over the same submissions and gets the same bytes, so
// the fingerprint five custodians read to each other is a comparison rather than
// an act of trust. Nobody here is assembling on anyone's behalf: a relay who
// substituted a submission changes this fingerprint on all five machines at
// once, which is the property that makes a distributed ceremony safe without a
// trusted coordinator.
func (s *session) handleAssemble(w http.ResponseWriter, r *http.Request) {
	if !posted(w, r, nil) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.paramsSet {
		fail(w, http.StatusConflict, errors.New("the ceremony parameters have not been agreed"))
		return
	}
	if !s.ready() {
		fail(w, http.StatusConflict, errors.New(missingFrom(s.params, s.submissions)))
		return
	}

	a, err := assembleGroup(s.params, s.submissions)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	s.assembledDoc = &a

	path := filepath.Join(s.out, "group.json")
	if err := writeJSONFile(path, a); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	genesisPath := filepath.Join(s.out, "group-genesis.json")
	if err := writeRawFile(genesisPath, a.Genesis); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	var presence string
	if len(s.committed) > 0 {
		own := s.committed[0]
		if err := a.presence(own.Address, own.Fingerprint); err != nil {
			presence = err.Error()
		}
	}
	reply(w, http.StatusOK, map[string]any{
		"assembled":    a,
		"path":         path,
		"genesis_path": genesisPath,
		"own_presence": presence,
		"read_aloud":   a.Fingerprint,
	})
}

func (s *session) handleGenesis(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Genesis json.RawMessage `json:"genesis"`
	}
	if !posted(w, r, &body) {
		return
	}

	s.mu.Lock()
	a := s.assembledDoc
	s.mu.Unlock()

	if a == nil {
		fail(w, http.StatusConflict, errors.New("the group has not been computed on this machine yet"))
		return
	}
	if err := checkGenesis(*a, body.Genesis); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{
		"matches":     true,
		"fingerprint": a.Fingerprint,
	})
}

// ---------------------------------------------------------------- attestation

func (s *session) handleAttest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name           string `json:"name"`
		Phrase         string `json:"phrase"`
		Index          uint32 `json:"index"`
		RestorePassed  bool   `json:"restore_drill_passed"`
		EnvelopeSealed bool   `json:"envelope_sealed"`
	}
	if !posted(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	a := s.assembledDoc
	if a == nil {
		fail(w, http.StatusConflict, errors.New(
			"nothing to attest to yet: this machine has not computed the group"))
		return
	}

	var own identity
	found := false
	for _, id := range s.committed {
		if strings.EqualFold(id.Name, strings.TrimSpace(body.Name)) {
			own, found = id, true
			break
		}
	}
	if !found {
		fail(w, http.StatusBadRequest, fmt.Errorf("no key from this ceremony belongs to %q", body.Name))
		return
	}
	// The custodian must be in the group they are about to sign for. This is the
	// check that makes a relay unable to substitute: an attestation is refused
	// outright when the signer's own address is not in the computed group.
	if err := a.presence(own.Address, own.Fingerprint); err != nil {
		fail(w, http.StatusConflict, err)
		return
	}

	// Re-derived from the sheet rather than from a key held since commit. The
	// phrase the custodian types here is the one they are about to seal in an
	// envelope, which makes this a check on the paper as it now stands — after
	// it has been written, folded and handled.
	secret, err := secretFromInput(body.Phrase)
	if secret != nil {
		defer secret.zero()
	}
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	priv, _, err := secret.derive(body.Index)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	defer zero(priv.Key)

	derived, err := identityOf(own.Name, roleCustodian, priv, own.HDPath, time.Now())
	if err != nil || derived.Address != own.Address {
		fail(w, http.StatusBadRequest, fmt.Errorf(
			"that phrase does not produce %s's key. The sheet and the record disagree, which means "+
				"the sheet is wrong: destroy this key and this sheet and generate a new one", own.Name))
		return
	}

	signed, err := signAttestation(attestation{
		CeremonyID:            s.params.ID,
		Name:                  own.Name,
		Address:               own.Address,
		GroupFingerprint:      a.Fingerprint,
		PolicyAddress:         a.PolicyAddress,
		TranscriptionVerified: true,
		RestoreDrillPassed:    body.RestorePassed || s.drilled[own.Address],
		EnvelopeSealed:        body.EnvelopeSealed,
		Virtualised:           s.rehearsal,
		SignedAt:              time.Now().UTC().Format(time.RFC3339),
	}, priv)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	s.attestations = append(s.attestations, signed)
	path := filepath.Join(s.out, fmt.Sprintf("attestation-%s.json", slug(own.Name)))
	if err := writeJSONFile(path, signed); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"attestation": signed, "path": path})
}

func (s *session) handleAttestation(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.Method == http.MethodGet {
		reply(w, http.StatusOK, map[string]any{"attestations": s.attestations})
		return
	}

	var signed signedAttestation
	if !posted(w, r, &signed) {
		return
	}
	if err := verifyAttestation(signed); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if s.assembledDoc != nil && signed.Attestation.GroupFingerprint != s.assembledDoc.Fingerprint {
		fail(w, http.StatusConflict, fmt.Errorf(
			"that attestation is for a different group: it names %s and this machine computed %s. "+
				"Two custodians looking at different groups is the failure the read-aloud step exists "+
				"to catch — stop and find out why",
			signed.Attestation.GroupFingerprint, s.assembledDoc.Fingerprint))
		return
	}
	for _, existing := range s.attestations {
		if existing.Attestation.Address == signed.Attestation.Address {
			reply(w, http.StatusOK, s.view())
			return
		}
	}
	s.attestations = append(s.attestations, signed)
	reply(w, http.StatusOK, s.view())
}

// ---------------------------------------------------------------- record

func (s *session) handleRecord(w http.ResponseWriter, r *http.Request) {
	var config recordConfig
	if !posted(w, r, &config) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if config.Ceremony == "" {
		config.Ceremony = s.params.Name
	}
	if config.ChainID == "" {
		config.ChainID = s.params.ChainID
	}
	if config.Threshold == 0 {
		config.Threshold = s.params.Threshold
	}
	if config.StartedAt == "" {
		config.StartedAt = s.startedAt.UTC().Format(time.RFC3339)
	}
	if config.CompletedAt == "" {
		config.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if config.BinaryHash == "" {
		config.BinaryHash = s.binaryHash
	}
	if s.assembledDoc != nil && config.PolicyAddress == "" {
		config.PolicyAddress = s.assembledDoc.PolicyAddress
	}
	config.Notes = append(config.Notes, s.notes...)
	if s.rehearsal {
		config.Notes = append(config.Notes,
			"REHEARSAL: this machine was detected as a virtual guest, so the keys generated here "+
				"must not be treated as production keys")
	}
	if s.acknowledged != "" {
		config.Notes = append(config.Notes, "network detected; proceeded because: "+s.acknowledged)
	}

	custodians := make([]identity, 0, len(s.submissions))
	for _, sub := range s.submissions {
		custodians = append(custodians, sub.Identity)
	}
	rendered, err := renderRecord(config, custodians)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	path := filepath.Join(s.out, "ceremony-record.md")
	if err := writeTextFile(path, rendered); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"record": rendered, "path": path})
}

func (s *session) handleComplete(w http.ResponseWriter, r *http.Request) {
	if !posted(w, r, nil) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Anything still in flight when the room says it is finished is a key nobody
	// wrote down, so it is destroyed rather than left for whoever uses the
	// machine next.
	if s.current != nil {
		s.notes = append(s.notes,
			fmt.Sprintf("key for %s destroyed unfinished when the ceremony closed", s.current.name))
		s.current.discard()
		s.current = nil
	}
	s.complete = true
	// Once. A page that posted this twice would otherwise panic the process on a
	// closed channel, and the last screen of a ceremony is exactly where a double
	// submit happens.
	s.finishing.Do(func() { close(s.finished) })

	// Asks the browser to drop everything for this origin. The profile is about
	// to be deleted anyway; this covers the --no-browser case where it is not.
	w.Header().Set("Clear-Site-Data", `"cache", "cookies", "storage"`)
	reply(w, http.StatusOK, map[string]any{
		"complete": true,
		"next": []string{
			"Close the browser window. The temporary profile is deleted when this program exits.",
			"Power the machine off and wipe it before it leaves the room.",
			"Each custodian leaves with their own sealed envelope and nothing else.",
		},
	})
}

// ---------------------------------------------------------------- files

// Everything written here is public: addresses, public keys, fingerprints,
// signed statements and the group document. No phrase and no private key is ever
// written by this program. Mode 0600 anyway, because the machine is wiped
// afterwards and a permissive file on a machine in a locked room is still a
// habit worth not having.
//
// These overwrite, unlike writeIdentity, which refuses. An identity is a
// once-only fact about a custodian and a silent overwrite would leave a group
// with a member who believes they are in it. The group document and the
// attestations are recomputed deterministically from the same inputs, so
// rewriting one is replacing a file with itself.

func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeRawFile(path, append(raw, 0x0a))
}

func writeRawFile(path string, raw []byte) error {
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("could not write %s: %w", path, err)
	}
	return nil
}

func writeTextFile(path, text string) error {
	return writeRawFile(path, []byte(text))
}
