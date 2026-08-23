package main

// The country enrolment ceremony: everything needed to make one country
// operational on a chain that is already running.
//
// It is the sibling of the foundation ceremony and not a second copy of it. The
// keys, the possession signatures, the group assembly and the fingerprint five
// people read to each other are the same code — `ceremony host` generates an
// office's keys in each super user's own browser exactly as it generates the
// foundation's, and this file consumes what that wrote. What differs is the
// other end. The foundation ceremony's output is a genesis fragment, because the
// foundation has to exist at height zero. A country's output is transactions and
// proposals, because the chain is already up.
//
// That difference is not cosmetic, and the whole of this file exists because of
// one consequence of it.
//
// # Why this is two phases, and why the first one cannot be skipped
//
// An x/group policy address derives from the group policy sequence number alone
// — not from the members, not from the threshold, not from the admin, not from
// the chain id. policyAddress() in group.go computes it offline for exactly that
// reason, and the launch runbook uses that property to put the foundation's group
// in genesis so the address and the membership are fixed by the same file.
//
// On a live chain the same property is a hazard rather than a convenience. If
// this tool predicted an office's address from a sequence number and composed a
// grant naming it, then anybody who created a group policy first would own the
// address the grant lands on — and the grant would be a real grant of
// PAYMENTS_AUTHORITY or ENFORCEMENT_AUTHORITY over a whole country, made to a
// stranger's group, by the foundation, with every signature valid. Nothing later
// in the process would notice, because every check downstream reads the same
// predicted address.
//
// So:
//
//  1. Create the group. `ceremony country groups` writes the transactions and
//     nothing else. It composes no grant and it writes no address.
//  2. Read the address back. `ceremony country confirm` takes the chain's own
//     answers — the queried transaction result and the group policy and members
//     it produced — and verifies that the policy at that address really is this
//     office: the same members, the same threshold, administering itself.
//  3. Only then compose the grant. `ceremony country grants` refuses outright for
//     any office whose address has not been read back and verified. There is no
//     flag that relaxes it and no --seq to fall back on.
//
// # Why this tool does not talk to the chain
//
// tools/ceremony contains no outbound network code, and the runbook makes that
// claim to people who are about to type a seed phrase in front of it. Adding an
// RPC client here to fetch the policy address would make the claim false for the
// sake of saving an operator one copy and paste.
//
// So the evidence arrives as files: the operator runs `blockchaind query ...
// -o json` and hands the answers in. That is weaker than fetching in exactly one
// way — a determined operator can fabricate a file — and stronger in another,
// which is the one that matters here. A fetched address would be trusted because
// it came over a socket. A handed-in one is verified against what the office is:
// the member set the super users signed for, the threshold they attested to, and
// an admin equal to the policy itself. A stranger's group policy pasted in fails
// on the members, not on the provenance of the file.
//
// # The bootstrap order
//
// Getting it wrong looks like a broken chain rather than like a mistake, so the
// tool enforces it rather than documenting it:
//
//	grant PAYMENTS_AUTHORITY to the country's payments office
//	  -> place the first applicant institutions, which only the foundation can do
//	     because no participant exists in the country yet to do it
//	  -> the payments office approves those participants
//	  -> those participants record the jurisdictions of the accounts they onboard
//
// The second step is the one that is easy to miss and it is not optional:
// x/paymsg's delegated approval path refuses an applicant the chain cannot place,
// so a payments authority with a perfectly good grant cannot admit the first bank
// in its own country until somebody has recorded where that bank is. Nobody may
// declare their own, and there is no participant yet, so the foundation does it —
// once, for the seed, as a foundation administrator.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	aliastypes "yamale/blockchain/x/alias/types"
)

// countryConfig is what a ceremony lead writes down before the day.
//
// Deliberately short, and note what is NOT in it: an office's threshold, its
// members, and its policy address. The first two come from the group file the
// hosted ceremony produced, so the config cannot disagree with what the super
// users signed for; the third comes from the chain. A config field for any of
// them would be a field somebody could fill in wrongly, and two of the three
// decide who ends up holding a country's authority.
//
// A REQUIRED MINIMUM is a different thing from an actual value, and it does
// belong here — see officeMinimum. The distinction is worth holding on to,
// because the two look alike in a JSON file and behave nothing alike. An
// office's actual threshold is an observation: reading it out of the config
// rather than out of the signed group file would let a config claim a
// three-of-five that the keys do not support. A required minimum is a
// constraint: it is the decision the country took before anybody generated a
// key, and the ceremony checks the attested group file against it. One is
// something to be read from the evidence; the other is what the evidence is
// held to.
type countryConfig struct {
	// Ceremony is the human name of this enrolment, printed on the record.
	Ceremony string `json:"ceremony"`

	// ChainID is which chain this is for. Required, because a grant is only
	// meaningful against a named chain and every artefact below carries it.
	ChainID string `json:"chain_id"`

	// Country is the ISO 3166-1 alpha-2 code being enrolled. One country per
	// dossier: two countries in one file would be two perimeters in one signed
	// record, and the point of the perimeter is that they are separate.
	Country string `json:"country"`

	// Foundation is the group policy address x/constitution pins as
	// enforcement_recovery_destination — the account that may admit a country.
	// Checked against a queried constitution rather than trusted; see
	// requireFoundation.
	Foundation string `json:"foundation"`

	// Offices are the country's offices, one per group.
	Offices []officeConfig `json:"offices"`

	// Waivers record a rule this enrolment is proceeding without, and why. The
	// same shape as preflight's --network-acknowledged: the rule still refuses,
	// and the only way past it is a written reason that ends up on the record
	// where somebody will read it later.
	Waivers []waiver `json:"waivers,omitempty"`
}

