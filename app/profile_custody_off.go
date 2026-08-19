//go:build settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
)

// x/custody is not linked into the settlement profile.
//
// It mints an on-chain claim once enough attestors agree that an asset is held
// somewhere off this chain. That is a bridge, and its safety rests entirely on
// the attestor set rather than on anything the ledger can check. Scope 3 gives
// the settlement profile the issuers' own currencies and nothing else, so a
// deployment carrying this module carries an attestor set a central bank did
// not ask for, cannot supervise, and is paying to have audited.

var (
	custodyModuleAccPerms  []*authmodulev1.ModuleAccountPermission
	custodyBlockedAccounts []string
	custodyInitGenesis     []string
	custodyModuleConfigs   []*appv1alpha1.ModuleConfig
)

type custodyKeepers struct{}

func (app *App) custodyDepinjectOutputs() []any { return nil }
