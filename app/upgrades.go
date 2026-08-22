package app

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	constitutiontypes "yamale/blockchain/x/constitution/types"
	nettingtypes "yamale/blockchain/x/netting/types"
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
	{
		// x/constitution is a new module with a new store, and a chain that
		// already holds value cannot simply grow one: the store has to be added
		// before any module migration runs, and the settlement has to be
		// written before x/enforcement is next asked to check itself against it.
		//
		// The handler adopts the four enforcement parameters in force rather
		// than choosing them, because they are what this chain already decided
		// and rewriting them here would be an amendment with no delay and no
		// ratification behind it. The concentration ceilings have no equivalent
		// to adopt — nothing on a running chain implies them — so they arrive at
		// the shipped defaults, and the first thing governance should do after
		// this upgrade is amend them deliberately. The upgrade fails loudly if
		// the result is not a settlement the chain can enforce, which is the
		// correct outcome: better a halted upgrade than a constitution nobody
		// read.
		Name:          "constitution",
		StoreUpgrades: storetypes.StoreUpgrades{Added: []string{constitutiontypes.StoreKey}},
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			params, err := app.EnforcementKeeper.Params.Get(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading the enforcement parameters to adopt: %w", err)
			}

			invariants := constitutiontypes.DefaultInvariants()
			invariants.EnforcementThresholdBps = params.ThresholdBps
			invariants.EnforcementRecoveryDestination = params.RecoveryDestination
			invariants.EnforcementVotingPeriodBlocks = params.VotingPeriodBlocks
			invariants.EnforcementProvisionalFreezeBlocks = params.ProvisionalFreezeBlocks

			genesis := constitutiontypes.DefaultGenesis()
			genesis.Invariants = invariants
			if err := app.ConstitutionKeeper.InitGenesis(ctx, *genesis); err != nil {
				return nil, fmt.Errorf("adopting a constitution from the parameters in force: %w", err)
			}

			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	},
	{
		// Two changes that ship together because they are one halt.
		//
		// x/netting is a new module with a new store, so the store has to be
		// added before any module migration runs. Its own InitGenesis then
		// arrives through the standard migration path, which is why there is no
		// hand-written state here.
		//
		// x/alias gained role grants, and those live under new key prefixes
		// inside the module's existing store — no store to add, no migration to
		// run, because a chain with no grants recorded is the correct starting
		// state and an upgrade that invented some would be granting authority
		// nobody voted for.
		//
		// The handler exists for the two assertions below rather than for any
		// state it writes.
		Name:          "netting-and-perimeter",
		StoreUpgrades: storetypes.StoreUpgrades{Added: []string{nettingtypes.StoreKey}},
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			// Netting must arrive switched off. A netting cycle length is a
			// divisor in an end blocker, and a window that opens on a chain
			// where nobody has posted a reserve collects obligations it cannot
			// settle. Off is the shipped default; asserted here so it is a
			// decision this upgrade made rather than one it inherited, and so a
			// future change to DefaultParams cannot quietly switch netting on
			// for a chain in the middle of an upgrade.
			params, err := app.NettingKeeper.Params.Get(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading the netting parameters this upgrade just wrote: %w", err)
			}
			if params.CycleBlocks != 0 {
				return nil, fmt.Errorf(
					"netting arrived with cycle_blocks=%d; it must arrive switched off and be "+
						"enabled by a deliberate governance proposal once participants have "+
						"posted reserves", params.CycleBlocks)
			}

			// Said out loud, in every validator's log, at the moment it becomes
			// true. From this height every authority action in x/land,
			// x/enforcement, x/stablecoin and x/paymsg is checked against a
			// jurisdictional grant — and this chain has none, so those actions
			// refuse until governance makes some. That is the perimeter working
			// as specified, and it is also the kind of change that gets
			// diagnosed as a broken node three days later by somebody who was
			// not in the room.
			ctx.Logger().Info(
				"the jurisdictional perimeter is now enforced",
				"consequence", "authority actions in land, enforcement, stablecoin and paymsg "+
					"refuse until governance grants a role covering the target's country, and "+
					"until each target account has a jurisdiction recorded",
				"next", "MsgGrantRole by governance, and MsgSetJurisdiction per account",
			)

			return vm, nil
		},
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
