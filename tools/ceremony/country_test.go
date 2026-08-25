package main

// Tests for the country enrolment ceremony.
//
// Read the confirm tests as a group. Individually they look like a list of ways to
// reject a JSON file; together they are the statement that there is no input on
// which this tool composes a grant naming an address it has not verified belongs
// to the office it is granting to. That is the only property here that would cost
// somebody a country's payments authority, and the failure mode of the whole
// design is not being too strict.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/x/group"
	"github.com/stretchr/testify/require"

	aliastypes "yamale/blockchain/x/alias/types"
)

// ---------------------------------------------------------------- fixtures

// officeKey is one super user: a fresh key and the private half to sign with.
//
// Fresh rather than drawn from the shared vectors, because these tests assert
// behaviour rather than bytes — the vectors exist to pin the cross-language
// encodings and borrowing their five phrases here would cap an office at five
// members and make two offices share keys, which is the one arrangement
// dossierFor refuses.
func officeKey(t *testing.T, name string, at time.Time) (identity, *secp256k1.PrivKey) {
	t.Helper()
	configureAddresses()

	s, err := newSecret()
	require.NoError(t, err)
	defer s.zero()

	priv, path, err := s.derive(0)
	require.NoError(t, err)
	id, err := identityOf(name, roleCustodian, priv, path, at)
	require.NoError(t, err)
	return id, priv
}

// officeFixture is one office's ceremony, as it would come off the hosted flow.
type officeFixture struct {
	Params      ceremonyParams
	Submissions []submission
	Assembled   assembled
	Keys        []identity
	Path        string
}

// superUser is one office member's identity and the key to sign a submission
// with.
//
// The private half is kept rather than zeroed on the way out, unlike the
// production path, because a test that needs the SAME person in two offices has to
// sign for them twice. These are throwaway keys generated in-process, and
// officeKey has already zeroed the phrase they came from.
type superUser struct {
	ID   identity
	Priv *secp256k1.PrivKey
}

func newSuperUser(t *testing.T, name string, at time.Time) superUser {
	t.Helper()
	id, priv := officeKey(t, name, at)
	return superUser{ID: id, Priv: priv}
}

// newOffice builds an office ceremony from fresh keys and writes its group file.
func newOffice(t *testing.T, dir, name, chainID, country string, roles []string, members []string, threshold int) officeFixture {
	t.Helper()
	at := time.Unix(1780000000, 0).UTC().Truncate(time.Second)
	people := make([]superUser, 0, len(members))
	for _, member := range members {
		people = append(people, newSuperUser(t, member, at))
	}
	return newOfficeFrom(t, dir, name, chainID, country, roles, people, threshold)
}

// newOfficeFrom builds an office ceremony from keys the caller already holds.
//
// Separate from newOffice so a test can put one person in two offices, which is
// the arrangement dossierFor refuses and which cannot be constructed by reusing a
// group FILE: a possession signature is bound to the ceremony id, so copying one
// office's submissions under another's parameters fails verification. The only way
// to build the clash is to sign twice — which is exactly what a coordinator
// running two ceremonies with one name on both rosters would produce.
func newOfficeFrom(
	t *testing.T, dir, name, chainID, country string, roles []string, people []superUser, threshold int,
) officeFixture {
	t.Helper()
	configureAddresses()

	id, err := newCeremonyID()
	require.NoError(t, err)

	members := make([]string, len(people))
	for i, person := range people {
		members[i] = person.ID.Name
	}

	params := ceremonyParams{
		ID:           id,
		Name:         name,
		ChainID:      chainID,
		Threshold:    threshold,
		Custodians:   members,
		PolicySeq:    1,
		VotingPeriod: "168h0m0s",
	}
	if country != "" {
		params.Office = &officeParams{Country: country, Roles: roles}
	}
	require.NoError(t, params.validate(), "the fixture's own parameters are invalid")

	submissions := make([]submission, 0, len(people))
	keys := make([]identity, 0, len(people))
	for _, person := range people {
		sub, err := signSubmission(id, person.ID, person.Priv)
		require.NoError(t, err)
		submissions = append(submissions, sub)
		keys = append(keys, person.ID)
	}

	built, err := assembleGroup(params, submissions)
	require.NoError(t, err)

	path := filepath.Join(dir, fmt.Sprintf("group-%s.json", slug(name)))
	writeOfficeFile(t, path, params, submissions, built)

	return officeFixture{Params: params, Submissions: submissions, Assembled: built, Keys: keys, Path: path}
}

// writeOfficeFile writes group.json the way the hosted ceremony's export does:
// the assembled document, plus the submissions it was computed from.
func writeOfficeFile(t *testing.T, path string, params ceremonyParams, submissions []submission, built assembled) {
	t.Helper()
	encoded, err := json.MarshalIndent(struct {
		assembled
		Submissions []submission `json:"submissions"`
	}{built, submissions}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(encoded, '\n'), 0o644))
	_ = params
}

const (
	fixtureChain   = "yamale-enrol-1"
	fixtureCountry = "SN"
)

// fixtureFoundation is a stand-in for the 3-of-5's policy address.
//
// A real group policy address rather than a made-up string, because half of what
// the tool does with it is decode it.
func fixtureFoundation(t *testing.T) string {
	t.Helper()
	configureAddresses()
	address, err := policyAddress(1)
	require.NoError(t, err)
	return address
}

// payingOffice is the common case: a payments-and-enforcement office, 2 of 3.
func payingOffice(t *testing.T, dir string) officeFixture {
	t.Helper()
	return newOffice(t, dir, "Senegal payments authority", fixtureChain, fixtureCountry,
		[]string{"ROLE_PAYMENTS_AUTHORITY", "ROLE_ENFORCEMENT_AUTHORITY"},
		[]string{"A. Diallo", "B. Sow", "C. Fall"}, 2)
}

func configFor(t *testing.T, office officeFixture, roles []string) countryConfig {
	t.Helper()
	return countryConfig{
		Ceremony:   "Senegal enrolment",
		ChainID:    fixtureChain,
		Country:    fixtureCountry,
		Foundation: fixtureFoundation(t),
		Offices: []officeConfig{{
			Name:    office.Params.Name,
			Roles:   roles,
			Group:   office.Path,
			Minimum: fixtureMinimum(),
		}},
	}
}

func paymentsRoles() []string {
	return []string{"ROLE_PAYMENTS_AUTHORITY", "ROLE_ENFORCEMENT_AUTHORITY"}
}

// fixtureMinimum is the required shape every fixture office exactly meets.
//
// Two-of-three, because that is what the fixtures generate. Exactly meeting it
// rather than exceeding it is deliberate: a fixture whose office was larger than
// its minimum would pass every test in this file even if the comparison were the
// wrong way round.
func fixtureMinimum() *officeMinimum {
	return &officeMinimum{Signatures: 2, Members: 3}
}

// ---------------------------------------------------------------- the config

func TestTheChainWideScopeIsNotACountry(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)
	config := configFor(t, office, paymentsRoles())
	config.Country = aliastypes.ChainWide

	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "chain-wide")
}

func TestTheFoundationsReservedCodeIsNotACountry(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)
	config := configFor(t, office, paymentsRoles())
	config.Country = aliastypes.FoundationCountry

	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "absence of a national perimeter")
}

// NX and QK are two letters and neither is a country. A shape check would pass
// them, and the perimeter they named would be one no authority holds.
func TestAnUnassignedCodeIsRefused(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)
	for _, code := range []string{"NX", "QK", "ZX", "S", "SEN", ""} {
		config := configFor(t, office, paymentsRoles())
		config.Country = code
		_, err := dossierFor(config, time.Now())
		require.Errorf(t, err, "%q was accepted as a country", code)
	}
}

func TestARoleThisChainDoesNotHaveIsRefused(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)
	config := configFor(t, office, []string{"ROLE_PAYMENTS_AUTHORITY", "ROLE_TREASURY_AUTHORITY"})

	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "ROLE_TREASURY_AUTHORITY")
}

// The zero value is spellable, so it has to be refused rather than merely
// reserved. A config naming it would produce a proposal three custodians vote for
// and the chain then rejects — after the vote.
func TestTheUnsetRoleIsRefusedInAConfig(t *testing.T) {
	_, err := rolesOf([]string{"ROLE_UNSPECIFIED"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unset default")
}

func TestARepeatedRoleIsRefused(t *testing.T) {
	_, err := rolesOf([]string{"ROLE_SUPERVISOR", "ROLE_SUPERVISOR"})
	require.Error(t, err)
}

func TestNoRolesAtAllIsRefused(t *testing.T) {
	_, err := rolesOf(nil)
	require.Error(t, err)
}

// rolesOf sorts, so the artefacts depend on the set of roles and not on the order
// somebody listed them in.
func TestRolesAreSortedRegardlessOfHowTheyWereListed(t *testing.T) {
	first, err := rolesOf([]string{"ROLE_SUPERVISOR", "ROLE_PAYMENTS_AUTHORITY"})
	require.NoError(t, err)
	second, err := rolesOf([]string{"ROLE_PAYMENTS_AUTHORITY", "ROLE_SUPERVISOR"})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, []aliastypes.Role{
		aliastypes.ROLE_PAYMENTS_AUTHORITY, aliastypes.ROLE_SUPERVISOR,
	}, first)
}

// A payments country needs both roles. Enrolled without the first it is inert;
// without the second money moves in a perimeter nobody can stop.
func TestAPaymentsCountryNeedsBothRolesOrAWrittenReason(t *testing.T) {
	dir := t.TempDir()
	office := newOffice(t, dir, "Senegal payments authority", fixtureChain, fixtureCountry,
		[]string{"ROLE_PAYMENTS_AUTHORITY"},
		[]string{"A. Diallo", "B. Sow", "C. Fall"}, 2)

	config := configFor(t, office, []string{"ROLE_PAYMENTS_AUTHORITY"})
	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "ROLE_ENFORCEMENT_AUTHORITY")

	// A waiver with a reason gets past it, and the reason travels to the record.
	config.Waivers = []waiver{{Rule: waivePaymentsMinimum, Reason: "registry-only pilot, agreed with the ministry"}}
	dossier, err := dossierFor(config, time.Now())
	require.NoError(t, err)
	require.Len(t, dossier.Waivers, 1)
	require.Equal(t, "registry-only pilot, agreed with the ministry", dossier.Waivers[0].Reason)
}

