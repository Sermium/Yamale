package app

import (
	"context"
	"testing"

	"cosmossdk.io/core/header"
	"cosmossdk.io/log"
	upgrade "cosmossdk.io/x/upgrade"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/collections"

	aliastypes "yamale/blockchain/x/alias/types"
	nettingtypes "yamale/blockchain/x/netting/types"
)

// The upgrade path is exercised here rather than left until it is needed.
//
// A chain that has never performed a coordinated upgrade finds out whether its
// mechanism works with every validator halted at a height, a deadline, and no
// way to test — the worst moment to discover a mistake in the one procedure
// that cannot be retried casually. The `upgrades` list is deliberately empty
// until there is a real one, so these tests register a synthetic upgrade and
// drive the whole sequence an operator actually performs.

func newUpgradeTestApp(t *testing.T) *App {
	t.Helper()
	return New(log.NewNopLogger(), dbm.NewMemDB(), nil, true, simtestutil.NewAppOptionsWithFlagHome(t.TempDir()))
}

// atHeight sets the height the upgrade module actually reads.
//
// It takes it from HeaderInfo rather than from the context's block height, and
// setting only the latter leaves every check running at height zero — which
// makes a test that should fail pass, quietly.
func atHeight(ctx sdk.Context, height int64) sdk.Context {
	return ctx.WithBlockHeight(height).WithHeaderInfo(header.Info{Height: height})
}

// The full sequence, in the order it happens on a real network:
//
//  1. governance schedules the upgrade while the old binary is running;
//  2. the old binary reaches the height, does not know the upgrade, and stops;
//  3. operators swap in the new binary, which does know it;
//  4. the upgrade applies and the chain continues.
//
// Step 2 is the part people are surprised by: the node stops rather than
// carrying on, and that is the whole point — every validator halts at the same
// height so none of them diverges.
func TestTheUpgradeSequenceFromScheduleToApplied(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	require.NoError(t, app.UpgradeKeeper.SetModuleVersionMap(ctx, app.ModuleManager.GetVersionMap()))

	const (
		name          = "rehearsal-v2"
		upgradeHeight = 10
	)

	// 1. Scheduled while the running binary has no handler for it.
	require.NoError(t, app.UpgradeKeeper.ScheduleUpgrade(ctx, upgradetypes.Plan{
		Name:   name,
		Height: upgradeHeight,
	}))

	// Before the height the old binary carries on as normal.
	_, err := upgrade.PreBlocker(atHeight(ctx, upgradeHeight-1), app.UpgradeKeeper)
	require.NoError(t, err, "an upgrade must not disturb the chain before its height")

	// 2. At the height, a binary that does not know the upgrade stops.
	at := atHeight(ctx, upgradeHeight)
	_, err = upgrade.PreBlocker(at, app.UpgradeKeeper)
	require.Error(t, err, "the old binary must stop rather than continue past the upgrade height")
	require.Contains(t, err.Error(), "UPGRADE \""+name+"\" NEEDED")

	// 3. The new binary knows it.
	ran := false
	app.UpgradeKeeper.SetUpgradeHandler(name,
		func(goCtx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			ran = true
			require.NotEmpty(t, fromVM, "the handler must receive the versions it is migrating from")
			return app.ModuleManager.RunMigrations(goCtx, app.Configurator(), fromVM)
		})

	// 4. It applies.
	_, err = upgrade.PreBlocker(at, app.UpgradeKeeper)
	require.NoError(t, err)
	require.True(t, ran, "the handler must run at the planned height")

	// The plan is cleared once applied, so a restarted node does not run it again.
	_, err = app.UpgradeKeeper.GetUpgradePlan(at)
	require.Error(t, err, "the plan must be cleared after it has been applied")

	versions, err := app.UpgradeKeeper.GetModuleVersionMap(at)
	require.NoError(t, err)
	require.Equal(t, app.ModuleManager.GetVersionMap(), versions,
		"the persisted module versions must match the binary after migrating")
}

// Deploying the new binary early stops the node, deliberately.
//
// This is the mistake an operator is most likely to make — the new binary is
// built and ready, so why not roll it out now — and the answer is that a node
// running new logic before the coordinated height would diverge from the rest
// of the network. The SDK refuses rather than letting that happen, and the
// message says exactly what to do.
func TestDeployingTheNewBinaryEarlyStopsTheNode(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	const name = "rehearsal-v2"
	app.UpgradeKeeper.SetUpgradeHandler(name,
		func(goCtx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			return fromVM, nil
		})

	require.NoError(t, app.UpgradeKeeper.ScheduleUpgrade(ctx, upgradetypes.Plan{
		Name:   name,
		Height: 10,
	}))

	_, err := upgrade.PreBlocker(atHeight(ctx, 9), app.UpgradeKeeper)
	require.Error(t, err)
	require.Contains(t, err.Error(), "BINARY UPDATED BEFORE TRIGGER",
		"the node must refuse to run new logic before the coordinated height")
}

// Every declared upgrade must have a handler registered by setupUpgradeHandlers,
// or the chain halts at its height. This walks the same list the app registers
// from, so an upgrade added without a handler fails here rather than live.
func TestEveryDeclaredUpgradeHasAHandler(t *testing.T) {
	app := newUpgradeTestApp(t)

	for _, u := range upgrades {
		require.True(t, app.UpgradeKeeper.HasHandler(u.Name),
			"upgrade %q is declared but has no handler registered", u.Name)
	}
}

