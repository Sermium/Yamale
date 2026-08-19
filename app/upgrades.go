package app

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// Coordinated upgrades.
//
// A chain that has never performed one discovers whether its upgrade path works
// at the worst possible moment: with validators halted at a height, a
// deadline, and no way to test. Registering the mechanism before it is needed —
// and testing it — is the point of this file existing while the list below is
// short.
//
// Adding an upgrade means appending to `upgrades` and nothing else. The wiring
// that follows reads the list.

// Upgrade is one named, planned migration.
type Upgrade struct {
	// Name is what a governance proposal refers to. It must never change once
	// proposed: the plan is matched by name, and a mismatch halts every node.
	Name string

	// StoreUpgrades declares stores added, renamed or deleted by this upgrade.
	// The store loader applies these before any module migration runs, so a
	// module whose store is new can read its own keys.
	StoreUpgrades storetypes.StoreUpgrades

	// Handler runs the module migrations and anything else the upgrade needs.
	// A nil handler means "run the standard module migrations and nothing
	// more", which is the common case.
	Handler func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error)
}

// upgrades is every upgrade this binary knows how to perform.
//
// The machinery is exercised beyond this list too: app/upgrades_test.go
// registers a synthetic upgrade and drives it end to end, so the path is known
// to work rather than assumed to.
var upgrades = []Upgrade{
	{
		// x/alias identifiers gained a country prefix, so the module's
		// migration retires every identifier issued before there was a
		// jurisdiction to check a prefix against. No store is added or removed:
		// the jurisdiction registry lives under new key prefixes inside the
		// module's existing store.
		//
		// The handler is nil, which runs the standard module migrations and
		// nothing else. That is the whole of this upgrade — a handler that
		// seeded jurisdictions here would be inventing perimeters no
		// participant attested to.
		Name: "jurisdiction",
	},
}

// setupUpgradeHandlers registers every upgrade with the upgrade keeper and
// configures the store loader for whichever one is due.
//
// Called from New before the app loads its stores, because the store loader has
// to be set before that happens — doing it afterwards silently has no effect,
// and the added store is then missing at the height the upgrade takes effect.
func (app *App) setupUpgradeHandlers() {
	for _, u := range upgrades {
		upgrade := u // captured per iteration

		app.UpgradeKeeper.SetUpgradeHandler(
			upgrade.Name,
			func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
				sdkCtx := sdk.UnwrapSDKContext(ctx)
				sdkCtx.Logger().Info("running upgrade", "name", upgrade.Name, "height", plan.Height)

				if upgrade.Handler != nil {
					return upgrade.Handler(sdkCtx, app, fromVM)
				}
				return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			},
		)
	}

	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(fmt.Errorf("reading upgrade info: %w", err))
	}

	// Skipped heights are an operator escape hatch, not something to override.
	if app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		return
	}

	for _, upgrade := range upgrades {
		if upgrade.Name != upgradeInfo.Name {
			continue
		}
		if len(upgrade.StoreUpgrades.Added) == 0 &&
			len(upgrade.StoreUpgrades.Renamed) == 0 &&
			len(upgrade.StoreUpgrades.Deleted) == 0 {
			continue
		}

		storeUpgrades := upgrade.StoreUpgrades
		app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, &storeUpgrades))
		return
	}
}
