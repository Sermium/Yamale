package main

// Tests for the appointment ceremony.
//
// What is worth testing here is not that a proposal comes out with the right keys
// in it. It is that the tool REFUSES in every case where composing something
// plausible would be worse than composing nothing — because MsgUpdateParams
// replaces the whole Params object, so each of those cases is a governance
// proposal that passes and silently changes a parameter nobody voted on.
//
// So most of what follows is refusals: an unconfirmed address, a payload_length
// that reads as zero, a group file from the wrong ceremony, an authority that is
// not the gov module. Those are the cases that would still fail if somebody
// "simplified" this by adding a default.

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

func liveParams(administrators ...string) aliasParams {
	return aliasParams{
		PayloadLength:            8,
		FoundationAdministrators: administrators,
		Authority:                adminGovAddr,
	}
}

// decodeProposal pulls the composed MsgUpdateParams back out of the document.
//
// Decoded rather than string-matched, because the whole reason the document is
// built through the proto codec is that a field renamed in the proto should break
// this rather than produce JSON that reads correctly and decodes to a zero.
func decodeProposal(t *testing.T, blob []byte) (govProposalDocument, aliastypes.MsgUpdateParams) {
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
	msg, ok := any.(*aliastypes.MsgUpdateParams)
	require.True(t, ok, "the proposal's message is %T, not a MsgUpdateParams", any)
	return doc, *msg
}

// ------------------------------------------------------------ the whole object

func TestTheProposalCarriesEveryExistingAdministrator(t *testing.T) {
	configureAddresses()
	// The failure the whole design exists to prevent. Composed by hand, the
	// proposal that appoints one administrator drops the others — and it passes,
	// because a shorter list is a valid list.
	existing := []string{adminFoundAddr, adminMemberA}
	blob, err := appointmentProposal(confirmedAppointment(), liveParams(existing...), "1000000uyml")
	require.NoError(t, err)

	_, msg := decodeProposal(t, blob)
	require.Len(t, msg.Params.FoundationAdministrators, 3)
	for _, address := range existing {
		require.Contains(t, msg.Params.FoundationAdministrators, address,
			"an existing administrator was dropped by the proposal")
	}
	require.Contains(t, msg.Params.FoundationAdministrators, adminGroupAddr)
}

func TestTheProposalCarriesPayloadLengthRatherThanResettingIt(t *testing.T) {
	configureAddresses()
	// A chain that had raised payload_length to 12 must not have it silently reset
	// to the default of 8 by a proposal that reads as an appointment.
	params := liveParams()
	params.PayloadLength = 12
	blob, err := appointmentProposal(confirmedAppointment(), params, "1000000uyml")
	require.NoError(t, err)

	_, msg := decodeProposal(t, blob)
	require.Equal(t, uint32(12), msg.Params.PayloadLength)
}

func TestTheResultingListIsSorted(t *testing.T) {
	configureAddresses()
	// So the object depends on the SET of administrators rather than on the order
	// appointments happened to be proposed in.
	one, err := appointmentProposal(confirmedAppointment(), liveParams(adminMemberA, adminFoundAddr), "1uyml")
	require.NoError(t, err)
	two, err := appointmentProposal(confirmedAppointment(), liveParams(adminFoundAddr, adminMemberA), "1uyml")
	require.NoError(t, err)

	_, first := decodeProposal(t, one)
	_, second := decodeProposal(t, two)
	require.Equal(t, first.Params.FoundationAdministrators, second.Params.FoundationAdministrators)
	require.IsIncreasing(t, first.Params.FoundationAdministrators)
}

func TestTheAuthorityIsTheGovernanceModuleAccount(t *testing.T) {
	configureAddresses()
	blob, err := appointmentProposal(confirmedAppointment(), liveParams(), "1000000uyml")
	require.NoError(t, err)
	_, msg := decodeProposal(t, blob)
	require.Equal(t, adminGovAddr, msg.Authority)
}

// ------------------------------------------------------------------- refusals

