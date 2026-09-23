// Command notify is the reference notification program for tools/custodian.
//
// # Why this is a separate program and not a mailer inside the service
//
// The custodian takes --notify-command and runs it. That indirection is
// deliberate and is argued in tools/custodian/notify.go: a deployment must
// choose how it reaches people, and the shapes differ enough — SMTP, a
// provider's API, an SMS gateway, push to enrolled devices — that building one
// in would mean building the wrong one.
//
// What was missing was any program at all, which meant recovery refused to
// start everywhere. This is one, and it is deliberately the dullest possible
// one: it writes the notice to a spool directory as a file.
//
// # Why a spool directory rather than SMTP
//
// Because a reference implementation that speaks SMTP invites being deployed,
// and then the question of who holds the mail credentials, whether the
// connection is TLS, what happens to a bounce, and whether a queued message is
// retried all become this program's problems — answered badly, by a program
// written as an example.
//
// Writing a file has none of those questions and one useful property: the
// custodian's contract is that a NON-ZERO EXIT ABORTS THE RECOVERY. A file
// write either succeeds or fails, locally, immediately, with no network in the
// way. So a deployment gets the guarantee the design depends on — no recovery
// starts unless the notice was durably recorded — and can then move the spool
// with whatever it already trusts to deliver mail.
//
// Point --notify-command at something else the moment there is something else.
//
//	custodian --notify-command /usr/local/bin/yamale-notify
//	echo /var/spool/yamale-notices > /etc/yamale/notify.conf
//
// # Why the spool directory comes from a FILE and not the environment
//
// This was written to read an environment variable, and it could never have worked.
// The custodian does not pass its environment to the command — deliberately,
// because that environment holds the paths to the sealing key and the pepper —
// so the child sees nothing but PATH. A deployment would have exported the
// variable in the unit file, watched this program refuse every notice, and had
// no way to see why from either side.
//
// So the directory is read from a configuration file this program finds on its
// own. The environment variable still works and takes precedence, because it is
// what a test or a one-off run reaches for, but nothing depends on inheritance.
//
// # What it must not do
//
// Log the email. The custodian goes to some length never to store an address,
// and a notifier that writes one into a shared log has undone that for the
// convenience of whoever reads the log. The address goes in the notice file,
// which is the thing that has to contain it, and nowhere else.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// notice is the JSON the custodian writes to this program's stdin.
//
// Duplicated from tools/custodian rather than imported, on purpose: this is the
// shape of a contract between two programs, and a deployment replacing this one
// will reimplement it in whatever language it likes. Sharing a struct would
// suggest the contract is Go's rather than the JSON's.
type notice struct {
	Event       string `json:"event"`
	Address     string `json:"address"`
	Email       string `json:"email"`
	DelayHours  int    `json:"delay_hours,omitempty"`
	FrozenUntil string `json:"frozen_until,omitempty"`
}

func main() {
	if err := run(); err != nil {
		// Stderr and a non-zero exit. The custodian treats that as the notice
		// having failed, which aborts a recovery that is starting — the whole
		// point of the contract.
		fmt.Fprintln(os.Stderr, "notify:", err)
		os.Exit(1)
	}
}

func run() error {
	dir, err := spoolDir()
	if err != nil {
		return err
	}

	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 64*1024))
	if err != nil {
		return fmt.Errorf("reading the notice: %w", err)
	}
	var n notice
	if err := json.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("the notice is not JSON this understands: %w", err)
	}
	if strings.TrimSpace(n.Email) == "" {
		return fmt.Errorf("the notice carries no address to write to")
	}

	body, err := compose(n)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("the spool directory cannot be created: %w", err)
	}
	// 0600, and a directory that should not be world-readable either: this file
	// contains an email address and the fact that its account is being
	// recovered, which together are a list of people worth phoning.
	return writeAtomic(filepath.Join(dir, name(n)), body)
}

// confPath is where this program looks when the environment is empty, which is
// the normal case: see the note at the top about the custodian not passing its
// environment on.
const confPath = "/etc/yamale/notify.conf"

// spoolDir is the directory notices are written to.
func spoolDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("YAMALE_NOTIFY_DIR")); dir != "" {
		return dir, nil
	}
	path := confPath
	if override := strings.TrimSpace(os.Getenv("YAMALE_NOTIFY_CONF")); override != "" {
		path = override
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		// Named in full, because the obvious diagnosis is wrong. Somebody who
		// exported YAMALE_NOTIFY_DIR in the service's unit file and is watching
		// every notice fail needs to be told that the child never sees it.
		return "", fmt.Errorf(
			"no spool directory: %s could not be read (%v), and YAMALE_NOTIFY_DIR is empty — "+
				"note that the custodian does NOT pass its environment to this command, so "+
				"exporting the variable in its unit file has no effect; put the directory in "+
				"the file instead", path, err)
	}

	// The first non-empty, non-comment line. Deliberately not a parser: this is
	// one value, and a format with sections is a format that will grow one.
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line, nil
	}
	return "", fmt.Errorf("%s names no directory", path)
}

// compose writes the message a person reads.
//
// Written for somebody who may not have asked for any of this, because the
// whole reason the notice exists is the case where they did not.
func compose(n notice) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "To: %s\n", n.Email)

	switch n.Event {
	case "recovery-started":
		fmt.Fprintf(&b, "Subject: Somebody has asked to recover your Yamale account\n\n")
		fmt.Fprintf(&b, "A request has been made to restore access to the account %s.\n\n", n.Address)
		fmt.Fprintf(&b, "Nothing will happen for %d hours.\n\n", n.DelayHours)
		b.WriteString("IF THIS WAS YOU, you do not need to do anything.\n\n")
		b.WriteString("IF THIS WAS NOT YOU, say so now. The delay exists for exactly this:\n")
		b.WriteString("a recovery can be cancelled by anybody, at any time, until it finishes.\n")
	case "recovery-completed":
		fmt.Fprintf(&b, "Subject: Your Yamale account has been recovered\n\n")
		fmt.Fprintf(&b, "The recovery of the account %s has completed.\n\n", n.Address)
		fmt.Fprintf(&b, "Access is handed over at %s.\n\n", n.FrozenUntil)
		b.WriteString("IF THIS WAS NOT YOU, it can still be cancelled until that time.\n")
		b.WriteString("After it, whoever asked for the recovery can rebuild the account's key.\n")
	default:
		// Refused rather than written as an empty message. An event this
		// program does not understand means the custodian and the notifier have
		// drifted, and failing loudly is how a deployment finds that out.
		return nil, fmt.Errorf("unknown event %q", n.Event)
	}
	return []byte(b.String()), nil
}

// name is the spool filename: sortable, unique enough, and it does NOT contain
// the email address. A directory listing should not be a mailing list.
func name(n notice) string {
	return fmt.Sprintf("%s-%s-%s.txt",
		time.Now().UTC().Format("20060102T150405.000000000"), n.Event, short(n.Address))
}

func short(address string) string {
	if len(address) <= 12 {
		return address
	}
	return address[len(address)-12:]
}

// writeAtomic writes through a temporary file and a rename.
//
// A half-written notice that the custodian counted as sent is a recovery
// running with nobody told, which is the one outcome the whole mechanism is
// built to prevent.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
