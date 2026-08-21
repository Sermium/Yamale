package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// console is every byte this program reads from or writes to a person.
//
// Routed through one type on purpose. "The mnemonic only ever reaches the
// screen" is a claim about the handful of call sites in this file rather than
// about the whole program, and the tests drive the transcription check by
// substituting a scripted reader here instead of by faking a terminal.
type console struct {
	in  *bufio.Reader
	out io.Writer

	// tty is the file descriptor when out really is a terminal, and -1
	// otherwise. It gates two things: password entry with echo off, and the
	// refusal in requireTerminal. A mnemonic displayed into a pipe is a
	// mnemonic in whatever that pipe leads to, which is the exact failure this
	// tool exists to prevent, so it is refused rather than warned about.
	tty int
}

func stdConsole() *console {
	fd := -1
	if term.IsTerminal(int(os.Stdout.Fd())) {
		fd = int(os.Stdout.Fd())
	}
	return &console{in: bufio.NewReader(os.Stdin), out: os.Stdout, tty: fd}
}

// requireTerminal refuses to show a secret to anything but a screen.
//
// `ceremony custodian > custodian.txt` is a one-character mistake that puts a
// twenty-four word phrase in a file, and a tool that merely warned about it
// would be relying on somebody reading a warning that has already scrolled
// past. The redirection is also how a well-meaning operator "keeps a copy of
// the session", which is the same failure with better intentions.
func (c *console) requireTerminal() error {
	if c.tty < 0 {
		return errors.New(
			"output is not a terminal, so this would write a recovery phrase into a file or a pipe.\n" +
				"Run it attached to a screen. If you are trying to keep a copy of the session, do not:\n" +
				"the whole point of the ceremony is that the phrase exists on paper and nowhere else")
	}
	return nil
}

func (c *console) printf(format string, args ...any) {
	fmt.Fprintf(c.out, format, args...)
}

func (c *console) println(args ...any) {
	fmt.Fprintln(c.out, args...)
}

// clear wipes the screen and the scrollback.
//
// [3J is the sequence usually left out and the one that matters: [2J clears
// only what is visible, so without it the phrase stays one scroll-wheel turn
// away from whoever walks past the machine next. It is best effort — a terminal
// that ignores [3J, or a host window managing its own buffer, will keep the
// history anyway — which is why the runbook has the operator power the machine
// off rather than trusting this.
func (c *console) clear() {
	fmt.Fprint(c.out, "\033[H\033[2J\033[3J")
}

func (c *console) readLine(prompt string) (string, error) {
	fmt.Fprint(c.out, prompt)
	line, err := c.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// confirm requires the word "yes" and nothing else.
//
// Not a y/n prompt. Every item this is used for is one an operator can wave
// through by holding return down, and a single keystroke is exactly what makes
// that possible.
func (c *console) confirm(prompt string) (bool, error) {
	answer, err := c.readLine(prompt + " [type yes] ")
	if err != nil {
		return false, err
	}
	return strings.EqualFold(answer, "yes"), nil
}

// pause waits for the room, so the person writing has as long as they need.
func (c *console) pause(prompt string) error {
	fmt.Fprint(c.out, prompt)
	_, err := c.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// readPassphrase reads a keystore passphrase without echoing it.
//
// Only reachable through --armor, which a paper ceremony does not use. It is
// here because an operator who has decided to carry an encrypted export should
// not be pushed towards typing the passphrase on a command line, where the
// shell history keeps it.
// minPassphrase is the floor for a keystore passphrase.
const minPassphrase = 12

func (c *console) readPassphrase() (string, error) {
	if c.tty < 0 {
		return "", errors.New("a passphrase cannot be read without a terminal")
	}

	// The requirement is stated before anything is typed, not after.
	//
	// It used to be checked at the end, which meant a person typed a passphrase
	// twice, was told it was too short, and typed a different one twice more —
	// having learned the rule from a rejection. A rule announced after the work
	// is a rule that made somebody do the work twice.
	fmt.Fprintln(c.out, "This passphrase encrypts the keystore. At least "+strconv.Itoa(minPassphrase)+" characters:")
	fmt.Fprintln(c.out, "it is the only thing between that file and this key.")
	fmt.Fprintln(c.out)

	for {
		fmt.Fprint(c.out, "Passphrase for the keystore: ")
		first, err := term.ReadPassword(c.tty)
		fmt.Fprintln(c.out)
		if err != nil {
			return "", err
		}

		// Length checked before asking for the confirmation. Making somebody
		// retype a passphrase that was already going to be rejected is the same
		// discourtesy in a smaller shape.
		if len(first) < minPassphrase {
			fmt.Fprintf(c.out, "That is %d characters; %d are needed. Again.", len(first), minPassphrase)
			fmt.Fprintln(c.out)
			fmt.Fprintln(c.out)
			continue
		}

		fmt.Fprint(c.out, "Again: ")
		second, err := term.ReadPassword(c.tty)
		fmt.Fprintln(c.out)
		if err != nil {
			return "", err
		}

		if string(first) != string(second) {
			// Retried rather than aborted. A mistyped confirmation is a typo,
			// and failing the whole command for it would mean regenerating a key
			// that is already on paper.
			fmt.Fprintln(c.out, "Those do not match. Again.")
			fmt.Fprintln(c.out)
			continue
		}
		return string(first), nil
	}
}
