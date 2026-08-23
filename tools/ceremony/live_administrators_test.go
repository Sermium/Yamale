package main

// The scriptable half of a foundation-administrator ceremony.
//
// It exists for the same reason TestOfficeCeremonyForEnrolment does, and the same
// caveat applies: the production entry point for these keys is a web page, and a
// browser cannot be scripted from a shell. Everything about the keys themselves is
// already verified by the rest of this package — host_test.go drives the whole
// hosted flow over HTTP, submission by submission, including the refusals. What
// this harness does is produce the artefacts a LIVE CHAIN needs, so that an
// appointment can be exercised end to end against a running node.
//
// It is skipped unless CEREMONY_ADMIN_DIR names an output directory.
//
// Two things it deliberately does not do, so nobody mistakes it for the ceremony:
//
//   - It does not go through `ceremony host`. It calls signSubmission and
//     assembleGroup directly, which is the same pure function the browser runs.
//     What is skipped is the HTTP layer, the invite tokens and the attestations.
//   - It writes ARMORED private keys, which a real ceremony never does. A
//     custodian's key belongs on their own device and nowhere else. These exist
//     because a devnet has to be able to sign a group vote, and they are written
//     under a passphrase into a directory somebody is expected to delete.
//
// So: this is the rehearsal harness, and docs/guides/foundation-administrators.md
// says a real appointment runs `ceremony host`.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestAdministratorCeremonyForAppointment(t *testing.T) {
	dir := os.Getenv("CEREMONY_ADMIN_DIR")
	if dir == "" {
		t.Skip("set CEREMONY_ADMIN_DIR to run the administrator ceremony harness against a chain")
	}
	configureAddresses()

	name := envOr("CEREMONY_ADMIN_NAME", "Yamale foundation administrators")
	chainID := envOr("CEREMONY_ADMIN_CHAIN", "yamale-local-1")
	members := splitList(envOr("CEREMONY_ADMIN_MEMBERS", "A. Diallo,B. Sow,C. Fall,D. Ba"))
	threshold, err := strconv.Atoi(envOr("CEREMONY_ADMIN_THRESHOLD", "3"))
	if err != nil {
		t.Fatalf("CEREMONY_ADMIN_THRESHOLD: %v", err)
	}
	passphrase := envOr("CEREMONY_ADMIN_PASSPHRASE", "administrator-rehearsal-passphrase")

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
		VotingPeriod: envOr("CEREMONY_ADMIN_VOTING_PERIOD", "168h0m0s"),
		// PolicySeq is set and is MEANINGLESS here, which is the whole point of the
		// two-phase appointment. The address it predicts is not this group's: the
		// chain decides that when the group is created, and `ceremony
		// administrators confirm` reads it back and checks it against these
		// members. On a live run of the country ceremony a predicted address came
		// out as the foundation's own, both being sequence 1.
		PolicySeq: 1,
		// The marker. Without it this would be a FOUNDATION ceremony: the group
		// would be recorded on chain as "Yamale foundation", and `ceremony
		// administrators init` would refuse the file — which is the check being
		// exercised by setting it.
		Administrators: true,
	}
	if err := params.validate(); err != nil {
		t.Fatalf("the administrator group's parameters are invalid: %v", err)
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

		armorPath := filepath.Join(dir, fmt.Sprintf("administrator-%d.asc", i+1))
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
		t.Logf("custodian %d: %s", i+1, sub.Identity.describe())
	}

	built, err := assembleGroup(params, submissions)
	if err != nil {
		t.Fatal(err)
	}
	// Asserted here rather than left to the appointment steps, because a harness
	// that had produced the fragment would have written the dangerous document to
	// the directory before anything downstream could refuse it.
	if built.Constitution != nil {
		t.Fatal("an administrator group must carry no constitutional invariants fragment")
	}

	groupPath := filepath.Join(dir, "group.json")
	if err := writeJSONFile(groupPath, struct {
		assembled
		Submissions []submission `json:"submissions"`
	}{built, submissions}); err != nil {
		t.Fatal(err)
	}

	t.Logf("ceremony          %s", name)
	t.Logf("rule              %d of %d", threshold, len(members))
	t.Logf("ceremony id       %s", ceremonyID)
	t.Logf("params print      %s", params.fingerprint())
	t.Logf("group print       %s", built.Fingerprint)
	t.Logf("recorded as       %s", groupLabel(params))
	t.Logf("group file        %s", groupPath)
	t.Logf("predicted address %s  <- NOT this group's address; the chain decides",
		built.PolicyAddress)
}

// TestAdministratorsCommand runs one `ceremony administrators` step, taking its
// arguments from the environment.
//
// A shim over the CLI's own entry point — it calls runAdministrators, which is
// exactly what main() calls — and it exists so an end-to-end script can drive the
// appointment against a live chain on a machine where a freshly built binary
// cannot be executed. Windows application control on the development machine
// refuses to run a `go build` output it has not seen before while permitting the
// package's test binary.
//
// It is not a substitute for the CLI and it tests nothing on its own: with
// CEREMONY_ADMIN_ARGS unset it skips. What it proves when the script that calls it
// passes is exactly what the CLI would prove, because it is the same function
// reading the same files and writing the same dossier.
//
//	CEREMONY_ADMIN_ARGS="init --config administrators.json --out ." \
//	  go test ./tools/ceremony -run TestAdministratorsCommand
func TestAdministratorsCommand(t *testing.T) {
	raw := os.Getenv("CEREMONY_ADMIN_ARGS")
	if raw == "" {
		t.Skip("set CEREMONY_ADMIN_ARGS to run one `ceremony administrators` step")
	}
	configureAddresses()
	args, err := splitArgs(raw)
	if err != nil {
		t.Fatalf("CEREMONY_ADMIN_ARGS: %v", err)
	}
	if err := runAdministrators(args); err != nil {
		t.Fatalf("ceremony administrators %s: %v", raw, err)
	}
}