// Upgrade names are matched by string, so a duplicate silently shadows the
// earlier registration and one of the two migrations never runs.
func TestUpgradeDeclarationsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}

	for _, u := range upgrades {
		require.NotEmpty(t, u.Name, "an upgrade with no name cannot be proposed")
		require.False(t, seen[u.Name], "duplicate upgrade name %q", u.Name)
		seen[u.Name] = true

		for _, added := range u.StoreUpgrades.Added {
			require.NotEmpty(t, added, "upgrade %q declares an empty added store key", u.Name)
		}
		for _, deleted := range u.StoreUpgrades.Deleted {
			require.NotEmpty(t, deleted, "upgrade %q declares an empty deleted store key", u.Name)
		}
		for _, renamed := range u.StoreUpgrades.Renamed {
			require.NotEmpty(t, renamed.OldKey, "upgrade %q renames from an empty key", u.Name)
			require.NotEmpty(t, renamed.NewKey, "upgrade %q renames to an empty key", u.Name)
		}
	}
}

// priorVersions is the version map a real upgrade actually receives.
//
// Not GetVersionMap(): that is the map the *new* binary reports, which already
// contains every new module at its current version — so RunMigrations sees
// nothing to do and never calls a new module's InitGenesis. A real upgrade reads
// the map stored on chain by the old binary, which does not know the module
// exists. Passing the wrong one makes an upgrade test pass while the upgrade it
// claims to exercise does nothing.
func priorVersions(app *App, without ...string) module.VersionMap {
	vm := app.ModuleManager.GetVersionMap()
	for _, name := range without {
		delete(vm, name)
	}
	return vm
}

// findUpgrade returns the declared upgrade by name, failing if it is gone.
//
// By name rather than by index, because the assertions below are about one
// specific upgrade's handler and an index would silently start testing a
// different one the next time the list grows.
func findUpgrade(t *testing.T, name string) Upgrade {
	t.Helper()
	for _, u := range upgrades {
		if u.Name == name {
			return u
		}
	}
	t.Fatalf("upgrade %q is no longer declared; if it was renamed, every node that "+
		"already has the old plan scheduled will halt on a name mismatch", name)
	return Upgrade{}
}

// The netting-and-perimeter upgrade must leave netting switched off.
//
// This is the assertion the handler exists for. A netting cycle length is a
// divisor in an end blocker and a window that opens on a chain where nobody has
// posted a reserve collects obligations that cannot settle — so the upgrade has
// to be the thing that decides netting is off, not something that inherits it
// from whatever DefaultParams happens to say later.
func TestNettingArrivesSwitchedOff(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	u := findUpgrade(t, "netting-and-perimeter")
	require.NotNil(t, u.Handler, "the assertions in this upgrade live in its handler")
	require.Contains(t, u.StoreUpgrades.Added, nettingtypes.StoreKey,
		"x/netting is a new module, so its store has to be added before migrations run")

	_, err := u.Handler(ctx, app, priorVersions(app, nettingtypes.ModuleName))
	require.NoError(t, err)

	params, err := app.NettingKeeper.Params.Get(ctx)
	require.NoError(t, err, "the upgrade must leave netting's parameters readable")
	require.Zero(t, params.CycleBlocks,
		"netting must arrive off and be enabled deliberately once reserves are posted")
}

// And it must refuse, loudly, if netting is somehow on.
//
// Without this the assertion above only tests DefaultParams, which is the thing
// most likely to change under it. Here the handler is given a chain where
// netting is already running and has to stop the upgrade rather than proceed.
func TestTheUpgradeRefusesToLeaveNettingRunning(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	u := findUpgrade(t, "netting-and-perimeter")
	_, err := u.Handler(ctx, app, priorVersions(app, nettingtypes.ModuleName))
	require.NoError(t, err)

	params, err := app.NettingKeeper.Params.Get(ctx)
	require.NoError(t, err)
	params.CycleBlocks = 100
	require.NoError(t, app.NettingKeeper.Params.Set(ctx, params))

	// The full version map this time, so RunMigrations does NOT re-run netting's
	// InitGenesis and reset what was just written. That is also the realistic
	// shape of the hazard: the handler checks whatever the parameters are once
	// migrations have finished, whether a migration wrote them or they were
	// already there.
	_, err = u.Handler(ctx, app, app.ModuleManager.GetVersionMap())
	require.Error(t, err, "an upgrade that switches netting on for a chain with no reserves must stop")
	require.Contains(t, err.Error(), "must arrive switched off")
}

// The perimeter arrives empty, which is the whole point.
//
// An upgrade that seeded a chain-wide grant to get the existing flows working
// again would be granting authority nobody voted for — on a chain where a grant
// is what permits freezing an account. So this asserts the absence, and the
// absence is why authority actions refuse until governance acts.
func TestThePerimeterArrivesWithNoGrants(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	u := findUpgrade(t, "netting-and-perimeter")
	_, err := u.Handler(ctx, app, priorVersions(app, nettingtypes.ModuleName))
	require.NoError(t, err)

	count := 0
	require.NoError(t, app.AliasKeeper.RoleGrants.Walk(ctx, nil,
		func(_ collections.Triple[string, int32, string], _ aliastypes.RoleGrant) (bool, error) {
			count++
			return false, nil
		}))
	require.Zero(t, count, "an upgrade must not invent authority; grants come from governance")
}
