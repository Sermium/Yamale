package main

// Launching the browser rather than printing a URL.
//
// The terminal version of this ceremony could make guarantees a page in somebody
// else's browser cannot. An extension can read the DOM. Session restore writes
// the state of open tabs to disk so a crash can rebuild them. Form autofill and
// a password manager both watch input fields. Sync uploads history. None of that
// is reachable from JavaScript, and none of it can be detected reliably either:
// the storage-quota and FileSystem heuristics people use to sniff private
// browsing are undocumented side effects that change with browser releases, and a
// ceremony whose safety rested on one would be a ceremony that silently stopped
// being safe at the next update.
//
// So the tool takes the profile out of the equation instead. It creates a
// temporary directory, launches the browser against that directory in private
// mode, and deletes the directory afterwards. A profile that has existed for four
// seconds has no extensions, no autofill entries, no saved passwords, no history,
// no sync account and no session to restore — not because it was asked not to,
// but because there was never anything there.
//
// If it cannot do that, it refuses. Printing a URL and hoping is the version of
// this that produces a ceremony where nobody knows whether the extensions were
// disabled, and the point of the whole exercise is to stop producing artefacts
// nobody can check afterwards.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// browserKind decides the flags, because the two families spell every one of
// them differently and get one of them dangerously wrong.
type browserKind int

const (
	chromium browserKind = iota
	firefox
)

// browserCandidate is one browser this tool knows how to drive with a profile it
// controls.
type browserCandidate struct {
	name string
	kind browserKind
	// paths are absolute locations tried in order; command is looked up on PATH.
	paths   []string
	command string
}

