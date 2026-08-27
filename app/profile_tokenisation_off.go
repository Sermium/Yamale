//go:build settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
)

// x/tokenisation is not linked into the settlement profile.
//
// It fractionalises a registered asset into transferable shares, which makes it
// half of the registry profile's product and none of this one's. It also
// depends on x/land, so leaving it linked would drag the cadastre into a
// monetary-union deployment - the one combination scope 2 rules out, since no
// state will place its cadastre on a ledger other states validate.

var (
	tokenisationModuleAccPerms  []*authmodulev1.ModuleAccountPermission
	tokenisationBlockedAccounts []string
	tokenisationInitGenesis     []string
	tokenisationModuleConfigs   []*appv1alpha1.ModuleConfig
)

type tokenisationKeepers struct{}

func (app *App) tokenisationDepinjectOutputs() []any { return nil }

// No module, no shareholding, nothing to settle.
func (app *App) registerTokenisationSendRestriction() {}
