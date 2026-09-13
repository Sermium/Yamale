//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	"cosmossdk.io/depinject/appconfig"

	herdmodulekeeper "yamale/blockchain/x/herd/keeper"
	_ "yamale/blockchain/x/herd/module" // registers the module with appconfig
	herdmoduletypes "yamale/blockchain/x/herd/types"
)

// The half of the x/herd contract that exists when the module is compiled in.
// See profile_herd_off.go for the other half.

var (
	herdInitGenesis = []string{herdmoduletypes.ModuleName}

	herdModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   herdmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&herdmoduletypes.Module{}),
		},
	}
)

// The keeper is held so the herd-syndicate-selfcustody upgrade handler can
// assert that no registry office arrives admitted. Embedded from here rather
// than declared in app.go for the reason the other profile-varying keepers are:
// a Go field cannot be removed by a build tag, and a field left in place would
// hold the zero keeper — usable, silently wrong, and indistinguishable at the
// call site from a working one.
type herdKeepers struct {
	HerdKeeper herdmodulekeeper.Keeper
}

func (app *App) herdDepinjectOutputs() []any {
	return []any{&app.HerdKeeper}
}
