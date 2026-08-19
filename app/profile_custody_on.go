//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	custodymodulekeeper "yamale/blockchain/x/custody/keeper"
	_ "yamale/blockchain/x/custody/module" // registers the module with appconfig
	custodymoduletypes "yamale/blockchain/x/custody/types"
)

// The half of the x/custody contract that exists when the module is compiled
// in. See profile_custody_off.go for the other half.

var (
	custodyModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		// Minter and Burner, and deliberately not Staking: a claim on an
		// externally-held asset must never be bonded, or the reserve backs a
		// position that cannot be unwound in time to honour a redemption.
		{Account: custodymoduletypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner}},
	}

	// Holds a claim between the redemption request and the burn, and holds the
	// fee it retains. A direct send here belongs to no deposit and no redemption,
	// so no payout path would ever release it.
	custodyBlockedAccounts = []string{custodymoduletypes.ModuleName}

	custodyInitGenesis = []string{custodymoduletypes.ModuleName}

	custodyModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   custodymoduletypes.ModuleName,
			Config: appconfig.WrapAny(&custodymoduletypes.Module{}),
		},
	}
)

type custodyKeepers struct {
	CustodyKeeper custodymodulekeeper.Keeper
}

func (app *App) custodyDepinjectOutputs() []any {
	return []any{&app.CustodyKeeper}
}
