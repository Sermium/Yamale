package main

import (
	"strings"
	"testing"
)

func testRecordConfig() recordConfig {
	return recordConfig{
		Ceremony:    "yamale-testnet-1 foundation",
		ChainID:     "yamale-testnet-1",
		Location:    "Geneva",
		StartedAt:   "2026-09-01T09:00:00Z",
		CompletedAt: "2026-09-01T14:30:00Z",
		Threshold:   3,
		Participants: []participant{
			{Name: "R. Lead", Role: "ceremony lead", Organisation: "Yamale Foundation"},
			{Name: "S. Scribe", Role: "scribe", Organisation: "Yamale Foundation"},
			{Name: "O. Observer", Role: "independent observer", Organisation: "External Auditors LLP"},
		},
		PolicyAddress: firstPolicyAddress,
	}
}

func TestRenderRecordNamesEveryCustodianAndTheirFingerprint(t *testing.T) {
	people := custodians(t, 5)

	rendered, err := renderRecord(testRecordConfig(), people)
	if err != nil {
		t.Fatal(err)
	}

	for _, person := range people {
		if !strings.Contains(rendered, person.Address) {
			t.Fatalf("%s's address is missing from the record", person.Name)
		}
		// The fingerprint is the whole point of the record five years later:
		// an envelope either recovers to a key that matches this row or it
		// does not.
		if !strings.Contains(rendered, person.Fingerprint) {
			t.Fatalf("%s's fingerprint is missing from the record", person.Name)
		}
	}
	if !strings.Contains(rendered, firstPolicyAddress) {
		t.Fatal("the record does not name the group policy address, which is the reason it exists")
	}
	if !strings.Contains(rendered, "3-of-5") {
		t.Fatal("the record does not say what the threshold is")
	}
}

func TestRenderRecordGivesEverybodyASignatureLine(t *testing.T) {
	people := custodians(t, 5)
	config := testRecordConfig()

	rendered, err := renderRecord(config, people)
	if err != nil {
		t.Fatal(err)
	}

	attestation := rendered[strings.Index(rendered, "## Attestation"):]
	for _, p := range config.Participants {
		if !strings.Contains(attestation, p.Name) {
			t.Fatalf("%s has no signature line", p.Name)
		}
	}
	for _, person := range people {
		if !strings.Contains(attestation, person.Name) {
			t.Fatalf("custodian %s has no signature line", person.Name)
		}
	}
}

func TestRenderRecordCarriesNoticesOfWhatWentWrong(t *testing.T) {
	people := custodians(t, 5)
	config := testRecordConfig()
	config.Notes = []string{
		"Custodian 2's first key was destroyed and regenerated after the screen was photographed.",
	}

	rendered, err := renderRecord(config, people)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "destroyed and regenerated") {
		t.Fatal("a recorded exposure did not reach the document everybody signs")
	}
}

func TestRenderRecordAssertsNothingHappenedWhenNothingWasRecorded(t *testing.T) {
	// An empty note list is a claim, and the record has to make it explicitly
	// rather than by omission — otherwise signing it says nothing about
	// whether the room had a problem it did not write down.
	people := custodians(t, 5)

	rendered, err := renderRecord(testRecordConfig(), people)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "Nothing was recorded as out of the ordinary") {
		t.Fatal("an uneventful ceremony produced a record that says nothing either way")
	}
}

func TestRenderRecordRefusesAGroupThatCannotSurviveALoss(t *testing.T) {
	people := custodians(t, 5)

	config := testRecordConfig()
	config.Threshold = 5
	if _, err := renderRecord(config, people); err == nil {
		t.Fatal("a 5-of-5 was recorded as if it were a working arrangement")
	}

	config.Threshold = 1
	if _, err := renderRecord(config, people); err == nil {
		t.Fatal("a 1-of-5 was recorded")
	}
}

func TestRenderRecordRequiresTheAddressAndTheChain(t *testing.T) {
	people := custodians(t, 5)

	config := testRecordConfig()
	config.PolicyAddress = ""
	if _, err := renderRecord(config, people); err == nil {
		t.Fatal("a record with no policy address was rendered")
	}

	config = testRecordConfig()
	config.ChainID = ""
	if _, err := renderRecord(config, people); err == nil {
		t.Fatal("a record naming an address but no chain was rendered")
	}
}

