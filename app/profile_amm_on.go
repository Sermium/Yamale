//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	ammmodulekeeper "yamale/blockchain/x/amm/keeper"
	_ "yamale/blockchain/x/amm/module" // registers the module with appconfig
	ammmoduletypes "yamale/blockchain/x/amm/types"
)

// The half of the x/amm contract that exists when the module is compiled in.
// See profile_amm_off.go for the other half and why the settlement profile
// does without it.

var (
	ammModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		{Account: ammmoduletypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner, authtypes.Staking}},
	}

	// The pools hold their reserves here against the module's own accounting. A
	// direct send credits the account and no pool, so the coins back no position
	// and no withdrawal path can ever release them.
	ammBlockedAccounts = []string{ammmoduletypes.ModuleName}

	ammBeginBlockers = []string{ammmoduletypes.ModuleName}
	ammEndBlockers   = []string{ammmoduletypes.ModuleName}
	ammInitGenesis   = []string{ammmoduletypes.ModuleName}

	ammModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   ammmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&ammmoduletypes.Module{}),
		},
	}
)

type ammKeepers struct {
	AmmKeeper ammmodulekeeper.Keeper
}

func (app *App) ammDepinjectOutputs() []any {
	return []any{&app.AmmKeeper}
}
