//go:build settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// x/builderfee is replaced in the settlement profile, not merely dropped.
//
// Its post handler splits a share of each gas fee out of the fee collector to a
// registered builder and leaves the remainder there for normal validator
// distribution. That remainder is the assumption this profile removes: there
// are no staking rewards here, the whole fee belongs to the treasury operating
// account, and validators are paid by service contract. Keeping both would put
// two mechanisms on one balance, with the builder's share taken before the
// sweep and no record of the difference anywhere the treasury can see.
//
// What routes the fee instead is in profile_settlementfee_on.go.

var (
	builderfeeModuleAccPerms  []*authmodulev1.ModuleAccountPermission
	builderfeeBlockedAccounts []string
	builderfeeBeginBlockers   []string
	builderfeeEndBlockers     []string
	builderfeeInitGenesis     []string
	builderfeeModuleConfigs   []*appv1alpha1.ModuleConfig
)

type builderfeeKeepers struct{}

func (app *App) builderfeeDepinjectOutputs() []any { return nil }

// builderfeePostDecorators contributes nothing to this profile's post handler.
func (app *App) builderfeePostDecorators() []sdk.PostDecorator { return nil }
