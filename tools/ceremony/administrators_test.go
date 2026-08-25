package main

// Tests for the appointment ceremony.
//
// What is worth testing here is not that a proposal comes out with the right keys
// in it. It is that the tool REFUSES in every case where composing something
// plausible would be worse than composing nothing — because every one of those
// cases is a governance proposal that costs a voting period and then does
// something other than what it says.
//
// So most of what follows is refusals: an unconfirmed address, a group file from
// the wrong ceremony, an authority that is not the gov module, a ninth
// administrator. Those are the cases that would still fail if somebody
// "simplified" this by adding a default.
//
// A second group of tests asserts things that are now true STRUCTURALLY, and they
// are the ones to read before deleting anything here. The appointment used to be a
// MsgUpdateParams, which replaced the whole Params object, so a proposal could
// drop the administrators it did not name or reset payload_length to a default
// nobody voted for. The tests that guarded those two failures are gone and have
// been replaced by tests that the failures cannot be expressed: the message names
// one holder and carries no parameters at all.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	aliastypes "yamale/blockchain/x/alias/types"
)

const (
	adminGroupAddr = "yml18czxlt9vah0h0tttyxs6t9ej7uem63ddpnat62au5vsn3p80r94swsyfk4"
	adminFoundAddr = "yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj"
	adminGovAddr   = "yml10d07y265gmmuvt4z0w9aw880jnsr700jz5s386"
	adminMemberA   = "yml1sy3fxls3xcg9y3n6xm3yczznf3grcae7mtjk5g"
	adminMemberB   = "yml14v6gumccm63wvlr8qrhmw4keakkekj8r45ldhq"
	adminMemberC   = "yml1qd6l4hxyzlfwq7hhkz50jmsnz069dnd32yp7dx"
	adminMemberD   = "yml1pf6j3n036fnummelt2w4x0smr4mcrz6hkj3792"
)

func adminTestTime() time.Time {
	return time.Date(2026, 8, 23, 11, 0, 0, 0, time.UTC)
}

// confirmedDossier is an appointment dossier that has been through `confirm`.
func confirmedAppointment() administratorsDossier {
	return administratorsDossier{
		Ceremony:  "Yamale foundation administrators",
		ChainID:   "yamale-devnet-2",
		CreatedAt: "2026-08-23T11:00:00Z",
		Reason:    "Appointed at the ceremony of 2026-08-23; fingerprints read aloud by all four.",
		Threshold: 3,
		Members: []administratorMember{
			{Name: "A", Address: adminMemberA, Fingerprint: "AAAA-1111"},
			{Name: "B", Address: adminMemberB, Fingerprint: "BBBB-2222"},
			{Name: "C", Address: adminMemberC, Fingerprint: "CCCC-3333"},
			{Name: "D", Address: adminMemberD, Fingerprint: "DDDD-4444"},
		},
		GroupFingerprint: "QSS5-EE16-X5R5-JT73",
		GroupFile:        "group.json",
		OnChain: &onChainGroup{
			PolicyAddress: adminGroupAddr,
			GroupID:       4,
			TxHash:        "ABCD",
			Height:        28000,
			ConfirmedAt:   "2026-08-23T11:00:00Z",
		},
	}
}

// standing is the chain's answer to "who already holds the role", plus the gov
// module account.
func standing(administrators ...string) standingGrants {
	return standingGrants{
		Administrators: administrators,
		Authority:      adminGovAddr,
	}
}

// chainWideGrantsFile writes what `query alias chain-wide-grants` returns.
//
// Every holder is written with the role and the scope the real answer carries,
// because the reader filters on both and a fixture that omitted them would test a
// filter that never runs.
func chainWideGrantsFile(t *testing.T, name string, holders ...string) string {
	t.Helper()
	grants := make([]map[string]any, 0, len(holders))
	for _, holder := range holders {
		grants = append(grants, map[string]any{
			"holder":            holder,
			"role":              "ROLE_FOUNDATION_ADMINISTRATOR",
			"jurisdiction":      "*",
			"granted_by":        adminGovAddr,
			"granted_at_height": "28100",
		})
	}
	body, err := json.Marshal(map[string]any{"grants": grants})
	require.NoError(t, err)
	return writeAdminJSON(t, name, string(body))
}

