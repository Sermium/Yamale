package main

// The scriptable half of a country office's key ceremony.
//
// It exists for the same reason TestFiveCustodianCeremony does, and the reason is
// worth restating because it looks like a shortcut and is not: the production
// entry point for an office's keys is a web page, and a browser cannot be
// scripted from a shell. Everything an office needs verified about its keys is
// already verified by the 150 tests in this package — host_test.go drives the
// whole hosted flow over HTTP, submission by submission, including the refusals.
// What this harness does is produce the artefacts a LIVE CHAIN needs so that the
// enrolment can be exercised end to end against a running node.
//
// It is skipped unless CEREMONY_OFFICE_DIR names an output directory.
//
// Two things it deliberately does not do, so that nobody mistakes it for the
// ceremony:
//
//   - It does not go through `ceremony host`. It calls signSubmission and
//     assembleGroup directly, which is the same pure function the browser runs
//     and the same one the coordinator runs; what is skipped is the HTTP layer,
//     the invite tokens and the attestations. Those are what host_test.go covers.
//   - It writes ARMORED private keys, which a real office ceremony never does.
//     A super user's key belongs on their own device and nowhere else. These
//     exist because a devnet has to be able to sign a group vote, and they are
//     written under a passphrase into a directory somebody is expected to delete.
//
// So: this is the rehearsal harness. A real office runs `ceremony host`, and the
// runbook says so.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOfficeCeremonyForEnrolment(t *testing.T) {
	dir := os.Getenv("CEREMONY_OFFICE_DIR")
	if dir == "" {
		t.Skip("set CEREMONY_OFFICE_DIR to run the office ceremony harness against a chain")
	}
	configureAddresses()

	name := envOr("CEREMONY_OFFICE_NAME", "Senegal payments authority")
	chainID := envOr("CEREMONY_OFFICE_CHAIN", "yamale-local-1")
	country := envOr("CEREMONY_OFFICE_COUNTRY", "SN")
	roles := splitList(envOr("CEREMONY_OFFICE_ROLES",
		"ROLE_PAYMENTS_AUTHORITY,ROLE_ENFORCEMENT_AUTHORITY"))
	members := splitList(envOr("CEREMONY_OFFICE_MEMBERS", "A. Diallo,B. Sow,C. Fall"))
	threshold, err := strconv.Atoi(envOr("CEREMONY_OFFICE_THRESHOLD", "2"))
	if err != nil {
		t.Fatalf("CEREMONY_OFFICE_THRESHOLD: %v", err)
	}
	passphrase := envOr("CEREMONY_OFFICE_PASSPHRASE", "office-rehearsal-passphrase")

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	ceremonyID, err := newCeremonyID()
	if err != nil {
		t.Fatal(err)
	}

	params := ceremonyParams{
		ID:           ceremonyID,
		Name:         name,
		ChainID:      chainID,
		Threshold:    threshold,
		Custodians:   members,
		VotingPeriod: envOr("CEREMONY_OFFICE_VOTING_PERIOD", "168h0m0s"),
		// PolicySeq is set and is MEANINGLESS for an office, which is the whole
		// point of the enrolment procedure. The address it predicts is not this
		// office's address: the chain decides that when the group is created, and
		// `ceremony country confirm` reads it back and checks it against these
		// members. It is populated only because the parameters carry it.
		PolicySeq: 1,
		Office:    &officeParams{Country: country, Roles: roles},
	}
	if err := params.validate(); err != nil {
		t.Fatalf("the office's parameters are invalid: %v", err)
	}

	at := time.Now().UTC().Truncate(time.Second)
	submissions := make([]submission, 0, len(members))
	for i, member := range members {
		secret, err := newSecret()
		if err != nil {
			t.Fatal(err)
		}
		priv, path, err := secret.derive(0)
		if err != nil {
			secret.zero()
			t.Fatal(err)
		}
		id, err := identityOf(member, roleCustodian, priv, path, at)
		if err != nil {
			zero(priv.Key)
			secret.zero()
			t.Fatal(err)
		}
		sub, err := signSubmission(ceremonyID, id, priv)
		if err != nil {
			zero(priv.Key)
			secret.zero()
			t.Fatal(err)
		}

		armorPath := filepath.Join(dir, fmt.Sprintf("office-%d.asc", i+1))
		if err := writeArmorTo(armorPath, priv, passphrase); err != nil {
			zero(priv.Key)
			secret.zero()
			t.Fatal(err)
		}
		// Zeroed on the way out of the iteration, exactly as runKeyCeremony does.
		// The armor file is the copy that leaves, and it is the only one.
		zero(priv.Key)
		secret.zero()

		submissions = append(submissions, sub)
		t.Logf("super user %d: %s", i+1, sub.Identity.describe())
	}

	built, err := assembleGroup(params, submissions)
	if err != nil {
		t.Fatal(err)
	}

	// group.json in the shape the hosted ceremony's export writes it: the
	// assembled document plus the submissions it was computed from, so that
	// `ceremony country init` can recompute rather than read.
	groupPath := filepath.Join(dir, "group.json")
	if err := writeJSONFile(groupPath, struct {
		assembled
		Submissions []submission `json:"submissions"`
	}{built, submissions}); err != nil {
		t.Fatal(err)
	}

	t.Logf("office            %s", name)
	t.Logf("country           %s", country)
	t.Logf("roles             %s", strings.Join(roles, ", "))
	t.Logf("rule              %d of %d", threshold, len(members))
	t.Logf("ceremony id       %s", ceremonyID)
	t.Logf("params print      %s", params.fingerprint())
	t.Logf("group print       %s", built.Fingerprint)
	t.Logf("group file        %s", groupPath)
	t.Logf("predicted address %s  <- NOT this office's address; the chain decides",
		built.PolicyAddress)
}