// officeConfig is one office.
type officeConfig struct {
	// Name is the office, e.g. "Banque Centrale du Sénégal". It has to match the
	// ceremony name in the group file, so that a config paired with the wrong
	// office's keys is a refusal rather than a grant to the wrong group.
	Name string `json:"name"`

	// Roles are the roles this office will hold, by enum name — ROLE_PAYMENTS_
	// AUTHORITY and so on. Names rather than numbers, because a number in a
	// hand-written config is a number nobody reviewing the file can check, and
	// the one that would do the most damage is the one somebody typed 4 for.
	Roles []string `json:"roles"`

	// Group is the path to the assembled group file the hosted ceremony wrote for
	// this office — group.json from `ceremony host --out`.
	Group string `json:"group"`

	// Minimum is the smallest M-of-N this office may be and still hold these
	// roles. Required.
	Minimum *officeMinimum `json:"minimum"`
}

// officeMinimum is the M-of-N an office must not fall below, decided before the
// ceremony and written onto every grant this enrolment makes.
//
// # Why it is decided in advance
//
// The alternative is to capture whatever the group file turns out to say, and
// that would be no requirement at all: it would ratify a one-of-one as readily as
// a three-of-five, because there would be nothing for the office to disagree
// with. The customer's own framing is the right one — the shape is a decision the
// country takes about how its authority is exercised, and a decision taken after
// seeing the answer is not a decision.
//
// So it is written in the config, before the day, by the person who also writes
// down which offices exist and what roles they hold. The ceremony then refuses to
// assemble an office whose signed group file does not meet it, and the grants it
// composes carry it onto the chain in required_shape, where every authority action
// is checked against it for as long as the grant exists.
//
// # Why the ceremony refuses rather than warns
//
// Because the alternative is a warning at the end of a long day. By the time an
// office's shape is wrong there are keys in five people's hands and a fingerprint
// somebody read aloud, and the cheapest moment to find out is before the groups
// are created — which is `country init`, which is where this refuses.
type officeMinimum struct {
	// Signatures is the fewest members that must sign for the office to act.
	//
	// Never one. A minimum of one signature is a minimum that permits the single
	// key this whole arrangement exists to abolish, and a config that could
	// express it would eventually contain it.
	Signatures int `json:"signatures"`

	// Members is the fewest members the office must have.
	//
	// Never equal to Signatures. A floor of three-of-three is a floor at which one
	// unreachable member freezes the office permanently, and the chain will hold
	// the office to the floor — so a unanimous minimum is a promise that the
	// office may never recover from a lost key. Two-of-three and three-of-five are
	// the shapes that work.
	//
	// The foundation's own ceremony refuses unanimity for its actual threshold, for
	// the same reason and one step earlier: see distributed.go, which will not
	// agree a threshold equal to the number of custodians. This is the same
	// reasoning applied to a floor rather than to a value.
	Members int `json:"members"`
}

func (m officeMinimum) rule() string {
	return fmt.Sprintf("%d-of-%d", m.Signatures, m.Members)
}

// shape is the requirement as the chain records it.
//
// Built here rather than assembled at each call site, so that the numbers on the
// signed record, the numbers checked against the group file and the numbers
// written into required_shape are one value that cannot drift into three.
//
// The conversion to uint32 is safe because every path to this method validates
// first — see requireOfficeMinimum, which re-validates a dossier an operator may
// have edited rather than trusting the config's validation of a different file.
func (m officeMinimum) shape() *aliastypes.OfficeShape {
	return &aliastypes.OfficeShape{
		Signatures: uint32(m.Signatures),
		Members:    uint32(m.Members),
	}
}

// validate refuses a minimum that is not one, before any key exists.
func (m officeMinimum) validate(office string) error {
	switch {
	case m.Signatures < 2:
		return fmt.Errorf(
			"%s's minimum asks for %d signature(s). A minimum of one is a minimum that permits a single key, "+
				"which is the arrangement every office on this chain exists to replace",
			office, m.Signatures)
	case m.Members < m.Signatures:
		return fmt.Errorf(
			"%s's minimum of %s asks for more signatures than members, which no office could satisfy",
			office, m.rule())
	case m.Members == m.Signatures:
		return fmt.Errorf(
			"%s's minimum of %s is unanimity. The chain holds an office to its minimum, so this one could never "+
				"lose a member: one unreachable super user would freeze it permanently. Two-of-three and "+
				"three-of-five are the shapes that work",
			office, m.rule())
	case m.Members > aliastypes.MaxOfficeMembers:
		return fmt.Errorf(
			"%s's minimum of %s asks for more than the %d members the chain can read a group's shape from",
			office, m.rule(), aliastypes.MaxOfficeMembers)
	}
	return nil
}