// A waiver with no reason is not a waiver. The reason is the whole mechanism.
func TestAWaiverWithoutAReasonIsRefused(t *testing.T) {
	dir := t.TempDir()
	office := newOffice(t, dir, "Senegal registry", fixtureChain, fixtureCountry,
		[]string{"ROLE_REGISTRY_AUTHORITY"},
		[]string{"A. Diallo", "B. Sow", "C. Fall"}, 2)

	config := configFor(t, office, []string{"ROLE_REGISTRY_AUTHORITY"})
	config.Waivers = []waiver{{Rule: waivePaymentsMinimum, Reason: "   "}}
	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no reason")
}

// A waiver naming nothing reads on the record as though it covered something.
func TestAWaiverOfAnUnknownRuleIsRefused(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)
	config := configFor(t, office, paymentsRoles())
	config.Waivers = []waiver{{Rule: "payments_minimum", Reason: "typo in the rule name"}}
	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a rule")
}

// ------------------------------------------------------------ the minimum

// The requirement is decided in advance, so a config that names none is refused.
//
// Not defaulted, and the difference matters: a default of "no minimum" would
// silently reproduce the state this field exists to end, which is an office that
// can vote itself to a single key and go on holding a national authority.
func TestAnOfficeWithNoMinimumIsRefused(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)
	config := configFor(t, office, paymentsRoles())
	config.Offices[0].Minimum = nil

	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no minimum")
	require.Contains(t, err.Error(), `"signatures": 3`,
		"the refusal has to say what to write, or it is a puzzle rather than a message")
}

// A minimum that is not a workable M-of-N is refused before any key exists.
func TestAMinimumThatIsNotAShapeIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		minimum officeMinimum
		says    string
	}{
		{"one signature is a single key", officeMinimum{Signatures: 1, Members: 3}, "permits a single key"},
		{"no signatures at all", officeMinimum{Signatures: 0, Members: 3}, "permits a single key"},
		{"more signatures than members", officeMinimum{Signatures: 4, Members: 3}, "no office could satisfy"},
		{"unanimity freezes on one loss", officeMinimum{Signatures: 3, Members: 3}, "is unanimity"},
		{
			"beyond what the chain can read",
			officeMinimum{Signatures: 3, Members: aliastypes.MaxOfficeMembers + 1},
			"members the chain can read",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			office := payingOffice(t, dir)
			config := configFor(t, office, paymentsRoles())
			config.Offices[0].Minimum = &tc.minimum

			_, err := dossierFor(config, time.Now())
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.says)
		})
	}
}

// The check the whole arrangement turns on: a group file that does not reach the
// minimum the country agreed in advance.
//
// The fixture office is a two-of-three, generated and signed for by three people.
// A config demanding more is a config whose signed record and whose chain state
// would disagree about what the office is, and the cheapest moment to find that
// out is here — before any group exists on the chain.
func TestAnOfficeBelowItsMinimumIsRefusedBeforeAnyGroupIsCreated(t *testing.T) {
	for _, tc := range []struct {
		name    string
		minimum officeMinimum
		says    string
	}{
		{"threshold short", officeMinimum{Signatures: 3, Members: 4}, "below the 3 signatures"},
		{"members short", officeMinimum{Signatures: 2, Members: 5}, "below the 5 this enrolment requires"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			office := payingOffice(t, dir)
			config := configFor(t, office, paymentsRoles())
			config.Offices[0].Minimum = &tc.minimum

			_, err := dossierFor(config, time.Now())
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.says)
			require.Contains(t, err.Error(), "2-of-3",
				"the refusal must name what the group file actually is")
		})
	}
}

// Note the unanimous minimum in the first case above is refused by validate()
// before the group file is read, so this asserts the group-file comparison
// separately — a minimum that is a legitimate shape and that the office does not
// reach.
func TestAnOfficeThatExceedsItsMinimumIsAccepted(t *testing.T) {
	dir := t.TempDir()
	// Three of five, generated for the same country and roles as the fixture.
	office := newOffice(t, dir, "Senegal payments authority", fixtureChain, fixtureCountry,
		paymentsRoles(),
		[]string{"A. Diallo", "B. Sow", "C. Fall", "D. Ba", "E. Ndiaye"}, 3)
	config := configFor(t, office, paymentsRoles())
	config.Offices[0].Minimum = &officeMinimum{Signatures: 2, Members: 3}

	dossier, err := dossierFor(config, time.Now())
	require.NoError(t, err, "an office larger than its minimum is more agreement, not less")
	require.Equal(t, 3, dossier.Offices[0].Threshold)
	require.Equal(t, "2-of-3", dossier.Offices[0].Minimum.rule(),
		"the dossier carries the agreed minimum, not the shape that turned up")
}

// ---------------------------------------------------------------- the dossier

func TestTheDossierTakesMembersAndThresholdFromTheCeremonyNotTheConfig(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)
	dossier, err := dossierFor(configFor(t, office, paymentsRoles()), time.Now())
	require.NoError(t, err)

	require.Len(t, dossier.Offices, 1)
	record := dossier.Offices[0]
	require.Equal(t, 2, record.Threshold)
	require.Len(t, record.Members, 3)
	require.Equal(t, office.Assembled.Fingerprint, record.GroupFingerprint)
	require.Equal(t, office.Params.ID, record.CeremonyID)

	// And no address, which is the point of the whole two-phase arrangement.
	require.Nil(t, record.OnChain)
}

// Pairing one office's name and roles with another office's keys is how an office
// ends up holding authority nobody granted it.
func TestAConfigCannotPairOneOfficesRolesWithAnothersKeys(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)
	config := configFor(t, office, paymentsRoles())
	config.Offices[0].Name = "Senegal lands commission"

	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "not the same office")
}

// The super users compared a fingerprint covering the country. A config naming a
// different one is a config that disagrees with what they signed.
func TestAnOfficeCannotBeGrantedAuthorityInACountryItsSuperUsersDidNotAgreeTo(t *testing.T) {
	dir := t.TempDir()
	office := newOffice(t, dir, "Senegal payments authority", fixtureChain, "SN",
		paymentsRoles(), []string{"A. Diallo", "B. Sow", "C. Fall"}, 2)

	config := configFor(t, office, paymentsRoles())
	config.Country = "NG"

	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "did not agree to this")
}

// Same for the roles: both sets are inside the fingerprint they compared.
func TestAnOfficeCannotBeGrantedRolesItsSuperUsersDidNotAgreeTo(t *testing.T) {
	dir := t.TempDir()
	office := newOffice(t, dir, "Senegal payments authority", fixtureChain, fixtureCountry,
		[]string{"ROLE_PAYMENTS_AUTHORITY", "ROLE_ENFORCEMENT_AUTHORITY"},
		[]string{"A. Diallo", "B. Sow", "C. Fall"}, 2)

	// A superset: the config adds MONETARY_AUTHORITY.
	config := configFor(t, office,
		[]string{"ROLE_PAYMENTS_AUTHORITY", "ROLE_ENFORCEMENT_AUTHORITY", "ROLE_MONETARY_AUTHORITY"})
	_, err := dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "disagrees with what they signed")

	// And a subset, which is the quiet one: a country left half-appointed with a
	// signed record saying otherwise.
	config = configFor(t, office, []string{"ROLE_PAYMENTS_AUTHORITY"})
	config.Waivers = []waiver{{Rule: waivePaymentsMinimum, Reason: "checking the subset case"}}
	_, err = dossierFor(config, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "disagrees with what they signed")
}

// A foundation ceremony's custodians attested to holding the chain's recovery
// destination. Nothing they signed says anything about administering a country.
func TestAFoundationCeremonyCannotBeUsedAsAnOffice(t *testing.T) {
	dir := t.TempDir()
	office := newOffice(t, dir, "Senegal payments authority", fixtureChain, "",
		nil, []string{"A. Diallo", "B. Sow", "C. Fall"}, 2)
	require.Nil(t, office.Params.Office, "the fixture should have produced a foundation ceremony")

	_, err := dossierFor(configFor(t, office, paymentsRoles()), time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "foundation ceremony")
}

