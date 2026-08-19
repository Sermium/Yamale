//go:build !ibc

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// IBC is absent unless opted into, and this is the default build.
//
// The assertion that matters is on the genesis order. IBC's store keys are
// registered imperatively in ibc.go rather than through app wiring, so in this
// build there are no ibc stores; naming ibc, transfer or interchainaccounts in
// InitGenesis anyway would fail InitChain at genesis rather than at compile
// time, which is the one failure a build tag is supposed to make impossible.
func TestIBCIsAbsentFromThisProfile(t *testing.T) {
	require.Empty(t, ibcModuleAccPerms)
	require.Empty(t, ibcBeginBlockers)
	require.Empty(t, ibcInitGenesis)

	require.Empty(t, RegisterIBC(nil),
		"the client-side module manager must have no IBC modules to register")

	app := &App{}
	require.Nil(t, app.ibcDepinjectSupplies(),
		"supplying the IBC keeper getter would put an ibc-go type into the container and undo the exclusion")
	require.NoError(t, app.registerIBCModules(nil),
		"the stub must be a no-op so app.New is identical across both builds")
}