// decodeProposal pulls the composed MsgGrantRole back out of the document.
//
// Decoded rather than string-matched, because the whole reason the document is
// built through the proto codec is that a field renamed in the proto should break
// this rather than produce JSON that reads correctly and decodes to a zero. The
// field that would go quietest is the scope, and a grant decoding to an empty
// jurisdiction is one the chain refuses while a grant decoding to a country is one
// it accepts.
func decodeProposal(t *testing.T, blob []byte) (govProposalDocument, aliastypes.MsgGrantRole) {
	t.Helper()
	var doc govProposalDocument
	require.NoError(t, json.Unmarshal(blob, &doc))
	require.Len(t, doc.Messages, 1)

	// Through the interface, because that is how the chain decodes it: a proposal's
	// messages arrive as Any and are resolved against the registry. Decoding into
	// the concrete type directly would skip the registration this document depends
	// on, and would pass for a type URL the chain cannot resolve.
	var any sdk.Msg
	require.NoError(t, administratorsCodec().UnmarshalInterfaceJSON(doc.Messages[0], &any))
	msg, ok := any.(*aliastypes.MsgGrantRole)
	require.True(t, ok, "the proposal's message is %T, not a MsgGrantRole", any)
	return doc, *msg
}

// -------------------------------------------------------------- the one grant

func TestTheProposalGrantsTheRoleChainWideToTheConfirmedGroup(t *testing.T) {
	configureAddresses()
	blob, err := appointmentProposal(confirmedAppointment(), standing(adminFoundAddr), "1000000uyml")
	require.NoError(t, err)

	_, msg := decodeProposal(t, blob)
	require.Equal(t, adminGroupAddr, msg.Holder)
	require.Equal(t, aliastypes.ROLE_FOUNDATION_ADMINISTRATOR, msg.Role)
	// The scope, asserted as the literal the chain stores rather than as
	// aliastypes.ChainWide, so that a change to that constant is a failure here
	// rather than a silently agreed change of meaning on both sides. A grant naming
	// a country would be refused by the chain; a grant naming an empty string would
	// be refused too; and the difference between those and this is one byte.
	require.Equal(t, "*", msg.Jurisdiction)
}

// The single best thing about the appointment being a grant, asserted rather than
// asserted about.
//
// The MsgUpdateParams this replaced carried the WHOLE administrator list, so
// composing it meant reading the current one and copying every entry across; the
// test that stood here checked that none of them was dropped, because a proposal
// that dropped one passed anyway. A grant names one holder. There is no field in
// the message for the others to be dropped from, and that is what this asserts:
// no address but this group's appears anywhere in the document.
func TestTheProposalNamesOneHolderAndCannotDropTheOthers(t *testing.T) {
	configureAddresses()
	existing := []string{adminFoundAddr, adminMemberA, adminMemberB}
	blob, err := appointmentProposal(confirmedAppointment(), standing(existing...), "1000000uyml")
	require.NoError(t, err)

	_, msg := decodeProposal(t, blob)
	require.Equal(t, adminGroupAddr, msg.Holder)

	var raw struct {
		Messages []map[string]any `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(blob, &raw))
	encoded, err := json.Marshal(raw.Messages[0])
	require.NoError(t, err)
	for _, address := range existing {
		require.NotContains(t, string(encoded), address,
			"an existing administrator appears in the message, so there is something for a "+
				"careless proposal to drop")
	}
}

// The other half of that, and the other failure that is now unexpressible.
//
// MsgUpdateParams replaced every parameter at once, so an appointment composed
// without reading payload_length reset the identifier length of a chain that had
// raised it — silently, while reading as an appointment. Two tests guarded that:
// one that a zero was refused rather than defaulted, one that a value the chain
// itself would refuse was not quietly corrected. Both are gone, and neither is
// missing: the message carries no parameters, so there is nothing in it that could
// re-parameterise the chain, and the tool no longer reads x/alias's parameters at
// all.
func TestTheProposalCarriesNoParametersAtAll(t *testing.T) {
	configureAddresses()
	blob, err := appointmentProposal(confirmedAppointment(), standing(), "1000000uyml")
	require.NoError(t, err)

	var raw struct {
		Messages []map[string]any `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(blob, &raw))
	require.NotContains(t, raw.Messages[0], "params")
	require.NotContains(t, string(blob), "payload_length")
}

func TestTheMessageDoesNotDependOnWhoElseHoldsTheRole(t *testing.T) {
	configureAddresses()
	// The old proposal's contents were a function of the current administrator
	// list, which is why a stale read of it was destructive. This one is not: the
	// standing set decides whether the proposal is composed at all — the cap and
	// the duplicate check — and nothing about what it says.
	one, err := appointmentProposal(confirmedAppointment(), standing(), "1uyml")
	require.NoError(t, err)
	two, err := appointmentProposal(confirmedAppointment(), standing(adminMemberA, adminFoundAddr), "1uyml")
	require.NoError(t, err)

	first, _ := decodeProposal(t, one)
	second, _ := decodeProposal(t, two)
	require.Equal(t, first.Messages, second.Messages)
}

func TestTheAuthorityIsTheGovernanceModuleAccount(t *testing.T) {
	configureAddresses()
	blob, err := appointmentProposal(confirmedAppointment(), standing(), "1000000uyml")
	require.NoError(t, err)
	_, msg := decodeProposal(t, blob)
	require.Equal(t, adminGovAddr, msg.Authority)
}