// One person with a vote in two of a country's offices is one person the
// separation between those offices does not apply to.
//
// Two differently-named offices sharing one member, which is the only shape that
// tests the check. An earlier version of this test reused one office's group file
// under two names, and it passed for the wrong reason: the names collided after
// trimming, so the duplicate-name check refused it and the shared-member check
// never ran. A mutation pass found that — disabling the clash check left the test
// green.
func TestASuperUserCannotSitInTwoOfficesOfOneCountry(t *testing.T) {
	dir := t.TempDir()
	at := time.Unix(1780000000, 0).UTC()

	shared := newSuperUser(t, "A. Diallo", at)
	payments := newOfficeFrom(t, dir, "Senegal payments authority", fixtureChain, fixtureCountry,
		paymentsRoles(),
		[]superUser{shared, newSuperUser(t, "B. Sow", at), newSuperUser(t, "C. Fall", at)}, 2)

	// The same person, in the lands commission, having signed for that ceremony
	// too — so both group files verify perfectly on their own.
	lands := newOfficeFrom(t, dir, "Senegal lands commission", fixtureChain, fixtureCountry,
		[]string{"ROLE_REGISTRY_AUTHORITY"},
		[]superUser{shared, newSuperUser(t, "D. Ba", at), newSuperUser(t, "E. Ndiaye", at)}, 2)

	// Each file is individually valid. That is the point: nothing about either
	// office looks wrong, and the arrangement is only visible from both at once.
	_, err := readAssembledGroup(payments.Path)
	require.NoError(t, err)
	_, err = readAssembledGroup(lands.Path)
	require.NoError(t, err)

	_, err = dossierFor(countryConfig{
		Ceremony:   "Senegal enrolment",
		ChainID:    fixtureChain,
		Country:    fixtureCountry,
		Foundation: fixtureFoundation(t),
		Offices: []officeConfig{
			{Name: payments.Params.Name, Roles: paymentsRoles(), Group: payments.Path, Minimum: fixtureMinimum()},
			{Name: lands.Params.Name, Roles: []string{"ROLE_REGISTRY_AUTHORITY"}, Group: lands.Path, Minimum: fixtureMinimum()},
		},
	}, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "A. Diallo")
	require.Contains(t, err.Error(), "separation between those offices")

	// Two offices with disjoint members are fine, so the refusal above is about
	// the shared key and not about having two offices at all.
	other := newOfficeFrom(t, dir, "Senegal supervisor", fixtureChain, fixtureCountry,
		[]string{"ROLE_SUPERVISOR"},
		[]superUser{newSuperUser(t, "F. Sarr", at), newSuperUser(t, "G. Cisse", at),
			newSuperUser(t, "H. Toure", at)}, 2)
	dossier, err := dossierFor(countryConfig{
		Ceremony:   "Senegal enrolment",
		ChainID:    fixtureChain,
		Country:    fixtureCountry,
		Foundation: fixtureFoundation(t),
		Offices: []officeConfig{
			{Name: payments.Params.Name, Roles: paymentsRoles(), Group: payments.Path, Minimum: fixtureMinimum()},
			{Name: other.Params.Name, Roles: []string{"ROLE_SUPERVISOR"}, Group: other.Path, Minimum: fixtureMinimum()},
		},
	}, time.Now())
	require.NoError(t, err)
	require.Len(t, dossier.Offices, 2)

	// And a submission cannot simply be replayed from one ceremony into another,
	// which is why the clash has to be built by signing twice.
	replayed := append([]submission(nil), payments.Submissions...)
	_, err = assembleGroup(lands.Params, replayed)
	require.Error(t, err)
}

func TestAGroupFileForAnotherChainIsRefused(t *testing.T) {
	dir := t.TempDir()
	office := newOffice(t, dir, "Senegal payments authority", "yamale-somewhere-else", fixtureCountry,
		paymentsRoles(), []string{"A. Diallo", "B. Sow", "C. Fall"}, 2)

	_, err := dossierFor(configFor(t, office, paymentsRoles()), time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "generated for chain")
}

// The group file is recomputed rather than read, so a file edited after the
// ceremony disagrees with itself.
//
// Two shapes of edit, because they are caught by two different checks and only
// testing one would leave the other unproven:
//
//   - a member ADDED, which makes a 2-of-3 into a 2-of-4. Caught by the count:
//     the roster fixed at the start of the ceremony says three.
//   - a member REPLACED, which keeps the count right. Caught by the roster: the
//     intruder's name is not one of the three agreed, and their possession
//     signature — which is genuine, over their own key — does not help them.
func TestAnEditedGroupFileIsRefused(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)

	at := time.Unix(1780000000, 0).UTC()
	intruder, priv := officeKey(t, "D. Interloper", at)
	sub, err := signSubmission(office.Params.ID, intruder, priv)
	require.NoError(t, err)
	// The intruder's own signature is valid; that is the point. What refuses them
	// is not the cryptography.
	require.NoError(t, func() error {
		_, err := verifySubmission(ceremonyParams{
			ID: office.Params.ID, ChainID: office.Params.ChainID, Threshold: 2,
			Custodians: []string{"D. Interloper", "x", "y"}, VotingPeriod: "168h0m0s",
		}, sub)
		return err
	}(), "the intruder's possession signature should be genuine")

	// Added: four submissions against a roster of three.
	added := append(append([]submission(nil), office.Submissions...), sub)
	_, err = assembleGroup(office.Params, added)
	require.Error(t, err)
	require.Contains(t, err.Error(), "3 custodians and 4 submissions")

	// Replaced: three submissions, one of them a stranger's.
	replaced := append([]submission(nil), office.Submissions...)
	replaced[2] = sub
	_, err = assembleGroup(office.Params, replaced)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not on the roster")

	// And the file on disk still verifies, so the refusals above are about the
	// edits and not about the fixture.
	rebuilt, err := readAssembledGroup(office.Path)
	require.NoError(t, err)
	require.Equal(t, office.Assembled.Fingerprint, rebuilt.Fingerprint)
}

// The policy document has to be readable by the command the runbook tells people
// to run.
//
// `blockchaind tx group create-group-with-policy` unmarshals that file into a
// group.DecisionPolicy, which means through an Any, which means it needs an
// "@type". Emitted with codec.MarshalJSON it has none, and the CLI refuses it with
// "failed to parse decision policy: Any JSON doesn't have '@type'" — which is what
// this file did from the day it was written. Nothing caught it because the
// foundation's launch path splices the genesis fragment instead, and the first
// caller that actually needed the document was a country office.
//
// So this test does what the CLI does, rather than checking for a string.
func TestThePolicyDocumentIsWhatTheCLIReads(t *testing.T) {
	dir := t.TempDir()
	office := payingOffice(t, dir)

	registry := codectypes.NewInterfaceRegistry()
	group.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	var policy group.DecisionPolicy
	require.NoError(t, cdc.UnmarshalInterfaceJSON(office.Assembled.Policy, &policy),
		"the policy document cannot be read the way the group CLI reads it")

	threshold, ok := policy.(*group.ThresholdDecisionPolicy)
	require.True(t, ok, "the ceremony produced a %T", policy)
	require.Equal(t, "2", threshold.Threshold)
	require.Equal(t, 168*time.Hour, threshold.Windows.VotingPeriod)

	// And the members document is the shape the same command reads.
	var members struct {
		Members []group.MemberRequest `json:"members"`
	}
	require.NoError(t, json.Unmarshal(office.Assembled.Members, &members))
	require.Len(t, members.Members, 3)
	for _, member := range members.Members {
		require.Equal(t, "1", member.Weight)
	}
}

// ---------------------------------------------------------------- confirm