// TestCountryCommand runs one `ceremony country` step, taking its arguments from
// the environment.
//
// A shim over the CLI's own entry point — it calls runCountry, which is exactly
// what main() calls — and it exists so an end-to-end script can drive the
// enrolment against a live chain on a machine where a freshly built binary cannot
// be executed. Windows application control on the development machine refuses to
// run a `go build` output it has not seen before while permitting the package's
// test binary, so the test binary is the only way to reach this code there. The
// same shim is useful anywhere for scripting the steps without a second build.
//
// It is not a substitute for the CLI and it does not test anything on its own:
// with CEREMONY_COUNTRY_ARGS unset it skips. What it proves when the script that
// calls it passes is exactly what the CLI would prove, because it is the same
// function reading the same files and writing the same dossier.
//
//	CEREMONY_COUNTRY_ARGS="init --config senegal.json --out ." \
//	  go test ./tools/ceremony -run TestCountryCommand
func TestCountryCommand(t *testing.T) {
	raw := os.Getenv("CEREMONY_COUNTRY_ARGS")
	if raw == "" {
		t.Skip("set CEREMONY_COUNTRY_ARGS to run one `ceremony country` step")
	}
	configureAddresses()

	args, err := splitArgs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := runCountry(args); err != nil {
		// Printed as well as failed, because the whole value of these steps is the
		// text of the refusal and `go test` truncates a long failure message.
		fmt.Println("ceremony:", err)
		t.Fatalf("ceremony country %s: %v", strings.Join(args, " "), err)
	}
}

// splitArgs splits a command line, honouring double quotes so an office name with
// spaces in it survives.
//
// Written out rather than taken from a library because the ceremony tool has no
// dependencies beyond the SDK, and because the only quoting this needs to handle
// is the one an office name requires.
func splitArgs(line string) ([]string, error) {
	var args []string
	var current strings.Builder
	quoted := false
	pending := false

	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
			pending = true
		case (r == ' ' || r == '\t' || r == '\n') && !quoted:
			if pending || current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
				pending = false
			}
		default:
			current.WriteRune(r)
			pending = true
		}
	}
	if quoted {
		return nil, fmt.Errorf("unbalanced quote in %q", line)
	}
	if pending || current.Len() > 0 {
		args = append(args, current.String())
	}
	return args, nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
