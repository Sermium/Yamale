package main

// The state a hosted ceremony holds, and the handlers over it.
//
// Read the list of what is in here and what is not, because the second list is
// the design:
//
//	Held: the roster, the invite tokens, the public half of each custodian's key,
//	      the possession signatures, the attestations, and who has reached which
//	      step.
//	Not held, ever, in any field, on any code path: a recovery phrase, a seed, a
//	      private key, or anything derived from one. There is no field for it and
//	      no request body that could carry it — see host.go and
//	      TestNoHostRouteAcceptsAPhrase.
//
// The coordinator relays. It does not assemble. assembleGroup runs in every
// custodian's browser, and the fingerprint each of them reads aloud is the one
// their own device computed from the submissions it was given. This process
// computes the group too — but only so the coordinator's screen can show the
// same value and the record can be exported; a custodian who took the
// coordinator's word for the fingerprint would be trusting the one party the
// 3-of-5 exists to distrust, which is why the page never displays a fingerprint
// it did not compute itself.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// invitePhase is how far one custodian has got.
//
// The first two are self-reported by the page and the last two are proved by a
// signature, and the coordinator's screen says which is which. A status board
// that presented "generated" with the same confidence as "submitted" would be
// claiming knowledge this process does not have: nothing here can see into a
// browser, and a page that never called back looks identical to a custodian who
// closed the tab.
type invitePhase string

const (
	phaseInvited   invitePhase = "invited"
	phaseOpened    invitePhase = "opened"
	phaseGenerated invitePhase = "generated"
	phaseSubmitted invitePhase = "submitted"
	phaseAttested  invitePhase = "attested"
)

// invite is one single-use link.
//
// Single use means two things precisely, and neither of them is "the URL stops
// working", because a page that died on a reload would cost a custodian their
// key for no security gain:
//
//   - It names one custodian. The name on a submission comes from this struct,
//     never from the request body, so a link that leaked into a group chat
//     cannot put a stranger's address into the group under a custodian's name.
//   - The generation grant is spent once. After the phrase has been shown, this
//     invite will not authorise another one. A custodian who lost their words
//     needs the coordinator to reissue, which revokes this token and abandons
//     that key — stated plainly on the coordinator's screen, because it costs a
//     key and there is no version of this where it does not.
type invite struct {
	Token       string    `json:"-"`
	Name        string    `json:"name"`
	Issue       int       `json:"issue"`
	IssuedAt    time.Time `json:"issued_at"`
	OpenedAt    time.Time `json:"-"`
	GeneratedAt time.Time `json:"-"`
	Revoked     bool      `json:"-"`
	Reason      string    `json:"-"`
}

type hostSession struct {
	mu sync.Mutex

	out       string
	public    *url.URL
	bundle    *bundleInfo
	startedAt time.Time

	params    ceremonyParams
	paramsSet bool

	invites map[string]*invite
	live    map[string]*invite

	submissions  map[string]submission
	attestations map[string]signedAttestation

	notes    []string
	complete bool
}

func newHostSession(out string, public *url.URL, bundle *bundleInfo) *hostSession {
	return &hostSession{
		out:          out,
		public:       public,
		bundle:       bundle,
		startedAt:    time.Now(),
		invites:      map[string]*invite{},
		live:         map[string]*invite{},
		submissions:  map[string]submission{},
		attestations: map[string]signedAttestation{},
	}
}

func (h *hostSession) routes() []hostRoute {
	return []hostRoute{
		{Path: "api/bundle", Audience: audiencePublic, Handle: h.handleBundle},
		{Path: "api/coordinator/state", Audience: audienceCoordinator, Handle: h.handleCoordinatorState},
		{Path: "api/coordinator/setup", Audience: audienceCoordinator, AcceptsBody: true, Handle: h.handleSetup},
		{Path: "api/coordinator/reissue", Audience: audienceCoordinator, AcceptsBody: true, Handle: h.handleReissue},
		{Path: "api/coordinator/export", Audience: audienceCoordinator, AcceptsBody: true, Handle: h.handleExport},
		{Path: "api/invite", Audience: audienceCustodian, Handle: h.handleInvite},
		{Path: "api/invite/opened", Audience: audienceCustodian, AcceptsBody: true, Handle: h.handleOpened},
		{Path: "api/invite/generated", Audience: audienceCustodian, AcceptsBody: true, Handle: h.handleGenerated},
		{Path: "api/invite/submission", Audience: audienceCustodian, AcceptsBody: true, Handle: h.handleHostSubmission},
		{Path: "api/invite/attestation", Audience: audienceCustodian, AcceptsBody: true, Handle: h.handleHostAttestation},
	}
}

// ---------------------------------------------------------------- the pages