// chainAnswers is what a healthy chain would say about a created office group.
func chainAnswers(office officeRecord, address string, groupID uint64) (txResult, policyInfo, groupMembers) {
	tx := txResult{Height: 42, TxHash: "ABCDEF0123456789", Code: 0}
	tx.Events = []struct {
		Type       string `json:"type"`
		Attributes []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"attributes"`
	}{
		{Type: eventCreateGroup, Attributes: attrs("group_id", fmt.Sprintf("%d", groupID))},
		{Type: eventCreateGroupPolicy, Attributes: attrs("address", address)},
	}

	var policy policyInfo
	policy.Info.Address = address
	policy.Info.GroupID = flexUint64(groupID)
	policy.Info.Admin = address
	policy.Info.Version = 1
	policy.Info.DecisionPolicy.Type = thresholdDecisionPolicyType
	policy.Info.DecisionPolicy.Threshold = fmt.Sprintf("%d", office.Threshold)

	var members groupMembers
	for _, member := range office.Members {
		entry := struct {
			GroupID flexUint64 `json:"group_id"`
			Member  struct {
				Address  string `json:"address"`
				Weight   string `json:"weight"`
				Metadata string `json:"metadata"`
			} `json:"member"`
		}{GroupID: flexUint64(groupID)}
		entry.Member.Address = member.Address
		entry.Member.Weight = "1"
		members.Members = append(members.Members, entry)
	}
	return tx, policy, members
}

func attrs(key, value string) []struct {
	Key   string `json:"key"`
	Value string `json:"value"`
} {
	quoted, _ := json.Marshal(value)
	return []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}{{Key: key, Value: string(quoted)}}
}

func confirmFixture(t *testing.T) (countryDossier, *officeRecord, string) {
	t.Helper()
	dir := t.TempDir()
	office := payingOffice(t, dir)
	dossier, err := dossierFor(configFor(t, office, paymentsRoles()), time.Now())
	require.NoError(t, err)

	// An address that is emphatically NOT the one policyAddress() would predict
	// for this ceremony's policy_seq, so a test that accidentally passed because
	// the two agreed is not possible.
	address, err := policyAddress(97)
	require.NoError(t, err)
	predicted, err := policyAddress(office.Params.PolicySeq)
	require.NoError(t, err)
	require.NotEqual(t, predicted, address)

	return dossier, &dossier.Offices[0], address
}

func TestConfirmAcceptsTheOfficesOwnGroup(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)

	confirmed, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.NoError(t, err)
	require.Equal(t, address, confirmed.PolicyAddress)
	require.Equal(t, uint64(7), confirmed.GroupID)
	require.Equal(t, int64(42), confirmed.Height)
	require.Equal(t, "ABCDEF0123456789", confirmed.TxHash)
}

// The check the whole two-phase design comes down to. A group with an extra
// member is a 2-of-4 wearing a 2-of-3's address.
func TestConfirmRefusesAGroupWithAMemberTheOfficeDoesNotHave(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)

	stranger, _ := officeKey(t, "Stranger", time.Unix(1780000000, 0).UTC())
	extra := members.Members[0]
	extra.Member.Address = stranger.Address
	members.Members = append(members.Members, extra)

	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "Do not grant anything to this address")
}

// And one missing. A 2-of-2 concentrates more authority in whoever remains than
// the ceremony gave them.
func TestConfirmRefusesAGroupMissingOneOfTheOfficesMembers(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	members.Members = members.Members[:len(members.Members)-1]

	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
}

// Entirely somebody else's group at an address that looks perfectly good. This is
// the attack a predicted address would have walked into.
func TestConfirmRefusesAStrangersGroupPolicy(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)

	for i := range members.Members {
		stranger, _ := officeKey(t, fmt.Sprintf("Stranger %d", i), time.Unix(1780000000, 0).UTC())
		members.Members[i].Member.Address = stranger.Address
	}

	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
}

func TestConfirmRefusesAWeightedMember(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	members.Members[0].Member.Weight = "2"

	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "weight")
}

// An admin outside the group can rewrite the membership, so the threshold is
// advisory and the grant would still go to the policy address.
func TestConfirmRefusesAGroupThatIsNotItsOwnAdmin(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	outsider, err := policyAddress(500)
	require.NoError(t, err)
	policy.Info.Admin = outsider

	_, err = confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "administered by")
}

func TestConfirmRefusesADifferentThreshold(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	policy.Info.DecisionPolicy.Threshold = "1"

	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "attested to")
}

// A percentage over a membership that can change is a threshold that changes with
// it.
func TestConfirmRefusesAPercentageDecisionPolicy(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	policy.Info.DecisionPolicy.Type = "/cosmos.group.v1.PercentageDecisionPolicy"

	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
}

func TestConfirmRefusesAPolicyForADifferentAddress(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	elsewhere, err := policyAddress(600)
	require.NoError(t, err)
	policy.Info.Address = elsewhere

	_, err = confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "different")
}

func TestConfirmRefusesAMismatchedGroupID(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	policy.Info.GroupID = 8

	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
}

func TestConfirmRefusesATransactionThatCreatedNoGroupPolicy(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	tx.Events = tx.Events[:1] // the group event, not the policy event

	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "created no group policy")
}

// An office confirmed at the foundation's own address would let the next phase
// compose a grant of a national role to the account that signs the grant.
func TestConfirmRefusesTheFoundationsOwnAddress(t *testing.T) {
	dossier, office, _ := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, dossier.Foundation, 7)

	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "foundation")
}

// A node renders a protobuf Any two ways depending on which door you knocked on,
// and both have to be readable.
//
// The exact documents below were copied from a running chain: the first is what
// `blockchaind query group group-policy-info -o json` prints, the second what the
// REST gateway returns for the same policy. A tool that read only one of them
// would tell an operator with a perfectly good office that it does not match.
func TestBothWireShapesOfADecisionPolicyAreRead(t *testing.T) {
	cli := []byte(`{
	  "type": "/cosmos.group.v1.ThresholdDecisionPolicy",
	  "value": {"threshold": "2", "windows": {"voting_period": "168h0m0s", "min_execution_period": "0s"}}
	}`)
	rest := []byte(`{
	  "@type": "/cosmos.group.v1.ThresholdDecisionPolicy",
	  "threshold": "2",
	  "windows": {"voting_period": "604800s", "min_execution_period": "0s"}
	}`)

	for name, raw := range map[string][]byte{"cli": cli, "rest": rest} {
		var policy decisionPolicyDoc
		require.NoErrorf(t, json.Unmarshal(raw, &policy), "%s form", name)
		require.Equalf(t, thresholdDecisionPolicyType, policy.Type, "%s form", name)
		require.Equalf(t, "2", policy.Threshold, "%s form", name)
		require.NotEmptyf(t, policy.VotingPeriod, "%s form", name)
	}

	// Neither key present leaves the type empty, and confirmOffice refuses an
	// empty type. That is the direction this has to fail in: a threshold read out
	// of a document whose policy shape is unknown is a threshold that can move.
	var absent decisionPolicyDoc
	require.NoError(t, json.Unmarshal([]byte(`{"threshold":"2"}`), &absent))
	require.Empty(t, absent.Type)

	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	policy.Info.DecisionPolicy = absent
	_, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.Error(t, err)

	// Two different type URLs in one document is refused rather than resolved.
	var conflicting decisionPolicyDoc
	err = json.Unmarshal([]byte(
		`{"@type":"/cosmos.group.v1.ThresholdDecisionPolicy","type":"/cosmos.group.v1.PercentageDecisionPolicy"}`),
		&conflicting)
	require.Error(t, err)
	require.Contains(t, err.Error(), "two different types")
}

// ---------------------------------------------------------------- tx results

// A broadcast response has height 0 and a code of 0 means "accepted into a
// mempool". Four bugs in this project came from reading it as "executed".
func TestABroadcastResponseIsNotEvidenceOfAnything(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broadcast.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"height":"0","txhash":"ABC","code":0,"raw_log":""}`), 0o644))

	_, err := readTxResult(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "BROADCAST response")
	require.Contains(t, err.Error(), "query tx")
}

func TestAFailedTransactionIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "failed.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"height":"12","txhash":"ABC","code":18,"codespace":"sdk","raw_log":"invalid request"}`), 0o644))

	_, err := readTxResult(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "code 18")
}

// Protobuf JSON writes int64 as a string and uint32 as a number, so a document
// mixing heights and codes mixes both forms. A parser that read zero for the
// wrong one would turn a failed transaction into a successful one.
func TestHeightsAndCodesAreReadInEitherJSONForm(t *testing.T) {
	dir := t.TempDir()
	for i, body := range []string{
		`{"height":"12","txhash":"A","code":0}`,
		`{"height":12,"txhash":"A","code":"0"}`,
	} {
		path := filepath.Join(dir, fmt.Sprintf("tx%d.json", i))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		result, err := readTxResult(path)
		require.NoErrorf(t, err, "form %d", i)
		require.Equal(t, int64(12), int64(result.Height))
	}

	// And a non-zero code in either form is still a failure.
	for i, body := range []string{
		`{"height":"12","txhash":"A","code":5}`,
		`{"height":"12","txhash":"A","code":"5"}`,
	} {
		path := filepath.Join(dir, fmt.Sprintf("bad%d.json", i))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		_, err := readTxResult(path)
		require.Errorf(t, err, "form %d was read as a success", i)
	}
}

