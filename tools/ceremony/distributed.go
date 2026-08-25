package main

// A ceremony where the five custodians are not in the same room.
//
// This is the stronger arrangement, not a concession to logistics. In the
// co-located ceremony one machine generates all five phrases in turn, so for the
// length of an afternoon there is a single computer whose memory has held every
// key the foundation has. Five custodians on five machines never produces that
// object: no custodian sees another's phrase, and no machine holds more than one
// key at any point in its life.
//
// What it costs is the room. In a room, five people watching each other IS the
// verification — the observer sees the same screen the custodian does. Remove
// the room and every check has to be reconstructed out of things that survive an
// untrusted channel:
//
//   - Proof of possession. A submission is signed by the key it announces, so a
//     public key arriving by email is a public key somebody can prove they hold.
//     Without it, anybody who can send a message can put their own address into
//     the group under a custodian's name.
//
//   - Assembly as a calculation rather than an act. Building the group and the
//     genesis fragment from five submissions is a pure function, so every
//     custodian's instance computes it and nobody is trusted to have done it
//     honestly. There is no coordinator role in this tool and no mode that
//     assembles on somebody else's behalf; whoever moves the files is a relay,
//     and a relay who withholds or delays is visible because the instance names
//     the submissions it does not have.
//
//   - One irreducible human step. The five custodians read the computed
//     fingerprint aloud to each other on a call and confirm it is the same
//     fingerprint. Nothing here can automate that, because the five instances
//     share no channel they can trust — which is the whole problem. A relay who
//     controls the channel can send each custodian a different but internally
//     consistent set of submissions, and five instances that each verified their
//     own set perfectly would each be looking at a different group. The only
//     thing that catches it is five people saying eighty bits out loud.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	aliastypes "yamale/blockchain/x/alias/types"
)

// The domains keep every digest and every signature in this program from being
// mistaken for, or replayed as, another. Versioned, because a change to any of
// the canonical encodings below has to invalidate old values rather than
// silently produce a different fingerprint for the same ceremony.
const (
	paramsDomain      = "yamale-ceremony-params-v2"
	groupDomain       = "yamale-ceremony-group-v1"
	possessionDomain  = "yamale-ceremony-possession-v1"
	attestationDomain = "yamale-ceremony-attestation-v1"
	ceremonyIDDomain  = "yamale-ceremony-id-v1"
)

// ceremonyParams is everything the five custodians agree before anybody
// generates a key.
//
// It is the input side of the pure function: given these values and five
// submissions, the group document and the genesis fragment are determined. So
// it has to be agreed first and it has to be checked, which is what
// fingerprint() below is for — read aloud on the call, before the first phrase
// exists, because a custodian who generates a key into the wrong ceremony has
// burned a key and a sheet.
//
// Custodians carries the roster of names. It is here rather than discovered from
// whatever submissions turn up, because a roster fixed in advance is what makes
// "we are missing Chipo's submission" a statement an instance can make. Without
// it a relay who supplied four honest submissions and one of their own would
// produce a group of five that every instance would happily compute.
//
// Office is the country-office half, and it is nil for the foundation. It is in
// the parameters — and therefore inside the fingerprint read aloud before
// anybody generates — because it is what the key is FOR. Without it a
// coordinator could take the keys five super users generated "for Senegal" and
// stand up an office granted authority over Nigeria, and nothing any of the five
// had seen would have said so.
// Administrators marks a ceremony for a foundation-administrator group, and it is
// in the parameters for the same reason Office is: it is what the key is FOR.
// A foundation administrator may correct the country recorded against any account
// on the chain — which moves that account out from under the authority
// investigating it — and may hold an identifier with no country at all. Somebody
// generating a key for "the Yamale foundation" has not agreed to that, and without
// this field inside the fingerprint they read aloud, nothing they saw would have
// distinguished the two.
//
// Mutually exclusive with Office, and validate() refuses both together. An
// administrator's authority is chain-wide by construction, so a ceremony that
// claimed both a national perimeter and the exemption from having one would be
// describing something that cannot exist.
type ceremonyParams struct {
	ID             string        `json:"ceremony_id"`
	Name           string        `json:"ceremony"`
	ChainID        string        `json:"chain_id"`
	Threshold      int           `json:"threshold"`
	Custodians     []string      `json:"custodians"`
	PolicySeq      uint64        `json:"policy_seq"`
	VotingPeriod   string        `json:"voting_period"`
	Office         *officeParams `json:"office,omitempty"`
	Administrators bool          `json:"foundation_administrators,omitempty"`
}

// administratorsMarker is the canonical tail that distinguishes an administrator
// ceremony's bytes from a foundation ceremony's. See canonical().
const administratorsMarker = "foundation-administrators"

// administratorsLabel is what an administrator group is called on chain.
//
// The suffix matters more than it looks. Two groups both recorded as "Yamale
// foundation" would be indistinguishable in the one field a human reads to find
// out what a group is — and on this chain the foundation already exists, so that
// is not hypothetical.
func administratorsLabel(name string) string {
	return name + " (foundation administrators)"
}

// onChain reports whether this ceremony's group is created by a transaction on a
// running chain rather than seeded at genesis.
//
// True for a country office and for an administrator group; false only for the
// foundation. It decides three things, and all three are omissions: no
// group-genesis.json (which names group id 1 and policy sequence 1, correct only
// for the first group on the chain), no constitution-invariants.json (which would
// declare this group the destination of every seized asset), and a policy address
// labelled as the prediction it is.
//
// One predicate rather than the same disjunction written at three call sites,
// because a rule spelled out three times is a rule with three places to stop
// being true — and the failure here is a file sitting in an output directory that
// somebody in a hurry splices into a genesis.
func (p ceremonyParams) onChain() bool {
	return p.Office != nil || p.Administrators
}

