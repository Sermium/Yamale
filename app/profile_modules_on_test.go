//go:build !settlement

package app

import (
	"slices"
	"testing"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/stretchr/testify/require"
)

// The counterpart in profile_modules_off_test.go asserts the opposite. The two
// together are what prove the tag changes the wiring and not merely the build:
// with a test only on this side, a `settlement` build that quietly kept every
// module registered would pass everything the default build passes.
func TestEveryModuleIsWiredIntoThisProfile(t *testing.T) {
	macc := GetMaccPerms()

	for _, name := range []string{"amm", "builderfee", "custody", "tokenisation", "emission"} {
		require.Contains(t, macc, name)
		require.Contains(t, runtimeConfig.InitGenesis, name)
	}

	// x/land holds no coins, so it has no module account — only genesis state.
	require.Contains(t, runtimeConfig.InitGenesis, "land")
	require.NotContains(t, macc, "land")

	require.Equal(t, []string{"amm"}, ammBeginBlockers)
	require.Equal(t, []string{"builderfee"}, builderfeeBeginBlockers)
}

// Ordering is the part of this wiring that breaks without failing. A module
// appended instead of spliced still starts and still runs; it is simply a block
// late, or reading a balance something else has already taken.
func TestTheOrderingsThatMatterAreHeld(t *testing.T) {
	begin := runtimeConfig.BeginBlockers

	// Emission mints into the fee collector, so it has to be there before
	// distribution allocates that balance this block.
	require.Less(t, slices.Index(begin, "emission"), slices.Index(begin, distrtypes.ModuleName))

	// Slashing after distribution, so nothing is left in a validator's fee pool
	// when it is slashed. This is the SDK's own constraint and it survives the
	// splicing.
	require.Less(t, slices.Index(begin, distrtypes.ModuleName), slices.Index(begin, "slashing"))

	// genutil after staking and auth, or the gentxs it delivers bond against
	// pools and accounts that do not exist yet.
	init := runtimeConfig.InitGenesis
	require.Less(t, slices.Index(init, "staking"), slices.Index(init, "genutil"))
	require.Less(t, slices.Index(init, authtypes.ModuleName), slices.Index(init, "genutil"))
}
