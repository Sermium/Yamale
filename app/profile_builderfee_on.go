//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	builderfeemodulekeeper "yamale/blockchain/x/builderfee/keeper"
	_ "yamale/blockchain/x/builderfee/module" // registers the module with appconfig
	builderfeemoduletypes "yamale/blockchain/x/builderfee/types"
)

// The half of the x/builderfee contract that exists when the module is compiled
// in. See profile_builderfee_off.go for why the settlement profile replaces it
// rather than simply dropping it.

var (
	builderfeeModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		{Account: builderfeemoduletypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner, authtypes.Staking}},
	}

	builderfeeBlockedAccounts = []string{builderfeemoduletypes.ModuleName}

	builderfeeBeginBlockers = []string{builderfeemoduletypes.ModuleName}
	builderfeeEndBlockers   = []string{builderfeemoduletypes.ModuleName}
	builderfeeInitGenesis   = []string{builderfeemoduletypes.ModuleName}

	builderfeeModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   builderfeemoduletypes.ModuleName,
			Config: appconfig.WrapAny(&builderfeemoduletypes.Module{}),
		},
	}
)

type builderfeeKeepers struct {
	BuilderfeeKeeper builderfeemodulekeeper.Keeper
}

func (app *App) builderfeeDepinjectOutputs() []any {
	return []any{&app.BuilderfeeKeeper}
}

// builderfeePostDecorators is the whole of this profile's post handler. The fee
// share runs after the message rather than in the ante chain because a split
// taken from a transaction that then failed would pay a builder for work the
// chain rejected.
func (app *App) builderfeePostDecorators() []sdk.PostDecorator {
	return []sdk.PostDecorator{builderfeemodulekeeper.NewFeeShareDecorator(app.BuilderfeeKeeper)}
}