// officeParams is the country-office half of a ceremony's parameters.
//
// Nil for the foundation ceremony, which belongs to no national perimeter.
type officeParams struct {
	Country string   `json:"country"`
	Roles   []string `json:"roles"`
}

// foundationLabel is what a group with no office is called.
//
// A constant rather than a literal in three places, because it is inside the
// group metadata that the group fingerprint covers: the foundation's bytes must
// not move when the country path is added, and the way to guarantee that is for
// there to be one spelling of it.
const foundationLabel = "Yamale foundation"

// groupLabel is what this ceremony's group is called, on chain, permanently.
//
// It is the one field a human reads to find out what a group is, which is why it
// is derived from the parameters rather than hard-coded. A country office
// recorded as "Yamale foundation" would be a lie in exactly the place nobody
// would think to check.
func groupLabel(p ceremonyParams) string {
	switch {
	case p.Office != nil:
		return p.Name + " (" + p.Office.Country + ")"
	case p.Administrators:
		return administratorsLabel(p.Name)
	default:
		return foundationLabel
	}
}

// newCeremonyID is a fresh ceremony identifier.
//
// A hundred and twenty bits, in Crockford's alphabet, grouped in sixes so it can
// be retyped off a call without a transcription error going unnoticed — a
// mistyped id changes the params fingerprint, which is the value everybody is
// about to read aloud, so the typo is caught at the next sentence rather than
// after five keys have been generated.
//
// Whoever starts the ceremony proposes it and it confers nothing. It is a
// namespace: it stops a submission made for a rehearsal being replayed into the
// real ceremony, and it stops a custodian being talked into signing for a
// ceremony they are not part of. Its integrity comes from the params fingerprint
// covering it, not from who chose it.
func newCeremonyID() (string, error) {
	raw := make([]byte, 15)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	encoded := crockford.EncodeToString(raw)
	groups := make([]string, 0, 4)
	for i := 0; i < len(encoded); i += 6 {
		groups = append(groups, encoded[i:i+6])
	}
	return strings.Join(groups, "-"), nil
}

// canonField appends one length-prefixed field.
//
// Length-prefixed rather than delimited, so no combination of field values can
// be reassembled into a different combination that digests the same. A custodian
// named "Okafor|Amara" must not be able to produce the same canonical bytes as
// two custodians named "Okafor" and "Amara".
func canonField(b []byte, v string) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(v)))
	return append(b, v...)
}

func canonBytes(b []byte, v []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(v)))
	return append(b, v...)
}

// canonical is the byte string the params fingerprint is taken over.
//
// Written out by hand rather than by marshalling the struct. A JSON encoding
// would make the fingerprint depend on field order, on whether a field was
// omitted when empty, and on the encoder's version — three things that can
// change without anybody intending to change what the custodians read aloud.
// This is the value five people compare over a telephone; it does not get to
// move because a struct tag was edited.
// The office block is last, and a nil office encodes IDENTICALLY to an office
// with an empty country and no roles: canonField("") followed by a count of
// zero. That ambiguity is deliberate and unreachable — validate() refuses an
// office whose country is not an assigned code, so no valid ceremony can produce
// the second of the two. What it buys is that the foundation's canonical bytes
// are the old bytes plus a fixed eight-byte tail, rather than two shapes a
// reader has to hold in their head.
func (p ceremonyParams) canonical() []byte {
	names := append([]string(nil), p.Custodians...)
	sort.Strings(names)

	officeCountry := ""
	var officeRoles []string
	if p.Office != nil {
		officeCountry = p.Office.Country
		// Sorted on a copy, for the same reason the custodian names are: the
		// encoding must depend on the SET of roles the office is being granted,
		// not on the order somebody happened to type them into a form.
		officeRoles = append([]string(nil), p.Office.Roles...)
		sort.Strings(officeRoles)
	}

	b := canonField(nil, paramsDomain)
	b = canonField(b, p.ID)
	b = canonField(b, p.Name)
	b = canonField(b, p.ChainID)
	b = canonField(b, fmt.Sprintf("%d", p.Threshold))
	b = canonField(b, fmt.Sprintf("%d", p.PolicySeq))
	b = canonField(b, p.VotingPeriod)
	b = binary.BigEndian.AppendUint32(b, uint32(len(names)))
	for _, name := range names {
		b = canonField(b, name)
	}
	b = canonField(b, officeCountry)
	b = binary.BigEndian.AppendUint32(b, uint32(len(officeRoles)))
	for _, role := range officeRoles {
		b = canonField(b, role)
	}
	// The administrators marker is APPENDED ONLY WHEN SET, and paramsDomain stays
	// at v2. That is a deliberate departure from how the office block was added,
	// which always encodes and therefore moved every fingerprint and bumped the
	// domain to v2. Both choices are defensible and this one is the right way
	// round here:
	//
	//   - Always encoding would move the FOUNDATION's params fingerprint, and that
	//     value is on paper, in ink, in five sealed envelopes from a ceremony that
	//     has already happened. Somebody checking an old record against a new
	//     binary would see a mismatch and have no way to tell whether the
	//     parameters had changed or the tool had.
	//   - Omitting it leaves exactly one ambiguity: a ceremony with the marker
	//     absent and one with it set to empty encode identically. That is
	//     unreachable, because the marker is a fixed non-empty constant and
	//     Administrators is a bool. An office ceremony cannot collide with it
	//     either: reaching these bytes with the marker appended requires an empty
	//     office country, which validate() refuses.
	//
	// This is the same argument the nil-office case above already relies on —
	// deliberate, unreachable ambiguity — applied to keep old fingerprints valid
	// rather than to keep one shape in a reader's head.
	if p.Administrators {
		b = canonField(b, administratorsMarker)
	}
	return b
}

