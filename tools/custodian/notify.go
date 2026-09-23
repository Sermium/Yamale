package main

import (
	"bytes"
	"context"
	"encoding/json"
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
// # Where the address to notify comes from, which is not the store
//
// This service does not store an email. The Account record holds a blind index
// and no personal data at all, deliberately, so for a while the notifier was
// handed an Account and a bech32 address and asked to reach a person — which it
// could not do, because nothing in what it was given says where to write. The
// 72-hour delay was protecting nobody, and nothing said so.
//
// The email comes from the OPERATOR, at the moment a recovery is initiated, and
// it is checked rather than trusted: the service computes the blind index of
// what was supplied and looks an account up by it. A different email produces a
// different index and finds nothing.
//
// That check is what makes this safe, and it is worth being precise about why.
// An operator who substitutes an attacker's address does not redirect the
// notice — they get "no account matches that email", the same refusal as an
// email nobody holds. Notice can therefore only ever go to the address the
// account was registered under, while the service continues to store no
// addresses at all. It is held in memory for the length of one request.
//
// The same rule applies at completion, which is why that endpoint asks for the
// email too rather than working from the recovery record: the record holds a
// blind index, and a blind index is one-way on purpose.
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
	//
	// `email` has been proved to belong to this account by the caller — it is
	// the address whose blind index found the account — so an implementation
	// may write to it without checking anything further, and must not write to
	// any other address it happens to know.
	RecoveryStarted(account Account, email string, delay time.Duration) error
	// RecoveryCompleted is best-effort: the recovery has already happened and
	// failing to say so must not undo it.
	RecoveryCompleted(account Account, email string, frozenUntil time.Time) error
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

func (refusingNotifier) RecoveryStarted(Account, string, time.Duration) error {
	return ErrNoNotifier
}

func (refusingNotifier) RecoveryCompleted(Account, string, time.Time) error {
	return ErrNoNotifier
}

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

// notice is what the command is given, as JSON on stdin.
//
// On STDIN and not in argv, and that is the whole reason this is a struct.
// /proc/<pid>/cmdline is world-readable on Linux, so an email passed as an
// argument is visible to every user on the host for as long as the process
// runs — which would take the one piece of personal data this service is
// careful never to store and publish it to `ps`. /proc/<pid>/environ is
// owner-only, so an environment variable would be better, and a pipe is better
// still because it reaches nothing but the child.
type notice struct {
	Event   string `json:"event"`
	Address string `json:"address"`
	Email   string `json:"email"`
	// DelayHours is set for recovery-started, FrozenUntil for
	// recovery-completed. Both are present as fields so the command can decode
	// one shape rather than switching on the event before it can parse.
	DelayHours  int    `json:"delay_hours,omitempty"`
	FrozenUntil string `json:"frozen_until,omitempty"`
}

func (c *commandNotifier) run(n notice) error {
	body, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("encoding the notice: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// Bounded. A notification command that hangs would hold a recovery open
	// indefinitely, and the timeout was a field on this struct that nothing
	// read until now.
	cmd := exec.CommandContext(ctx, c.command, n.Event)
	cmd.Stdin = bytes.NewReader(body)
	cmd.Env = childEnv()

	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s did not finish within %s", c.command, c.timeout)
		}
		return fmt.Errorf("%s: %w: %s", c.command, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// notifyEnvPrefix is the one family of variables that crosses into the command.
const notifyEnvPrefix = "YAMALE_NOTIFY_"

// childEnv is what the notification command is given, and it is an ALLOWLIST.
//
// The environment is not inherited wholesale. `EnvironmentFile=` in the unit
// puts CUSTODIAN_SEALING_KEY and CUSTODIAN_PEPPER here — the paths, not the
// contents, but a notification program has no business knowing where the
// sealing key lives, and handing it over would make every future notifier a
// place those paths could leak from.
//
// Passing NOTHING was the first version and it was wrong in a way that took a
// working notifier and a working notify program to notice: a command with no
// environment cannot be configured by one, so the reference program could never
// have been told where to write. A deployment would have set a variable in the
// unit file, watched every notice fail, and had nothing on either side saying
// why.
//
// So: PATH, because a program has to be able to find its own tools, and
// anything named YAMALE_NOTIFY_*. The prefix is the whole mechanism — an
// operator configuring the notifier names its variables for it, and nothing
// else can be reached by accident.
func childEnv() []string {
	env := []string{"PATH=" + os.Getenv("PATH")}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, notifyEnvPrefix) {
			env = append(env, kv)
		}
	}
	return env
}

func (c *commandNotifier) RecoveryStarted(account Account, email string, delay time.Duration) error {
	return c.run(notice{
		Event:      "recovery-started",
		Address:    account.Address,
		Email:      email,
		DelayHours: int(delay.Hours()),
	})
}

func (c *commandNotifier) RecoveryCompleted(account Account, email string, frozenUntil time.Time) error {
	return c.run(notice{
		Event:       "recovery-completed",
		Address:     account.Address,
		Email:       email,
		FrozenUntil: frozenUntil.UTC().Format(time.RFC3339),
	})
}

func (c *commandNotifier) Problem(what string, err error) {
	log.Printf("%s: %v", what, err)
}
