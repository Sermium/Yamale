package main

import (
	"strings"
	"testing"
	"time"
)

func testTime() time.Time { return time.Unix(1700000000, 0).UTC() }

// allConfirmed is the room answering yes to every checklist item.
func allConfirmed() []string {
	answers := make([]string, len(checklist))
	for i := range answers {
		answers[i] = "yes"
	}
	return answers
}

func TestPreflightPassesWhenNothingIsDetectedAndEverythingIsConfirmed(t *testing.T) {
	c, out := scripted(allConfirmed()...)

	answers, err := preflightWith(c, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(answers) != len(checklist) {
		t.Fatalf("%d answers recorded for %d items", len(answers), len(checklist))
	}
	for _, answer := range answers {
		if !answer.Confirmed {
			t.Fatalf("item %q was recorded as unconfirmed", answer.ID)
		}
	}

	rendered := out.String()
	// The unknowns must be printed every time, next to the result. A tool that
	// only said "no network detected" would be implying a guarantee it cannot
	// make, and the room would stop looking.
	for _, unknown := range unknowns {
		if !strings.Contains(rendered, unknown) {
			t.Fatalf("the pre-flight did not say it could not check: %s", unknown)
		}
	}
	if !strings.Contains(rendered, "consistent with an air-gapped machine") {
		t.Fatal("a clean scan did not report what it found")
	}
	if strings.Contains(rendered, "is air-gapped") {
		t.Fatal("the pre-flight claimed the machine is air-gapped, which it cannot know")
	}
}

func TestPreflightRefusesADetectedNetwork(t *testing.T) {
	c, _ := scripted(allConfirmed()...)

	findings := []networkFinding{{what: "the kernel has a default route", detail: "10.0.0.5:0"}}
	_, err := preflightWith(c, "", findings)
	if err == nil {
		t.Fatal("a key ceremony was allowed to start on a machine with a default route")
	}
	if !strings.Contains(err.Error(), "network-acknowledged") {
		t.Fatalf("the refusal does not say what the operator has to do: %v", err)
	}
}

func TestPreflightRecordsAWrittenAcknowledgement(t *testing.T) {
	c, out := scripted(allConfirmed()...)

	findings := []networkFinding{{what: "interface eth0 is up", detail: "10.0.0.5"}}
	if _, err := preflightWith(c, "rehearsal only, no key will be used", findings); err != nil {
		t.Fatal(err)
	}
	// The reason has to appear where somebody reads it back later. There is no
	// --force precisely so this cannot be waved through silently.
	if !strings.Contains(out.String(), "rehearsal only, no key will be used") {
		t.Fatal("the acknowledgement was accepted without being printed")
	}
	if !strings.Contains(out.String(), "appears to be CONNECTED") {
		t.Fatal("a detected network was not reported")
	}
}

func TestPreflightStopsAtTheFirstUnconfirmedItem(t *testing.T) {
	for index, item := range checklist {
		answers := allConfirmed()
		answers[index] = "no"

		c, _ := scripted(answers...)
		_, err := preflightWith(c, "", nil)
		if err == nil {
			t.Fatalf("the ceremony continued with %q unconfirmed", item.id)
		}
		// Naming the item matters: an operator told only "pre-flight failed"
		// re-runs and answers yes to everything.
		if !strings.Contains(err.Error(), item.id) {
			t.Fatalf("the refusal for %q does not name it: %v", item.id, err)
		}
	}
}

func TestPreflightRefusesAnythingThatIsNotTheWordYes(t *testing.T) {
	// A y/n prompt is answered by holding return down, which is exactly what
	// makes a checklist theatre.
	for _, answer := range []string{"y", "", "sure", "ok", "1"} {
		answers := allConfirmed()
		answers[0] = answer

		c, _ := scripted(answers...)
		if _, err := preflightWith(c, "", nil); err == nil {
			t.Fatalf("%q was accepted as confirmation", answer)
		}
	}
}

func TestChecklistCoversWhatTheScanCannotSee(t *testing.T) {
	// The checklist and the scan are complements, and the items below are the
	// ones the scan is structurally blind to. Losing any of them silently is
	// how a ceremony ends up on a laptop with a swap file.
	required := []string{"swap", "radios", "virtual", "recording", "binary"}
	present := map[string]bool{}
	for _, item := range checklist {
		present[item.id] = true
	}
	for _, id := range required {
		if !present[id] {
			t.Fatalf("the pre-flight no longer asks about %q, which nothing in scanNetwork can detect", id)
		}
	}
}

func TestScanNetworkSendsNothingAndReturnsFindings(t *testing.T) {
	// Not an assertion about this machine — a build host may be connected or
	// not. What is checked is that the scan completes, and that every finding
	// it reports says what it found rather than returning a bare boolean the
	// operator cannot argue with.
	for _, finding := range scanNetwork() {
		if finding.what == "" || finding.detail == "" {
			t.Fatalf("a finding was reported with nothing to act on: %+v", finding)
		}
	}
}

func TestRequireTerminalRefusesAPipe(t *testing.T) {
	c, _ := scripted()
	if err := c.requireTerminal(); err == nil {
		t.Fatal("a phrase would have been displayed into a pipe")
	}
}