// The grant records no required shape, and that is a decision rather than an
// omission — see appointmentProposal.
//
// A shape captured from the group that turned up is not a requirement: it ratifies
// a one-of-one as readily as a three-of-five, which is the reason a country's
// offices declare their minimum in the enrolment config before the day. This
// ceremony's config has no such field, so the only numbers to hand are the
// dossier's own threshold and member count — exactly the captured number that
// argument refuses.
//
// Asserted rather than left implicit, because a future reader with the country
// enrolment's grants in front of them will notice the difference and the useful
// thing to find is a test saying it was deliberate.
func TestTheGrantRecordsNoRequiredShapeBecauseNobodyDecidedOne(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()
	require.Equal(t, 3, dossier.Threshold, "this test is about the 3-of-5 NOT being recorded")

	blob, err := appointmentProposal(dossier, standing(), "1000000uyml")
	require.NoError(t, err)
	_, msg := decodeProposal(t, blob)
	require.Nil(t, msg.RequiredShape)

	// Null in the document rather than an object, which is the distinction the
	// chain's own pointer exists to keep. A message field has real presence in
	// proto3, so nil means "nobody asked for a shape" while a zero-valued
	// OfficeShape means "somebody asked for a shape of zero" — and
	// OfficeShape.Validate refuses the second, so a document carrying 0-of-0 would
	// be a proposal that passed its vote and failed when it executed.
	var raw struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(blob, &raw))
	if shape, present := raw.Messages[0]["required_shape"]; present {
		require.JSONEq(t, "null", string(shape))
	}
}

// ------------------------------------------------------------------- refusals

