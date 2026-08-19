//go:build settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
)

// The settlement profile has no native token, so it has no issuance: x/emission
// is not merely unregistered here, it is not linked into the binary. That is
// the whole point of the tag. A module disabled by configuration is still code
// an auditor must read and an operator could re-enable; a module absent from
// the binary is absent from both.
//
// Nothing in this file imports x/emission — not even its types package — because
// a single type reference would pull the module's transitive dependencies back
// into the linked set and make the exclusion cosmetic.

var (
	emissionModuleAccPerms  []*authmodulev1.ModuleAccountPermission
	emissionBlockedAccounts []string
	emissionBeginBlockers   []string
	emissionEndBlockers     []string
	emissionInitGenesis     []string
	emissionModuleConfigs   []*appv1alpha1.ModuleConfig
)

type emissionKeepers struct{}

func (app *App) emissionDepinjectOutputs() []any { return nil }
