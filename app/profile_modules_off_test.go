//go:build settlement

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Scope §3 gives the settlement profile x/paymsg, x/stablecoin, x/treasury,
// x/oracle and governance. Everything else that ships in the default build is
// compiled out of it.
//
// These assert the wiring is empty rather than merely that the build succeeded.
// A compiled-out module still named in a begin blocker or in the genesis order
// halts the chain at startup on a module that does not exist, and one still
// granted a module account leaves a Minter permission on an address nothing
// owns — both of which compile perfectly.
//
// No test can assert a module is absent from the *binary*: naming the package
// to check it would put the package back. `go list -deps` and the CI job that
// runs it are what prove absence.
func TestExcludedModulesAreUnwiredInThisProfile(t *testing.T) {
	macc := GetMaccPerms()

	for _, name := range []string{"amm", "builderfee", "custody", "tokenisation", "land", "emission"} {
		require.NotContains(t, macc, name,
			"a compiled-out module must not keep a module account")
	}

	require.Empty(t, ammModuleAccPerms)
	require.Empty(t, ammBlockedAccounts)
	require.Empty(t, ammBeginBlockers)
	require.Empty(t, ammEndBlockers)
	require.Empty(t, ammInitGenesis)
	require.Empty(t, ammModuleConfigs)

	require.Empty(t, builderfeeModuleAccPerms)
	require.Empty(t, builderfeeBlockedAccounts)
	require.Empty(t, builderfeeBeginBlockers)
	require.Empty(t, builderfeeEndBlockers)
	require.Empty(t, builderfeeInitGenesis)
	require.Empty(t, builderfeeModuleConfigs)

	require.Empty(t, custodyModuleAccPerms)
	require.Empty(t, custodyBlockedAccounts)
	require.Empty(t, custodyInitGenesis)
	require.Empty(t, custodyModuleConfigs)

	require.Empty(t, tokenisationModuleAccPerms)
	require.Empty(t, tokenisationBlockedAccounts)
	require.Empty(t, tokenisationInitGenesis)
	require.Empty(t, tokenisationModuleConfigs)

	require.Empty(t, landInitGenesis)
	require.Empty(t, landModuleConfigs)
}

// x/enforcement stays. Scope §4 keeps enforcement oversight and calls it
// critical for sovereign sale, and §6 puts it in the same workstream as this
// profile: together they answer the question a central bank board actually
// asks, which is who can stop or take our money. A settlement build without it
// cannot answer that at all.
//
// x/alias stays for the same class of reason: the jurisdictional perimeter is a
// property of an account, and roles-and-perimeter.md makes it load-bearing in
// every profile, not an extra.
func TestTheSovereignModulesAreNotTreatedAsExtras(t *testing.T) {
	for _, name := range []string{
		"enforcement", "alias", "treasury", "oracle", "paymsg", "stablecoin", "validatorgov",
	} {
		require.Contains(t, runtimeConfig.InitGenesis, name)
	}
}

// The excluded modules must be gone from every ordering as well as from the
// module set. A name left in one of these lists is a module the runtime is
// asked to find and cannot, which fails at startup rather than at review.
func TestExcludedModulesAreGoneFromEveryOrdering(t *testing.T) {
	for _, name := range []string{"amm", "builderfee", "custody", "tokenisation", "land", "emission"} {
		require.NotContains(t, runtimeConfig.BeginBlockers, name)
		require.NotContains(t, runtimeConfig.EndBlockers, name)
		require.NotContains(t, runtimeConfig.InitGenesis, name)
		require.NotContains(t, runtimeConfig.PreBlockers, name)
	}
}

// The profile name is what an error message uses to tell an operator which
// binary refused their genesis.
func TestProfileNameNamesTheTag(t *testing.T) {
	require.Contains(t, ProfileName(), "settlement")
}
