//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"

	jewelmodulekeeper "yamale/blockchain/x/jewel/keeper"
	_ "yamale/blockchain/x/jewel/module" // registers the module with appconfig
	jewelmoduletypes "yamale/blockchain/x/jewel/types"
)

// The half of the x/jewel contract that exists when the module is compiled
// in. See profile_jewel_off.go for the other half.

var (
	// After x/nft in the genesis order, which the SDK block already places
	// earlier: a jewel names an x/nft class and token.
	jewelInitGenesis = []string{jewelmoduletypes.ModuleName}

	// The marketplace fee account. No permissions: it neither mints nor burns,
	// it holds the fees BuyJewel pays in until governance pays them out.
	jewelModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		{Account: jewelmoduletypes.ModuleName},
	}

	// Blocked, like every account here that holds funds against its module's
	// own reckoning: a fee sent by hand is not a fee, and only governance can
	// move it out again.
	jewelBlockedAccounts = []string{jewelmoduletypes.ModuleName}

	jewelModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   jewelmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&jewelmoduletypes.Module{}),
		},
	}
)

type jewelKeepers struct {
	JewelKeeper jewelmodulekeeper.Keeper
}

func (app *App) jewelDepinjectOutputs() []any {
	return []any{&app.JewelKeeper}
}