func TestAnUnconfirmedDossierProposesNothing(t *testing.T) {
	configureAddresses()
	// The refusal the two-phase design exists to make. No confirmed address, no
	// proposal, no fallback to a predicted one.
	dossier := confirmedAppointment()
	dossier.OnChain = nil
	_, err := appointmentProposal(dossier, liveParams(), "1000000uyml")
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
	_, err = appointmentProposal(dossier, liveParams(), "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "should be impossible")

	// And on the verification path, where without the guard the failure would read
	// as "the group is not in the administrator list" — which is true of the empty
	// string and tells nobody anything.
	path := writeAdminJSON(t, "p.json", `{"params":{"payload_length":8}}`)
	_, err = verifyAppointment(dossier, path, adminTestTime())
	require.Error(t, err)
	require.Contains(t, err.Error(), "should be impossible")
}

func TestAPayloadLengthOfZeroIsRefusedNotDefaulted(t *testing.T) {
	configureAddresses()
	// Proto3 cannot tell a zero from a field nobody filled in, so a zero means
	// UNKNOWN. Defaulting it to eight would compose a proposal that reset the
	// identifier length of a chain that had raised it, with no sign of it anywhere
	// a person would look.
	params := liveParams()
	params.PayloadLength = 0
	_, err := appointmentProposal(confirmedAppointment(), params, "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "will not guess it")
	require.Contains(t, err.Error(), "Proto3")
}

func TestAPayloadLengthTheChainWouldRefuseIsNotQuietlyCorrected(t *testing.T) {
	configureAddresses()
	// The message is asserted, and a mutation pass is why. With this bounds check
	// removed the chain's own Params.Validate() further down still refused every
	// one of these, so the test passed — but the message had become "the parameters
	// this proposal would set are ones the chain refuses", which points at the
	// proposal. The value came off the chain. If the chain reports a
	// payload_length the chain would reject, the problem is upstream of anything
	// this tool is composing, and the refusal has to say so or somebody will go on
	// editing the proposal.
	for _, length := range []uint32{1, aliastypes.MinPayloadLen - 1, aliastypes.MaxPayloadLen + 1, 99} {
		params := liveParams()
		params.PayloadLength = length
		_, err := appointmentProposal(confirmedAppointment(), params, "1000000uyml")
		require.Error(t, err, "payload_length %d should be refused", length)
		require.Contains(t, err.Error(), "will not quietly correct it",
			"payload_length %d was refused for the wrong reason", length)
	}
	// And the boundaries themselves are accepted, so the rule is the chain's and
	// not one narrower.
	for _, length := range []uint32{aliastypes.MinPayloadLen, aliastypes.MaxPayloadLen} {
		params := liveParams()
		params.PayloadLength = length
		_, err := appointmentProposal(confirmedAppointment(), params, "1000000uyml")
		require.NoError(t, err, "payload_length %d should be accepted", length)
	}
}

func TestAMissingAuthorityIsRefused(t *testing.T) {
	configureAddresses()
	params := liveParams()
	params.Authority = ""
	_, err := appointmentProposal(confirmedAppointment(), params, "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "governance module account")

	// And one that is not an address this chain can read.
	params.Authority = "not-an-address"
	_, err = appointmentProposal(confirmedAppointment(), params, "1000000uyml")
	require.Error(t, err)
}

func TestADuplicateAppointmentIsRefusedBeforeTheVote(t *testing.T) {
	configureAddresses()
	// Params.Validate() refuses a list naming one address twice, so this would
	// fail when it executed — after the vote.
	_, err := appointmentProposal(confirmedAppointment(), liveParams(adminGroupAddr), "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already a foundation administrator")
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

	_, err := appointmentProposal(confirmedAppointment(), liveParams(eight...), "1000000uyml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "caps the list at 8")

	// The eighth is allowed, so the cap is the cap and not one less.
	_, err = appointmentProposal(confirmedAppointment(), liveParams(eight[:7]...), "1000000uyml")
	require.NoError(t, err)
}

func TestAProposalWithNoDepositIsRefused(t *testing.T) {
	configureAddresses()
	// A proposal submitted with less than the minimum is accepted, sits in the
	// deposit period, and never enters a vote — which looks from the outside
	// exactly like a proposal nobody got round to voting on.
	_, err := appointmentProposal(confirmedAppointment(), liveParams(), "  ")
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

// -------------------------------------------------------------- the parameters

func writeAdminJSON(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestTheAuthorityMustBeTheGovModuleAccountAndIsChecked(t *testing.T) {
	configureAddresses()
	params := writeAdminJSON(t, "p.json", `{"params":{"payload_length":8,"foundation_administrators":[]}}`)

	// The right one.
	good := writeAdminJSON(t, "gov.json",
		`{"account":{"value":{"address":"`+adminGovAddr+`","name":"gov"}}}`)
	read, err := readAliasParamsFiles(params, good)
	require.NoError(t, err)
	require.Equal(t, adminGovAddr, read.Authority)
	require.Equal(t, uint32(8), read.PayloadLength)

	// A different module account. x/alias refuses MsgUpdateParams from any signer
	// but governance, so this would pass a vote and then be refused at execution.
	wrong := writeAdminJSON(t, "distribution.json",
		`{"account":{"value":{"address":"`+adminFoundAddr+`","name":"distribution"}}}`)
	_, err = readAliasParamsFiles(params, wrong)
	require.Error(t, err)
	require.Contains(t, err.Error(), `"distribution"`)
}

func TestBothShapesOfTheModuleAccountAnswerAreRead(t *testing.T) {
	configureAddresses()
	params := writeAdminJSON(t, "p.json", `{"params":{"payload_length":8}}`)

	// The CLI's shape and the gateway's flatter one. Both, because guessing one
	// would mean reading an empty address out of a file that plainly contains it,
	// and then refusing with a message about the authority being missing.
	cli := writeAdminJSON(t, "cli.json",
		`{"account":{"value":{"address":"`+adminGovAddr+`","name":"gov"}}}`)
	rest := writeAdminJSON(t, "rest.json",
		`{"account":{"name":"gov","base_account":{"address":"`+adminGovAddr+`"}}}`)

	for _, path := range []string{cli, rest} {
		read, err := readAliasParamsFiles(params, path)
		require.NoError(t, err, path)
		require.Equal(t, adminGovAddr, read.Authority)
	}
}

func TestPayloadLengthIsReadAsANumberOrAString(t *testing.T) {
	configureAddresses()
	gov := writeAdminJSON(t, "gov.json",
		`{"account":{"value":{"address":"`+adminGovAddr+`","name":"gov"}}}`)

	// The CLI renders a uint32 as a number and the gateway has rendered it as a
	// string. A type accepting only one would read zero from the other — and zero
	// is the value that means "unknown" here, so the tool would refuse a perfectly
	// good answer.
	for _, body := range []string{
		`{"params":{"payload_length":12}}`,
		`{"params":{"payload_length":"12"}}`,
	} {
		read, err := readAliasParamsFiles(writeAdminJSON(t, "p.json", body), gov)
		require.NoError(t, err, body)
		require.Equal(t, uint32(12), read.PayloadLength, body)
	}
}

func TestAnAbsentAdministratorListReadsAsEmpty(t *testing.T) {
	configureAddresses()
	// This is what the CLI actually returns on the live devnet: the empty repeated
	// field is omitted entirely. Read as empty rather than refused, because for a
	// repeated field absent and empty ARE the same value — there is no third state
	// to confuse them with, unlike payload_length.
	gov := writeAdminJSON(t, "gov.json",
		`{"account":{"value":{"address":"`+adminGovAddr+`","name":"gov"}}}`)
	read, err := readAliasParamsFiles(writeAdminJSON(t, "p.json", `{"params":{"payload_length":8}}`), gov)
	require.NoError(t, err)
	require.Empty(t, read.FoundationAdministrators)
}

// ---------------------------------------------------------------- verification

func TestVerifyRefusesUntilTheGroupIsActuallyInTheParameters(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()

	// Not appointed. A proposal that PASSED can still fail when it executes, which
	// leaves the parameters exactly as they were and says so only in a transaction
	// log nobody is watching.
	absent := writeAdminJSON(t, "p.json",
		`{"params":{"payload_length":8,"foundation_administrators":["`+adminFoundAddr+`"]}}`)
	_, err := verifyAppointment(dossier, absent, adminTestTime())
	require.Error(t, err)
	require.Contains(t, err.Error(), "is NOT in alias.params.foundation_administrators")
	require.Contains(t, err.Error(), "can still fail when it executes")

	// Appointed. The whole list is recorded, not just this group.
	present := writeAdminJSON(t, "q.json",
		`{"params":{"payload_length":8,"foundation_administrators":["`+adminFoundAddr+`","`+adminGroupAddr+`"]}}`)
	verified, err := verifyAppointment(dossier, present, adminTestTime())
	require.NoError(t, err)
	require.Equal(t, adminGroupAddr, verified.PolicyAddress)
	require.Len(t, verified.Administrators, 2)
	require.Equal(t, uint32(8), verified.PayloadLength)
	require.Equal(t, "2026-08-23T11:00:00Z", verified.VerifiedAt)
}

func TestVerifyRefusesParametersItCannotRead(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()
	// Zero payload_length means the file is not what the chain holds, so nothing
	// is recorded from it — including an appointment that happens to be listed.
	bad := writeAdminJSON(t, "p.json",
		`{"params":{"foundation_administrators":["`+adminGroupAddr+`"]}}`)
	_, err := verifyAppointment(dossier, bad, adminTestTime())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no usable payload_length")
}

func TestVerifyRefusesAnUnconfirmedDossier(t *testing.T) {
	configureAddresses()
	dossier := confirmedAppointment()
	dossier.OnChain = nil
	path := writeAdminJSON(t, "p.json", `{"params":{"payload_length":8}}`)
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
	blob, err := appointmentProposal(confirmedAppointment(), liveParams(adminFoundAddr), "1000000uyml")
	require.NoError(t, err)
	doc, _ := decodeProposal(t, blob)

	// The power, because "foundation_administrators" reads like a list of people
	// with logins and gives no hint of what it confers. This is the assertion that
	// found the bug: composed as one sentence with the address and the ceremony
	// name first, this was the part that fell off the end of the 255-byte cap.
	require.Contains(t, doc.Summary, "correct the country recorded against ANY account")
	require.Contains(t, doc.Summary, "reissues its identifier")
	require.Contains(t, doc.Summary, aliastypes.FoundationCountry)
	// The counts, because a list that SHRANK is the only visible evidence of a
	// proposal composed from a stale read of the parameters.
	require.Contains(t, doc.Summary, "from 1 to 2")
	require.Contains(t, doc.Summary, adminGroupAddr)
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
	blob, err := appointmentProposal(confirmedAppointment(), liveParams(adminFoundAddr), "1uyml")
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

	blob, err := appointmentProposal(dossier, liveParams(adminFoundAddr, adminMemberA), "1uyml")
	require.NoError(t, err)
	doc, _ := decodeProposal(t, blob)

	require.LessOrEqual(t, len(doc.Summary), maxSummaryLen)
	require.Contains(t, doc.Summary, "correct the country recorded against ANY account")
	require.Contains(t, doc.Summary, "from 2 to 3")
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

	_, err := appointmentProposal(dossier, liveParams(), "1uyml")
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
	blob, err := appointmentProposal(dossier, liveParams(), "1000000uyml")
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
	blob, err := appointmentProposal(confirmedAppointment(), liveParams(), "1000000uyml")
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

func TestTheMessageTypeIsTheAliasUpdateParams(t *testing.T) {
	configureAddresses()
	blob, err := appointmentProposal(confirmedAppointment(), liveParams(), "1000000uyml")
	require.NoError(t, err)

	var doc struct {
		Messages []map[string]any `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(blob, &doc))
	require.Len(t, doc.Messages, 1)
	// The proto package, not the REST path: the REST prefix carries `yamale` and
	// the proto package does not.
	require.Equal(t, "/blockchain.alias.v1.MsgUpdateParams", doc.Messages[0]["@type"])
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
