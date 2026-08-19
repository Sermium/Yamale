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