// servePage serves the one bundle both flows run.
//
// The Content-Security-Policy is stricter than the one `serve` uses, and it can
// be: this page's script is a separate file, so script-src is 'self' with no
// 'unsafe-inline'. That matters more here than it does on an air-gapped machine.
// The air-gapped page cannot reach a network at all; this one can, so the policy
// that keeps a phrase from leaving has to be enforced by the browser rather than
// by the absence of a route to anywhere.
//
// connect-src 'self' is the load-bearing line. Even a page altered after the
// hash was published could not POST a phrase to another origin without the
// browser refusing it.
func (h *hostSession) servePage(w http.ResponseWriter) {
	page, ok := h.bundle.Files["index.html"]
	if !ok {
		http.Error(w, "the interface is missing from this binary", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; img-src data:; "+
			"connect-src 'self'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'")
	_, _ = w.Write(page)
}

func (h *hostSession) serveAsset(name string) http.HandlerFunc {
	data := h.bundle.Files[name]
	contentType := "application/octet-stream"
	switch {
	case strings.HasSuffix(name, ".js"):
		contentType = "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		contentType = "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		contentType = "text/html; charset=utf-8"
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// The bundle is published and its digest is published, so it is the one
		// thing here that may be cached. Short, so a rebuild during a rehearsal
		// does not leave one custodian on yesterday's code with today's hash on
		// screen.
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
	}
}

// ---------------------------------------------------------------- credentials

type inviteContextKey struct{}

func withInvite(ctx context.Context, i *invite) context.Context {
	return context.WithValue(ctx, inviteContextKey{}, i)
}

func inviteFromContext(ctx context.Context) *invite {
	i, _ := ctx.Value(inviteContextKey{}).(*invite)
	return i
}

// inviteFor resolves a presented token.
//
// The error text is the same for an unknown token and a revoked one only in that
// both refuse; they say different things, because a custodian whose link was
// reissued needs to know that rather than to conclude they mistyped it.
func (h *hostSession) inviteFor(token string) (*invite, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if token == "" {
		return nil, errors.New("this link is missing its invitation code. Use the link the coordinator sent you, in full")
	}
	i, ok := h.invites[token]
	if !ok {
		return nil, errors.New(
			"this invitation is not one this ceremony issued. Links are one per custodian and they are long: " +
				"the likeliest cause is a link that was cut short when it was copied")
	}
	if i.Revoked {
		return nil, fmt.Errorf(
			"this invitation has been withdrawn and replaced%s. Ask the coordinator for the new link — "+
				"and if you had already written words down against this one, destroy that sheet: "+
				"the key it holds is not going into the group", reasonSuffix(i.Reason))
	}
	return i, nil
}

// stillLive re-checks the invite under the lock the mutation is about to happen
// under.
//
// hostGuard checks Revoked while resolving the token and then releases the lock,
// so a reissue landing in that window would let one already-authorised request
// through — and the request it would let through is a submission of the very key
// the coordinator just abandoned. Narrow, and the wrong way round to leave open:
// the whole point of reissuing is that the old key is not going into the group.
func (h *hostSession) stillLive(w http.ResponseWriter, i *invite) bool {
	if !i.Revoked {
		return true
	}
	fail(w, http.StatusForbidden, errors.New(
		"this invitation was withdrawn while this request was on its way. Whatever was generated against it is "+
			"not going into the group — destroy that sheet and use the new link"))
	return false
}

func reasonSuffix(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return ""
	}
	return ": " + reason
}

func newInviteToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ---------------------------------------------------------------- views

