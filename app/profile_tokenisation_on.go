//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	_ "yamale/blockchain/x/tokenisation/module" // registers the module with appconfig
	tokenisationmoduletypes "yamale/blockchain/x/tokenisation/types"
)

// The half of the x/tokenisation contract that exists when the module is
// compiled in. See profile_tokenisation_off.go for the other half.

var (
	tokenisationModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		// Mints a shareholding when an asset is fractionalised, burns it as
		// holders redeem.
		{Account: tokenisationmoduletypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner}},
	}

	// The vaults hold what shareholders are owed. Money arriving here outside
	// MsgFundVault raises no index and is owed to nobody.
	tokenisationBlockedAccounts = []string{tokenisationmoduletypes.ModuleName}

	tokenisationInitGenesis = []string{tokenisationmoduletypes.ModuleName}

	tokenisationModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   tokenisationmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&tokenisationmoduletypes.Module{}),
		},
	}
)