// Typed event attribute values arrive JSON-encoded, so a string comes wrapped in
// quotes. Read with the quotes still on it, an address compares unequal to itself.
func TestEventAttributesAreUnwrapped(t *testing.T) {
	tx := txResult{}
	tx.Events = []struct {
		Type       string `json:"type"`
		Attributes []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"attributes"`
	}{{Type: eventCreateGroupPolicy, Attributes: attrs("address", "yml1whatever")}}

	value, found := tx.attribute(eventCreateGroupPolicy, "address")
	require.True(t, found)
	require.Equal(t, "yml1whatever", value)
}

// ---------------------------------------------------------------- the proposal

// The refusal the two-phase design exists to make.
func TestNoGrantIsComposedForAnOfficeWithNoConfirmedAddress(t *testing.T) {
	dossier, office, _ := confirmFixture(t)
	require.Nil(t, office.OnChain)

	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())
	_, err := enrolmentProposal(dossier, custodian.Address)
	require.Error(t, err)
	require.Contains(t, err.Error(), "has not been read back from the chain")
	require.Contains(t, err.Error(), "will not guess one")
}

// The tool must never fall back to a derived address for an office. This is the
// assertion that the whole rest of the file is arranged to protect.
func TestTheProposalNeverNamesAPredictedPolicyAddress(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	confirmed, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.NoError(t, err)
	office.OnChain = &confirmed

	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())
	document, err := enrolmentProposal(dossier, custodian.Address)
	require.NoError(t, err)

	// Every address policyAddress() could have produced for a small sequence
	// number is absent from the document, and the one the chain actually gave is
	// present.
	body := string(document)
	require.Contains(t, body, address)
	for seq := uint64(0); seq < 20; seq++ {
		predicted, err := policyAddress(seq)
		require.NoError(t, err)
		if predicted == address || predicted == dossier.Foundation {
			continue
		}
		require.NotContainsf(t, body, predicted,
			"the proposal names policy address for sequence %d, which nobody read off a chain", seq)
	}
}

func TestTheProposalCarriesAPlacementAndAGrantPerRole(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	confirmed, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.NoError(t, err)
	office.OnChain = &confirmed

	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())
	document, err := enrolmentProposal(dossier, custodian.Address)
	require.NoError(t, err)

	var parsed struct {
		GroupPolicyAddress string            `json:"group_policy_address"`
		Messages           []json.RawMessage `json:"messages"`
		Proposers          []string          `json:"proposers"`
		Title              string            `json:"title"`
		Summary            string            `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(document, &parsed))

	require.Equal(t, dossier.Foundation, parsed.GroupPolicyAddress,
		"the enrolment is the foundation's act, so the proposal belongs to its group")
	require.Equal(t, []string{custodian.Address}, parsed.Proposers)
	// One placement plus one grant per role.
	require.Len(t, parsed.Messages, 3)

	types := make([]string, 0, len(parsed.Messages))
	for _, raw := range parsed.Messages {
		var envelope struct {
			Type         string `json:"@type"`
			Authority    string `json:"authority"`
			Recorder     string `json:"recorder"`
			Holder       string `json:"holder"`
			Account      string `json:"account"`
			Role         string `json:"role"`
			Jurisdiction string `json:"jurisdiction"`
			Country      string `json:"country"`
		}
		require.NoError(t, json.Unmarshal(raw, &envelope))
		types = append(types, envelope.Type)

		switch envelope.Type {
		case "/blockchain.alias.v1.MsgSetJurisdiction":
			require.Equal(t, dossier.Foundation, envelope.Recorder)
			require.Equal(t, address, envelope.Account)
			require.Equal(t, fixtureCountry, envelope.Country)
		case "/blockchain.alias.v1.MsgGrantRole":
			require.Equal(t, dossier.Foundation, envelope.Authority)
			require.Equal(t, address, envelope.Holder)
			require.Equal(t, fixtureCountry, envelope.Jurisdiction)
			require.NotEqual(t, aliastypes.ChainWide, envelope.Jurisdiction)
		default:
			t.Fatalf("unexpected message %q in an enrolment proposal", envelope.Type)
		}
	}
	require.Equal(t, []string{
		"/blockchain.alias.v1.MsgSetJurisdiction",
		"/blockchain.alias.v1.MsgGrantRole",
		"/blockchain.alias.v1.MsgGrantRole",
	}, types, "the office is placed before it is granted anything")

	// The summary states every grant, because it is the part a custodian reads.
	require.Contains(t, parsed.Summary, "ROLE_PAYMENTS_AUTHORITY")
	require.Contains(t, parsed.Summary, "ROLE_ENFORCEMENT_AUTHORITY")
	require.Contains(t, parsed.Summary, "required 2-of-3",
		"a custodian who reads only the summary must see what the office is held to")
	require.LessOrEqual(t, len(parsed.Summary), maxMetadataLen)
	require.LessOrEqual(t, len(parsed.Title), maxMetadataLen)
}

// Every grant in the proposal carries the minimum the config agreed.
//
// Read as a claim about the whole chain of custody: the number in the config, the
// number `country init` held the signed group file against, the number on the
// record, and the number in required_shape are one number. A grant that reached
// the chain without it would be a grant that constrains nothing while the record
// says otherwise.
func TestEveryGrantInTheProposalCarriesTheRequiredShape(t *testing.T) {
	dossier, _ := confirmedDossier(t)
	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())

	document, err := enrolmentProposal(dossier, custodian.Address)
	require.NoError(t, err)

	var parsed struct {
		Messages []json.RawMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(document, &parsed))

	grants := 0
	for _, raw := range parsed.Messages {
		var envelope struct {
			Type string `json:"@type"`
			// A uint32 is a JSON number in protobuf JSON, unlike the 64-bit fields
			// beside it which are strings. Asserted as a number here so that this
			// test would notice a field width change, and read through flexUint64 in
			// the verifier, which accepts either.
			RequiredShape *struct {
				Signatures uint32 `json:"signatures"`
				Members    uint32 `json:"members"`
			} `json:"required_shape"`
		}
		require.NoError(t, json.Unmarshal(raw, &envelope))
		if envelope.Type != "/blockchain.alias.v1.MsgGrantRole" {
			continue
		}
		grants++
		require.NotNil(t, envelope.RequiredShape,
			"a grant with no required shape leaves the office free to vote itself to one key")
		require.Equal(t, uint32(2), envelope.RequiredShape.Signatures)
		require.Equal(t, uint32(3), envelope.RequiredShape.Members)
	}
	require.Equal(t, 2, grants)
}

// A dossier with no minimum composes nothing.
//
// The state a dossier written before this field existed would be in. Refused
// rather than composed with a zero, which the chain would refuse too — but only
// after three custodians had voted for it.
func TestADossierWithNoMinimumComposesNoGrant(t *testing.T) {
	dossier, _ := confirmedDossier(t)
	dossier.Offices[0].Minimum = nil
	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())

	_, err := enrolmentProposal(dossier, custodian.Address)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no minimum in this dossier")
	require.Contains(t, err.Error(), "single key")
}

// The chain-wide backstop inside the proposal composer, reached the only way it
// can be.
//
// dossierFor refuses "*" as a country, so on every path a person can take this
// check is unreachable — which is exactly what a mutation pass found: deleting it
// broke no test. It is kept because it guards the one message in this tool whose
// mistake would be a grant of authority over every country signed by the
// foundation, and the failures it guards against are a bug in rolesOf or in the
// config parser rather than an operator's input.
//
// So the test constructs the state a bug would produce — a dossier whose country
// is the chain-wide marker, built as a struct rather than through dossierFor —
// and asserts that nothing is composed from it. A backstop nothing exercises is a
// backstop that stops working without anybody noticing.
func TestTheProposalRefusesTheChainWideScopeEvenFromABrokenDossier(t *testing.T) {
	dossier, _ := confirmedDossier(t)
	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())

	// Not reachable through dossierFor, which is the point.
	require.Error(t, countryConfig{
		Ceremony: "x", ChainID: fixtureChain, Country: aliastypes.ChainWide,
		Foundation: dossier.Foundation,
		Offices:    []officeConfig{{Name: "x", Roles: paymentsRoles(), Group: "x", Minimum: fixtureMinimum()}},
	}.validate())

	broken := dossier
	broken.Country = aliastypes.ChainWide

	_, err := enrolmentProposal(broken, custodian.Address)
	require.Error(t, err)
	require.Contains(t, err.Error(), "chain-wide")
	require.Contains(t, err.Error(), "may not manufacture authority over every country")

	// The seed and the validator placement carry the country too, as a
	// jurisdiction record rather than a grant. Both had NO backstop until this
	// test was written: from a dossier whose country had become "*", both composed
	// a MsgSetJurisdiction naming it. The chain refuses that, so it was never
	// exploitable — but a tool that composes a message the chain will reject has
	// asked three custodians to vote for nothing.
	applicant, _ := officeKey(t, "Applicant", time.Unix(1780000000, 0).UTC())
	_, err = seedProposal(broken, custodian.Address, []string{applicant.Address})
	require.Error(t, err)
	require.Contains(t, err.Error(), "chain-wide")

	_, err = validatorPlacementProposal(broken, custodian.Address, validatorPlacement{
		Candidate: applicant.Address, Declared: aliastypes.ChainWide, Agreement: agreementUnrecorded,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "chain-wide")

	// And the foundation's reserved code, which is the other value that is legal
	// elsewhere in x/alias and must never reach a record or a grant.
	reserved := dossier
	reserved.Country = aliastypes.FoundationCountry
	_, err = enrolmentProposal(reserved, custodian.Address)
	require.Error(t, err)
	require.Contains(t, err.Error(), "absence of a national perimeter")
}

// An office proposing its own authority would be an office granting itself a role.
func TestAnOfficeSuperUserCannotProposeTheEnrolment(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	confirmed, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.NoError(t, err)
	office.OnChain = &confirmed

	_, err = enrolmentProposal(dossier, office.Members[0].Address)
	require.Error(t, err)
	require.Contains(t, err.Error(), "super user")
}

func TestTheFoundationsPolicyAddressIsNotAProposer(t *testing.T) {
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	confirmed, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.NoError(t, err)
	office.OnChain = &confirmed

	_, err = enrolmentProposal(dossier, dossier.Foundation)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a custodian's key")
}

// ---------------------------------------------------------------- verify

func confirmedDossier(t *testing.T) (countryDossier, string) {
	t.Helper()
	dossier, office, address := confirmFixture(t)
	tx, policy, members := chainAnswers(*office, address, 7)
	confirmed, err := confirmOffice(office, dossier.Foundation, tx, policy, members, time.Now())
	require.NoError(t, err)
	office.OnChain = &confirmed
	return dossier, address
}

func writeGrants(t *testing.T, dir string, grants ...map[string]any) string {
	t.Helper()
	// Every grant carries the fixture's required shape unless the case says
	// otherwise, so that a test about some other property does not have to
	// restate it. A case that wants the field ABSENT — the grant made before
	// required_shape existed, or by a tool that dropped it — sets it to nil
	// explicitly, which marshals to null and decodes to the nil pointer that
	// means "no requirement".
	// Numbers rather than strings, because that is what the chain emits: protobuf
	// JSON renders a uint32 as a number and only the 64-bit fields beside it as
	// strings. One case below hands strings in on purpose, to hold the reader to
	// accepting both.
	for _, grant := range grants {
		if _, set := grant["required_shape"]; !set {
			grant["required_shape"] = map[string]any{"signatures": 2, "members": 3}
		}
	}
	path := filepath.Join(dir, "grants.json")
	encoded, err := json.Marshal(map[string]any{"grants": grants})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, encoded, 0o644))
	return path
}