func TestAnUnconfirmedDossierProposesNothing(t *testing.T) {
	configureAddresses()
	// The refusal the two-phase design exists to make. No confirmed address, no
	// proposal, no fallback to a predicted one.
	dossier := confirmedAppointment()
	dossier.OnChain = nil
	_, err := appointmentProposal(dossier, standing(), "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "will not guess one")
	require.Contains(t, err.Error(), "FOUNDATION'S OWN")

	// And a confirmation record with a blank address is refused too, rather than
	// producing a message naming the empty string.
	//
	// The MESSAGE is asserted, and a mutation pass is why: with the blank check
	// removed, requireAccountAddress downstream still refused it, so the test
	// passed while the explanation had become a bech32 decoding error. That matters
	// because the two say different things about where to look. "This should be
	// impossible; do not proceed" points at the dossier; a decoding error points at
	// an address somebody typed.
	dossier.OnChain = &onChainGroup{PolicyAddress: "   "}
	_, err = appointmentProposal(dossier, standing(), "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "should be impossible")

	// And on the verification path, where without the guard the failure would read
	// as "this address holds no chain-wide grant" — which is true of the empty
	// string and tells nobody anything.
	path := chainWideGrantsFile(t, "grants.json")
	_, err = verifyAppointment(dossier, path, adminTestTime())
	require.Error(t, err)
	require.Contains(t, err.Error(), "should be impossible")
}

func TestAMissingAuthorityIsRefused(t *testing.T) {
	configureAddresses()
	current := standing()
	current.Authority = ""
	_, err := appointmentProposal(confirmedAppointment(), current, "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "governance module account")

	// And one that is not an address this chain can read.
	current.Authority = "not-an-address"
	_, err = appointmentProposal(confirmedAppointment(), current, "1000000uyml")
	require.Error(t, err)
}

// A second grant to an account that already holds the role is refused, and the
// reason it is refused has changed completely.
//
// It used to be that the chain would reject it: Params.Validate() refused a list
// naming one address twice, so the proposal failed when it executed. That is not
// what happens now. GrantRole counts the cap EXCLUDING the holder being granted,
// deliberately, so that a grant can be re-made — which is how a proposal
// resubmitted after a timeout arrives and how a required shape is added to a grant
// that had none. The chain would accept this one.
//
// It is refused here because this ceremony records no required shape, so a second
// grant has nothing to change: it would be a governance vote that passes,
// executes, reads on the record as an appointment, and leaves the chain exactly as
// it was. The message is asserted for that reason — a refusal claiming the chain
// would reject this would send an operator to look for a rule that is not there.
func TestASecondGrantToTheSameHolderIsRefusedBeforeTheVote(t *testing.T) {
	configureAddresses()
	_, err := appointmentProposal(confirmedAppointment(), standing(adminGroupAddr), "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already holds ROLE_FOUNDATION_ADMINISTRATOR chain-wide")
	require.Contains(t, err.Error(), "would change nothing")
	require.Contains(t, err.Error(), "The chain would ACCEPT it")
}

func TestTheNinthAdministratorIsRefusedAtTheCap(t *testing.T) {
	configureAddresses()
	eight := make([]string, 0, aliastypes.MaxFoundationAdministrators)
	for _, address := range []string{
		adminFoundAddr, adminMemberA, adminMemberB, adminMemberC, adminMemberD,
		"yml1jea9l5x2pltecepj3yd42xn840x2y6ajs0f5g5",
		"yml136520zqy3hhy3dtchp5zp6qlxgrz7lwm87ayyh",
		"yml1qhd5c3fa0nny9jwmd37d3r9kwls7uymaekf7qd",
	} {
		eight = append(eight, address)
	}
	require.Len(t, eight, aliastypes.MaxFoundationAdministrators)

	_, err := appointmentProposal(confirmedAppointment(), standing(eight...), "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "the chain caps it at 8")

	// The eighth is allowed, so the cap is the cap and not one less.
	_, err = appointmentProposal(confirmedAppointment(), standing(eight[:7]...), "1000000uyml")
	require.NoError(t, err)
}

// The cap counts grants of THIS role at THIS scope, and nothing else.
//
// Worth its own test because the query the operator runs answers a wider question
// than the cap asks: chain-wide-grants lists every grant no border bounds, and a
// chain with supervisors or auditors granted chain-wide would have entries in it
// that the cap does not count. A tool that counted the whole response would refuse
// a perfectly appointable ninth-of-eight — and the operator's only recourse would
// be to edit the evidence file.
//
// The scope is filtered as well as the role, and the third entry below is a record
// the chain would never write: this role is chain-wide or nothing. It is in the
// fixture because the file an operator passes is not always the file they think it
// is — `role-grants <holder>` renders the identical shape and does list country
// grants — and a filter on the role alone would count those.
func TestTheCapCountsOnlyChainWideGrantsOfThisRole(t *testing.T) {
	configureAddresses()
	path := writeAdminJSON(t, "mixed.json", `{"grants":[
		{"holder":"`+adminFoundAddr+`","role":"ROLE_FOUNDATION_ADMINISTRATOR","jurisdiction":"*"},
		{"holder":"`+adminMemberA+`","role":"ROLE_SUPERVISOR","jurisdiction":"*"},
		{"holder":"`+adminMemberB+`","role":"ROLE_FOUNDATION_ADMINISTRATOR","jurisdiction":"SN"}
	]}`)
	held, err := chainWideGrantsOf(path, aliastypes.ROLE_FOUNDATION_ADMINISTRATOR)
	require.NoError(t, err)
	require.Len(t, held, 1)
	require.Equal(t, adminFoundAddr, held[0].Holder)
}

func TestAProposalWithNoDepositIsRefused(t *testing.T) {
	configureAddresses()
	// A proposal submitted with less than the minimum is accepted, sits in the
	// deposit period, and never enters a vote — which looks from the outside
	// exactly like a proposal nobody got round to voting on.
	_, err := appointmentProposal(confirmedAppointment(), standing(), "  ")
	require.Error(t, err)
	require.Contains(t, err.Error(), "deposit is required")
}

// ------------------------------------------------------------ the wrong ceremony

func TestAFoundationCeremonyCannotBecomeAnAdministratorGroup(t *testing.T) {
	configureAddresses()
	// Recorded on chain as "Yamale foundation", an administrator group built from
	// the foundation's keys would be indistinguishable — in the one field a human
	// reads — from the account that holds every seized asset. And the custodians
	// never saw what the key was for: the marker is inside the parameters
	// fingerprint they read aloud, and theirs did not carry it.
	err := requireAdministratorsCeremony(ceremonyParams{
		Name: "Yamale foundation", Threshold: 3,
	}, "Yamale foundation")
	require.Error(t, err)
	require.Contains(t, err.Error(), "FOUNDATION's ceremony")
	require.Contains(t, err.Error(), "fingerprint")
}

func TestACountryOfficeCannotBecomeAnAdministratorGroup(t *testing.T) {
	configureAddresses()
	err := requireAdministratorsCeremony(ceremonyParams{
		Name:   "Senegal payments authority",
		Office: &officeParams{Country: "SN", Roles: []string{"ROLE_PAYMENTS_AUTHORITY"}},
	}, "Senegal payments authority")
	require.Error(t, err)
	require.Contains(t, err.Error(), "country office in SN")
}

func TestAnAdministratorCeremonyForAnotherAppointmentIsRefused(t *testing.T) {
	configureAddresses()
	err := requireAdministratorsCeremony(ceremonyParams{
		Name: "Some other group", Administrators: true,
	}, "Yamale foundation administrators")
	require.Error(t, err)
	require.Contains(t, err.Error(), "is not this group")

	// Case and surrounding whitespace do not matter — a config and a ceremony form
	// are typed by two people at two times.
	require.NoError(t, requireAdministratorsCeremony(
		ceremonyParams{Name: "Yamale Foundation Administrators", Administrators: true},
		"  yamale foundation administrators  "))
}

// ---------------------------------------------------------- the chain's answer

func writeAdminJSON(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestTheAuthorityMustBeTheGovModuleAccountAndIsChecked(t *testing.T) {
	configureAddresses()
	grants := chainWideGrantsFile(t, "grants.json", adminFoundAddr)

	// The right one.
	good := writeAdminJSON(t, "gov.json",
		`{"account":{"value":{"address":"`+adminGovAddr+`","name":"gov"}}}`)
	read, err := readStandingGrants(grants, good)
	require.NoError(t, err)
	require.Equal(t, adminGovAddr, read.Authority)
	require.Equal(t, []string{adminFoundAddr}, read.Administrators)

	// A different module account. A chain-wide grant is refused from any signer but
	// governance, so this would pass a vote and then be refused at execution.
	wrong := writeAdminJSON(t, "distribution.json",
		`{"account":{"value":{"address":"`+adminFoundAddr+`","name":"distribution"}}}`)
	_, err = readStandingGrants(grants, wrong)
	require.Error(t, err)
	require.Contains(t, err.Error(), `"distribution"`)
}

func TestBothShapesOfTheModuleAccountAnswerAreRead(t *testing.T) {
	configureAddresses()
	grants := chainWideGrantsFile(t, "grants.json")

	// The CLI's shape and the gateway's flatter one. Both, because guessing one
	// would mean reading an empty address out of a file that plainly contains it,
	// and then refusing with a message about the authority being missing.
	cli := writeAdminJSON(t, "cli.json",
		`{"account":{"value":{"address":"`+adminGovAddr+`","name":"gov"}}}`)
	rest := writeAdminJSON(t, "rest.json",
		`{"account":{"name":"gov","base_account":{"address":"`+adminGovAddr+`"}}}`)

	for _, path := range []string{cli, rest} {
		read, err := readStandingGrants(grants, path)
		require.NoError(t, err, path)
		require.Equal(t, adminGovAddr, read.Authority)
	}
}

func TestTheGrantHeightIsReadAsANumberOrAString(t *testing.T) {
	configureAddresses()
	// The CLI renders an int64 as a number and the gateway has rendered it as a
	// string. A type accepting only one would read zero from the other, and a
	// signed record saying this appointment landed at height zero is a record
	// pointing at a block that is not the one anybody should go and read.
	//
	// This is what the payload_length version of this test used to guard, moved to
	// the field that is still read: the parameters are not read at all any more.
	for _, height := range []string{`28100`, `"28100"`} {
		path := writeAdminJSON(t, "grants.json", `{"grants":[{"holder":"`+adminGroupAddr+`",`+
			`"role":"ROLE_FOUNDATION_ADMINISTRATOR","jurisdiction":"*","granted_by":"`+adminGovAddr+`",`+
			`"granted_at_height":`+height+`}]}`)
		verified, err := verifyAppointment(confirmedAppointment(), path, adminTestTime())
		require.NoError(t, err, height)
		require.Equal(t, int64(28100), verified.GrantedAtHeight, height)
	}
}

func TestAnAbsentGrantListReadsAsEmpty(t *testing.T) {
	configureAddresses()
	// This is what the CLI actually returns on the live devnet: the empty repeated
	// field is omitted entirely. Read as empty rather than refused, because for a
	// repeated field absent and empty ARE the same value — and on a chain that has
	// appointed nobody yet, empty is the truth.
	gov := writeAdminJSON(t, "gov.json",
		`{"account":{"value":{"address":"`+adminGovAddr+`","name":"gov"}}}`)
	read, err := readStandingGrants(writeAdminJSON(t, "grants.json", `{}`), gov)
	require.NoError(t, err)
	require.Empty(t, read.Administrators)

	// And a proposal composed against it is the first appointment on the chain,
	// which is a state this ceremony has to be able to reach.
	_, err = appointmentProposal(confirmedAppointment(), read, "1000000uyml")
	require.NoError(t, err)
}

// ---------------------------------------------------------------- verification

func TestVerifyRefusesUntilTheGroupActuallyHoldsTheGrant(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()

	// Not appointed. A proposal that PASSED can still fail when it executes, which
	// leaves the chain exactly as it was and says so only in a transaction log
	// nobody is watching.
	absent := chainWideGrantsFile(t, "absent.json", adminFoundAddr)
	_, err := verifyAppointment(dossier, absent, adminTestTime())
	require.Error(t, err)
	require.Contains(t, err.Error(), `holds no ROLE_FOUNDATION_ADMINISTRATOR grant at the "*" scope`)
	require.Contains(t, err.Error(), "can still fail when it executes")

	// Appointed. The whole set is recorded, not just this group, and the chain's
	// own account of who granted it and when is recorded with it.
	present := chainWideGrantsFile(t, "present.json", adminFoundAddr, adminGroupAddr)
	verified, err := verifyAppointment(dossier, present, adminTestTime())
	require.NoError(t, err)
	require.Equal(t, adminGroupAddr, verified.PolicyAddress)
	require.Equal(t, "*", verified.Jurisdiction)
	require.Equal(t, adminGovAddr, verified.GrantedBy)
	require.Equal(t, int64(28100), verified.GrantedAtHeight)
	require.Len(t, verified.Administrators, 2)
	require.Equal(t, "2026-08-23T11:00:00Z", verified.VerifiedAt)
}

// The two ways a grant can be present and not be this appointment.
//
// Neither is a state the chain can reach — it refuses a country scope for this
// role, and a grant of some other role is a different record entirely — but verify
// is the step that turns a queried file into a signed record, and the file is
// whichever one the operator passed. A verify that matched on the holder alone
// would sign "this group is a foundation administrator" on the strength of a
// supervisor grant.
func TestVerifyRefusesAGrantOfTheWrongRoleOrTheWrongScope(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()

	for name, body := range map[string]string{
		"another role": `{"grants":[{"holder":"` + adminGroupAddr + `","role":"ROLE_SUPERVISOR",` +
			`"jurisdiction":"*"}]}`,
		"a country": `{"grants":[{"holder":"` + adminGroupAddr + `",` +
			`"role":"ROLE_FOUNDATION_ADMINISTRATOR","jurisdiction":"SN"}]}`,
	} {
		_, err := verifyAppointment(dossier, writeAdminJSON(t, "g.json", body), adminTestTime())
		require.Error(t, err, name)
		require.Contains(t, err.Error(), "holds nothing", name)
	}
}

func TestVerifyRefusesAnUnconfirmedDossier(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()
	dossier.OnChain = nil
	path := chainWideGrantsFile(t, "grants.json", adminGroupAddr)
	_, err := verifyAppointment(dossier, path, adminTestTime())
	require.Error(t, err)
	require.Contains(t, err.Error(), "will not guess one")
}

// --------------------------------------------------------------------- config

func TestTheConfigRefusesWhatItCannotProceedWithout(t *testing.T) {
	base := administratorsConfig{
		Ceremony: "Yamale foundation administrators",
		ChainID:  "yamale-devnet-2",
		Group:    "group.json",
		Reason:   "Because.",
	}
	require.NoError(t, base.validate())

	for field, mutate := range map[string]func(administratorsConfig) administratorsConfig{
		"ceremony": func(c administratorsConfig) administratorsConfig { c.Ceremony = "  "; return c },
		"chain_id": func(c administratorsConfig) administratorsConfig { c.ChainID = ""; return c },
		"group":    func(c administratorsConfig) administratorsConfig { c.Group = ""; return c },
		// The reason is required, not optional. It becomes the proposal's summary,
		// which is the only explanation most of the voting set will read — and what
		// they are agreeing to is that this group may move any account out from
		// under the authority investigating it.
		"reason": func(c administratorsConfig) administratorsConfig { c.Reason = ""; return c },
	} {
		require.Error(t, mutate(base).validate(), "%s should be required", field)
	}
}

// ---------------------------------------------------------------- the summary

func TestTheSummaryStatesThePowerAndTheCounts(t *testing.T) {
	configureAddresses()
	blob, err := appointmentProposal(confirmedAppointment(), standing(adminFoundAddr), "1000000uyml")
	require.NoError(t, err)
	doc, _ := decodeProposal(t, blob)

	// The power, because "foundation administrator" reads like somebody with a
	// login and gives no hint of what it confers. This is the assertion that found
	// the bug: composed as one sentence with the address and the ceremony name
	// first, this was the part that fell off the end of the 255-byte cap.
	require.Contains(t, doc.Summary, "correct the country recorded against ANY account")
	require.Contains(t, doc.Summary, "reissues its identifier")
	require.Contains(t, doc.Summary, aliastypes.FoundationCountry)
	// The counts, because one message about one holder says nothing about how many
	// accounts already stand outside every national perimeter, and that number is
	// what the cap of eight exists to keep visible.
	require.Contains(t, doc.Summary, "from 1 to 2 of a maximum of 8")
	require.Contains(t, doc.Summary, adminGroupAddr)
	// And the summary says in words what the message shape guarantees, because a
	// voter reading the summary is not reading the message: this proposal takes
	// nothing away from anybody.
	require.Contains(t, doc.Summary, "no account that holds the role today loses it")
	// The chain's two limits, which are NOT the same number: the title is capped at
	// MaxMetadataLen and the summary at forty times that. Asserted separately
	// because conflating them is what made an earlier version of this cut the
	// description of the power out of the summary to fit a limit that did not exist.
	require.LessOrEqual(t, len(doc.Summary), maxSummaryLen)
	require.NotEmpty(t, doc.Title)
	require.LessOrEqual(t, len(doc.Title), maxMetadataLen)
}

// TestTheSummaryLimitIsTheChainsAndNotFortyTimesStricter.
//
// x/gov and x/group both check a proposal's summary against 40*MaxMetadataLen and
// its title against MaxMetadataLen. This tool used 255 for both, which is forty
// times stricter than the chain in the one field that states in words what a
// proposal does — so a summary was being truncated, with a marker, for no reason
// at all.
func TestTheSummaryLimitIsTheChainsAndNotFortyTimesStricter(t *testing.T) {
	require.Equal(t, 255, maxMetadataLen)
	require.Equal(t, 40*maxMetadataLen, maxSummaryLen)

	configureAddresses()
	// A summary comfortably over the old 255 and well under the real limit must
	// come through whole, with no truncation marker.
	blob, err := appointmentProposal(confirmedAppointment(), standing(adminFoundAddr), "1uyml")
	require.NoError(t, err)
	doc, _ := decodeProposal(t, blob)
	require.Greater(t, len(doc.Summary), maxMetadataLen,
		"this summary should be longer than the old cap, or the test proves nothing")
	require.NotContains(t, doc.Summary, "see the appointment record")
	require.Contains(t, doc.Summary, confirmedAppointment().Reason)
}

// TestThePowerSurvivesTheCapWhateverElseDoesNot is the regression test for that.
//
// A ceremony name and a reason are both arbitrarily long and both, composed
// naively, push the description of the power past the 255 bytes x/gov accepts.
// The description is the part a voter cannot reconstruct, so it is the part that
// must never be the one that goes.
func TestThePowerSurvivesTheCapWhateverElseDoesNot(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()
	dossier.Reason = strings.Repeat("And a very long reason as well. ", 12)

	blob, err := appointmentProposal(dossier, standing(adminFoundAddr, adminMemberA), "1uyml")
	require.NoError(t, err)
	doc, _ := decodeProposal(t, blob)

	require.LessOrEqual(t, len(doc.Summary), maxSummaryLen)
	require.Contains(t, doc.Summary, "correct the country recorded against ANY account")
	require.Contains(t, doc.Summary, "from 2 to 3 of a maximum of 8")
}

// TestALongCeremonyNameIsRefusedWithSomethingToDoAboutIt.
//
// The title is refused rather than truncated, because it is a sentence built
// around a name a person chose and silently shortening it in the field the whole
// voting set reads first is not a decision to make on their behalf. So the refusal
// has to say which part to change.
func TestALongCeremonyNameIsRefusedWithSomethingToDoAboutIt(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()
	dossier.Ceremony = strings.Repeat("An Extremely Long Ceremony Name ", 8)

	_, err := appointmentProposal(dossier, standing(), "1uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Shorten the ceremony name")
	require.Contains(t, err.Error(), "group metadata")
}

func TestAnOverlongSummaryIsTruncatedRatherThanRefused(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()
	// Past the REAL limit, which takes a genuinely enormous reason — 10,200 bytes.
	// Refusing would mean an appointment that cannot be proposed at all because
	// somebody pasted a document into the reason field.
	dossier.Reason = strings.Repeat("The ceremony lead wrote a very long explanation. ", 300)
	require.Greater(t, len(dossier.Reason), maxSummaryLen)
	blob, err := appointmentProposal(dossier, standing(), "1000000uyml")
	require.NoError(t, err)
	doc, _ := decodeProposal(t, blob)
	require.LessOrEqual(t, len(doc.Summary), maxSummaryLen)
	require.Contains(t, doc.Summary, "see the appointment record")
	// And the description of the power is still there, which is the whole point of
	// the priority ordering.
	require.Contains(t, doc.Summary, "correct the country recorded against ANY account")
}

// -------------------------------------------------------- the proposal is gov

func TestTheProposalIsAGovernanceProposalAndNotAGroupOne(t *testing.T) {
	configureAddresses()
	// The distinction the whole ceremony turns on. An x/group proposal carries a
	// group_policy_address and proposers; a governance one carries a deposit. The
	// foundation's 3-of-5 cannot appoint an administrator, so composing the wrong
	// shape here would produce a document three custodians would vote on and that
	// would change nothing.
	blob, err := appointmentProposal(confirmedAppointment(), standing(), "1000000uyml")
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(blob, &raw))
	require.Contains(t, raw, "deposit")
	require.NotContains(t, raw, "group_policy_address")
	require.NotContains(t, raw, "proposers")
	// And not expedited. Shortening the vote on the appointment of the account
	// that can move any customer out from under their regulator buys nothing but
	// less time for somebody to notice.
	require.NotContains(t, raw, "expedited")
	require.Equal(t, "1000000uyml", raw["deposit"])
}

func TestTheMessageTypeIsTheAliasGrantRole(t *testing.T) {
	configureAddresses()
	blob, err := appointmentProposal(confirmedAppointment(), standing(), "1000000uyml")
	require.NoError(t, err)

	var doc struct {
		Messages []map[string]any `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(blob, &doc))
	require.Len(t, doc.Messages, 1)
	// The proto package, not the REST path: the REST prefix carries `yamale` and
	// the proto package does not.
	require.Equal(t, "/blockchain.alias.v1.MsgGrantRole", doc.Messages[0]["@type"])
}

// ---------------------------------------------------------------- the dossier

func TestTheDossierTakesMembersAndThresholdFromTheCeremonyNotTheAppointmentConfig(t *testing.T) {
	configureAddresses()
	// Same rule the country enrolment follows: the config contributes the name,
	// the chain and the reason, and nothing that decides who holds the power.
	dir := t.TempDir()
	groupPath := filepath.Join(dir, "group.json")

	params := ceremonyParams{
		ID: "5NQ8HD-7XVBKR-2WCT0M-9JZFPA", Name: "Yamale foundation administrators",
		ChainID: "yamale-devnet-2", Threshold: 3,
		Custodians: []string{"A. Okafor", "Naledi Ngũgĩ", "Bank of Yamale", "J. Mwangi"},
		PolicySeq:  11, VotingPeriod: "168h0m0s", Administrators: true,
	}
	require.NoError(t, params.validate())

	subs := make([]submission, 0, len(params.Custodians))
	for i, name := range params.Custodians {
		s, err := secretFromInput(fixturePhrase(i))
		require.NoError(t, err)
		priv, path, err := s.derive(0)
		require.NoError(t, err)
		generated, err := time.Parse(time.RFC3339, vectorTimes[i])
		require.NoError(t, err)
		id, err := identityOf(name, roleCustodian, priv, path, generated)
		require.NoError(t, err)
		sub, err := signSubmission(params.ID, id, priv)
		require.NoError(t, err)
		subs = append(subs, sub)
	}
	assembled, err := assembleGroup(params, subs)
	require.NoError(t, err)
	require.NoError(t, writeJSONFile(groupPath, exportedGroup{assembled: assembled, Submissions: subs}))

	dossier, err := administratorsDossierFor(administratorsConfig{
		Ceremony: "Yamale foundation administrators",
		ChainID:  "yamale-devnet-2",
		Group:    groupPath,
		Reason:   "Because.",
	}, adminTestTime())
	require.NoError(t, err)

	require.Equal(t, 3, dossier.Threshold)
	require.Len(t, dossier.Members, 4)
	require.Equal(t, assembled.Fingerprint, dossier.GroupFingerprint)
	require.Nil(t, dossier.OnChain, "a fresh dossier must carry no address")
	require.Nil(t, dossier.Appointed)
	// Sorted by address, so the dossier and x/group's answer can be compared
	// directly — x/group returns members in its own order.
	require.IsIncreasing(t, dossier.memberAddresses())
}

func TestTheDossierRefusesAGroupFileFromAnotherKindOfCeremony(t *testing.T) {
	configureAddresses()
	dir := t.TempDir()
	groupPath := filepath.Join(dir, "group.json")
	// A foundation ceremony: Administrators is false.
	params := ceremonyParams{
		ID: "K4T9RM-2QWXVZ-8H0PBN-5CJDGF", Name: "Yamale foundation",
		ChainID: "yamale-devnet-2", Threshold: 3,
		Custodians: []string{"A. Okafor", "Naledi Ngũgĩ", "Bank of Yamale", "J. Mwangi"},
		PolicySeq:  1, VotingPeriod: "168h0m0s",
	}
	subs := make([]submission, 0, len(params.Custodians))
	for i, name := range params.Custodians {
		s, err := secretFromInput(fixturePhrase(i))
		require.NoError(t, err)
		priv, path, err := s.derive(0)
		require.NoError(t, err)
		generated, err := time.Parse(time.RFC3339, vectorTimes[i])
		require.NoError(t, err)
		id, err := identityOf(name, roleCustodian, priv, path, generated)
		require.NoError(t, err)
		sub, err := signSubmission(params.ID, id, priv)
		require.NoError(t, err)
		subs = append(subs, sub)
	}
	assembled, err := assembleGroup(params, subs)
	require.NoError(t, err)
	require.NoError(t, writeJSONFile(groupPath, exportedGroup{assembled: assembled, Submissions: subs}))

	_, err = administratorsDossierFor(administratorsConfig{
		Ceremony: "Yamale foundation", ChainID: "yamale-devnet-2",
		Group: groupPath, Reason: "Because.",
	}, adminTestTime())
	require.Error(t, err)
	require.Contains(t, err.Error(), "FOUNDATION's ceremony")
}
