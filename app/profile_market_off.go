//go:build settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
)

// x/market is not linked into the settlement profile.
//
// It has no choice in the matter, which makes this the least interesting of the
// profile splits: the module trades x/tokenisation's fraction denominations and
// asks x/tokenisation which vehicle each one belongs to and what it settles in.
// A settlement binary carries neither the register nor its shares, so a market
// over them would be a market over nothing.
//
// The reasoning that keeps the register out applies here unchanged, and a little
// more sharply. The settlement profile is a monetary-union binary: what it
// carries is what several states have to agree to validate for each other. A
// secondary market in the ownership of private vehicles in one member's
// jurisdiction is a commercial product, and every other state would be
// validating blocks containing its price formation.

var (
	marketModuleAccPerms  []*authmodulev1.ModuleAccountPermission
	marketBlockedAccounts []string
	marketInitGenesis     []string
	marketModuleConfigs   []*appv1alpha1.ModuleConfig
)

type marketKeepers struct{}

func (app *App) marketDepinjectOutputs() []any { return nil }

// No register, no shares, and nothing to hold in escrow.
func (app *App) registerMarketEscrow() {}
