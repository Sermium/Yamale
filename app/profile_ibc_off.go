//go:build !ibc

package app

import (
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/core/appmodule"
	"github.com/cosmos/cosmos-sdk/codec"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

// IBC is excluded unless the `ibc` tag is passed, and this is the default.
//
// The polarity is deliberate and it is the opposite of every other tag here.
// IBC has never been exercised on this chain and is disabled at genesis; the
// scope requires it absent from the binary and therefore from audit scope. If
// inclusion were the default, the failure mode of a forgotten tag would be a
// sovereign deployment shipping an untested cross-chain entry point — and an
// audit priced without it. With exclusion as the default, a forgotten tag
// produces a chain that cannot speak IBC, which is what the chain does today
// anyway. The expensive mistake is the one made impossible.
//
// ibc-go stays in go.mod because the `ibc` build needs it. Absence is a
// property of the linked binary, not of the dependency graph, and `go list
// -deps` on the built package is what proves it.

var (
	ibcModuleAccPerms []*authmodulev1.ModuleAccountPermission
	ibcBeginBlockers  []string
	ibcInitGenesis    []string
)

// ibcKeepers is empty here so that App.IBCKeeper and friends do not exist in a
// build without IBC. See emissionKeepers for why the fields are embedded
// rather than left in place and zeroed.
type ibcKeepers struct{}

func (app *App) ibcDepinjectSupplies() []any { return nil }

// registerIBCModules is the no-op stub that keeps app.New identical across both
// builds. IBC modules do not support app wiring, so the real one in ibc.go is
// called imperatively rather than resolved from the container.
func (app *App) registerIBCModules(_ servertypes.AppOptions) error { return nil }

// RegisterIBC returns nothing to add to the client-side module manager, because
// there are no IBC modules in this binary to add.
func RegisterIBC(_ codec.Codec) map[string]appmodule.AppModule {
	return map[string]appmodule.AppModule{}
}
