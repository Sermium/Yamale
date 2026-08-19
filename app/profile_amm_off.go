//go:build settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
)

// x/amm is not linked into the settlement profile.
//
// Scope 3 puts the market modules in the consortium profile, which is not
// offered to sovereign buyers. The reason is not that an AMM is unsafe but that
// it is the wrong instrument here: a monetary-union ledger settles payments in
// currencies its issuers control, and a constant-product pool prices those
// currencies against each other with no issuer, supervisor or oracle in the
// path. Shipping one inside a central bank's deployment offers a route around
// the supervised rate the deployment was bought to enforce.
//
// Nothing here imports x/amm, not even its types package, because one type
// reference pulls the module's transitive dependencies back into the linked set
// and makes the exclusion cosmetic. See profile_emission_off.go.

var (
	ammModuleAccPerms  []*authmodulev1.ModuleAccountPermission
	ammBlockedAccounts []string
	ammBeginBlockers   []string
	ammEndBlockers     []string
	ammInitGenesis     []string
	ammModuleConfigs   []*appv1alpha1.ModuleConfig
)

// ammKeepers is empty so that App.AmmKeeper does not exist in this build. A
// field left in place would hold the zero keeper: usable, silently wrong, and
// indistinguishable at the call site from a working one.
type ammKeepers struct{}

func (app *App) ammDepinjectOutputs() []any { return nil }