type custodianStatus struct {
	Name  string      `json:"name"`
	Phase invitePhase `json:"phase"`
	// Link is only ever sent to the coordinator. It is the credential.
	Link        string `json:"link,omitempty"`
	Issue       int    `json:"issue"`
	Address     string `json:"address,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	// Proved says whether this row rests on a signature or on the page's own
	// word for it. Shown, because "generated" is a self-report and a status
	// board that hid the difference would be overstating what it knows.
	Proved       bool   `json:"proved"`
	WaitingSince string `json:"waiting_since,omitempty"`
}

type hostStateView struct {
	Ceremony     string              `json:"ceremony"`
	StartedAt    string              `json:"started_at"`
	ParamsSet    bool                `json:"params_set"`
	Params       ceremonyParams      `json:"params"`
	Fingerprint  string              `json:"params_fingerprint"`
	Custodians   []custodianStatus   `json:"custodians"`
	Missing      string              `json:"missing"`
	Ready        bool                `json:"ready"`
	Submissions  []submission        `json:"submissions"`
	Attestations []signedAttestation `json:"attestations"`
	Assembled    *assembled          `json:"assembled"`
	BundleHash   string              `json:"bundle_hash"`
	Complete     bool                `json:"complete"`
	Notes        []string            `json:"notes"`
}

func (h *hostSession) view(includeLinks bool) hostStateView {
	v := hostStateView{
		Ceremony:     h.params.Name,
		StartedAt:    h.startedAt.UTC().Format(time.RFC3339),
		ParamsSet:    h.paramsSet,
		Params:       h.params,
		BundleHash:   h.bundle.Hash,
		Complete:     h.complete,
		Notes:        h.notes,
		Submissions:  []submission{},
		Attestations: []signedAttestation{},
		Custodians:   []custodianStatus{},
	}
	if v.Params.Custodians == nil {
		v.Params.Custodians = []string{}
	}
	if v.Notes == nil {
		v.Notes = []string{}
	}
	if !h.paramsSet {
		return v
	}
	v.Fingerprint = h.params.fingerprint()

	for _, name := range h.params.Custodians {
		status := custodianStatus{Name: name, Phase: phaseInvited}
		if i := h.live[name]; i != nil {
			status.Issue = i.Issue
			status.WaitingSince = i.IssuedAt.UTC().Format(time.RFC3339)
			if includeLinks {
				status.Link = h.inviteLink(i)
			}
			if !i.OpenedAt.IsZero() {
				status.Phase = phaseOpened
				status.WaitingSince = i.OpenedAt.UTC().Format(time.RFC3339)
			}
			if !i.GeneratedAt.IsZero() {
				status.Phase = phaseGenerated
				status.WaitingSince = i.GeneratedAt.UTC().Format(time.RFC3339)
			}
		}
		if sub, ok := h.submissions[name]; ok {
			status.Phase = phaseSubmitted
			status.Address = sub.Identity.Address
			status.Fingerprint = sub.Identity.Fingerprint
			status.Proved = true
			if _, attested := h.attestations[sub.Identity.Address]; attested {
				status.Phase = phaseAttested
			}
		}
		v.Custodians = append(v.Custodians, status)
	}

	// Ordered by the roster, so the coordinator's screen does not reshuffle
	// itself every time somebody submits — a board that reorders is a board
	// nobody can scan for the name they are waiting on.
	subs := make([]submission, 0, len(h.submissions))
	for _, name := range h.params.Custodians {
		if sub, ok := h.submissions[name]; ok {
			subs = append(subs, sub)
		}
	}
	v.Submissions = subs
	v.Missing = missingFrom(h.params, subs)
	v.Ready = len(subs) == len(h.params.Custodians)

	addresses := make([]string, 0, len(h.attestations))
	for address := range h.attestations {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	for _, address := range addresses {
		v.Attestations = append(v.Attestations, h.attestations[address])
	}

	if v.Ready {
		if a, err := assembleGroup(h.params, subs); err == nil {
			v.Assembled = &a
		}
	}
	return v
}

func (h *hostSession) inviteLink(i *invite) string {
	link := *h.public
	query := link.Query()
	query.Set("i", i.Token)
	link.RawQuery = query.Encode()
	return link.String()
}

// ---------------------------------------------------------------- coordinator

func (h *hostSession) handleBundle(w http.ResponseWriter, _ *http.Request) {
	reply(w, http.StatusOK, map[string]any{
		"hash":  h.bundle.Hash,
		"files": h.bundle.PerFile,
	})
}

func (h *hostSession) handleCoordinatorState(w http.ResponseWriter, _ *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	reply(w, http.StatusOK, h.view(true))
}

// setupRequest is the coordinator's form.
//
// Country and Roles are what make this the same flow for a country office as for
// the foundation. Both blank is the foundation ceremony; a country turns it into
// an enrolment for one national office, and both values end up inside the
// parameters fingerprint the super users read aloud before generating — which is
// the whole reason they are here rather than in a config file the coordinator
// fills in afterwards.
type setupRequest struct {
	Ceremony     string   `json:"ceremony"`
	ChainID      string   `json:"chain_id"`
	Threshold    int      `json:"threshold"`
	Custodians   []string `json:"custodians"`
	PolicySeq    uint64   `json:"policy_seq"`
	VotingPeriod string   `json:"voting_period"`
	Country      string   `json:"country"`
	Roles        []string `json:"roles"`
	// Administrators asks for a foundation-administrator group rather than the
	// foundation itself or a country office. A separate flag rather than a
	// sentinel country, because the two are opposites and validate() refuses them
	// together: an administrator's identifier carries the code that marks the
	// absence of a perimeter, and an office holds authority inside one.
	Administrators bool `json:"foundation_administrators"`
}

func (h *hostSession) handleSetup(w http.ResponseWriter, r *http.Request) {
	var body setupRequest
	if !postedStrict(w, r, &body) {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Refused rather than merged. Every fingerprint already read aloud covers
	// the parameters, so changing them once a submission exists silently
	// invalidates all of it — and the custodian who had already generated would
	// be holding a key for a ceremony that no longer exists.
	if len(h.submissions) > 0 {
		fail(w, http.StatusConflict, errors.New(
			"the ceremony cannot be set up again once a custodian has submitted: the parameters are covered by "+
				"every fingerprint already read aloud. Start a new ceremony instead"))
		return
	}
	// Refused once anybody has been shown their words, which is earlier than the
	// first submission and is the point at which the cost becomes real. Setting
	// the ceremony up again mints a new ceremony id, so that custodian would be
	// holding a key — on paper, in ink — belonging to a ceremony that no longer
	// exists, and the first they would know of it is their submission being
	// refused for the wrong id.
	for _, live := range h.live {
		if !live.GeneratedAt.IsZero() {
			fail(w, http.StatusConflict, fmt.Errorf(
				"%s has already been shown twenty-four words for this ceremony, so it cannot be set up again: "+
					"that key would belong to a ceremony that no longer exists, with the words already on paper. "+
					"Reissue that one invitation instead, or start a new ceremony", live.Name))
			return
		}
	}

	roster := make([]string, 0, len(body.Custodians))
	for _, name := range body.Custodians {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			roster = append(roster, trimmed)
		}
	}

	// A blank country is the foundation ceremony and Office stays nil. Trimmed and
	// uppercased on the way in because a form field is typed by a person, and then
	// params.validate() refuses anything the normalisation would have had to
	// change beyond that — so "sn " becomes "SN" and is accepted, while a code
	// that is not a country is refused rather than recorded.
	var office *officeParams
	if country := strings.ToUpper(strings.TrimSpace(body.Country)); country != "" {
		roles := make([]string, 0, len(body.Roles))
		for _, role := range body.Roles {
			if trimmed := strings.ToUpper(strings.TrimSpace(role)); trimmed != "" {
				roles = append(roles, trimmed)
			}
		}
		office = &officeParams{Country: country, Roles: roles}
	}

	id, err := newCeremonyID()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	params := ceremonyParams{
		ID:           id,
		Name:         strings.TrimSpace(body.Ceremony),
		ChainID:      strings.TrimSpace(body.ChainID),
		Threshold:    body.Threshold,
		Custodians:   roster,
		PolicySeq:    body.PolicySeq,
		VotingPeriod: strings.TrimSpace(body.VotingPeriod),
		Office:       office,
		// Passed through rather than reconciled with the country here. If the form
		// somehow sends both, validate() refuses the pair with a sentence naming
		// the contradiction — which is better than this handler silently deciding
		// which one the coordinator meant.
		Administrators: body.Administrators,
	}
	if err := params.validate(); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	h.params = params
	h.paramsSet = true
	h.invites = map[string]*invite{}
	h.live = map[string]*invite{}
	for _, name := range roster {
		if err := h.issueLocked(name, ""); err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
	}
	reply(w, http.StatusOK, h.view(true))
}

// issueLocked mints a fresh invite and retires any previous one for that name.
func (h *hostSession) issueLocked(name, reason string) error {
	token, err := newInviteToken()
	if err != nil {
		return err
	}
	issue := 1
	if previous := h.live[name]; previous != nil {
		previous.Revoked = true
		previous.Reason = reason
		issue = previous.Issue + 1
	}
	i := &invite{Token: token, Name: name, Issue: issue, IssuedAt: time.Now()}
	h.invites[token] = i
	h.live[name] = i
	return nil
}

type reissueRequest struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// handleReissue is the answer to a custodian who closed the tab.
//
// Before the phrase was shown it costs nothing: no key existed. After it was
// shown it costs that key, and there is no honest alternative. The phrase was
// never transmitted, so nothing here can show it again, and a ceremony that
// pretended otherwise would be one where a custodian holds a sheet that recovers
// nothing. So this revokes the old link, abandons whatever key was generated
// against it, and issues a new one — and the coordinator's screen says exactly
// that before the button is pressed.
func (h *hostSession) handleReissue(w http.ResponseWriter, r *http.Request) {
	var body reissueRequest
	if !postedStrict(w, r, &body) {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	name := strings.TrimSpace(body.Name)
	if !onRoster(h.params.Custodians, name) {
		fail(w, http.StatusBadRequest, fmt.Errorf("%q is not on this ceremony's roster", name))
		return
	}
	if _, submitted := h.submissions[name]; submitted {
		fail(w, http.StatusConflict, fmt.Errorf(
			"%s has already submitted a public key, and it is in the group everybody is computing. "+
				"Reissuing now would leave the ceremony with two keys for one custodian and no way to say "+
				"which one they hold. If that key really has to be replaced, start the ceremony again", name))
		return
	}

	previous := h.live[name]
	abandoned := previous != nil && !previous.GeneratedAt.IsZero()
	if err := h.issueLocked(name, strings.TrimSpace(body.Reason)); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	if abandoned {
		// Written into the record rather than only shown on screen. A record
		// that did not mention an abandoned key would be a record claiming five
		// keys were generated once each.
		h.notes = append(h.notes, fmt.Sprintf(
			"%s's invitation was reissued after a phrase had already been shown; that key was abandoned and the "+
				"sheet destroyed (issue %d)", name, h.live[name].Issue))
	}
	reply(w, http.StatusOK, h.view(true))
}

type exportRequest struct {
	Location     string        `json:"location"`
	Participants []participant `json:"participants"`
	Notes        []string      `json:"notes"`
}

// exportedGroup is group.json: the assembled document plus the submissions it was
// computed from.
//
// The submissions are here so that a consumer can RECOMPUTE rather than read.
// This file travels — the country enrolment ceremony picks it up on another
// machine to find out which addresses make up an office — and the member set in
// it is the single field that decides who holds a country's payments and
// enforcement authority. A consumer that trusted the custodians field would put
// an added member into a real office's group, and every check after that point
// would agree with it because they all read the same field. With the submissions
// present, an edited file disagrees with itself.
//
// Additive: every existing key keeps its name and its place, because
// scripts/devnet/init-devnet.sh reads policy_address, group_genesis and
// constitution_invariants out of this file by name with a tolerant JSON decoder.
// Nothing here reaches assembled.canonical(), so no fingerprint moves.
type exportedGroup struct {
	assembled
	Submissions []submission `json:"submissions"`
	// PolicyNote is present only for a country office, where policy_address is a
	// prediction rather than a fact. A person reads this file.
	PolicyNote string `json:"policy_address_note,omitempty"`
}

// policyAddressNote qualifies policy_address for a country office.
//
// The address is derived from the policy sequence number and nothing else, so for
// the foundation — whose group is seeded at genesis, with the sequence fixed by
// the same file — it is a fact. For an office, whose group is created by a
// transaction on a chain that has been running for months, it is a guess about
// how many group policies that chain has created. The chain decides, and
// `ceremony country confirm` reads the real address back. Said in the file
// because a person reads the file.
func policyAddressNote(params ceremonyParams) string {
	if !params.onChain() {
		return ""
	}
	if params.Administrators {
		// Worth its own sentence rather than sharing the office's. On a live run of
		// the country ceremony a predicted address came out as the FOUNDATION'S
		// OWN, because both were policy sequence 1 — and an appointment proposal
		// naming that address would have appointed the foundation, passed, and read
		// as correct.
		return fmt.Sprintf(
			"predicted from policy_seq %d and almost certainly WRONG; an administrator group is created by a "+
				"transaction on a running chain, so the chain decides the sequence. A prediction here has come "+
				"out as the foundation's own address before now, because both were sequence 1. This is not the "+
				"group's address until `ceremony administrators confirm` has read it back off the chain",
			params.PolicySeq)
	}
	return fmt.Sprintf(
		"predicted from policy_seq %d; for a country office the chain decides the sequence, so this is not the "+
			"office's address until `ceremony country confirm` has read it back", params.PolicySeq)
}

// handleExport renders the record and writes the launch material.
//
// A file, not a step. The custodian journey never touches one: everything a
// custodian needs moves over the connection they already have. This exists so
// the coordinator can keep the record and the genesis fragment afterwards, and
// the same content is on the screen whether or not anybody presses it.
func (h *hostSession) handleExport(w http.ResponseWriter, r *http.Request) {
	var body exportRequest
	if !postedStrict(w, r, &body) {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.paramsSet {
		fail(w, http.StatusConflict, errors.New("the ceremony has not been set up"))
		return
	}
	subs := make([]submission, 0, len(h.submissions))
	custodians := make([]identity, 0, len(h.submissions))
	for _, name := range h.params.Custodians {
		sub, ok := h.submissions[name]
		if !ok {
			continue
		}
		subs = append(subs, sub)
		custodians = append(custodians, sub.Identity)
	}

	notes := append([]string(nil), h.notes...)
	notes = append(notes, body.Notes...)
	notes = append(notes,
		"Generated over the hosted path: each custodian's phrase was created in their own browser and never "+
			"transmitted. The coordinator held public keys and signatures only. The air-gapped binary remains "+
			"the stronger option; this was the networked one.",
		"Client bundle SHA-256: "+h.bundle.Hash)

	config := recordConfig{
		Ceremony:     h.params.Name,
		ChainID:      h.params.ChainID,
		Location:     strings.TrimSpace(body.Location),
		StartedAt:    h.startedAt.UTC().Format(time.RFC3339),
		CompletedAt:  time.Now().UTC().Format(time.RFC3339),
		Participants: body.Participants,
		Threshold:    h.params.Threshold,
		Notes:        notes,
		// Carried so the record describes the thing that was actually made. For an
		// office this suppresses the foundation paragraph, every sentence of which
		// would be false about it.
		Office:         h.params.Office,
		Administrators: h.params.Administrators,
	}

	files := map[string]string{}
	if len(subs) == len(h.params.Custodians) && len(subs) > 0 {
		a, err := assembleGroup(h.params, subs)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		// Written to the record either way, because renderRecord requires it and a
		// foundation record is meaningless without it. For a country office it is
		// a PREDICTION and it is labelled as one: the address is derived from the
		// policy sequence number alone, and an office's group is created by a
		// transaction on a running chain, so the chain decides the sequence and
		// `ceremony country confirm` reads the real address back. Labelled rather
		// than omitted, because a record with a blank where an address belongs
		// reads as a value nobody bothered to fill in.
		config.PolicyAddress = a.PolicyAddress
		if note := policyAddressNote(h.params); note != "" {
			config.PolicyAddress += "\n" + note
		}
		groupPath := filepath.Join(h.out, "group.json")
		if err := writeJSONFile(groupPath, exportedGroup{
			assembled:   a,
			Submissions: subs,
			PolicyNote:  policyAddressNote(h.params),
		}); err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		files["group"] = groupPath

		// Neither of these is written for a country office, and both omissions are
		// the point.
		//
		// group-genesis.json names group id 1 and policy sequence 1. That is
		// correct for the foundation, whose group IS the first one on the chain and
		// is seeded at height zero. An office's group is the Nth, created by
		// transaction on a chain that has been running for months — so a fragment
		// claiming id 1 is a file somebody in a hurry could splice into a launch
		// and get a genesis that starts and is wrong.
		//
		// constitution-invariants.json is worse: it says "send every seized asset
		// on this chain to this address", with the address being the office's.
		// Nobody should have to know not to use a file sitting in the output
		// directory.
		if !h.params.onChain() {
			genesisPath := filepath.Join(h.out, "group-genesis.json")
			if err := writeRawFile(genesisPath, a.Genesis); err != nil {
				fail(w, http.StatusInternalServerError, err)
				return
			}
			constitutionPath := filepath.Join(h.out, "constitution-invariants.json")
			if err := writeRawFile(constitutionPath, a.Constitution); err != nil {
				fail(w, http.StatusInternalServerError, err)
				return
			}
			files["group_genesis"] = genesisPath
			files["constitution"] = constitutionPath
		}
	}

	rendered, err := renderRecord(config, custodians)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	recordPath := filepath.Join(h.out, "ceremony-record.md")
	if err := writeTextFile(recordPath, rendered); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	files["record"] = recordPath

	for _, signed := range h.attestations {
		path := filepath.Join(h.out, fmt.Sprintf("attestation-%s.json", slug(signed.Attestation.Name)))
		if err := writeJSONFile(path, signed); err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
	}

	// Complete means every custodian attested, not that somebody pressed export.
	// A record rendered from a partial roster is a legitimate thing to want
	// mid-ceremony; a board claiming the ceremony finished because of it would be
	// the interface lying about the only fact it exists to report.
	h.complete = len(h.params.Custodians) > 0 && len(h.attestations) == len(h.params.Custodians)
	reply(w, http.StatusOK, map[string]any{"record": rendered, "files": files, "complete": h.complete})
}

// ---------------------------------------------------------------- custodian

type inviteView struct {
	Name  string      `json:"name"`
	Phase invitePhase `json:"phase"`
	Issue int         `json:"issue"`
	// Generated is the single-use grant. Once it is true this link will not
	// authorise another phrase, and the page says so rather than quietly
	// offering the button again.
	Generated   bool           `json:"generated"`
	ParamsSet   bool           `json:"params_set"`
	Params      ceremonyParams `json:"params"`
	Fingerprint string         `json:"params_fingerprint"`
	// Submissions is the whole relayed set, so the page can compute the group
	// itself. It is public material: five public keys and five signatures.
	Submissions []submission `json:"submissions"`
	// Waiting names who has not submitted. Named rather than counted, because a
	// relay who withheld one submission would be indistinguishable from a slow
	// connection behind a spinner.
	Waiting      []string            `json:"waiting"`
	Attested     []string            `json:"attested"`
	OwnAddress   string              `json:"own_address"`
	Attestations []signedAttestation `json:"attestations"`
	BundleHash   string              `json:"bundle_hash"`
	Threshold    int                 `json:"threshold"`
	Complete     bool                `json:"complete"`
}

func (h *hostSession) inviteView(i *invite) inviteView {
	v := inviteView{
		Name:         i.Name,
		Phase:        phaseInvited,
		Issue:        i.Issue,
		Generated:    !i.GeneratedAt.IsZero(),
		ParamsSet:    h.paramsSet,
		Params:       h.params,
		BundleHash:   h.bundle.Hash,
		Threshold:    h.params.Threshold,
		Complete:     h.complete,
		Submissions:  []submission{},
		Waiting:      []string{},
		Attested:     []string{},
		Attestations: []signedAttestation{},
	}
	if v.Params.Custodians == nil {
		v.Params.Custodians = []string{}
	}
	if h.paramsSet {
		v.Fingerprint = h.params.fingerprint()
	}
	if !i.OpenedAt.IsZero() {
		v.Phase = phaseOpened
	}
	if v.Generated {
		v.Phase = phaseGenerated
	}

	for _, name := range h.params.Custodians {
		sub, ok := h.submissions[name]
		if !ok {
			v.Waiting = append(v.Waiting, name)
			continue
		}
		v.Submissions = append(v.Submissions, sub)
		if _, attested := h.attestations[sub.Identity.Address]; attested {
			v.Attested = append(v.Attested, name)
		}
	}
	if own, ok := h.submissions[i.Name]; ok {
		v.OwnAddress = own.Identity.Address
		v.Phase = phaseSubmitted
		if _, attested := h.attestations[own.Identity.Address]; attested {
			v.Phase = phaseAttested
		}
	}
	for _, signed := range h.attestations {
		v.Attestations = append(v.Attestations, signed)
	}
	sort.Slice(v.Attestations, func(a, b int) bool {
		return v.Attestations[a].Attestation.Address < v.Attestations[b].Attestation.Address
	})
	return v
}

func (h *hostSession) handleInvite(w http.ResponseWriter, r *http.Request) {
	i := inviteFromContext(r.Context())
	h.mu.Lock()
	defer h.mu.Unlock()
	reply(w, http.StatusOK, h.inviteView(i))
}

func (h *hostSession) handleOpened(w http.ResponseWriter, r *http.Request) {
	if !postedStrict(w, r, nil) {
		return
	}
	i := inviteFromContext(r.Context())

	h.mu.Lock()
	defer h.mu.Unlock()
	if i.OpenedAt.IsZero() {
		i.OpenedAt = time.Now()
	}
	reply(w, http.StatusOK, h.inviteView(i))
}

// handleGenerated spends the generation grant.
//
// The phrase itself is generated in the browser and this process never sees it,
// so this cannot serve the words once and refuse the second time the way the
// air-gapped page does. What it can do — and what "single use" means here — is
// refuse to authorise a second generation on the same link. A custodian who lost
// their words does not get another key on this invitation; the coordinator
// reissues, which is visible on the status board and written into the record.
func (h *hostSession) handleGenerated(w http.ResponseWriter, r *http.Request) {
	if !postedStrict(w, r, nil) {
		return
	}
	i := inviteFromContext(r.Context())

	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.stillLive(w, i) {
		return
	}
	if !h.paramsSet {
		fail(w, http.StatusConflict, errors.New("the coordinator has not set this ceremony up yet"))
		return
	}
	if !i.GeneratedAt.IsZero() {
		fail(w, http.StatusGone, errors.New(
			"a phrase has already been generated on this link and it cannot be reissued from here — nothing on "+
				"the coordinator's side ever held it. If you have your twenty-four words, enter them to carry on. "+
				"If you do not, ask the coordinator to reissue your invitation: that abandons this key, so destroy "+
				"any sheet you started"))
		return
	}
	if _, exists := h.submissions[i.Name]; exists {
		fail(w, http.StatusConflict, errors.New(
			"a key has already been recorded for you in this ceremony, and it is in the group the others are "+
				"computing. Generating another would leave two keys for one custodian"))
		return
	}
	i.GeneratedAt = time.Now()
	reply(w, http.StatusOK, h.inviteView(i))
}

// handleHostSubmission takes the public half.
//
// The name is taken from the invite and written over whatever the body claimed.
// That is the property that makes a leaked link survivable: it can only ever
// speak for the one custodian it was issued to, so the worst a stolen link can
// do is submit a key under its own custodian's name — which the custodian's own
// presence check and the fingerprint read aloud on the call are there to catch.
func (h *hostSession) handleHostSubmission(w http.ResponseWriter, r *http.Request) {
	var sub submission
	if !postedStrict(w, r, &sub) {
		return
	}
	i := inviteFromContext(r.Context())

	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.stillLive(w, i) {
		return
	}
	if !h.paramsSet {
		fail(w, http.StatusConflict, errors.New("the coordinator has not set this ceremony up yet"))
		return
	}
	if sub.Identity.Name != i.Name {
		fail(w, http.StatusForbidden, fmt.Errorf(
			"this invitation is %s's, and the submission is for %q. A link speaks for one custodian only",
			i.Name, sub.Identity.Name))
		return
	}

	// Verified, not trusted. Without the possession signature this is "here is
	// an address", and anyone who can reach this endpoint can send one of those.
	// Everything derivable is re-derived: a submission whose address field named
	// an attacker while its public key belonged to an honest custodian would
	// otherwise put the attacker in the group, and the fingerprint read aloud,
	// the presence check and the record would all agree with it.
	id, err := verifySubmission(h.params, sub)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	if existing, ok := h.submissions[i.Name]; ok {
		if existing.Identity.Address == id.Address {
			reply(w, http.StatusOK, h.inviteView(i))
			return
		}
		fail(w, http.StatusConflict, fmt.Errorf(
			"a different key is already recorded for %s. One of the two is not the key that custodian holds, "+
				"and guessing which would put a stranger in the group", i.Name))
		return
	}
	h.submissions[i.Name] = sub
	reply(w, http.StatusOK, h.inviteView(i))
}

// handleHostAttestation records one custodian's signed statement.
//
// Verified through verifyAttestation, unchanged from the air-gapped path, and
// then checked against the group this process computed. A mismatch is not
// smoothed over: two custodians looking at different groups is the exact failure
// the read-aloud step exists to catch, so it is a refusal with both fingerprints
// in it.
func (h *hostSession) handleHostAttestation(w http.ResponseWriter, r *http.Request) {
	var signed signedAttestation
	if !postedStrict(w, r, &signed) {
		return
	}
	i := inviteFromContext(r.Context())

	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.stillLive(w, i) {
		return
	}
	if signed.Attestation.Name != i.Name {
		fail(w, http.StatusForbidden, fmt.Errorf(
			"this invitation is %s's and the attestation is signed for %q", i.Name, signed.Attestation.Name))
		return
	}
	if err := verifyAttestation(signed); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	own, ok := h.submissions[i.Name]
	if !ok {
		fail(w, http.StatusConflict, errors.New("no key has been submitted for you yet, so there is nothing to attest with"))
		return
	}
	if signed.Attestation.Address != own.Identity.Address {
		fail(w, http.StatusBadRequest, fmt.Errorf(
			"the attestation is about %s and the key recorded for you is %s",
			signed.Attestation.Address, own.Identity.Address))
		return
	}
	if signed.Attestation.CeremonyID != h.params.ID {
		fail(w, http.StatusBadRequest, fmt.Errorf(
			"that attestation is for ceremony %q and this is %q", signed.Attestation.CeremonyID, h.params.ID))
		return
	}

	subs := make([]submission, 0, len(h.submissions))
	for _, name := range h.params.Custodians {
		if s, ok := h.submissions[name]; ok {
			subs = append(subs, s)
		}
	}
	if len(subs) != len(h.params.Custodians) {
		fail(w, http.StatusConflict, errors.New(missingFrom(h.params, subs)))
		return
	}
	a, err := assembleGroup(h.params, subs)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if signed.Attestation.GroupFingerprint != a.Fingerprint {
		fail(w, http.StatusConflict, fmt.Errorf(
			"that attestation names group %s and the submissions here compute %s. Two custodians looking at "+
				"different groups is the failure the read-aloud step exists to catch — stop the ceremony and "+
				"find out why", signed.Attestation.GroupFingerprint, a.Fingerprint))
		return
	}
	if err := a.presence(signed.Attestation.Address, own.Identity.Fingerprint); err != nil {
		fail(w, http.StatusConflict, err)
		return
	}

	h.attestations[signed.Attestation.Address] = signed
	reply(w, http.StatusOK, h.inviteView(i))
}

// ---------------------------------------------------------------- transport

// postedStrict decodes a request body and refuses any field it does not know.
//
// DisallowUnknownFields is the mechanism behind the claim this whole command
// rests on. It is not there to catch typos: it is there so that no request to
// this process can carry a phrase, a seed or a private key even if a modified
// page tried to send one. A tolerant decoder would accept the field, ignore it,
// and leave the words sitting in this process's memory and in any access log
// that recorded a body — which is precisely the arrangement this tool exists to
// abolish.
func postedStrict(w http.ResponseWriter, r *http.Request, into any) bool {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, errors.New("this step is a POST"))
		return false
	}
	// A step that takes no arguments still decodes, into a struct with no
	// fields. Skipping the decode for those would leave exactly the endpoints
	// with nothing to validate as the ones that would silently accept a body
	// with a phrase in it, and those are half the routes.
	if into == nil {
		into = &struct{}{}
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("could not read the request: %w", err))
		return false
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return true
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf(
			"this request carried something this ceremony does not accept: %w. Nothing here takes a recovery "+
				"phrase, a seed or a private key, and a body containing one is refused rather than ignored", err))
		return false
	}
	return true
}
