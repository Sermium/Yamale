package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Notice: the thing that makes the 72-hour delay worth having.
//
// # Why an interface rather than an SMTP client
//
// The delay protects an account only if its owner is told the clock started. A
// recovery nobody was notified of is not a slow recovery, it is a silent one —
// and three days of silence is exactly what an attacker wants. So notice is not
// a nicety attached to recovery; it is the half that does the protecting, and
// Initiate ABORTS if it fails.
//
// That is also why this is an interface with a deliberately unhelpful default.
// A deployment must choose how it reaches people, and the shapes differ enough
// — SMTP, a provider's API, an SMS gateway, push to enrolled devices — that
// building one in would mean building the wrong one. What is built in is the
// refusal to run without one.
//
// # What is NOT here, and is a real gap
//
// Notice to every enrolled device. The design says email *and* every enrolled
// device, and device enrolment does not exist yet, so today this reaches one
// address. Email is also where password resets land, which makes it one factor
// wearing two hats — the design says so about second factors and it is just as
// true here.

// Notifier tells an account holder that something is happening to their
// account.
type Notifier interface {
	// RecoveryStarted must reach the account holder, and its error aborts the
	// recovery. Everything about this method's contract follows from that.
	RecoveryStarted(account Account, delay time.Duration) error
	// RecoveryCompleted is best-effort: the recovery has already happened and
	// failing to say so must not undo it.
	RecoveryCompleted(account Account, frozenUntil time.Time) error
	// Problem records something an operator should see.
	Problem(what string, err error)
}

// ErrNoNotifier is what a service without a configured notifier answers with.
//
// It reaches the operator initiating the recovery, so it is written for them.
var ErrNoNotifier = errors.New(
	"this service has no way to notify account holders, so it will not start a recovery; " +
		"configure --notify-command")

// refusingNotifier is the default, and it refuses.
//
// The alternative default — log a line and carry on — would produce a service
// that runs the recovery process correctly and silently, which is the single
// most dangerous configuration this code can be in: every rule enforced, and
// the one person who could object never told.
type refusingNotifier struct{}

func (refusingNotifier) RecoveryStarted(Account, time.Duration) error { return ErrNoNotifier }

func (refusingNotifier) RecoveryCompleted(Account, time.Time) error { return ErrNoNotifier }

func (refusingNotifier) Problem(what string, err error) { log.Printf("%s: %v", what, err) }

// commandNotifier hands the message to an external program.
//
// A command rather than SMTP because every deployment already has a way to send
// mail and none of them agree on it, and because a subprocess is auditable by
// somebody who does not read Go: the operator can run it themselves and see
// exactly what their customers receive.
//
// The account's EMAIL IS NOT AVAILABLE HERE. This service stores a blind index
// and never the address, which is the property that makes a stolen store much
// less useful — so the command is given the account's chain ADDRESS and must
// resolve its own way to a person. That is a real constraint on deployments and
// is stated in the guide rather than worked around here.
type commandNotifier struct {
	command string
	timeout time.Duration
}

func newCommandNotifier(command string) *commandNotifier {
	return &commandNotifier{command: command, timeout: 20 * time.Second}
}

func (c *commandNotifier) run(event string, account Account, extra ...string) error {
	args := append([]string{event, account.Address}, extra...)
	cmd := exec.Command(c.command, args...)
	// The environment is not inherited. A notification command should not need
	// this process's environment, and this process's environment is where the
	// paths to the sealing key and the pepper are.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", c.command, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *commandNotifier) RecoveryStarted(account Account, delay time.Duration) error {
	return c.run("recovery-started", account, fmt.Sprintf("%.0f", delay.Hours()))
}

func (c *commandNotifier) RecoveryCompleted(account Account, frozenUntil time.Time) error {
	return c.run("recovery-completed", account, frozenUntil.UTC().Format(time.RFC3339))
}

func (c *commandNotifier) Problem(what string, err error) {
	log.Printf("%s: %v", what, err)
}