func TestVerifyReadsEveryGrantBackOrRefuses(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)
	office := &dossier.Offices[0]

	path := writeGrants(t, dir,
		map[string]any{
			"holder": address, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		},
		map[string]any{
			"holder": address, "role": "ROLE_ENFORCEMENT_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		})

	verified, extra, err := verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
	require.NoError(t, err)
	require.Len(t, verified, 2)
	require.Empty(t, extra)
	require.Equal(t, int64(51), verified[0].GrantedAtHeight)
}

// An accepted proposal that failed in execution leaves the grants absent and
// reports nothing anybody is watching.
func TestVerifyRefusesWhenAGrantIsMissing(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)
	office := &dossier.Offices[0]

	path := writeGrants(t, dir, map[string]any{
		"holder": address, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": fixtureCountry,
		"granted_by": dossier.Foundation, "granted_at_height": "51",
	})

	_, _, err := verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "ROLE_ENFORCEMENT_AUTHORITY")
	require.Contains(t, err.Error(), "accepted and not executed")
}

// A grant in the right country made by somebody else is somebody else's act.
func TestVerifyRefusesAGrantMadeByAnotherAuthority(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)
	office := &dossier.Offices[0]
	elsewhere, err := policyAddress(900)
	require.NoError(t, err)

	path := writeGrants(t, dir,
		map[string]any{
			"holder": address, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": elsewhere, "granted_at_height": "51",
		},
		map[string]any{
			"holder": address, "role": "ROLE_ENFORCEMENT_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		})

	_, _, err = verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "somebody else's act")
}

// A grant in another country does not satisfy this one.
func TestVerifyRefusesAGrantInAnotherCountry(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)
	office := &dossier.Offices[0]

	path := writeGrants(t, dir,
		map[string]any{
			"holder": address, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": "NG",
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		},
		map[string]any{
			"holder": address, "role": "ROLE_ENFORCEMENT_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		})

	_, _, err := verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
	require.Error(t, err)
}

// A response about a different holder is not evidence about this office.
func TestVerifyRefusesAResponseAboutAnotherAccount(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)
	office := &dossier.Offices[0]
	somebodyElse, err := policyAddress(901)
	require.NoError(t, err)

	path := writeGrants(t, dir, map[string]any{
		"holder": somebodyElse, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": fixtureCountry,
		"granted_by": dossier.Foundation, "granted_at_height": "51",
	})

	_, _, err = verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "different account")
}

// A chain-wide grant is reported rather than swallowed, because an office holding
// one is the single most important thing a reader of the record could want to know.
func TestVerifyReportsGrantsBeyondTheEnrolment(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)
	office := &dossier.Offices[0]

	path := writeGrants(t, dir,
		map[string]any{
			"holder": address, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		},
		map[string]any{
			"holder": address, "role": "ROLE_ENFORCEMENT_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		},
		map[string]any{
			"holder": address, "role": "ROLE_SUPERVISOR", "jurisdiction": aliastypes.ChainWide,
			"granted_by": "gov", "granted_at_height": "9",
		})

	verified, extra, err := verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
	require.NoError(t, err)
	require.Len(t, verified, 2)
	require.Len(t, extra, 1)
	require.Contains(t, extra[0], "ROLE_SUPERVISOR")
	require.Contains(t, extra[0], aliastypes.ChainWide)
}

// A grant that landed without the required shape is refused, and the message says
// what is wrong with it rather than that something is missing.
//
// This is the case a chain running an older build produces, and the case a
// hand-edited proposal produces. Both leave an office holding a real authority
// that nothing constrains, with a signed record claiming a two-of-three.
func TestVerifyRefusesAGrantThatLandedWithNoRequiredShape(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)
	office := &dossier.Offices[0]

	path := writeGrants(t, dir,
		map[string]any{
			"holder": address, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
			"required_shape": nil,
		})

	_, _, err := verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "records no required shape")
	require.Contains(t, err.Error(), "2-of-3")
	require.Contains(t, err.Error(), "down to a single key")
}

// A grant weaker than the record claims is refused; a stronger one is accepted and
// recorded as what the chain actually said.
func TestVerifyHoldsTheOnChainShapeAgainstTheAgreedMinimum(t *testing.T) {
	for _, tc := range []struct {
		name       string
		signatures string
		members    string
		refused    string
		recorded   string
	}{
		{name: "as agreed", signatures: "2", members: "3", recorded: "2-of-3"},
		{name: "stronger than agreed", signatures: "3", members: "5", recorded: "3-of-5"},
		{name: "fewer signatures", signatures: "1", members: "3", refused: "1-of-3"},
		{name: "fewer members", signatures: "2", members: "2", refused: "2-of-2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dossier, address := confirmedDossier(t)
			office := &dossier.Offices[0]

			// Strings here, numbers everywhere else. A gateway that stringifies a
			// uint32 and one that does not must both be readable, because refusing an
			// honest grant over a JSON spelling would be a refusal an operator cannot
			// act on.
			path := writeGrants(t, dir,
				map[string]any{
					"holder": address, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": fixtureCountry,
					"granted_by": dossier.Foundation, "granted_at_height": "51",
					"required_shape": map[string]any{"signatures": tc.signatures, "members": tc.members},
				},
				map[string]any{
					"holder": address, "role": "ROLE_ENFORCEMENT_AUTHORITY", "jurisdiction": fixtureCountry,
					"granted_by": dossier.Foundation, "granted_at_height": "51",
					"required_shape": map[string]any{"signatures": tc.signatures, "members": tc.members},
				})

			verified, _, err := verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
			if tc.refused != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.refused)
				require.Contains(t, err.Error(), "weaker than the decision on the record")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.recorded, verified[0].RequiredShape,
				"the record quotes the chain rather than restating its own intention")
		})
	}
}

// ---------------------------------------------------------------- placements

func writeJurisdiction(t *testing.T, dir string, record map[string]any) string {
	t.Helper()
	path := filepath.Join(dir, "jurisdiction.json")
	encoded, err := json.Marshal(map[string]any{"jurisdiction": record})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, encoded, 0o644))
	return path
}

func TestAPlacementIsCheckedForCountryAndRecorder(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)

	path := writeJurisdiction(t, dir, map[string]any{
		"address": address, "country": fixtureCountry,
		"recorded_by": dossier.Foundation, "recorded_at_height": "51",
	})
	placed, err := verifyPlacement(address, dossier.Country, path, time.Now())
	require.NoError(t, err)
	require.Equal(t, fixtureCountry, placed.Country)
	require.Equal(t, dossier.Foundation, placed.RecordedBy)
}

// An account free to name its own perimeter would name the one with no authority
// watching it.
func TestASelfDeclaredPlacementIsRefused(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)

	path := writeJurisdiction(t, dir, map[string]any{
		"address": address, "country": fixtureCountry,
		"recorded_by": address, "recorded_at_height": "51",
	})
	_, err := verifyPlacement(address, dossier.Country, path, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "own jurisdiction")
}

func TestAPlacementInTheWrongCountryIsRefused(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)

	path := writeJurisdiction(t, dir, map[string]any{
		"address": address, "country": "NG",
		"recorded_by": dossier.Foundation, "recorded_at_height": "51",
	})
	_, err := verifyPlacement(address, dossier.Country, path, time.Now())
	require.Error(t, err)
}

