//go:build settlement

package app

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	selfcustodytypes "yamale/blockchain/x/selfcustody/types"
)

// The half of the herd-syndicate-selfcustody upgrade that exists in a settlement
// build. See profile_upgrades_on.go for the other.
//
// A settlement deployment gets the door out and neither commercial module. That
// is the same split app_config.go already makes and for the same reasons: a
// livestock register and a private-placement platform are things one state or
// one company operates, and no central bank is going to validate blocks carrying
// the cap table of a private company in somebody else's country. x/selfcustody
// is in every profile because it is not a product — it is the right of a person
// held in custody to stop being held that way, and those accounts exist on a
// settlement chain exactly as they do here.
//
// The NAME is deliberately identical to the default profile's. These are
// different chains and never the same one, so the names can never collide; what
// a shared name buys is that the runbook, the proposal text and this document
// say one thing rather than two, and an operator on either deployment proposes
// `herd-syndicate-selfcustody` and gets what their binary carries.

var profileUpgrades = []Upgrade{
	{
		Name:          "herd-syndicate-selfcustody",
		StoreUpgrades: storetypes.StoreUpgrades{Added: []string{selfcustodytypes.StoreKey}},
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			// The same assertion the default profile makes, and for the same
			// reason: a published acknowledgement hash that this binary's
			// embedded text does not produce refuses every holder, and nobody
			// can take their own key. See profile_upgrades_on.go.
			params, err := app.SelfCustodyKeeper.Params.Get(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading the key-disclosure parameters this upgrade just wrote: %w", err)
			}
			if want := selfcustodytypes.AcknowledgementHash(); params.AcknowledgementHash != want {
				return nil, fmt.Errorf(
					"the chain would publish acknowledgement hash %s but this binary's embedded text "+
						"hashes to %s; every key disclosure would be refused and no holder could take "+
						"their own key", params.AcknowledgementHash, want)
			}

			ctx.Logger().Info(
				"the key-disclosure pathway arrived",
				"selfcustody", "a holder asks, waits the chain-enforced delay, and asks again; the "+
					"operator can neither refuse nor accelerate it",
				"absent", "x/herd and x/syndicate are not linked into a settlement build",
			)

			return vm, nil
		},
	},
	{
		// The other half of syndicate-documents. See profile_upgrades_on.go.
		//
		// x/syndicate is not linked into a settlement build, so there is nothing
		// to do here — but the upgrade must EXIST under this name, and that is
		// the whole point of the entry. An upgrade the binary does not know is
		// not a no-op: x/upgrade panics the node at the halt height and it stays
		// down until somebody installs a binary that knows the name. A settlement
		// chain that never adopts this proposal never reaches the height and
		// never runs this; a settlement chain that does adopt it carries on.
		Name: "syndicate-documents",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}
			ctx.Logger().Info(
				"deal documents: nothing to do",
				"absent", "x/syndicate is not linked into a settlement build, so there are no deals "+
					"to attach papers to",
			)
			return vm, nil
		},
	},
	{
		// The other half of tokenisation-prorata. x/tokenisation is not linked
		// into a settlement build, so there is nothing to migrate — but the
		// upgrade must EXIST under this name, which is the whole point of the
		// entry: a name the binary does not know panics the node at the halt
		// height and it stays down until somebody installs one that does.
		Name: "tokenisation-prorata",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}
			ctx.Logger().Info(
				"pro rata temporis: nothing to do",
				"absent", "x/tokenisation is not linked into a settlement build, so there are "+
					"no vaults to apportion",
			)
			return vm, nil
		},
	},
}