func TestRenderRecordContainsNoKeyMaterial(t *testing.T) {
	// The record is meant to be published. A regression that let a phrase or a
	// private key reach it would be the same failure this whole tool exists to
	// prevent, arriving through the one document everybody has been told is
	// safe to hand around.
	configureAddresses()

	s, err := secretFromInput(vectorPhrase)
	if err != nil {
		t.Fatal(err)
	}
	defer s.zero()
	priv, path, err := s.derive(0)
	if err != nil {
		t.Fatal(err)
	}
	known, err := identityOf("Known Custodian", roleCustodian, priv, path, testTime())
	if err != nil {
		t.Fatal(err)
	}

	people := append(custodians(t, 4), known)
	config := testRecordConfig()
	rendered, err := renderRecord(config, people)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(rendered, vectorPhrase) {
		t.Fatal("the published record contains a recovery phrase")
	}
	// Four consecutive words is well short of a usable phrase and well beyond
	// coincidence, so it catches a partial leak too.
	words := strings.Fields(vectorPhrase)
	for i := 0; i+4 <= len(words); i++ {
		window := strings.Join(words[i:i+4], " ")
		if strings.Contains(rendered, window) {
			t.Fatalf("the published record contains %q from a custodian's phrase", window)
		}
	}
	if strings.Contains(rendered, "BEGIN TENDERMINT PRIVATE KEY") {
		t.Fatal("the published record contains an armored private key")
	}
}

// TestRenderRecordDoesNotCallAnAdministratorGroupTheFoundation.
//
// A mutation pass found the administrator branch of renderRecord untested: with it
// deleted, every existing case still passed and an administrator group's record
// silently became the FOUNDATION's record. That document states that the address
// is the constitution's recovery destination and the place every seized asset on
// the chain is sent — false in the most consequential way a signed record can be
// false, and somebody would act on it.
func TestRenderRecordDoesNotCallAnAdministratorGroupTheFoundation(t *testing.T) {
	people := custodians(t, 4)
	config := testRecordConfig()
	config.Ceremony = "Yamale foundation administrators"
	config.Administrators = true
	// No address, deliberately: at the moment these keys are made the chain has not
	// created the group, so the record must not print one.
	config.PolicyAddress = ""

	rendered, err := renderRecord(config, people)
	if err != nil {
		t.Fatal(err)
	}

	// Substrings short enough to survive the record's hard line wrapping — it is
	// Markdown wrapped at about 80 columns, so "correct the country recorded
	// against" straddles a newline and would never match.
	for _, want := range []string{
		"may correct the country",         // what the power actually is
		"reissues the",                    // that a correction reissues the identifier
		"`ZZ`",                            // the reserved code
		"`ROLE_FOUNDATION_ADMINISTRATOR`", // what the appointment actually is
		"chain-wide `*` scope",            // and the scope that makes it governance's alone
		"cannot do\n",                     // and who cannot
		"has no address yet",              // the two-phase warning
		"foundation's own",                // the address a prediction produced on a live run
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the administrator record does not say %q", want)
		}
	}

	// And none of the foundation's claims, every one of which would be false of it.
	for _, forbidden := range []string{
		"## The foundation account",
		"enforcement_recovery_destination",
		"ever seizes is sent",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("the administrator record makes the foundation's claim %q", forbidden)
		}
	}
}

// TestAConfirmedAdministratorRecordPrintsTheAddressItReadBack.
//
// The record rendered before `confirm` says the group has no address, which is
// true. Rendered after, it must print the real one — this is the document somebody
// acts on, and "this group has no address" would be false in the field they came
// to read. The first version of this got it wrong in exactly that direction.
func TestAConfirmedAdministratorRecordPrintsTheAddressItReadBack(t *testing.T) {
	people := custodians(t, 4)
	config := testRecordConfig()
	config.Ceremony = "Yamale foundation administrators"
	config.Administrators = true
	config.PolicyAddress = "yml1dlszg2sst9r69my4f84l3mj66zxcf3umcgujys30t84srg95dgvsrmuayr"

	rendered, err := renderRecord(config, people)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, config.PolicyAddress) {
		t.Error("a confirmed administrator record does not print the address it read back")
	}
	if strings.Contains(rendered, "has no address yet") {
		t.Error("a confirmed administrator record still claims the group has no address")
	}
	if !strings.Contains(rendered, "read back off the chain") {
		t.Error("the record does not say where the address came from")
	}
	// The hazard stays on the record either way: it is the reason the address is
	// read rather than derived, and a reader years later needs the reason.
	if !strings.Contains(rendered, "foundation's own") {
		t.Error("the record drops the reason the address is read rather than predicted")
	}
}

// TestRenderRecordStillRequiresAnAddressForTheFoundation.
//
// The administrator and office branches may have no address, because the chain has
// not chosen one yet. The foundation's may not: its address is fixed by the genesis
// file it is going into, and a record with a blank where that belongs is the whole
// reason the document exists, missing.
func TestRenderRecordStillRequiresAnAddressForTheFoundation(t *testing.T) {
	people := custodians(t, 5)
	config := testRecordConfig()
	config.PolicyAddress = ""

	if _, err := renderRecord(config, people); err == nil {
		t.Fatal("a foundation record with no policy address should have been refused")
	}
}