// An empty response is what a query for an unplaced account returns, and it must
// not read as a placement.
func TestAnAbsentPlacementIsRefused(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)

	path := filepath.Join(dir, "empty.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"jurisdiction":{}}`), 0o644))
	_, err := verifyPlacement(address, dossier.Country, path, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no jurisdiction record")
}

// ---------------------------------------------------------------- the order

// The bootstrap order, enforced. An admission proposal composed before the grant
// has been read back would be voted on by the office's super users and then
// refused when it executed.
func TestAdmissionIsRefusedBeforeTheGrantHasBeenVerified(t *testing.T) {
	dossier, _ := confirmedDossier(t)
	applicant, _ := officeKey(t, "Banque Atlantique", time.Unix(1780000000, 0).UTC())
	proposer, _ := officeKey(t, "Super user", time.Unix(1780000000, 0).UTC())

	_, err := admissionProposal(dossier, proposer.Address, applicant.Address, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "has not been verified to hold ROLE_PAYMENTS_AUTHORITY")
}

// And the step nothing announces: the applicant has to be placed first, and the
// office's grant does not help because the perimeter check refuses an unplaceable
// target before it consults any grant.
func TestAdmissionIsRefusedBeforeTheApplicantHasBeenPlaced(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)
	office := &dossier.Offices[0]

	path := writeGrants(t, dir,
		map[string]any{
			"holder": address, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		},
		map[string]any{
			"holder": address, "role": "ROLE_ENFORCEMENT_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		})
	verified, _, err := verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
	require.NoError(t, err)
	office.Granted = verified

	applicant, _ := officeKey(t, "Banque Atlantique", time.Unix(1780000000, 0).UTC())
	proposer, _ := officeKey(t, "Super user", time.Unix(1780000000, 0).UTC())

	_, err = admissionProposal(dossier, proposer.Address, applicant.Address, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no verified jurisdiction record")
	require.Contains(t, err.Error(), "ceremony country seed")

	// Once the applicant has been placed, the proposal is composed — and it
	// belongs to the OFFICE's group, not the foundation's. That is what enrolling
	// the country bought.
	dossier.Seeded = append(dossier.Seeded, placement{
		Account: applicant.Address, Country: fixtureCountry, RecordedBy: dossier.Foundation,
	})
	document, err := admissionProposal(dossier, proposer.Address, applicant.Address, true)
	require.NoError(t, err)

	var parsed struct {
		GroupPolicyAddress string            `json:"group_policy_address"`
		Messages           []json.RawMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(document, &parsed))
	require.Equal(t, address, parsed.GroupPolicyAddress)
	require.NotEqual(t, dossier.Foundation, parsed.GroupPolicyAddress)
	require.Len(t, parsed.Messages, 1)

	var envelope struct {
		Type        string `json:"@type"`
		Authority   string `json:"authority"`
		Participant string `json:"participant"`
		Approve     bool   `json:"approve"`
	}
	require.NoError(t, json.Unmarshal(parsed.Messages[0], &envelope))
	require.Equal(t, "/blockchain.paymsg.v1.MsgApproveParticipant", envelope.Type)
	require.Equal(t, address, envelope.Authority)
	require.Equal(t, applicant.Address, envelope.Participant)
	require.True(t, envelope.Approve)
}

// The seed places applicants and refuses to place an office, which the enrolment
// proposal already does.
func TestTheSeedRefusesToPlaceAnOfficeAgain(t *testing.T) {
	dossier, address := confirmedDossier(t)
	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())

	_, err := seedProposal(dossier, custodian.Address, []string{address})
	require.Error(t, err)
	require.Contains(t, err.Error(), "own group account")
}

func TestTheSeedRefusesAnEmptyList(t *testing.T) {
	dossier, _ := confirmedDossier(t)
	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())

	_, err := seedProposal(dossier, custodian.Address, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one applicant")
}

func TestTheSeedIsAFoundationProposal(t *testing.T) {
	dossier, _ := confirmedDossier(t)
	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())
	applicant, _ := officeKey(t, "Banque Atlantique", time.Unix(1780000000, 0).UTC())

	document, err := seedProposal(dossier, custodian.Address, []string{applicant.Address})
	require.NoError(t, err)

	var parsed struct {
		GroupPolicyAddress string            `json:"group_policy_address"`
		Messages           []json.RawMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(document, &parsed))
	require.Equal(t, dossier.Foundation, parsed.GroupPolicyAddress)
	require.Len(t, parsed.Messages, 1)

	var envelope struct {
		Type     string `json:"@type"`
		Recorder string `json:"recorder"`
		Account  string `json:"account"`
		Country  string `json:"country"`
	}
	require.NoError(t, json.Unmarshal(parsed.Messages[0], &envelope))
	require.Equal(t, "/blockchain.alias.v1.MsgSetJurisdiction", envelope.Type)
	require.Equal(t, dossier.Foundation, envelope.Recorder)
	require.Equal(t, applicant.Address, envelope.Account)
	require.Equal(t, fixtureCountry, envelope.Country)
}

func TestTheSeedRefusesARepeatedAccount(t *testing.T) {
	dossier, _ := confirmedDossier(t)
	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())
	applicant, _ := officeKey(t, "Banque Atlantique", time.Unix(1780000000, 0).UTC())

	_, err := seedProposal(dossier, custodian.Address,
		[]string{applicant.Address, applicant.Address})
	require.Error(t, err)
	require.Contains(t, err.Error(), "twice")
}

// ---------------------------------------------------------------- the chain's word

// The account that may admit a country is the one the constitution pins. A
// proposal built for any other address is one three custodians vote through and
// the chain then refuses.
func TestTheFoundationIsCheckedAgainstTheConstitution(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)

	good := filepath.Join(dir, "invariants.json")
	require.NoError(t, os.WriteFile(good, []byte(fmt.Sprintf(
		`{"invariants":{"enforcement_recovery_destination":%q,"foundation_custodian_count":5,`+
			`"foundation_signature_threshold":3}}`, dossier.Foundation)), 0o644))
	threshold, err := requireFoundation(dossier, good)
	require.NoError(t, err)
	// The threshold is read from the constitution rather than assumed to be three,
	// so an operator on a chain whose invariant says four is told to collect four.
	require.Equal(t, 3, threshold)

	elsewhere, err := policyAddress(777)
	require.NoError(t, err)
	bad := filepath.Join(dir, "other.json")
	require.NoError(t, os.WriteFile(bad, []byte(fmt.Sprintf(
		`{"invariants":{"enforcement_recovery_destination":%q,"foundation_signature_threshold":3}}`,
		elsewhere)), 0o644))
	_, err = requireFoundation(dossier, bad)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pins")

	// An unwritten destination is a refusal, never a match against an empty
	// dossier field. This is the proto3 zero-value trap in a string costume.
	empty := filepath.Join(dir, "empty.json")
	require.NoError(t, os.WriteFile(empty, []byte(`{"invariants":{}}`), 0o644))
	_, err = requireFoundation(dossier, empty)
	require.Error(t, err)

	// And a threshold of one is refused: an invariant saying the foundation acts
	// on a single signature is the arrangement the key ceremony exists to abolish,
	// so this tool will not build an enrolment on top of it.
	single := filepath.Join(dir, "single.json")
	require.NoError(t, os.WriteFile(single, []byte(fmt.Sprintf(
		`{"invariants":{"enforcement_recovery_destination":%q,"foundation_signature_threshold":1}}`,
		dossier.Foundation)), 0o644))
	_, err = requireFoundation(dossier, single)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no foundation to speak of")
}

// Placing an office's own account needs the foundation to hold
// ROLE_FOUNDATION_ADMINISTRATOR at the chain-wide scope — a governance decision,
// and a different mechanism from the constitutional invariant that lets it grant a
// role inside a country. Two amendment costs, and an enrolment needs both.
func TestTheFoundationMustAlsoBeAFoundationAdministrator(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)

	good := filepath.Join(dir, "chain-wide-grants.json")
	require.NoError(t, os.WriteFile(good, []byte(fmt.Sprintf(
		`{"grants":[{"holder":%q,"role":"ROLE_FOUNDATION_ADMINISTRATOR","jurisdiction":"*"}]}`,
		dossier.Foundation)), 0o644))
	require.NoError(t, requireFoundationAdministrator(dossier, good))

	bad := filepath.Join(dir, "no-grants.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{"grants":[]}`), 0o644))
	err := requireFoundationAdministrator(dossier, bad)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ROLE_FOUNDATION_ADMINISTRATOR")
	require.Contains(t, err.Error(), "governance proposal")

	// A grant of the role to somebody else is not the foundation's, and a
	// chain-wide grant of another role to the foundation is not this one. Both are
	// refused, because either would be read by a check matching on one field alone
	// as an entitlement to place an office.
	for name, body := range map[string]string{
		"another holder": fmt.Sprintf(
			`{"grants":[{"holder":%q,"role":"ROLE_FOUNDATION_ADMINISTRATOR","jurisdiction":"*"}]}`,
			dossier.Offices[0].OnChain.PolicyAddress),
		"another role": fmt.Sprintf(
			`{"grants":[{"holder":%q,"role":"ROLE_SUPERVISOR","jurisdiction":"*"}]}`, dossier.Foundation),
	} {
		path := filepath.Join(dir, slug(name)+".json")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		require.Error(t, requireFoundationAdministrator(dossier, path), name)
	}
}

// ---------------------------------------------------------------- validators

func writeReconciliation(t *testing.T, dir string, rows ...map[string]any) string {
	t.Helper()
	path := filepath.Join(dir, "reconciliation.json")
	encoded, err := json.Marshal(map[string]any{"records": rows})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, encoded, 0o644))
	return path
}

