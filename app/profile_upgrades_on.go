//go:build !settlement

package app

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	herdtypes "yamale/blockchain/x/herd/types"
	selfcustodytypes "yamale/blockchain/x/selfcustody/types"
	syndicatetypes "yamale/blockchain/x/syndicate/types"
)

// The half of the herd-syndicate-selfcustody upgrade that exists when all three
// modules are linked. See profile_upgrades_off.go for the other.

var profileUpgrades = []Upgrade{
	{
		// Three new modules, one halt.
		//
		// They ship together because they are one coordinated stop and because
		// splitting them buys nothing: none of the three reads another's state,
		// so there is no ordering between them to get wrong, and three halts on
		// a chain being demonstrated is three chances to be halfway through.
		//
		// All three are new stores. A store has to be added before any module
		// migration runs, or the module's own InitGenesis — which arrives through
		// the standard migration path — writes into a store the app has not
		// mounted. That is why there is no hand-written genesis state below: the
		// modules initialise themselves, and this handler exists for the
		// assertions instead.
		Name: "herd-syndicate-selfcustody",
		StoreUpgrades: storetypes.StoreUpgrades{Added: []string{
			herdtypes.StoreKey,
			syndicatetypes.StoreKey,
			selfcustodytypes.StoreKey,
		}},
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			// The door out must actually open.
			//
			// This is the one assertion here that is not ceremonial.
			// Params.AcknowledgementHash is what every RequestDisclosure and
			// ConfirmDisclosure is checked against. If the chain publishes a hash
			// that this binary's embedded acknowledgement text does not produce,
			// MatchesPublished refuses every holder and NOBODY CAN TAKE THEIR KEY
			// AT ALL — which is precisely the failure x/selfcustody exists to
			// prevent, arriving silently at the moment the module is installed.
			//
			// Checked after the migration, because InitGenesis is what writes the
			// parameters being checked.
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

			// x/syndicate arrives with nobody admitted to operate a platform, and
			// x/herd with no office admitted to register an animal. Both are
			// governance decisions. An upgrade that seeded either would be
			// admitting a platform, or an authority over somebody's livestock,
			// that nobody voted for — so the emptiness is asserted rather than
			// assumed, and a future change to either DefaultGenesis cannot
			// quietly install one during a halt.
			operators := 0
			if err := app.SyndicateKeeper.Operator.Walk(ctx, nil,
				func(string, syndicatetypes.Operator) (bool, error) { operators++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the syndicate operators this upgrade just wrote: %w", err)
			}
			if operators != 0 {
				return nil, fmt.Errorf(
					"x/syndicate arrived with %d operator(s) admitted; a platform that takes other "+
						"people's money is admitted by a vote, not by a migration", operators)
			}

			authorities := 0
			if err := app.HerdKeeper.Authority.Walk(ctx, nil,
				func(string, herdtypes.Authority) (bool, error) { authorities++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the herd authorities this upgrade just wrote: %w", err)
			}
			if authorities != 0 {
				return nil, fmt.Errorf(
					"x/herd arrived with %d authority/authorities admitted; a registry office is "+
						"admitted by a vote, not by a migration", authorities)
			}

			// Said out loud, in every validator's log, at the moment it becomes
			// true. The first of these changes what a bank transfer can do — a
			// deal token will not move to an address that has not been onboarded,
			// and that restriction binds on an ordinary MsgSend, through authz and
			// through a treasury alike. An operator who has not read this will
			// diagnose a refused transfer as a broken node.
			ctx.Logger().Info(
				"three modules arrived",
				"herd", "a livestock register where parentage is refused at consensus; no office is "+
					"admitted yet, so nothing can be registered until governance admits one",
				"syndicate", "private placements with escrowed commitments; transfers of a deal token "+
					"are gated by a bank send restriction from this height, so both sides must have "+
					"completed onboarding however the tokens are moved",
				"selfcustody", "the key-disclosure pathway; a holder asks, waits "+
					"the chain-enforced delay, and asks again, and the operator can neither refuse "+
					"nor accelerate it",
			)

			return vm, nil
		},
	},
}
