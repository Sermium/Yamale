//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	"cosmossdk.io/depinject/appconfig"

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