// candidates are ordered by how completely the tool can control the profile.
//
// Chromium-family first, not out of preference for the browser but because
// --user-data-dir is a documented, stable, load-bearing flag that the whole
// automated-testing world depends on, so it will not quietly change behaviour.
// Firefox is second because its equivalent needs -no-remote to be safe and the
// failure mode without it is silent — see launchFirefox.
func candidates() []browserCandidate {
	switch runtime.GOOS {
	case "windows":
		programFiles := os.Getenv("ProgramFiles")
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		localAppData := os.Getenv("LOCALAPPDATA")
		return []browserCandidate{
			{name: "Google Chrome", kind: chromium, command: "chrome", paths: []string{
				filepath.Join(programFiles, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(programFilesX86, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(localAppData, `Google\Chrome\Application\chrome.exe`),
			}},
			{name: "Microsoft Edge", kind: chromium, command: "msedge", paths: []string{
				filepath.Join(programFiles, `Microsoft\Edge\Application\msedge.exe`),
				filepath.Join(programFilesX86, `Microsoft\Edge\Application\msedge.exe`),
			}},
			{name: "Brave", kind: chromium, command: "brave", paths: []string{
				filepath.Join(programFiles, `BraveSoftware\Brave-Browser\Application\brave.exe`),
				filepath.Join(programFilesX86, `BraveSoftware\Brave-Browser\Application\brave.exe`),
			}},
			{name: "Chromium", kind: chromium, command: "chromium"},
			{name: "Firefox", kind: firefox, command: "firefox", paths: []string{
				filepath.Join(programFiles, `Mozilla Firefox\firefox.exe`),
				filepath.Join(programFilesX86, `Mozilla Firefox\firefox.exe`),
			}},
		}
	case "darwin":
		return []browserCandidate{
			{name: "Google Chrome", kind: chromium, paths: []string{
				"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			}},
			{name: "Chromium", kind: chromium, command: "chromium", paths: []string{
				"/Applications/Chromium.app/Contents/MacOS/Chromium",
			}},
			{name: "Brave", kind: chromium, paths: []string{
				"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			}},
			{name: "Microsoft Edge", kind: chromium, paths: []string{
				"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			}},
			{name: "Firefox", kind: firefox, paths: []string{
				"/Applications/Firefox.app/Contents/MacOS/firefox",
			}},
		}
	default:
		return []browserCandidate{
			{name: "Google Chrome", kind: chromium, command: "google-chrome"},
			{name: "Chromium", kind: chromium, command: "chromium"},
			{name: "Chromium", kind: chromium, command: "chromium-browser"},
			{name: "Brave", kind: chromium, command: "brave-browser"},
			{name: "Microsoft Edge", kind: chromium, command: "microsoft-edge"},
			{name: "Firefox", kind: firefox, command: "firefox"},
		}
	}
}

// resolve finds the executable for a candidate, or reports that it is absent.
func (c browserCandidate) resolve() (string, bool) {
	for _, path := range c.paths {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	if c.command != "" {
		if path, err := exec.LookPath(c.command); err == nil {
			return path, true
		}
	}
	return "", false
}

// controlledBrowser is a running browser and the profile directory it was given.
type controlledBrowser struct {
	Name    string
	Path    string
	Profile string

	cmd     *exec.Cmd
	started time.Time
}

// launchBrowser starts a browser on a profile this program created.
//
// preferred names an executable the operator chose explicitly. It is honoured
// exactly — no falling back to something else — because an operator who named a
// browser has a reason, and quietly launching a different one would break the
// only assumption they made.
func launchBrowser(url, preferred, profileParent string) (*controlledBrowser, error) {
	list := candidates()
	if preferred != "" {
		kind := chromium
		base := strings.ToLower(filepath.Base(preferred))
		if strings.Contains(base, "firefox") {
			kind = firefox
		}
		list = []browserCandidate{{name: filepath.Base(preferred), kind: kind, paths: []string{preferred}}}
	}

	var tried []string
	for _, candidate := range list {
		path, ok := candidate.resolve()
		if !ok {
			continue
		}

		// One directory per launch, created by this program with 0700. Not
		// reused between runs: a profile reused from a previous ceremony is a
		// profile that has a previous ceremony's history in it.
		profile, err := os.MkdirTemp(profileParent, "ceremony-profile-")
		if err != nil {
			return nil, fmt.Errorf("could not create a temporary browser profile: %w", err)
		}
		if err := os.Chmod(profile, 0o700); err != nil {
			_ = os.RemoveAll(profile)
			return nil, err
		}

		var cmd *exec.Cmd
		switch candidate.kind {
		case firefox:
			cmd = firefoxCommand(path, profile, url)
		default:
			cmd = chromiumCommand(path, profile, url)
		}

		if err := cmd.Start(); err != nil {
			_ = os.RemoveAll(profile)
			tried = append(tried, fmt.Sprintf("%s (%s): %v", candidate.name, path, err))
			continue
		}
		return &controlledBrowser{
			Name:    candidate.name,
			Path:    path,
			Profile: profile,
			cmd:     cmd,
			started: time.Now(),
		}, nil
	}

	detail := "none of the browsers this tool knows how to control are installed"
	if len(tried) > 0 {
		detail = "every browser found refused to start: " + strings.Join(tried, "; ")
	}
	return nil, fmt.Errorf(
		"%s.\n"+
			"Refusing to print a URL instead. A page opened in your everyday browser runs alongside\n"+
			"every extension you have installed, with session restore writing your open tabs to disk\n"+
			"and a password manager watching the input fields — and none of that can be checked from\n"+
			"inside the page, which is why this tool controls the profile rather than asking you to.\n"+
			"Install Chrome, Chromium, Edge, Brave or Firefox, or point --browser at one.\n"+
			"If you have read docs/guides/key-ceremony.md and are preparing the profile yourself,\n"+
			"--no-browser prints the URL and puts the preparation on you", detail)
}

// chromiumCommand is the flag set, and each flag is here for a reason.
func chromiumCommand(path, profile, url string) *exec.Cmd {
	return exec.Command(path,
		// The profile. Everything else on this list is belt to its braces.
		"--user-data-dir="+profile,
		// Incognito on top of a fresh profile, because incognito also stops the
		// session being written to disk for restore and stops history being
		// recorded at all, rather than merely into a directory we delete later.
		// A directory we delete later is a directory that exists in the interim.
		"--incognito",
		// Policy-installed extensions are forced into every profile on a managed
		// machine, including a brand new one. A fresh profile alone does not
		// exclude them; this does.
		"--disable-extensions",
		"--disable-component-extensions-with-background-pages",
		// A first-run flow that offers to sign in and sync is a flow that can
		// attach an account to the profile holding the ceremony.
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--disable-features=Translate,MediaRouter,OptimizationHints",
		// The page needs no network beyond loopback, and a browser that is not
		// speaking to Google is a browser whose traffic an observer can account
		// for entirely.
		"--disable-background-networking",
		"--disable-breakpad",
		"--no-service-autorun",
		"--metrics-recording-only",
		// A window rather than a tab in something already open: there is nothing
		// already open in a profile that did not exist a moment ago, and the
		// explicit flag means a future browser change cannot reinterpret it.
		"--new-window",
		url,
	)
}

// firefoxCommand is the same idea, and -no-remote is the flag that makes it
// true.
//
// Without -no-remote, firefox with a --profile argument does not start a new
// browser at all: it finds the instance the operator already has open, hands the
// URL to it, and exits. The profile flag is silently ignored, the ceremony opens
// in the operator's everyday session with every extension running, and the tool
// would report a clean profile because its own exec succeeded. This is the single
// most dangerous line in this file.
func firefoxCommand(path, profile, url string) *exec.Cmd {
	return exec.Command(path,
		"-no-remote",
		"-new-instance",
		"-profile", profile,
		"-private-window", url,
	)
}

// aliveBriefly reports whether the browser is still running shortly after
// launch.
//
// Some browsers exec a launcher that hands off and exits. With a fresh
// --user-data-dir that does not happen, but the failure mode if it ever did is
// the tool concluding "the operator closed the browser" one second in and
// shutting the ceremony down, so the distinction is made explicitly rather than
// assumed.
func (b *controlledBrowser) exitedImmediately() bool {
	return time.Since(b.started) < 3*time.Second
}

// wait blocks until the browser exits.
func (b *controlledBrowser) wait() error {
	if b.cmd == nil {
		return errors.New("no browser was launched")
	}
	return b.cmd.Wait()
}

// close stops the browser and removes the profile.
//
// The removal is retried. On Windows the browser's own files stay locked for a
// short time after the process is signalled, and a single RemoveAll that failed
// would leave a profile directory behind on the machine the ceremony was
// supposed to leave nothing on. The final state is reported rather than
// swallowed: an operator who was promised a wiped profile needs to be told when
// it did not happen, so they can delete it themselves before the machine is.
func (b *controlledBrowser) close() error {
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		_, _ = b.cmd.Process.Wait()
	}
	if b.Profile == "" {
		return nil
	}

	var err error
	for attempt := 0; attempt < 20; attempt++ {
		if err = os.RemoveAll(b.Profile); err == nil {
			if _, statErr := os.Stat(b.Profile); os.IsNotExist(statErr) {
				return nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf(
		"the temporary browser profile at %s could not be removed (%v). Delete it by hand before this machine leaves the room",
		b.Profile, err)
}

// manualPreparation is what an operator using --no-browser has to do for
// themselves.
//
// Printed in full rather than referred to, because the operator who reaches for
// this flag is by definition the one who is not going to open the guide.
var manualPreparation = []string{
	"Use a browser profile created for this ceremony and nothing else, and delete it afterwards.",
	"Open a private / incognito window. Not a normal one.",
	"Disable every extension. On a managed machine, check for policy-installed extensions too — a fresh profile does not exclude those.",
	"Turn session restore off, so a crash cannot rebuild the page from a file on disk.",
	"Sign the browser out of any sync account, and turn off form autofill and the password manager.",
	"Close every other tab in that window. Nothing else should be running in this browser.",
}