// waiver is a rule this enrolment is knowingly proceeding without.
type waiver struct {
	Rule   string `json:"rule"`
	Reason string `json:"reason"`
}

// The rules a waiver can name. A closed set, so a typo in a waiver is a refusal
// rather than a waiver of nothing that reads on the record as though it covered
// something.
const (
	// waivePaymentsMinimum lets a country be enrolled without both of the roles a
	// payments country needs. See requirePaymentsMinimum.
	waivePaymentsMinimum = "payments-minimum"
)

var waivableRules = map[string]string{
	waivePaymentsMinimum: "the country is enrolled without both PAYMENTS_AUTHORITY and ENFORCEMENT_AUTHORITY",
}

// countryDossier is the ceremony's state, advanced by one command at a time.
//
// A file rather than a database, and a file each command reads and rewrites
// rather than appends to, so that what has happened is legible to somebody who
// opens it in an editor. It is the thing that makes the bootstrap order
// enforceable: each phase's evidence is written here by the command that
// verified it, and the next phase refuses when it is absent.
//
// Nothing in here is secret. Addresses, thresholds, public fingerprints and
// transaction hashes — the same class of material as the foundation's ceremony
// record, and meant to be published beside it.
type countryDossier struct {
	Ceremony   string `json:"ceremony"`
	ChainID    string `json:"chain_id"`
	Country    string `json:"country"`
	Foundation string `json:"foundation"`
	CreatedAt  string `json:"created_at"`

	Offices []officeRecord `json:"offices"`

	// Seeded is the evidence for the step the bootstrap order needs and nobody
	// expects: the first applicant institutions placed in the country by the
	// foundation, because no participant exists yet to place them.
	Seeded []placement `json:"seeded,omitempty"`

	// Admitted is the evidence that the payments office approved those
	// applicants. Written by `confirm-participants`.
	Admitted []admission `json:"admitted,omitempty"`

	Waivers []waiver `json:"waivers,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

// officeRecord is one office and everything that has been proved about it.
type officeRecord struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`

	// GroupFile is where this office's ceremony record lives.
	//
	// Kept so that every later command re-reads and re-verifies it rather than
	// trusting the members copied into this dossier. The dossier is a file an
	// operator edits; the ceremony record is a file five people's signatures
	// cover, and the second one is what a group is created from.
	GroupFile string `json:"group_file"`

	// Threshold and Members come from the group file, which the office's super
	// users signed. They are what an on-chain policy is checked against.
	Threshold int            `json:"threshold"`
	Members   []officeMember `json:"members"`

	// Minimum is the M-of-N this office may never fall below, from the config.
	//
	// Carried here rather than re-read from the config on every command, because
	// the config is not an input to the later phases at all — the dossier is, and
	// the grant composed in the last phase has to name the same minimum the first
	// phase checked the group file against. A pointer so that a dossier written
	// before this field existed is an explicit refusal rather than a grant
	// requiring zero of zero.
	Minimum *officeMinimum `json:"minimum"`

	// CeremonyID and GroupFingerprint tie this office back to the ceremony that
	// made its keys. The fingerprint is the eighty bits its super users read to
	// each other; carrying it means the signed record of the enrolment and the
	// signed record of the key generation name the same thing.
	CeremonyID       string `json:"ceremony_id"`
	GroupFingerprint string `json:"group_fingerprint"`

	// OnChain is absent until `confirm` has read the group back from the chain
	// and verified it is this office. No grant may be composed against an office
	// whose OnChain is absent, and that is the whole reason this field is a
	// pointer rather than a string that could be empty.
	OnChain *onChainGroup `json:"on_chain,omitempty"`

	// Granted is what has been verified to have landed, one entry per role.
	Granted []grantEvidence `json:"granted,omitempty"`

	// Placed is the office's own jurisdiction record, once verified.
	Placed *placement `json:"placed,omitempty"`
}

// officeMember is one super user, as the ceremony recorded them.
type officeMember struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	Fingerprint string `json:"fingerprint"`
}

// onChainGroup is the evidence that an office's group exists, and that the
// address is the office's rather than somebody else's.
//
// Every field here was read out of a chain query response and cross-checked
// against the office. See confirmOffice for what "verified" means, because the
// presence of this struct is what every later phase treats as permission.
type onChainGroup struct {
	PolicyAddress string `json:"policy_address"`
	GroupID       uint64 `json:"group_id"`
	// TxHash is the transaction that created it, and Height is the height that
	// transaction was included at — both from the queried result rather than from
	// a broadcast response. A broadcast that came back code 0 has been accepted
	// into a mempool and has not executed.
	TxHash string `json:"tx_hash"`
	Height int64  `json:"height"`
	// ConfirmedAt is when this tool verified it, for the record. It is not part
	// of the verification.
	ConfirmedAt string `json:"confirmed_at"`
}

