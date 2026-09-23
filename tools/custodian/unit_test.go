package main

import (
	"flag"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The systemd unit is checked against the flags this binary actually defines.
//
// This exists because the unit spent the whole of the threshold-to-sealed
// rewrite passing --preparam-pool and --second-factor-above, neither of which
// survived it. Go's flag package refuses an unknown flag and exits, so the
// service would not have started at all — and nothing in the repository knew:
// the unit is data to every test, the binary compiles perfectly, and the two
// only meet on a host nobody had deployed to yet.
//
// The failure would have arrived as "the service will not start" on the day
// somebody finally installed it, a long way from the commit that caused it.

var unitFlag = regexp.MustCompile(`--([a-z][a-z0-9-]*)`)

func TestTheSystemdUnitOnlyPassesFlagsThisBinaryHas(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/systemd/yamale-custodian.service")
	if err != nil {
		t.Fatalf("reading the unit: %v", err)
	}

	// Only the ExecStart line matters; comments in the unit discuss flags that
	// were deliberately removed and must not be read as configuration.
	exec := execStart(string(raw))
	if exec == "" {
		t.Fatal("no ExecStart found in the unit")
	}

	defined := definedFlags(t)
	for _, m := range unitFlag.FindAllStringSubmatch(exec, -1) {
		name := m[1]
		if _, ok := defined[name]; !ok {
			t.Errorf(
				"the unit passes --%s and this binary does not define it; "+
					"the flag package exits on an unknown flag, so the service would not start",
				name)
		}
	}
}

// execStart returns the ExecStart directive, following its line continuations.
func execStart(unit string) string {
	var b strings.Builder
	inExec := false
	for _, line := range strings.Split(unit, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "ExecStart=") {
			inExec = true
		}
		if !inExec {
			continue
		}
		b.WriteString(strings.TrimSuffix(trimmed, "\\"))
		b.WriteString(" ")
		if !strings.HasSuffix(trimmed, "\\") {
			break
		}
	}
	return b.String()
}

// definedFlags asks the flag package rather than parsing main.go.
//
// main() defines its flags on the default CommandLine set, and calling it here
// would start a server. So this re-declares nothing and instead reads what a
// real run would have: the set main() populates, which is empty until main runs
// — so the flags are read from the source after all, but by the SAME regex the
// unit is read with, which keeps the two sides symmetric.
func definedFlags(t *testing.T) map[string]struct{} {
	t.Helper()
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	defined := map[string]struct{}{}
	for _, m := range regexp.MustCompile(`flag\.\w+\("([a-z][a-z0-9-]*)"`).
		FindAllStringSubmatch(string(raw), -1) {
		defined[m[1]] = struct{}{}
	}
	if len(defined) == 0 {
		t.Fatal("no flags found in main.go, so this test would pass against anything")
	}
	// Sanity: the flag package is imported here only to make the dependency on
	// it explicit for a reader wondering why this is not reflective.
	_ = flag.CommandLine
	return defined
}

func TestTheEnvExampleSetsEveryVariableTheUnitReads(t *testing.T) {
	// The other half of the same failure. A unit referencing ${FOO} that the
	// environment file never sets expands to an EMPTY STRING rather than
	// failing, so the service starts with a flag whose value is missing — which
	// for --role means it refuses, and for --listen means it binds nothing
	// anybody expects.
	unit, err := os.ReadFile("../../deploy/systemd/yamale-custodian.service")
	if err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile("../../deploy/systemd/custodian.env.example")
	if err != nil {
		t.Fatal(err)
	}

	set := map[string]struct{}{}
	for _, line := range strings.Split(string(env), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if name, _, ok := strings.Cut(line, "="); ok {
			set[strings.TrimSpace(name)] = struct{}{}
		}
	}

	for _, m := range regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`).
		FindAllStringSubmatch(execStart(string(unit)), -1) {
		if _, ok := set[m[1]]; !ok {
			t.Errorf("the unit reads ${%s} and the example env file never sets it; "+
				"systemd expands an unset variable to nothing rather than failing", m[1])
		}
	}
}
