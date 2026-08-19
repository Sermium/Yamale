//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	emissionmodulekeeper "yamale/blockchain/x/emission/keeper"
	_ "yamale/blockchain/x/emission/module" // registers the module with appconfig
	emissionmoduletypes "yamale/blockchain/x/emission/types"
)

// The half of the emission contract that exists when the module is compiled
// in. See profile_emission_off.go for the other half and app_config.go for why
// the contract is shaped this way.

var (
	emissionModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		{Account: emissionmoduletypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner, authtypes.Staking}},
	}

	emissionBlockedAccounts = []string{emissionmoduletypes.ModuleName}

	// emission replaces the standard x/mint module and must run first, same as
	// x/mint did, so newly-minted coins are in the fee collector before
	// distribution allocates them this block.
	emissionBeginBlockers = []string{emissionmoduletypes.ModuleName}

	emissionEndBlockers = []string{emissionmoduletypes.ModuleName}

	emissionInitGenesis = []string{emissionmoduletypes.ModuleName}

	emissionModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   emissionmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&emissionmoduletypes.Module{}),
		},
	}
)

// emissionKeepers is embedded in App so that App.EmissionKeeper exists only in
// a build that has the module. A plain field could not be removed by a tag,
// and a field left in place would be the zero keeper — usable, silently
// wrong, and indistinguishable from a working one at the call site.
type emissionKeepers struct {
	EmissionKeeper emissionmodulekeeper.Keeper
}

func (app *App) emissionDepinjectOutputs() []any {
	return []any{&app.EmissionKeeper}
}
