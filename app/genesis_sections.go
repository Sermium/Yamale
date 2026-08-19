package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrUnknownGenesisSections is returned when a genesis document carries a
// top-level app_state section this binary has no module for.
//
// The failure this prevents is the worst shape a consensus bug takes, because
// the symptom appears nowhere near the cause. A settlement binary handed a
// genesis that still contains an `emission` section starts, validates clean and
// produces blocks: module.Manager.InitGenesis iterates its own module list and
// never looks at the keys it was given, so a section no module claims is read
// by nobody and reported by nobody. A validator set running a mix of profiles
// then diverges on the app hash at height 1 — one half initialised state from
// that section, the other half did not — and neither log names a genesis
// section as the reason.
//
// The Cosmos SDK offers no hook for this. Neither module.Manager.InitGenesis
// nor module.BasicManager.ValidateGenesis iterates the genesis keys; both walk
// the registered modules and index into the map, so an unrecognised key is
// invisible to every code path the SDK has, `genesis validate` included. Hence
// the check here, at InitChain, which is the one moment every node on the
// network reads the file.
var ErrUnknownGenesisSections = errors.New("genesis contains sections this binary has no module for")

// CheckGenesisSections compares the genesis document's top-level sections
// against the modules this binary actually links.
//
// `known` is deliberately the full registered module set rather than only the
// modules that carry genesis state. It is a superset, so the check can never
// refuse a genesis it should have accepted; the direction being defended is a
// section with no module, not a module with no section.
//
// It is exported so that `genesis validate` can refuse exactly the file the
// node would refuse. An operator told a genesis is valid and then unable to
// start the node with it has been told two different things by one binary.
func CheckGenesisSections(appState map[string]json.RawMessage, known []string) error {
	var unknown []string
	for section := range appState {
		if !slices.Contains(known, section) {
			unknown = append(unknown, section)
		}
	}
	if len(unknown) == 0 {
		return nil
	}

	// Sorted because the map iteration order is random and an error message
	// that reorders itself between runs reads like two different faults.
	slices.Sort(unknown)

	return fmt.Errorf(
		"%w: %s. This binary is the %q build profile, which does not link %s. "+
			"Start the binary built for the profile this genesis was written for, or remove "+
			"the section(s) from app_state — a node that ignores them produces a different "+
			"app hash from one that does not",
		ErrUnknownGenesisSections,
		strings.Join(unknown, ", "),
		ProfileName(),
		pluralModules(unknown),
	)
}

func pluralModules(names []string) string {
	if len(names) == 1 {
		return "the " + names[0] + " module"
	}
	return "the " + strings.Join(names, "/") + " modules"
}