// An unplaced validator that declared this country is placed, and the proposal
// belongs to the foundation because a validator banks nowhere.
func TestAnUnplacedValidatorIsPlacedByTheFoundation(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)
	candidate, _ := officeKey(t, "Validator", time.Unix(1780000000, 0).UTC())
	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())

	path := writeReconciliation(t, dir, map[string]any{
		"candidate": candidate.Address, "declared_jurisdiction": fixtureCountry,
		"agreement": "JURISDICTION_AGREEMENT_UNRECORDED",
	})
	var response reconciliationResponse
	require.NoError(t, readJSONFile(path, &response))

	placement, err := findValidator(response, candidate.Address, dossier.Country)
	require.NoError(t, err)
	require.Equal(t, agreementUnrecorded, placement.Agreement)

	document, err := validatorPlacementProposal(dossier, custodian.Address, placement)
	require.NoError(t, err)

	var parsed struct {
		GroupPolicyAddress string            `json:"group_policy_address"`
		Messages           []json.RawMessage `json:"messages"`
		Summary            string            `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(document, &parsed))
	require.Equal(t, dossier.Foundation, parsed.GroupPolicyAddress)
	require.Len(t, parsed.Messages, 1)

	var envelope struct {
		Type     string `json:"@type"`
		Recorder string `json:"recorder"`
		Account  string `json:"account"`
		Country  string `json:"country"`
	}
	require.NoError(t, json.Unmarshal(parsed.Messages[0], &envelope))
	require.Equal(t, "/blockchain.alias.v1.MsgSetJurisdiction", envelope.Type)
	require.Equal(t, dossier.Foundation, envelope.Recorder)
	require.Equal(t, candidate.Address, envelope.Account)
	require.Equal(t, fixtureCountry, envelope.Country)
	// It writes the record and says nothing about touching the declaration,
	// because it does not.
	require.Contains(t, parsed.Summary, "declaration stays where it is")
}

// The refusal this command exists for: placing a validator in a country it did not
// declare would manufacture the disagreement the reconciliation query reveals,
// with the foundation's signature on it.
func TestAValidatorIsNotPlacedInACountryItDidNotDeclare(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)
	candidate, _ := officeKey(t, "Validator", time.Unix(1780000000, 0).UTC())

	path := writeReconciliation(t, dir, map[string]any{
		"candidate": candidate.Address, "declared_jurisdiction": "NG",
		"agreement": "JURISDICTION_AGREEMENT_UNRECORDED",
	})
	var response reconciliationResponse
	require.NoError(t, readJSONFile(path, &response))

	_, err := findValidator(response, candidate.Address, dossier.Country)
	require.Error(t, err)
	require.Contains(t, err.Error(), "declared NG")
	require.Contains(t, err.Error(), "rather than resolve one")
}

// An application is not an approval.
func TestAValidatorTheChainHasNotApprovedIsRefused(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)
	candidate, _ := officeKey(t, "Validator", time.Unix(1780000000, 0).UTC())
	other, _ := officeKey(t, "Somebody else", time.Unix(1780000000, 0).UTC())

	path := writeReconciliation(t, dir, map[string]any{
		"candidate": other.Address, "declared_jurisdiction": fixtureCountry,
		"agreement": "JURISDICTION_AGREEMENT_AGREE",
	})
	var response reconciliationResponse
	require.NoError(t, readJSONFile(path, &response))

	_, err := findValidator(response, candidate.Address, dossier.Country)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an approved validator")
}

// Already agreeing is not an error and not a proposal; it is "there is nothing to
// do", said out loud.
func TestAValidatorThatAlreadyAgreesIsNotPlacedAgain(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)
	candidate, _ := officeKey(t, "Validator", time.Unix(1780000000, 0).UTC())

	path := writeReconciliation(t, dir, map[string]any{
		"candidate": candidate.Address, "declared_jurisdiction": fixtureCountry,
		"recorded_jurisdiction": fixtureCountry, "recorded_by": dossier.Foundation,
		"agreement": "JURISDICTION_AGREEMENT_AGREE",
	})
	var response reconciliationResponse
	require.NoError(t, readJSONFile(path, &response))

	_, err := findValidator(response, candidate.Address, dossier.Country)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nothing to place")
}

// A disagreement IS placed — correcting a recorded country is a foundation
// administrator's job — but the proposal says it is a correction and names what is
// being overwritten.
func TestADisagreeingValidatorIsCorrectedAndLabelledAsSuch(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)
	candidate, _ := officeKey(t, "Validator", time.Unix(1780000000, 0).UTC())
	custodian, _ := officeKey(t, "Custodian", time.Unix(1780000000, 0).UTC())

	path := writeReconciliation(t, dir, map[string]any{
		"candidate": candidate.Address, "declared_jurisdiction": fixtureCountry,
		"recorded_jurisdiction": "NG", "recorded_by": dossier.Foundation,
		"agreement": "JURISDICTION_AGREEMENT_DISAGREE",
	})
	var response reconciliationResponse
	require.NoError(t, readJSONFile(path, &response))

	placement, err := findValidator(response, candidate.Address, dossier.Country)
	require.NoError(t, err)
	require.Equal(t, agreementDisagree, placement.Agreement)
	require.Equal(t, "NG", placement.Recorded)

	document, err := validatorPlacementProposal(dossier, custodian.Address, placement)
	require.NoError(t, err)
	var parsed struct {
		Summary string `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(document, &parsed))
	require.Contains(t, parsed.Summary, "CORRECTS")
	require.Contains(t, parsed.Summary, "NG")
}

// The reserved unspecified state is refused rather than mapped onto anything.
// A correct chain never produces it, which is exactly why a tool that guessed
// would be guessing about a chain it does not understand.
func TestTheUnspecifiedAgreementIsRefused(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)
	candidate, _ := officeKey(t, "Validator", time.Unix(1780000000, 0).UTC())

	for _, agreement := range []any{"JURISDICTION_AGREEMENT_UNSPECIFIED", 0} {
		path := writeReconciliation(t, dir, map[string]any{
			"candidate": candidate.Address, "declared_jurisdiction": fixtureCountry,
			"agreement": agreement,
		})
		var response reconciliationResponse
		require.NoError(t, readJSONFile(path, &response))
		_, err := findValidator(response, candidate.Address, dossier.Country)
		require.Errorf(t, err, "%v was accepted as a finding", agreement)
		require.Contains(t, err.Error(), "reserved unspecified state")
	}

	// And an absent field, which is what protobuf JSON writes for the zero value,
	// is the same thing rather than a missing row.
	path := writeReconciliation(t, dir, map[string]any{
		"candidate": candidate.Address, "declared_jurisdiction": fixtureCountry,
	})
	var response reconciliationResponse
	require.NoError(t, readJSONFile(path, &response))
	_, err := findValidator(response, candidate.Address, dossier.Country)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved unspecified state")
}

// The enum arrives as a name from the CLI and can arrive as a number from a
// gateway configured with enums_as_ints. Both are read; anything else is refused.
func TestTheAgreementEnumIsReadInEitherForm(t *testing.T) {
	for name, raw := range map[string]string{
		"name":   `"JURISDICTION_AGREEMENT_DISAGREE"`,
		"number": `2`,
	} {
		got, err := agreementOf(json.RawMessage(raw))
		require.NoErrorf(t, err, "%s form", name)
		require.Equalf(t, agreementDisagree, got, "%s form", name)
	}

	_, err := agreementOf(json.RawMessage(`9`))
	require.Error(t, err)
	_, err = agreementOf(json.RawMessage(`{"x":1}`))
	require.Error(t, err)
}

// An empty reconciliation is reported rather than treated as "nothing to do".
// A chain with no approved validators and a query pointed at the wrong node look
// identical from here, and only one of them means the operator can proceed.
func TestAnEmptyReconciliationIsNotAnAnswer(t *testing.T) {
	dir := t.TempDir()
	dossier, _ := confirmedDossier(t)
	candidate, _ := officeKey(t, "Validator", time.Unix(1780000000, 0).UTC())

	path := filepath.Join(dir, "empty.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o644))
	var response reconciliationResponse
	require.NoError(t, readJSONFile(path, &response))

	_, err := findValidator(response, candidate.Address, dossier.Country)
	require.Error(t, err)
	require.Contains(t, err.Error(), "names no validators")
}

// ---------------------------------------------------------------- the record

func TestTheRecordRefusesToAttestToAnUnverifiedEnrolment(t *testing.T) {
	dossier, _ := confirmedDossier(t)
	config := enrolmentRecordConfig{
		Location:    "Dakar",
		StartedAt:   "2026-09-01T09:00:00Z",
		CompletedAt: "2026-09-01T13:00:00Z",
		Participants: []participant{
			{Name: "R. Lead", Role: "enrolment lead", Organisation: "Yamale Foundation"},
		},
	}

	// Confirmed but not verified: the grants have not been read back.
	_, err := renderEnrolmentRecord(dossier, config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not been read back off the chain")
}

func TestTheRecordNamesTheAddressesAndTheGrants(t *testing.T) {
	dir := t.TempDir()
	dossier, address := confirmedDossier(t)
	office := &dossier.Offices[0]

	path := writeGrants(t, dir,
		map[string]any{
			"holder": address, "role": "ROLE_PAYMENTS_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		},
		map[string]any{
			"holder": address, "role": "ROLE_ENFORCEMENT_AUTHORITY", "jurisdiction": fixtureCountry,
			"granted_by": dossier.Foundation, "granted_at_height": "51",
		})
	verified, _, err := verifyGrants(office, dossier.Country, dossier.Foundation, path, time.Now())
	require.NoError(t, err)
	office.Granted = verified

	rendered, err := renderEnrolmentRecord(dossier, enrolmentRecordConfig{
		Location:    "Dakar",
		StartedAt:   "2026-09-01T09:00:00Z",
		CompletedAt: "2026-09-01T13:00:00Z",
		ProposalID:  "4",
		Participants: []participant{
			{Name: "O. Observer", Role: "independent observer", Organisation: "External Auditors LLP"},
		},
	})
	require.NoError(t, err)

	require.Contains(t, rendered, address)
	require.Contains(t, rendered, dossier.Foundation)
	require.Contains(t, rendered, "ROLE_PAYMENTS_AUTHORITY")
	require.Contains(t, rendered, "ROLE_ENFORCEMENT_AUTHORITY")
	require.Contains(t, rendered, "O. Observer")
	require.Contains(t, rendered, "votes-by-proposal 4")
	// Every super user has a signature line.
	for _, member := range office.Members {
		require.Contains(t, rendered, member.Name)
		require.Contains(t, rendered, member.Fingerprint)
	}
	// And the record says how to check itself.
	require.Contains(t, rendered, "blockchaind query alias role-grants")
	require.Contains(t, rendered, "chain-wide-grants")

	// Both shapes are on the record: what the office is, and what it may never
	// fall below. A reader of an old record is usually asking whether the office
	// can still act, and the second number is the only one that answers that.
	require.Contains(t, rendered, "**Rule:** 2 of 3")
	require.Contains(t, rendered, "**Required minimum:** 2-of-3")
	require.Contains(t, rendered, "Required shape",
		"the grants table names the shape the chain reported for each grant")

	// It contains no phrase and no private material. The same claim the
	// foundation's record makes, asserted rather than assumed.
	require.NotContains(t, strings.ToLower(rendered), "mnemonic")
	require.NotContains(t, strings.ToLower(rendered), "recovery phrase\n")
}