// fingerprint is the eighty bits the custodians read to each other before
// anybody generates a key.
func (p ceremonyParams) fingerprint() string {
	return longDigest(paramsDomain, p.canonical())
}

func (p ceremonyParams) votingPeriod() (time.Duration, error) {
	return time.ParseDuration(p.VotingPeriod)
}

// validate refuses parameters that could not produce a working group.
//
// Checked here as well as in buildGroup, because these values are agreed at the
// start and the group is computed at the end: a threshold nobody can meet should
// stop the ceremony before five people spend an afternoon on it, not after.
func (p ceremonyParams) validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.New("the ceremony has no id, so a submission could not be tied to it")
	}
	if strings.TrimSpace(p.ChainID) == "" {
		return errors.New("chain_id is required: an address is only meaningful against a named chain")
	}
	if len(p.Custodians) < 3 {
		return fmt.Errorf("%d custodians is not a group worth distributing", len(p.Custodians))
	}
	seen := map[string]bool{}
	for _, name := range p.Custodians {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return errors.New("a custodian on the roster has no name")
		}
		if seen[trimmed] {
			return fmt.Errorf("%q appears twice on the roster; two custodians cannot be told apart by name", trimmed)
		}
		seen[trimmed] = true
	}
	if p.Threshold < 2 {
		return fmt.Errorf("a threshold of %d means one custodian acts alone, which is the single key this ceremony replaces", p.Threshold)
	}
	if p.Threshold >= len(p.Custodians) {
		return fmt.Errorf(
			"a threshold of %d over %d custodians leaves no redundancy: losing one key would freeze the foundation account forever, with the chain still sending seizures to it",
			p.Threshold, len(p.Custodians))
	}
	period, err := p.votingPeriod()
	if err != nil {
		return fmt.Errorf("voting_period %q is not a duration: %w", p.VotingPeriod, err)
	}
	// Guarded here rather than left to the SDK, because the SDK's check is
	// narrower than it looks: ThresholdDecisionPolicy.ValidateBasic refuses a
	// voting period of exactly zero and says nothing about a negative one, and
	// x/group's GenesisState.Validate never goes deeper than ValidateBasic. So a
	// voting period of "-1h" produces a genesis file that imports cleanly and a
	// group whose proposals expire before they are made — three custodians
	// agreeing on something the chain will never execute.
	if period <= 0 {
		return fmt.Errorf(
			"a voting period of %s gives the other custodians no window to vote in, so this group could never "+
				"execute anything three of them agreed on", p.VotingPeriod)
	}
	// Bounded because a browser holds this as a JavaScript number, which is
	// exact only to 2^53. Beyond that the page would derive a DIFFERENT POLICY
	// ADDRESS from this binary — silently, for the one value that decides where
	// every seized asset on the chain is sent, while every other check agreed
	// because they all read the same field. 2^40 is far past any sequence a real
	// chain could reach and far inside what both sides represent exactly.
	const maxPolicySeq = 1 << 40
	if p.PolicySeq > maxPolicySeq {
		return fmt.Errorf(
			"policy_seq %d is past anything a chain could have reached, and past the range a browser holds exactly",
			p.PolicySeq)
	}
	return p.validateOffice()
}

// validateOffice checks the country-office half.
//
// Nil is the foundation ceremony and there is nothing to check. A non-nil office
// is checked against the chain's own tables — x/alias's assigned-country list and
// its role enum — rather than against a copy written here, because a tool with
// its own copy of either is a tool that will eventually produce a ceremony for a
// perimeter or a role the chain does not have. The super users would have
// generated keys, read a fingerprint aloud, and signed attestations for an office
// the chain then refuses to grant anything to.
func (p ceremonyParams) validateOffice() error {
	if p.Administrators && p.Office != nil {
		return fmt.Errorf(
			"this ceremony claims both a country office in %q and the foundation-administrator exemption, and "+
				"those are opposites. An administrator's authority is chain-wide and its identifier carries the "+
				"reserved code that marks the ABSENCE of a national perimeter; an office holds authority inside "+
				"one. A group cannot be both, and a ceremony that said so would have every super user generating "+
				"a key for something that does not exist",
			p.Office.Country)
	}
	if p.Office == nil {
		return nil
	}

	country := p.Office.Country
	// Refused rather than normalised. The country is inside the parameters
	// fingerprint the super users read aloud before generating, so a value this
	// program silently rewrote would be a value none of them agreed to — the same
	// rule checkCanonicalTimestamp applies to generated_at, and for the same
	// reason.
	if country != aliastypes.NormaliseCountry(country) {
		return fmt.Errorf(
			"the office's country is %q and this ceremony writes %q — two uppercase letters. A value silently "+
				"rewritten is a value the super users did not read aloud before generating",
			country, aliastypes.NormaliseCountry(country))
	}
	switch {
	case country == aliastypes.ChainWide:
		return fmt.Errorf(
			"%q is the chain-wide scope, which is the foundation's alone and is not a country. An office holds "+
				"authority inside one perimeter; a ceremony that could name %q would be a ceremony for handing a "+
				"national office authority over every country",
			country, aliastypes.ChainWide)
	case country == aliastypes.FoundationCountry:
		return fmt.Errorf(
			"%q is the reserved code that marks the ABSENCE of a national perimeter, not a country. An office "+
				"recorded there would hold authority over nowhere while reading to a human as authority over "+
				"everywhere. A ceremony with no perimeter is the foundation's: leave the country blank",
			country)
	}
	if len(country) != aliastypes.CountryLength || !upperASCII(country) {
		return fmt.Errorf(
			"the office's country is %q; a country code here is exactly two uppercase letters, A to Z", country)
	}
	if !aliastypes.AssignedCountry(country) {
		return fmt.Errorf(
			"%q is not an assigned ISO 3166-1 alpha-2 country code, so no authority's perimeter could contain "+
				"anything granted in it. NX, QK and ZX are all two letters and none of them is a country",
			country)
	}

	if len(p.Office.Roles) == 0 {
		return errors.New(
			"the office holds no roles, so the chain would refuse every action it ever attempted. " +
				"An office worth a key ceremony holds at least one")
	}
	seen := map[string]bool{}
	for _, name := range p.Office.Roles {
		if name != strings.ToUpper(strings.TrimSpace(name)) {
			return fmt.Errorf(
				"the role %q is not written the way this chain spells it, %q. The roles are covered by the "+
					"fingerprint the super users read aloud, so this is refused rather than tidied up",
				name, strings.ToUpper(strings.TrimSpace(name)))
		}
		value, known := aliastypes.Role_value[name]
		if !known {
			return fmt.Errorf(
				"%q is not a role this chain has. They are %s",
				name, strings.Join(ceremonyRoleNames(), ", "))
		}
		// ROLE_UNSPECIFIED is spellable, and it is the zero value. Proto3 cannot
		// tell a zero from a field nobody filled in, which is why the enum
		// reserves it — so an office that named it would produce a grant the
		// chain rejects AFTER three custodians had voted for it.
		if !aliastypes.ValidRole(aliastypes.Role(value)) {
			return fmt.Errorf(
				"%q is the unset default and is never a role. Proto3 cannot tell a zero from a field nobody "+
					"filled in, which is why it is reserved", name)
		}
		// A role the chain grants chain-wide or not at all cannot be held by an
		// office that administers one country, so it is refused here rather than
		// discovered when the grant is proposed — which would be after five
		// custodians had generated keys and read a fingerprint aloud for an
		// authority the chain will never grant.
		//
		// Named separately from the "not a role this chain has" refusal above,
		// because it is a different mistake with a different fix: the role is
		// real, and what is wrong is the office asking for it.
		if aliastypes.ChainWideOnly(aliastypes.Role(value)) {
			return fmt.Errorf(
				"%s is granted %q or not at all, so an office administering %s cannot hold it. "+
					"It is appointed by `ceremony administrators`, whose group belongs to no national perimeter",
				name, aliastypes.ChainWide, country)
		}
		if seen[name] {
			return fmt.Errorf(
				"%s is listed twice. The roles are a set, and a list that repeats one reads on the record as "+
					"though the office were granted it twice", name)
		}
		seen[name] = true
	}
	return nil
}

