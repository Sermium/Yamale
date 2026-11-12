//go:build settlement

package app

import appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"

// x/jewel is a commercial register for jewelers, not settlement
// infrastructure, so a settlement binary does not carry it.

var (
	jewelInitGenesis   []string
	jewelModuleConfigs []*appv1alpha1.ModuleConfig
)

type jewelKeepers struct{}

func (app *App) jewelDepinjectOutputs() []any { return nil }
