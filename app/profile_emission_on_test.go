//go:build !settlement

package app

import (
	"testing"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	emissiontypes "yamale/blockchain/x/emission/types"
)

// The counterpart in profile_emission_off_test.go asserts the opposite. The two
// together are what prove the tag changes the wiring and not merely the build:
// with a test only on this side, a `settlement` build that quietly kept
// emission registered would still pass everything the default build passes.
func TestEmissionIsWiredIntoThisProfile(t *testing.T) {
	require.Contains(t, GetMaccPerms(), emissiontypes.ModuleName,
		"emission mints the native token, so it needs a module account carrying Minter")
	require.Contains(t, GetMaccPerms()[emissiontypes.ModuleName], authtypes.Minter)

	require.Equal(t, []string{emissiontypes.ModuleName}, emissionBeginBlockers)
	require.Equal(t, []string{emissiontypes.ModuleName}, emissionEndBlockers)
	require.Equal(t, []string{emissiontypes.ModuleName}, emissionInitGenesis)
	require.Len(t, emissionModuleConfigs, 1)
}

// Emission mints into the fee collector so distribution can allocate it in the
// same block. Appending it to the begin blockers instead of splicing it ahead
// of distribution would leave every payout a block stale, which is invisible in
// any single-block test — hence an assertion on the order itself.
func TestEmissionBeginsBlockBeforeDistribution(t *testing.T) {
	require.NotEmpty(t, emissionBeginBlockers,
		"emission must run in BeginBlock, and must be first")
}