// grantEvidence is one role grant that has been verified to exist on chain.
type grantEvidence struct {
	Role         string `json:"role"`
	Jurisdiction string `json:"jurisdiction"`
	// GrantedBy is read from the grant the chain returned, not from what this
	// tool composed. A grant that landed with a different authority than the
	// proposal named is a grant somebody else made.
	GrantedBy       string `json:"granted_by"`
	GrantedAtHeight int64  `json:"granted_at_height"`
	VerifiedAt      string `json:"verified_at"`

	// RequiredShape is the M-of-N the chain says this grant records, rendered as
	// "3-of-5". Read back off the chain and checked against the office's minimum
	// rather than copied from the dossier, for the same reason GrantedBy is: a
	// grant that landed without the shape the proposal named is a grant that does
	// not constrain the office, and the record must not claim otherwise.
	RequiredShape string `json:"required_shape"`
}

// placement is a jurisdiction record that has been verified to exist.
type placement struct {
	Account    string `json:"account"`
	Country    string `json:"country"`
	RecordedBy string `json:"recorded_by"`
	VerifiedAt string `json:"verified_at"`
}

// admission is an approved participant that has been verified to exist.
type admission struct {
	Participant string `json:"participant"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	VerifiedAt  string `json:"verified_at"`
}

// ---------------------------------------------------------------- validation

// validate refuses a config that could not produce a working enrolment.
//
// Run before anything is written, because the alternative is a dossier that is
// half a ceremony: three offices created on chain and a fourth whose roles were
// misspelled, with no way to tell from the file whether the misspelling was
// noticed.
func (c countryConfig) validate() error {
	if strings.TrimSpace(c.Ceremony) == "" {
		return errors.New("the enrolment has no name; it goes on the record and the record is the point")
	}
	if strings.TrimSpace(c.ChainID) == "" {
		return errors.New("chain_id is required: a grant is only meaningful against a named chain")
	}

	country := aliastypes.NormaliseCountry(c.Country)
	switch {
	case country == aliastypes.ChainWide:
		return fmt.Errorf(
			"%q is the chain-wide scope, which is the foundation's alone and is not a country. "+
				"An enrolment grants authority inside one perimeter; a ceremony that could name %q would be a "+
				"ceremony for handing a national office authority over every country",
			c.Country, aliastypes.ChainWide)
	case country == aliastypes.FoundationCountry:
		return fmt.Errorf(
			"%q is the reserved code that marks the absence of a national perimeter, not a country. "+
				"A grant naming it would confer authority over nowhere while reading to a human as authority "+
				"over everywhere",
			c.Country)
	case !aliastypes.AssignedCountry(country):
		return fmt.Errorf(
			"%q is not an assigned ISO 3166-1 alpha-2 country code, so no authority's perimeter could contain "+
				"anything granted in it", c.Country)
	}

	if strings.TrimSpace(c.Foundation) == "" {
		return errors.New(
			"foundation is required: it is the account that admits the country, and it has to be the address " +
				"x/constitution pins as enforcement_recovery_destination")
	}
	if err := requireAccountAddress("foundation", c.Foundation); err != nil {
		return err
	}

	if len(c.Offices) == 0 {
		return errors.New("an enrolment with no offices grants nothing to nobody")
	}

	seen := map[string]bool{}
	for _, office := range c.Offices {
		name := strings.TrimSpace(office.Name)
		if name == "" {
			return errors.New("an office has no name")
		}
		if seen[name] {
			return fmt.Errorf(
				"%q appears twice; two offices that cannot be told apart by name cannot be told apart on the "+
					"record either", name)
		}
		seen[name] = true

		if strings.TrimSpace(office.Group) == "" {
			return fmt.Errorf("%s has no group file, so there are no keys for it", name)
		}
		if _, err := rolesOf(office.Roles); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		// Required, not defaulted. A default would be this tool deciding how a
		// country's authority is exercised, and a default of "no minimum" would
		// silently produce the state this field exists to end: an office that can
		// vote itself down to one key and go on holding a national authority. An
		// omitted minimum is therefore a refusal that says what to write.
		if office.Minimum == nil {
			return fmt.Errorf(
				"%s has no minimum. Every office needs one: it is the M-of-N the office may never fall below, "+
					"decided before anybody generates a key, checked against the group file, and written onto "+
					"every grant so the chain refuses an action by an office that has shrunk.\n"+
					"  \"minimum\": {\"signatures\": 3, \"members\": 5}",
				name)
		}
		if err := office.Minimum.validate(name); err != nil {
			return err
		}
	}

	for _, w := range c.Waivers {
		if _, known := waivableRules[w.Rule]; !known {
			return fmt.Errorf(
				"%q is not a rule this ceremony can waive. A waiver naming nothing reads on the record as "+
					"though it covered something", w.Rule)
		}
		if strings.TrimSpace(w.Reason) == "" {
			return fmt.Errorf(
				"the waiver of %q has no reason. The reason is the whole mechanism: the rule still refuses, and "+
					"what gets past it is a sentence somebody has to sign their name under", w.Rule)
		}
	}

	return c.requirePaymentsMinimum()
}

// requirePaymentsMinimum refuses a country enrolled without the two roles a
// payments deployment cannot work without.
//
// PAYMENTS_AUTHORITY, because without it no institution in the country can be
// admitted and therefore no account in the country can be onboarded or placed —
// the country would be enrolled and inert. ENFORCEMENT_AUTHORITY, because a
// perimeter where money can move and nobody can stop it is not a perimeter a
// regulator would recognise, and the office that ought to hold it is much harder
// to appoint after the fact than before.
//
// Refused rather than warned about, because a warning printed at the end of a
// long day is a warning nobody reads. Waivable, because a registry-only or
// supervision-only country is a legitimate thing to want — but only in writing.
func (c countryConfig) requirePaymentsMinimum() error {
	if c.waived(waivePaymentsMinimum) {
		return nil
	}

	held := map[aliastypes.Role]bool{}
	for _, office := range c.Offices {
		roles, err := rolesOf(office.Roles)
		if err != nil {
			return err
		}
		for _, role := range roles {
			held[role] = true
		}
	}

	var missing []string
	for _, required := range []aliastypes.Role{
		aliastypes.ROLE_PAYMENTS_AUTHORITY,
		aliastypes.ROLE_ENFORCEMENT_AUTHORITY,
	} {
		if !held[required] {
			missing = append(missing, aliastypes.RoleName(required))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"no office in this enrolment holds %s. A payments country needs both %s and %s: without the first "+
			"nobody in the country can be admitted to the rail, so no account in it can be placed and the "+
			"country is enrolled and inert; without the second money can move in the perimeter and no office "+
			"can stop it.\n"+
			"If that is deliberate — a registry-only or supervision-only country — say so in the config and it "+
			"goes on the record:\n"+
			"  \"waivers\": [{\"rule\": %q, \"reason\": \"...\"}]",
		strings.Join(missing, " or "),
		aliastypes.RoleName(aliastypes.ROLE_PAYMENTS_AUTHORITY),
		aliastypes.RoleName(aliastypes.ROLE_ENFORCEMENT_AUTHORITY),
		waivePaymentsMinimum)
}

func (c countryConfig) waived(rule string) bool {
	for _, w := range c.Waivers {
		if w.Rule == rule && strings.TrimSpace(w.Reason) != "" {
			return true
		}
	}
	return false
}

// rolesOf turns the config's role names into roles.
//
// Through aliastypes.Role_value rather than a table written here, so the set of
// names this tool accepts is exactly the set the chain has. A tool with its own
// copy of the enum is a tool that will eventually compose a grant of a role
// numbered for a different chain.
//
// Sorted and deduplicated, so the artefacts depend on the set of roles rather
// than the order somebody listed them in — the same reason readIdentities sorts
// custodians by address.
func rolesOf(names []string) ([]aliastypes.Role, error) {
	if len(names) == 0 {
		return nil, errors.New("no roles: an office that holds none is an office the chain will refuse every action from")
	}

	seen := map[aliastypes.Role]bool{}
	roles := make([]aliastypes.Role, 0, len(names))
	for _, raw := range names {
		name := strings.ToUpper(strings.TrimSpace(raw))
		value, known := aliastypes.Role_value[name]
		if !known {
			return nil, fmt.Errorf(
				"%q is not a role this chain has. The five are %s",
				raw, strings.Join(knownRoleNames(), ", "))
		}
		role := aliastypes.Role(value)
		// The zero value is reserved as unspecified and refused everywhere the
		// chain writes a grant, so refusing it here as well is not belt and
		// braces: ROLE_UNSPECIFIED is spellable, and a config that named it would
		// otherwise produce a proposal that three custodians vote for and the
		// chain then rejects, after the vote.
		if !aliastypes.ValidRole(role) {
			return nil, fmt.Errorf(
				"%q is the unset default and is never a role. Proto3 cannot tell a zero from a field nobody "+
					"filled in, which is why it is reserved", raw)
		}
		if seen[role] {
			return nil, fmt.Errorf("%s is listed twice", aliastypes.RoleName(role))
		}
		seen[role] = true
		roles = append(roles, role)
	}

	sort.Slice(roles, func(i, j int) bool { return roles[i] < roles[j] })
	return roles, nil
}

func knownRoleNames() []string {
	names := make([]string, 0, len(aliastypes.Role_name))
	for value, name := range aliastypes.Role_name {
		if aliastypes.ValidRole(aliastypes.Role(value)) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func roleNames(roles []aliastypes.Role) []string {
	names := make([]string, len(roles))
	for i, role := range roles {
		names[i] = aliastypes.RoleName(role)
	}
	return names
}

// ---------------------------------------------------------------- the dossier

// dossierFor builds the initial dossier from a config and the group files.
//
// The members and the threshold are lifted out of each office's group file and
// re-verified on the way through — assembleGroup is called again rather than the
// file's own claims being read, so a group file edited after the ceremony is a
// refusal rather than an office with a member somebody added.
func dossierFor(config countryConfig, now time.Time) (countryDossier, error) {
	if err := config.validate(); err != nil {
		return countryDossier{}, err
	}

	dossier := countryDossier{
		Ceremony:   strings.TrimSpace(config.Ceremony),
		ChainID:    strings.TrimSpace(config.ChainID),
		Country:    aliastypes.NormaliseCountry(config.Country),
		Foundation: strings.TrimSpace(config.Foundation),
		CreatedAt:  now.UTC().Truncate(time.Second).Format(time.RFC3339),
		Waivers:    config.Waivers,
	}

	addresses := map[string]string{}
	for _, office := range config.Offices {
		roles, err := rolesOf(office.Roles)
		if err != nil {
			return countryDossier{}, fmt.Errorf("%s: %w", office.Name, err)
		}

		assembled, err := readAssembledGroup(office.Group)
		if err != nil {
			return countryDossier{}, fmt.Errorf("%s: %w", office.Name, err)
		}

		// The office named in the config and the ceremony named in the group file
		// have to be the same office. Without this a config could pair one
		// office's name and roles with another office's keys, and the result would
		// be a grant of the wrong authority to a real group whose members all
		// signed for something else.
		if !sameOffice(office.Name, assembled.Params.Name) {
			return countryDossier{}, fmt.Errorf(
				"the config calls this office %q and %s was generated for %q. Those are not the same office, and "+
					"pairing one office's roles with another's keys is how an office ends up holding authority "+
					"nobody granted it",
				office.Name, office.Group, assembled.Params.Name)
		}
		if assembled.Params.ChainID != dossier.ChainID {
			return countryDossier{}, fmt.Errorf(
				"%s was generated for chain %q and this enrolment is for %q",
				office.Group, assembled.Params.ChainID, dossier.ChainID)
		}
		if err := requireOfficeAttested(office, dossier.Country, roles, assembled.Params); err != nil {
			return countryDossier{}, err
		}
		if err := requireOfficeMeetsMinimum(office, assembled); err != nil {
			return countryDossier{}, err
		}

		record := officeRecord{
			Name:             strings.TrimSpace(office.Name),
			Roles:            roleNames(roles),
			GroupFile:        office.Group,
			Threshold:        assembled.Params.Threshold,
			Minimum:          office.Minimum,
			CeremonyID:       assembled.Params.ID,
			GroupFingerprint: assembled.Fingerprint,
		}
		for _, member := range assembled.Custodians {
			record.Members = append(record.Members, officeMember{
				Name:        member.Name,
				Address:     member.Address,
				Fingerprint: member.Fingerprint,
			})
			// One person, one office. A super user in two offices holds a vote in
			// both, so an office that needs two signatures and shares a member
			// with the office that oversees it is not the separation the roles
			// describe. Refused rather than warned about, because it is invisible
			// once the groups exist: both policies look correct on their own.
			if other, clash := addresses[member.Address]; clash {
				return countryDossier{}, fmt.Errorf(
					"%s holds a key in both %q and %q. One person with a vote in two of a country's offices is "+
						"one person the separation between those offices does not apply to",
					member.Name, other, office.Name)
			}
			addresses[member.Address] = strings.TrimSpace(office.Name)
		}

		// The foundation must not be one of the offices. A grant to the
		// foundation's own address would be the foundation appointing itself a
		// national authority, which is the one shape of this ceremony that
		// widens its own powers.
		for _, member := range record.Members {
			if member.Address == dossier.Foundation {
				return countryDossier{}, fmt.Errorf(
					"%s's address is the foundation's own. An office whose member is the foundation is the "+
						"foundation appointing itself", member.Name)
			}
		}

		dossier.Offices = append(dossier.Offices, record)
	}

	return dossier, nil
}

// requireOfficeAttested checks that the office's super users generated their keys
// for this country and for these roles.
//
// This is the check that makes putting the country and the roles inside the
// parameters fingerprint worth anything, and it is the reason the office block
// exists on ceremonyParams at all.
//
// Without it, the sequence is: five people are told they are generating keys for
// Senegal's payments authority, they read a fingerprint aloud, they sign, and then
// a config pairs their group with a grant of ENFORCEMENT_AUTHORITY over Nigeria.
// Every signature would verify. The parameters fingerprint they compared covers
// the office block, so the only way to reach that state is a config that
// disagrees with what they signed — which is exactly what this refuses.
//
// A ceremony with no office block at all is refused here too. That is a foundation
// ceremony, and its custodians attested to holding the chain's recovery
// destination rather than to administering a country.
func requireOfficeAttested(office officeConfig, country string, roles []aliastypes.Role, params ceremonyParams) error {
	if params.Office == nil {
		return fmt.Errorf(
			"%s was generated as a foundation ceremony, with no country and no roles in its parameters.\n"+
				"Its super users read a fingerprint that says nothing about administering %s, so nothing they "+
				"signed covers the authority this enrolment would grant them. Run the office's ceremony with "+
				"its country and roles set — `ceremony host` takes both, and they go into the fingerprint "+
				"everybody compares before generating a key",
			office.Group, country)
	}

	attested := aliastypes.NormaliseCountry(params.Office.Country)
	if attested != country {
		return fmt.Errorf(
			"%s's super users generated their keys for %s and this enrolment would grant them authority in %s.\n"+
				"They compared a fingerprint covering %s. Whatever else is true, they did not agree to this",
			office.Name, attested, country, attested)
	}

	attestedRoles, err := rolesOf(params.Office.Roles)
	if err != nil {
		return fmt.Errorf("%s: the roles in its ceremony parameters are unusable: %w", office.Name, err)
	}
	if !sameRoleSet(roles, attestedRoles) {
		return fmt.Errorf(
			"%s's super users generated their keys for an office holding %s, and this enrolment would grant %s.\n"+
				"Both sets are covered by the fingerprint they compared before generating, so this is a config "+
				"that disagrees with what they signed rather than a value somebody can reconcile",
			office.Name,
			strings.Join(roleNames(attestedRoles), ", "),
			strings.Join(roleNames(roles), ", "))
	}
	return nil
}

// requireOfficeMeetsMinimum refuses to assemble an office whose signed group file
// does not reach the minimum the config demands.
//
// This is the moment the whole arrangement turns on, so read what is being
// compared with what. The minimum comes from the config, written before the day.
// The threshold and the membership come from the assembled group file, which the
// office's super users signed and whose fingerprint they read to each other. So
// this is not a config checking itself: it is a decision taken in advance, held
// against evidence produced independently of it.
//
// Refused here, at `country init`, because this is the last moment at which a
// wrong shape costs nothing. Afterwards there are groups on the chain, a proposal
// three custodians have voted on, and an office that has been told it holds an
// authority.
//
// Note the direction of both comparisons. The office may EXCEED the minimum
// freely — a country that agreed three-of-five and turned up with four-of-seven
// has more people and more agreement than it promised, and the chain will hold it
// to the promise rather than to the excess. What it may not do is fall short,
// because a config demanding three-of-five over a group file that says one-of-one
// is a config whose signed record and whose chain state would disagree about what
// the office is.
func requireOfficeMeetsMinimum(office officeConfig, signed assembled) error {
	if office.Minimum == nil {
		// Unreachable through validate(), which refuses an absent minimum before
		// any group file is read. Kept because this function is the one that must
		// never be satisfiable by an absent value: a nil dereference here would be
		// a crash, and a silent `return nil` would be the check removing itself.
		return fmt.Errorf("%s has no minimum, so there is nothing to hold its group file to", office.Name)
	}
	minimum := *office.Minimum
	actual := fmt.Sprintf("%d-of-%d", signed.Params.Threshold, len(signed.Custodians))

	if signed.Params.Threshold < minimum.Signatures {
		return fmt.Errorf(
			"%s must be at least %s and %s was generated as %s: its threshold of %d is below the %d signatures "+
				"this enrolment requires.\n"+
				"The minimum is the decision the country took before anybody generated a key, and the group file "+
				"is what its super users signed. Run the office's ceremony again with the agreed threshold, or "+
				"change the minimum in the config and have the change agreed — but not silently, because the "+
				"chain will hold this office to whichever number ends up on the grant",
			office.Name, minimum.rule(), office.Group, actual,
			signed.Params.Threshold, minimum.Signatures)
	}
	if len(signed.Custodians) < minimum.Members {
		return fmt.Errorf(
			"%s must be at least %s and %s was generated as %s: %d super users is below the %d this enrolment "+
				"requires.\n"+
				"An office cannot be topped up later without the foundation's involvement: the chain refuses an "+
				"action by an office below the shape its grant records, so enrolling this one now would produce a "+
				"national authority that cannot act",
			office.Name, minimum.rule(), office.Group, actual,
			len(signed.Custodians), minimum.Members)
	}
	return nil
}

// sameRoleSet compares two sorted, deduplicated role lists.
//
// Set equality both ways, for the same reason confirmMembers insists on it: an
// office granted a role its super users did not agree to hold is the obvious
// failure, and an office granted fewer than they agreed to is a silent one that
// leaves a country's authority half-appointed with a signed record saying
// otherwise.
func sameRoleSet(a, b []aliastypes.Role) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sameOffice compares an office name in a config with the ceremony name in a
// group file, tolerantly enough to be usable and strictly enough to be a check.
//
// Case and surrounding whitespace are ignored, because the two values are typed
// by different people on different days. Nothing else is: a ceremony named
// "Senegal payments authority" and a config office named "Senegal payments" do
// not match, and they should not, because the second could equally be a
// different office.
func sameOffice(configured, ceremony string) bool {
	return strings.EqualFold(strings.TrimSpace(configured), strings.TrimSpace(ceremony))
}

// office finds one office by name.
func (d *countryDossier) office(name string) (*officeRecord, error) {
	for i := range d.Offices {
		if sameOffice(name, d.Offices[i].Name) {
			return &d.Offices[i], nil
		}
	}
	names := make([]string, len(d.Offices))
	for i, office := range d.Offices {
		names[i] = office.Name
	}
	return nil, fmt.Errorf("%q is not an office in this enrolment. It has: %s", name, strings.Join(names, ", "))
}

// requireOfficeMinimum returns the office's minimum, or refuses.
//
// A dossier written before the minimum existed decodes with the field absent, and
// this is where that becomes a refusal rather than a grant requiring zero of zero
// — which the chain would refuse anyway, but after three custodians had voted for
// it. The message names the fix, because the fix is cheap: re-run `country init`
// against a config that has the minimums in it, before the groups are created.
func requireOfficeMinimum(office officeRecord) (officeMinimum, error) {
	if office.Minimum == nil {
		return officeMinimum{}, fmt.Errorf(
			"%s has no minimum in this dossier, so a grant to it would record no required shape — and an office "+
				"with no recorded shape can vote itself down to a single key and go on holding a national "+
				"authority.\n"+
				"Add \"minimum\": {\"signatures\": 3, \"members\": 5} to each office in the config and run "+
				"`ceremony country init` again",
			office.Name)
	}
	if err := office.Minimum.validate(office.Name); err != nil {
		// Re-validated rather than trusted, because a dossier is a file an operator
		// edits and this one is the number the chain will hold the office to
		// forever. The config's own validation ran on a different file.
		return officeMinimum{}, err
	}
	return *office.Minimum, nil
}

// holdsRole reports whether an office is configured to hold a role.
func (o officeRecord) holdsRole(role aliastypes.Role) bool {
	name := aliastypes.RoleName(role)
	for _, held := range o.Roles {
		if held == name {
			return true
		}
	}
	return false
}

// grantVerified reports whether one of this office's grants has been read back
// from the chain.
func (o officeRecord) grantVerified(role aliastypes.Role, country string) bool {
	name := aliastypes.RoleName(role)
	for _, g := range o.Granted {
		if g.Role == name && g.Jurisdiction == country {
			return true
		}
	}
	return false
}

// memberAddresses is the office's roster, sorted, for comparison against what a
// chain returned.
func (o officeRecord) memberAddresses() []string {
	addresses := make([]string, len(o.Members))
	for i, member := range o.Members {
		addresses[i] = member.Address
	}
	sort.Strings(addresses)
	return addresses
}

// paymentsOffice returns the office holding PAYMENTS_AUTHORITY.
//
// Named rather than assumed to be the first, and an error when there is more than
// one: two payments authorities in one country is a state the chain permits and
// this ceremony will not compose, because the participant approvals afterwards go
// to a specific office's group and "whichever one" is not an answer.
func (d *countryDossier) paymentsOffice() (*officeRecord, error) {
	var found *officeRecord
	for i := range d.Offices {
		if !d.Offices[i].holdsRole(aliastypes.ROLE_PAYMENTS_AUTHORITY) {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf(
				"both %q and %q hold %s in %s. The participant approvals go to one office's group, and this "+
					"ceremony will not choose which",
				found.Name, d.Offices[i].Name, aliastypes.RoleName(aliastypes.ROLE_PAYMENTS_AUTHORITY), d.Country)
		}
		found = &d.Offices[i]
	}
	if found == nil {
		return nil, fmt.Errorf(
			"no office in this enrolment holds %s, so there is nobody in %s who can admit an institution",
			aliastypes.RoleName(aliastypes.ROLE_PAYMENTS_AUTHORITY), d.Country)
	}
	return found, nil
}

// ---------------------------------------------------------------- files

// readAssembledGroup loads and re-verifies a group file from the hosted ceremony.
//
// Strict decoding, then assembleGroup over the submissions the file carries. The
// second part is what makes this a verification rather than a read: a group file
// is a public document that travels between machines, and everything in it that
// matters — the members, the policy address, the fingerprint — is derivable from
// the submissions. Recomputing means an edited file disagrees with itself and is
// refused, where reading its claims would put an added member into an office.
func readAssembledGroup(path string) (assembled, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return assembled{}, err
	}

	// The hosted ceremony writes two documents that both describe a group: the
	// full record with the submissions in it, and the assembled summary. Only the
	// first can be re-verified, so only the first is accepted.
	var file struct {
		Params      ceremonyParams `json:"params"`
		Submissions []submission   `json:"submissions"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	// Not DisallowUnknownFields here, and this is the one place in this file where
	// that is deliberate. The group record carries the whole assembled document
	// alongside the submissions, and refusing the fields this function does not
	// read would mean refusing every file the ceremony actually writes. What
	// protects the values is that they are recomputed below rather than read.
	if err := decoder.Decode(&file); err != nil {
		return assembled{}, fmt.Errorf("%s: %w", path, err)
	}
	if len(file.Submissions) == 0 {
		return assembled{}, fmt.Errorf(
			"%s carries no submissions, so nothing in it can be verified. Use the record the ceremony wrote "+
				"for this office, which has the public keys and the possession signatures in it", path)
	}

	result, err := assembleGroup(file.Params, file.Submissions)
	if err != nil {
		return assembled{}, fmt.Errorf("%s does not verify: %w", path, err)
	}
	return result, nil
}

// readDossier loads the dossier, strictly.
func readDossier(path string) (countryDossier, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return countryDossier{}, err
	}
	var dossier countryDossier
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dossier); err != nil {
		return countryDossier{}, fmt.Errorf("%s: %w", path, err)
	}
	if dossier.Country == "" || dossier.Foundation == "" {
		return countryDossier{}, fmt.Errorf("%s is not an enrolment dossier", path)
	}
	return dossier, nil
}

// writeDossier saves it.
func writeDossier(path string, dossier countryDossier) error {
	encoded, err := json.MarshalIndent(dossier, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), append(encoded, '\n'), 0o644)
}

// requireAccountAddress refuses anything that is not an address on this chain.
func requireAccountAddress(what, address string) error {
	if _, err := decodeAccountAddress(address); err != nil {
		return fmt.Errorf("%s %q is not an address this chain can read: %w", what, address, err)
	}
	return nil
}