// upperASCII reports whether every byte is A–Z.
//
// Written out rather than reached for through unicode, because the question is
// specifically about the two ASCII letters an ISO 3166-1 alpha-2 code is made of.
// A Cyrillic А that looks identical is not one of them.
func upperASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return len(s) > 0
}

// ceremonyRoleNames is every role name a ceremony will accept, sorted.
//
// Built from the chain's own enum and filtered by the chain's own predicates, so
// the set is derived rather than maintained. It is also what the cross-language
// fixture publishes as role_names for the browser, which cannot import a Go enum
// — the same arrangement the two SDK constants in policy_derivation already use.
//
// Two filters, and the second one is not a tidying:
//
//   - ValidRole, which drops the reserved zero value. An office that named it
//     would produce a grant the chain rejects AFTER three custodians voted.
//   - ChainWideOnly, which drops ROLE_FOUNDATION_ADMINISTRATOR. Every role a
//     ceremony assembles an office for is granted IN A COUNTRY, and that role is
//     chain-wide or nothing — so offering it here would put a role on the
//     coordinator's form, inside the fingerprint five super users read aloud,
//     that the chain will refuse when the grant is proposed. A ceremony whose
//     output cannot be granted is worse than one that refused the request: the
//     refusal arrives after the keys exist and after the room has gone home.
//
// The foundation's own administrators are still appointed by a ceremony — see
// administrators.go — and it is not this one. That ceremony assembles a group
// and carries no office at all, which is exactly why the office's role list can
// exclude the role without excluding the appointment.
func ceremonyRoleNames() []string {
	names := make([]string, 0, len(aliastypes.Role_name))
	for value, name := range aliastypes.Role_name {
		role := aliastypes.Role(value)
		if aliastypes.ValidRole(role) && !aliastypes.ChainWideOnly(role) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// submission is what leaves a custodian's machine, and it is all that leaves.
//
// The private half never appears here and never appears in any struct this
// program serialises. What travels is a public key, a name, and a signature
// proving the sender holds the key — which is exactly the material the group
// needs and nothing else.
type submission struct {
	CeremonyID string   `json:"ceremony_id"`
	Identity   identity `json:"identity"`
	// Possession is a signature by the announced key over the identity's own
	// fields. It is the difference between "here is an address" and "here is an
	// address somebody can prove they control".
	Possession string `json:"possession"`
}

// possessionMessage is what a custodian signs to prove the key is theirs.
//
// The name is in it deliberately. The name is what goes into the group's
// metadata and onto the signed record, so a signature that covered only the key
// would let a relay keep an honest custodian's key and attach a different
// person's name to it — and every later check reads the name from the same
// place.
//
// The address and the fingerprint are NOT in it, because both are derived from
// the public key and re-derived by every verifier. Signing a value nobody reads
// would be theatre.
func possessionMessage(ceremonyID string, id identity) []byte {
	b := canonField(nil, possessionDomain)
	b = canonField(b, ceremonyID)
	b = canonField(b, string(id.Role))
	b = canonField(b, id.Name)
	b = canonField(b, id.PubKey.Key)
	b = canonField(b, id.HDPath)
	b = canonField(b, id.GeneratedAt)
	return b
}

// signSubmission produces the custodian's own submission.
func signSubmission(ceremonyID string, id identity, priv *secp256k1.PrivKey) (submission, error) {
	signature, err := priv.Sign(possessionMessage(ceremonyID, id))
	if err != nil {
		return submission{}, err
	}
	return submission{
		CeremonyID: ceremonyID,
		Identity:   id,
		Possession: base64.StdEncoding.EncodeToString(signature),
	}, nil
}

// verifySubmission checks a submission from somebody else's machine.
//
// Everything derivable is re-derived rather than read. The address and the
// fingerprint in an incoming file are treated as claims to be checked against
// the public key, not as facts — a submission whose address field named an
// attacker while its public key belonged to an honest custodian would otherwise
// put the attacker in the group, and the fingerprint read aloud, the presence
// check and the record would all agree with it because they all read that field.
//
// Returns the identity this program will use, which is the derived one.
func verifySubmission(params ceremonyParams, s submission) (identity, error) {
	if s.CeremonyID != params.ID {
		return identity{}, fmt.Errorf(
			"this submission is for ceremony %q and we are running %q. It is either from a different ceremony or from somebody who was told a different id",
			s.CeremonyID, params.ID)
	}
	if s.Identity.Role != roleCustodian {
		return identity{}, fmt.Errorf(
			"%s is recorded as %q, not a custodian; a validator operator key does not belong in the foundation group",
			s.Identity.Name, s.Identity.Role)
	}
	if !onRoster(params.Custodians, s.Identity.Name) {
		return identity{}, fmt.Errorf(
			"%q is not on the roster of %d custodians agreed at the start of this ceremony. Either the roster is wrong or this submission is from somebody who was not invited",
			s.Identity.Name, len(params.Custodians))
	}
	if s.Identity.PubKey.Type != secp256k1PubKeyType {
		return identity{}, fmt.Errorf("%s's key is a %q; this chain's accounts are secp256k1", s.Identity.Name, s.Identity.PubKey.Type)
	}

	raw, err := base64.StdEncoding.DecodeString(s.Identity.PubKey.Key)
	if err != nil {
		return identity{}, fmt.Errorf("%s's public key is not base64: %w", s.Identity.Name, err)
	}
	if len(raw) != secp256k1.PubKeySize {
		return identity{}, fmt.Errorf(
			"%s's public key is %d bytes; a compressed secp256k1 key is %d",
			s.Identity.Name, len(raw), secp256k1.PubKeySize)
	}
	pub := &secp256k1.PubKey{Key: raw}

	signature, err := base64.StdEncoding.DecodeString(s.Possession)
	if err != nil {
		return identity{}, fmt.Errorf("%s's proof of possession is not base64: %w", s.Identity.Name, err)
	}
	if !pub.VerifySignature(possessionMessage(params.ID, s.Identity), signature) {
		return identity{}, fmt.Errorf(
			"%s's proof of possession does not verify. Whoever produced this file does not hold the key it announces — do not put this address in the group, and say so on the call",
			s.Identity.Name)
	}

	if err := checkCanonicalTimestamp(s.Identity.Name, s.Identity.GeneratedAt); err != nil {
		return identity{}, err
	}
	if err := checkHDPath(s.Identity.Name, s.Identity.HDPath); err != nil {
		return identity{}, err
	}

	derived, err := identityFromPubKey(s.Identity.Name, roleCustodian, pub, s.Identity.HDPath, time.Now())
	if err != nil {
		return identity{}, err
	}
	// Carried across rather than regenerated: generated_at is the custodian's
	// own clock and it feeds the deterministic timestamp below, so it has to be
	// the value they signed.
	derived.GeneratedAt = s.Identity.GeneratedAt
	derived.Ceremony = params.Name

	if s.Identity.Address != "" && s.Identity.Address != derived.Address {
		return identity{}, fmt.Errorf(
			"%s's submission claims address %s but its public key derives %s. The file has been edited",
			s.Identity.Name, s.Identity.Address, derived.Address)
	}
	if s.Identity.Fingerprint != "" && s.Identity.Fingerprint != derived.Fingerprint {
		return identity{}, fmt.Errorf(
			"%s's submission claims fingerprint %s but its public key derives %s. The file has been edited",
			s.Identity.Name, s.Identity.Fingerprint, derived.Fingerprint)
	}
	return derived, nil
}

// secp256k1PubKeyType is the type URL every SDK account key carries. Pinned as a
// constant so a submission announcing some other curve is a refusal rather than
// a key this chain cannot verify a signature from.
const secp256k1PubKeyType = "/cosmos.crypto.secp256k1.PubKey"

// checkCanonicalTimestamp refuses any generated_at that is not the exact form
// this ceremony writes: UTC, whole seconds, trailing Z.
//
// This looks like pedantry and it is the one place two honest implementations of
// assembleGroup can disagree. The latest generated_at among the submissions
// becomes the timestamp inside the genesis fragment, and the group fingerprint
// covers those bytes. This binary parses the value and re-emits it normalised;
// the browser compares the strings and uses the winner verbatim, which is
// correct for this one format and for no other — lexical order is chronological
// order only when every value is UTC with the same width.
//
// So a submission carrying "2026-03-02T11:15:00+02:00" — valid RFC 3339, signed
// by its own custodian, the same instant as an earlier Z value that sorts before
// it — would have five browsers computing one fingerprint and this binary
// computing another. That is the failure the read-aloud step cannot tell apart
// from an attack. It is refused at the door rather than normalised quietly,
// because a value silently rewritten is a value the custodian did not sign.
func checkCanonicalTimestamp(name, value string) error {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return fmt.Errorf("%s's submission has an unreadable generated_at %q: %w", name, value, err)
	}
	canonical := parsed.UTC().Truncate(time.Second).Format(time.RFC3339)
	if canonical != value {
		return fmt.Errorf(
			"%s's submission is timestamped %q and this ceremony writes %q — UTC, whole seconds, trailing Z. "+
				"Two spellings of one instant would give a browser and this binary different group fingerprints "+
				"from the same five submissions",
			name, value, canonical)
	}
	return nil
}

// checkHDPath refuses a submission announcing a derivation path that is not this
// chain's.
//
// The path is inside the possession signature, so it cannot be altered in
// transit — but it is chosen by whoever generated the key, and it is what the
// ceremony record tells somebody to derive at years from now. A record naming
// m/44'/60'/... would send a custodian recovering the account to an address that
// is not in the group, with the envelope in their hand and nothing to say which
// of the two is wrong.
func checkHDPath(name, path string) error {
	// Built from the chain's own parameters rather than written out, so a change
	// to coinType moves this with it. Only the account index varies, and it is
	// the last element.
	base := strings.TrimSuffix(hd.NewFundraiserParams(0, coinType, 0).String(), "/0")
	if !strings.HasPrefix(path, base+"/") {
		return fmt.Errorf(
			"%s's submission was derived at %q, and this chain's accounts live under %s. A key derived somewhere "+
				"else is a key the recovery instructions on the record would not find",
			name, path, base)
	}
	return nil
}

func onRoster(roster []string, name string) bool {
	for _, candidate := range roster {
		if strings.TrimSpace(candidate) == strings.TrimSpace(name) {
			return true
		}
	}
	return false
}

// assembled is the group every instance computes for itself.
type assembled struct {
	Params     ceremonyParams `json:"params"`
	Custodians []identity     `json:"custodians"`
	// PolicyAddress is derived from PolicySeq alone. For the foundation that is
	// a fact, because the group is seeded at genesis and the sequence number is
	// fixed by the same file. For a country office it is a PREDICTION: the
	// office's group is created by a transaction on a running chain and the chain
	// decides the sequence, so the real address is read back afterwards by
	// `ceremony country confirm`. It stays here because the group fingerprint
	// covers it and the foundation record needs it.
	PolicyAddress string `json:"policy_address"`
	// Fingerprint is the eighty bits the five custodians read to each other. It
	// covers the parameters, all five public keys, and the exact bytes of the
	// genesis fragment below.
	Fingerprint string          `json:"fingerprint"`
	Genesis     json.RawMessage `json:"group_genesis"`
	// Constitution is nil for a country office, and that is load-bearing rather
	// than an omission. The fragment says "send every seized asset on the chain
	// to this address"; produced for Senegal's payments office it is a
	// ready-to-splice document handing that office the whole chain's seizures.
	// canonical() encodes nil as a bare zero length, so the fingerprint covers
	// its absence.
	Constitution json.RawMessage `json:"constitution_invariants,omitempty"`
	Members      json.RawMessage `json:"group_members"`
	Policy       json.RawMessage `json:"group_policy"`
	CreateMsg    json.RawMessage `json:"group_create_msg"`
	// ComputedAt is the deterministic timestamp inside the genesis fragment,
	// not the time this ran. See assembleGroup.
	ComputedAt string `json:"computed_at"`
}

// assembleGroup is the pure function.
//
// Same parameters and same five submissions produce the same bytes on every
// machine, and that is load-bearing rather than tidy: the whole scheme rests on
// five instances independently computing a value they then compare by voice. If
// two honest instances could differ, the comparison would fail for innocent
// reasons, the custodians would learn to shrug it off, and the check that
// defeats a hostile relay would be gone.
//
// So nothing local reaches the output. In particular the timestamp inside the
// genesis fragment is the LATEST generated_at among the five submissions rather
// than time.Now() — a value carried in the signed inputs, identical on all five
// machines, and truncated to the second because a genesis compared byte for byte
// across a validator set cannot carry nanoseconds anybody would have to retype.
// A custodian with a badly wrong clock skews it, and that shows up in the
// fingerprint everybody is about to read aloud, which is the correct place for
// it to show up.
func assembleGroup(params ceremonyParams, submissions []submission) (assembled, error) {
	if err := params.validate(); err != nil {
		return assembled{}, err
	}
	if len(submissions) != len(params.Custodians) {
		return assembled{}, fmt.Errorf(
			"the roster has %d custodians and %d submissions are present. %s",
			len(params.Custodians), len(submissions), missingFrom(params, submissions))
	}

	custodians := make([]identity, 0, len(submissions))
	latest := time.Time{}
	for _, s := range submissions {
		id, err := verifySubmission(params, s)
		if err != nil {
			return assembled{}, err
		}
		custodians = append(custodians, id)

		generated, err := time.Parse(time.RFC3339, id.GeneratedAt)
		if err != nil {
			return assembled{}, fmt.Errorf("%s's submission has an unreadable generated_at %q: %w", id.Name, id.GeneratedAt, err)
		}
		if generated.After(latest) {
			latest = generated
		}
	}

	votingPeriod, err := params.votingPeriod()
	if err != nil {
		return assembled{}, err
	}

	// buildGroup sorts by address and rejects duplicates, derived accounts and
	// non-custodian roles. Called rather than reimplemented: the co-located path
	// and the distributed one must produce the same group from the same five
	// keys, and two builders would eventually stop doing that.
	documents, err := buildGroup(custodians, purposeFor(params), params.Threshold, votingPeriod, params.PolicySeq, latest.UTC())
	if err != nil {
		return assembled{}, err
	}

	sort.Slice(custodians, func(i, j int) bool { return custodians[i].Address < custodians[j].Address })

	result := assembled{
		Params:        params,
		Custodians:    custodians,
		PolicyAddress: documents.policyAddr,
		Genesis:       json.RawMessage(documents.genesis),
		Constitution:  json.RawMessage(documents.constitution),
		Members:       json.RawMessage(documents.members),
		Policy:        json.RawMessage(documents.policy),
		CreateMsg:     json.RawMessage(documents.msg),
		ComputedAt:    latest.UTC().Truncate(time.Second).Format(time.RFC3339),
	}
	result.Fingerprint = longDigest(groupDomain, result.canonical())
	return result, nil
}

// canonical is what the group fingerprint is taken over.
//
// It includes the raw genesis fragment, not a summary of it. The value the
// custodians compare has to certify the bytes that will be spliced into the file
// that launches the chain; a fingerprint over the membership alone would agree
// across five machines that had each computed a different genesis.
func (a assembled) canonical() []byte {
	b := canonField(nil, groupDomain)
	b = canonBytes(b, a.Params.canonical())
	b = canonField(b, a.PolicyAddress)
	b = canonField(b, a.ComputedAt)
	b = binary.BigEndian.AppendUint32(b, uint32(len(a.Custodians)))
	for _, custodian := range a.Custodians {
		b = canonField(b, custodian.Address)
		b = canonField(b, custodian.PubKey.Key)
		b = canonField(b, custodian.Name)
	}
	b = canonBytes(b, a.Genesis)
	b = canonBytes(b, a.Constitution)
	return b
}

// presence returns where a custodian's own address sits in the assembled group.
//
// This is the substitution check, and it is the one new risk a distributed
// ceremony introduces. It exists because the co-located ceremony had the room:
// five people watched the same screen print the same five fingerprints. Remove
// that and a custodian's own key could simply be left out of the group they are
// about to attest to, and nothing they had seen so far would say so.
func (a assembled) presence(address, fingerprint string) error {
	for i, custodian := range a.Custodians {
		if custodian.Address != address {
			continue
		}
		if custodian.Fingerprint != fingerprint {
			return fmt.Errorf(
				"your address is in the group at position %d but under fingerprint %s, and your key's fingerprint is %s. Those cannot both be true; stop and say so on the call",
				i+1, custodian.Fingerprint, fingerprint)
		}
		return nil
	}
	return fmt.Errorf(
		"YOUR KEY IS NOT IN THIS GROUP. %s does not appear among the %d custodians. "+
			"Do not attest to it. Whoever assembled the material you were given left you out or replaced you, "+
			"and the group they are proposing is not the one this ceremony agreed",
		address, len(a.Custodians))
}

// missingFrom names who has not been received.
//
// A relay can withhold or delay and that is tolerable, because it is visible —
// but only if the interface says which submissions are absent. A progress
// spinner would make a relay stalling one custodian indistinguishable from a
// slow email.
func missingFrom(params ceremonyParams, submissions []submission) string {
	present := map[string]bool{}
	for _, s := range submissions {
		present[strings.TrimSpace(s.Identity.Name)] = true
	}
	var missing []string
	for _, name := range params.Custodians {
		if !present[strings.TrimSpace(name)] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return "Every name on the roster has a submission."
	}
	return "Still missing: " + strings.Join(missing, ", ") + "."
}

// attestation is what one custodian signs at the end.
//
// The record of a distributed ceremony is these, and nothing else. There is no
// sheet everybody signs because there is no table to put it on, and there is no
// summary anybody could sign on behalf of the group — each statement is signed
// by the key it is about, over the fingerprint of the group that custodian
// computed on their own machine. Nobody attests to a group they did not compute.
type attestation struct {
	CeremonyID string `json:"ceremony_id"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	// GroupFingerprint is what makes this an attestation rather than a
	// signature. It binds the statement to one specific set of five keys and one
	// specific genesis fragment, so an attestation cannot be carried over to a
	// group the custodian never saw.
	GroupFingerprint      string `json:"group_fingerprint"`
	PolicyAddress         string `json:"policy_address"`
	TranscriptionVerified bool   `json:"transcription_verified"`
	RestoreDrillPassed    bool   `json:"restore_drill_passed"`
	EnvelopeSealed        bool   `json:"envelope_sealed"`
	// Virtualised records that the machine looked like a guest. An attestation
	// that hid it would be worse than one that admits it: a reader years from
	// now needs to know which of the five keys was generated somewhere a
	// hypervisor operator could have snapshotted memory.
	Virtualised bool   `json:"virtualised"`
	SignedAt    string `json:"signed_at"`
}

func (a attestation) canonical() []byte {
	b := canonField(nil, attestationDomain)
	b = canonField(b, a.CeremonyID)
	b = canonField(b, a.Name)
	b = canonField(b, a.Address)
	b = canonField(b, a.GroupFingerprint)
	b = canonField(b, a.PolicyAddress)
	b = canonField(b, fmt.Sprintf("%t", a.TranscriptionVerified))
	b = canonField(b, fmt.Sprintf("%t", a.RestoreDrillPassed))
	b = canonField(b, fmt.Sprintf("%t", a.EnvelopeSealed))
	b = canonField(b, fmt.Sprintf("%t", a.Virtualised))
	b = canonField(b, a.SignedAt)
	return b
}

// signedAttestation is the file a custodian sends at the end of the ceremony.
type signedAttestation struct {
	Attestation attestation `json:"attestation"`
	PubKey      pubKeyJSON  `json:"pubkey"`
	Signature   string      `json:"signature"`
}

func signAttestation(a attestation, priv *secp256k1.PrivKey) (signedAttestation, error) {
	signature, err := priv.Sign(a.canonical())
	if err != nil {
		return signedAttestation{}, err
	}
	pub, ok := priv.PubKey().(*secp256k1.PubKey)
	if !ok {
		return signedAttestation{}, errors.New("the chain's key generator returned an unexpected public key type")
	}
	return signedAttestation{
		Attestation: a,
		PubKey:      pubKeyJSON{Type: secp256k1PubKeyType, Key: base64.StdEncoding.EncodeToString(pub.Bytes())},
		Signature:   base64.StdEncoding.EncodeToString(signature),
	}, nil
}

// verifyAttestation checks a statement somebody else signed.
//
// The address is re-derived from the public key in the file, then compared with
// the address the statement claims — so an attestation cannot be signed by one
// key and filed under another custodian's address.
func verifyAttestation(s signedAttestation) error {
	raw, err := base64.StdEncoding.DecodeString(s.PubKey.Key)
	if err != nil {
		return fmt.Errorf("%s's attestation has an unreadable public key: %w", s.Attestation.Name, err)
	}
	if len(raw) != secp256k1.PubKeySize {
		return fmt.Errorf("%s's attestation carries a %d-byte public key", s.Attestation.Name, len(raw))
	}
	pub := &secp256k1.PubKey{Key: raw}

	derived, err := identityFromPubKey(s.Attestation.Name, roleCustodian, pub, "", time.Now())
	if err != nil {
		return err
	}
	if derived.Address != s.Attestation.Address {
		return fmt.Errorf(
			"%s's attestation is about address %s but is signed by the key for %s",
			s.Attestation.Name, s.Attestation.Address, derived.Address)
	}

	signature, err := base64.StdEncoding.DecodeString(s.Signature)
	if err != nil {
		return fmt.Errorf("%s's attestation signature is not base64: %w", s.Attestation.Name, err)
	}
	if !pub.VerifySignature(s.Attestation.canonical(), signature) {
		return fmt.Errorf("%s's attestation signature does not verify", s.Attestation.Name)
	}
	return nil
}

// checkGenesis verifies that a built genesis file carries the group this
// instance computed.
//
// The fingerprint the five custodians agreed covers the genesis FRAGMENT. It
// does not cover the file somebody assembles afterwards, and between those two
// things is a person with a text editor. So the last check of the ceremony is
// this one: the group in the file that will actually launch the chain is
// byte-identical to the group in the fragment, and the recovery destination in
// both the places that must agree is the policy address.
//
// Compared as decoded JSON rather than as text, because splicing a fragment into
// a genesis file legitimately reindents it.
func checkGenesis(a assembled, genesisFile []byte) error {
	var file struct {
		ChainID  string `json:"chain_id"`
		AppState struct {
			Group        json.RawMessage `json:"group"`
			Constitution struct {
				Invariants struct {
					RecoveryDestination string `json:"enforcement_recovery_destination"`
					CustodianCount      any    `json:"foundation_custodian_count"`
					SignatureThreshold  any    `json:"foundation_signature_threshold"`
				} `json:"invariants"`
			} `json:"constitution"`
			Enforcement struct {
				Params struct {
					RecoveryDestination string `json:"recovery_destination"`
				} `json:"params"`
			} `json:"enforcement"`
		} `json:"app_state"`
	}
	if err := json.Unmarshal(genesisFile, &file); err != nil {
		return fmt.Errorf("that is not a genesis file this can read: %w", err)
	}

	if file.ChainID != a.Params.ChainID {
		return fmt.Errorf(
			"this genesis is for chain %q and the ceremony agreed %q",
			file.ChainID, a.Params.ChainID)
	}
	if len(file.AppState.Group) == 0 {
		return errors.New("this genesis has no app_state.group at all, so the foundation group is not in it and the chain would start with the recovery destination pointing at an account nobody controls yet")
	}

	equal, err := sameJSON(file.AppState.Group, a.Genesis)
	if err != nil {
		return err
	}
	if !equal {
		return fmt.Errorf(
			"the group in this genesis is NOT the group this ceremony computed. Fingerprint %s was agreed; whatever is in app_state.group is something else. Do not launch this file",
			a.Fingerprint)
	}

	if got := file.AppState.Constitution.Invariants.RecoveryDestination; got != a.PolicyAddress {
		return fmt.Errorf(
			"app_state.constitution.invariants.enforcement_recovery_destination is %q; the ceremony's policy address is %s",
			got, a.PolicyAddress)
	}
	if got := file.AppState.Enforcement.Params.RecoveryDestination; got != a.PolicyAddress {
		return fmt.Errorf(
			"app_state.enforcement.params.recovery_destination is %q; the ceremony's policy address is %s. "+
				"These two have to agree with each other and with the group, and a chain will start perfectly happily when they do not",
			got, a.PolicyAddress)
	}
	if got := asInt(file.AppState.Constitution.Invariants.CustodianCount); got != len(a.Custodians) {
		return fmt.Errorf("the constitution says %d custodians; the group has %d", got, len(a.Custodians))
	}
	if got := asInt(file.AppState.Constitution.Invariants.SignatureThreshold); got != a.Params.Threshold {
		return fmt.Errorf("the constitution says a threshold of %d; the ceremony agreed %d", got, a.Params.Threshold)
	}
	return nil
}

// asInt reads a constitution invariant that may be a JSON number or, because
// the chain's uint64 fields are serialised as strings, a quoted one.
func asInt(v any) int {
	switch typed := v.(type) {
	case float64:
		return int(typed)
	case string:
		var n int
		if _, err := fmt.Sscanf(typed, "%d", &n); err != nil {
			return -1
		}
		return n
	}
	return -1
}

// sameJSON compares two documents by value.
func sameJSON(a, b []byte) (bool, error) {
	var left, right any
	if err := json.Unmarshal(a, &left); err != nil {
		return false, fmt.Errorf("the group section of that genesis is not valid JSON: %w", err)
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false, err
	}
	canonicalLeft, err := json.Marshal(left)
	if err != nil {
		return false, err
	}
	canonicalRight, err := json.Marshal(right)
	if err != nil {
		return false, err
	}
	return string(canonicalLeft) == string(canonicalRight), nil
}
