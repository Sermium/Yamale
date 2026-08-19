//go:build settlement

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The settlement profile has no native token and therefore no issuance. This
// asserts the wiring is empty rather than merely that the build succeeded: a
// compiled-out module that was still named in the begin blockers or the genesis
// order would halt the chain at startup on a missing module, and a compiled-out
// module that was still granted a module account would leave a Minter
// permission on an address nothing owns.
//
// It cannot assert that x/emission is absent from the binary — no test can, as
// importing the package to name it would put it back. `go list -deps` and the
// CI job that runs it are what prove absence.
func TestEmissionIsAbsentFromThisProfile(t *testing.T) {
	require.NotContains(t, GetMaccPerms(), "emission",
		"a compiled-out module must not keep a module account, least of all a minting one")

	require.Empty(t, emissionModuleAccPerms)
	require.Empty(t, emissionBlockedAccounts)
	require.Empty(t, emissionBeginBlockers)
	require.Empty(t, emissionEndBlockers)
	require.Empty(t, emissionInitGenesis)
	require.Empty(t, emissionModuleConfigs)
}
