package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The contract this program exists to honour is one line of tools/custodian:
// a non-zero exit aborts the recovery. So every test here is about whether it
// fails when it should, rather than about the prose it writes.

func TestAStartedRecoveryIsRecordedForTheRightPerson(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YAMALE_NOTIFY_DIR", dir)
	withStdin(t, `{"event":"recovery-started","address":"yml1abc","email":"amara@example.org","delay_hours":72}`)

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	body := onlyFile(t, dir)
	for _, want := range []string{"amara@example.org", "yml1abc", "72 hours", "NOT YOU"} {
		if !strings.Contains(body, want) {
			t.Errorf("the notice does not mention %q:\n%s", want, body)
		}
	}
}

func TestTheFilenameIsNotAMailingList(t *testing.T) {
	// A directory listing is readable by anybody who can list the spool, and it
	// must not be a register of who is being recovered.
	dir := t.TempDir()
	t.Setenv("YAMALE_NOTIFY_DIR", dir)
	withStdin(t, `{"event":"recovery-started","address":"yml1abcdefghijklmn","email":"amara@example.org"}`)
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "amara") || strings.Contains(e.Name(), "@") {
			t.Fatalf("the filename carries the address: %s", e.Name())
		}
	}
}

func TestItFailsRatherThanLosingTheNotice(t *testing.T) {
	// Each of these must be a non-zero exit, because the custodian reads that
	// as "the notice did not happen" and refuses to start the recovery. A
	// program that shrugged would turn a silent recovery into the default.
	for name, setup := range map[string]func(t *testing.T) string{
		"no spool directory configured": func(t *testing.T) string {
			t.Setenv("YAMALE_NOTIFY_DIR", "")
			return `{"event":"recovery-started","email":"a@b.test"}`
		},
		"a notice with no address": func(t *testing.T) string {
			t.Setenv("YAMALE_NOTIFY_DIR", t.TempDir())
			return `{"event":"recovery-started","address":"yml1abc"}`
		},
		"an event this program does not know": func(t *testing.T) string {
			t.Setenv("YAMALE_NOTIFY_DIR", t.TempDir())
			return `{"event":"account-sold","email":"a@b.test"}`
		},
		"not JSON at all": func(t *testing.T) string {
			t.Setenv("YAMALE_NOTIFY_DIR", t.TempDir())
			return `<html>`
		},
	} {
		t.Run(name, func(t *testing.T) {
			withStdin(t, setup(t))
			if err := run(); err == nil {
				t.Fatal("this should have failed, and a recovery would have started unannounced")
			}
		})
	}
}

func TestTheNoticeFileIsNotWorldReadable(t *testing.T) {
	// It names a person and says their account is being recovered. Together
	// that is a list worth phoning.
	//
	// Skipped on Windows, and the skip is real rather than a convenience:
	// Chmod there sets the read-only attribute and nothing else, so Go reports
	// 0666 for a file created with 0600 and this assertion cannot hold. The
	// service deploys on Linux, where it can.
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not carry POSIX permission bits; this holds where it deploys")
	}
	dir := t.TempDir()
	t.Setenv("YAMALE_NOTIFY_DIR", dir)
	withStdin(t, `{"event":"recovery-completed","address":"yml1abc","email":"a@b.test","frozen_until":"2026-09-24T12:00:00Z"}`)
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("the notice is readable beyond its owner: %v", mode)
	}
}

/* ------------------------------------------------------------ helpers */

func withStdin(t *testing.T, body string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(body); err != nil {
		t.Fatal(err)
	}
	w.Close()

	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; r.Close() })
}

func onlyFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one notice, found %d", len(entries))
	}
	raw, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
