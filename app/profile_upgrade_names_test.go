package app

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// The two profiles must know the same upgrade names.
//
// This is the failure the two profile files already warn about in prose, and
// warning about it in prose is what let it happen: x/market arrived registered
// in the default profile and absent from the settlement one.
//
// An upgrade name the binary does not know is NOT a no-op. x/upgrade panics the
// node at the halt height, and it stays down until somebody builds and installs
// a binary that knows the name — at a moment when the chain is stopped, the
// operator is under time pressure, and the missing piece is an empty handler
// somebody judged not worth writing.
//
// The two lists are behind opposing build tags, so no test that imports them
// can see both: whichever profile the test binary is built for, the other list
// does not exist. This reads the source instead. That is the only vantage point
// from which the question can be asked at all, which is why the check is here
// and not beside the handlers it guards.
//
// The handlers are deliberately allowed to differ — a settlement build that
// carries neither x/tokenisation nor x/market has nothing to migrate and says
// so in its log. It is the NAME that must match, because the name is what an
// operator puts in a proposal and what x/upgrade looks up at the halt height.

var upgradeName = regexp.MustCompile(`(?m)^\s*Name:\s*"([^"]+)"`)

func namesIn(t *testing.T, path string) []string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var names []string
	for _, m := range upgradeName.FindAllStringSubmatch(string(src), -1) {
		names = append(names, m[1])
	}
	sort.Strings(names)
	return names
}

func TestBothProfilesKnowTheSameUpgradeNames(t *testing.T) {
	on := namesIn(t, "profile_upgrades_on.go")
	off := namesIn(t, "profile_upgrades_off.go")

	if len(on) == 0 || len(off) == 0 {
		t.Fatalf("read %d name(s) from the default profile and %d from settlement; "+
			"a check that finds nothing passes for the wrong reason", len(on), len(off))
	}

	missing := difference(on, off)
	extra := difference(off, on)

	for _, name := range missing {
		t.Errorf("upgrade %q exists in the default profile and not in the settlement one. "+
			"A settlement chain that adopted this proposal would panic at the halt height and "+
			"stay down. Add an entry under the same name whose handler runs the migrations and "+
			"logs what is absent.", name)
	}
	for _, name := range extra {
		t.Errorf("upgrade %q exists in the settlement profile and not in the default one. "+
			"Same failure, other direction.", name)
	}
}

func TestNeitherProfileRegistersAnUpgradeTwice(t *testing.T) {
	// Two entries under one name: x/upgrade takes one of them, and which one is
	// not something the proposal or the runbook can say.
	for _, path := range []string{"profile_upgrades_on.go", "profile_upgrades_off.go"} {
		seen := map[string]int{}
		for _, name := range namesIn(t, path) {
			seen[name]++
		}
		for name, n := range seen {
			if n > 1 {
				t.Errorf("%s registers %q %d times", path, name, n)
			}
		}
	}
}

func TestTheCheckWouldCatchAMissingName(t *testing.T) {
	// A set comparison that never fires looks exactly like two profiles in
	// agreement, so the comparison is exercised on its own.
	on := []string{"a", "b", "market"}
	off := []string{"a", "b"}
	if got := difference(on, off); len(got) != 1 || got[0] != "market" {
		t.Fatalf("difference did not find the missing name: %v", got)
	}
	if got := difference(off, on); len(got) != 0 {
		t.Fatalf("difference invented a name: %v", got)
	}
}

// difference is everything in a that is not in b.
func difference(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, s := range b {
		in[s] = true
	}
	var out []string
	for _, s := range a {
		if !in[s] {
			out = append(out, s)
		}
	}
	return out
}
