//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	"cosmossdk.io/depinject/appconfig"

	_ "yamale/blockchain/x/land/module" // registers the module with appconfig
	landmoduletypes "yamale/blockchain/x/land/types"
)

// The half of the x/land contract that exists when the module is compiled in.
// See profile_land_off.go for the other half.

var (
	landInitGenesis = []string{landmoduletypes.ModuleName}

	landModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   landmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&landmoduletypes.Module{}),
		},
	}
)
