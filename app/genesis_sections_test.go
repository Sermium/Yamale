package app

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// A section belonging to a module this binary does not link must be refused,
// named, and attributed to the profile.
//
// The assertion is on the message and not merely on the error, because the
// message is the entire point of the check: the divergence it prevents is
// already detectable — as an app hash mismatch at height 1, hours later, on
// half the validator set. What was missing was anything that said why.
func TestGenesisWithAnUnknownSectionIsRefused(t *testing.T) {
	appState := map[string]json.RawMessage{
		"auth":     json.RawMessage(`{}`),
		"bank":     json.RawMessage(`{}`),
		"emission": json.RawMessage(`{"params":{}}`),
	}

	err := CheckGenesisSections(appState, []string{"auth", "bank"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnknownGenesisSections)
	require.Contains(t, err.Error(), "emission", "the operator has to be told which section")
	require.Contains(t, err.Error(), ProfileName(), "and which build refused it")
}

// Every unknown section is reported at once. Reporting only the first turns one
// wrong-binary diagnosis into as many restart-and-retry rounds as there are
// extra modules.
func TestEveryUnknownSectionIsNamed(t *testing.T) {
	appState := map[string]json.RawMessage{
		"auth":     json.RawMessage(`{}`),
		"amm":      json.RawMessage(`{}`),
		"emission": json.RawMessage(`{}`),
		"land":     json.RawMessage(`{}`),
	}

	err := CheckGenesisSections(appState, []string{"auth"})
	require.Error(t, err)
	for _, name := range []string{"amm", "emission", "land"} {
		require.Contains(t, err.Error(), name)
	}
}

// A module registered but absent from the genesis is not an error here. The SDK
// skips it, which leaves the module with no state — loud on first use, and
// identical on every node running the same file, so it does not diverge a set.
// Refusing it would also break every legitimately sparse genesis.
func TestAMissingSectionIsNotRefused(t *testing.T) {
	appState := map[string]json.RawMessage{"auth": json.RawMessage(`{}`)}

	require.NoError(t, CheckGenesisSections(appState, []string{"auth", "bank", "staking"}))
}

func TestAMatchingGenesisIsAccepted(t *testing.T) {
	appState := map[string]json.RawMessage{
		"auth": json.RawMessage(`{}`),
		"bank": json.RawMessage(`{}`),
	}

	require.NoError(t, CheckGenesisSections(appState, []string{"auth", "bank", "staking"}))
}
