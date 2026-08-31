package app

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	aliastypes "yamale/blockchain/x/alias/types"
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
	{
		// Two roles that existed and conferred nothing now confer something, and
		// two authorities that were parameters become grants. They ship as one
		// upgrade because they are one halt and because the second half cannot be
		// done without the first: the emergency authority's replacement is a grant
		// of ROLE_ENFORCEMENT_AUTHORITY, and that role only became usable by an
		// office in this same change.
		//
		// No store is added or removed. Both modules' migrations work inside their
		// existing stores: x/alias 2-to-3 turns the foundation_administrators
		// parameter into chain-wide grants, and x/enforcement 1-to-2 rewrites its
		// parameters without the retired emergency_authority.
		//
		// The handler exists for the half x/enforcement cannot do for itself.
		// Grants live in x/alias and x/enforcement has no edge into it —
		// deliberately, so the perimeter cannot be widened from inside the module
		// it constrains — so the address that held emergency_authority is read
		// here, BEFORE the module migrations discard it, and written as a grant
		// afterwards.
		Name: "roles-that-do-something",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			// Read first. After RunMigrations the parameter is gone from the
			// store, and the whole reason this is read as raw bytes rather than
			// off the struct is that a reserved field decodes to nothing with no
			// error at all — see x/internal/legacyparams.
			emergencyAuthority, hadEmergencyAuthority, err := app.EnforcementKeeper.LegacyEmergencyAuthority(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading the emergency authority this upgrade has to carry across: %w", err)
			}

			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			if hadEmergencyAuthority {
				// Chain-wide, and that is the honest reading of what the
				// parameter was rather than a preference. emergency_authority had
				// no territorial limit: it could freeze any account on this
				// chain. Granting a country scope here would be this upgrade
				// deciding to narrow an authority nobody voted to narrow, and
				// deciding which country on top of it. Governance can revoke this
				// grant and issue country ones the moment it wants to; an upgrade
				// that silently removed the emergency path is not something
				// anybody can undo before they notice.
				//
				// It is worth being plain that this is a widening in one respect,
				// because collapsing two mechanisms into one always is. Holding
				// ROLE_ENFORCEMENT_AUTHORITY also lets this account open an
				// ordinary case, including a seizure accusation, which
				// emergency_authority could not do. What it does not let it do is
				// decide one: a seizure still needs two thirds of bonded voting
				// power, and this account has no vote unless it is also a
				// validator.
				grant := aliastypes.RoleGrant{
					Holder:       emergencyAuthority,
					Role:         aliastypes.ROLE_ENFORCEMENT_AUTHORITY,
					Jurisdiction: aliastypes.ChainWide,
					// Attributed to governance, because governance set the
					// parameter. Attributing it to the upgrade would lose the one
					// fact granted_by exists to record.
					GrantedBy:       app.AliasKeeper.GetAuthority(),
					GrantedAtHeight: ctx.BlockHeight(),
				}
				if err := app.AliasKeeper.GrantForUpgrade(ctx, grant); err != nil {
					return nil, fmt.Errorf(
						"carrying the emergency authority %s across as a role grant: %w", emergencyAuthority, err)
				}
				ctx.Logger().Info(
					"the emergency authority is a role grant now",
					"holder", emergencyAuthority,
					"role", aliastypes.RoleName(grant.Role),
					"jurisdiction", grant.Jurisdiction,
					"note", "chain-wide because the parameter it replaces had no territorial limit; "+
						"governance can revoke it and grant country scopes instead",
				)
			}

			// Said out loud, in every validator's log, at the moment it becomes
			// true. Both of these change who may send a message that already
			// exists, which is the kind of change that gets diagnosed as a broken
			// node three days later by somebody who was not in the room.
			ctx.Logger().Info(
				"two roles that conferred nothing now confer something",
				"supervisor", "a holder of ROLE_SUPERVISOR covering a country is entitled to be a viewing-key "+
					"recipient for payloads settling there, and a country's appointed regulator must now hold that role",
				"enforcement", "a holder of ROLE_ENFORCEMENT_AUTHORITY may open a case and freeze in an emergency "+
					"without being a bonded validator; the validator set still decides every case",
				"administrators", "appointing a foundation administrator is MsgGrantRole with "+
					"ROLE_FOUNDATION_ADMINISTRATOR and the chain-wide scope, and the holder must be an x/group account",
			)

			return vm, nil
		},
	},
	{
		// x/tokenisation could not pay anybody and could not be exited.
		//
		// Five defects, four of which lost money silently and none of which any
		// test had ever exercised, because the sale pipeline had no test at all.
		// The two that matter here changed behaviour rather than state, which is
		// why they need an upgrade and not a migration: every validator has to
		// start applying the new rules at the same height, or they disagree
		// about what a block did.
		//
		//   - SendRestrictionFn is registered now, so a share transfer settles
		//     both sides. It was written and referenced nowhere, so no transfer
		//     ever settled and every holder's entitlement read zero however much
		//     the vault held.
		//   - Fractionalise seeds the first holder's position, so somebody who
		//     never transfers is not treated as a newcomer on the way out and
		//     paid nothing for the whole life of the vehicle.
		//   - AttestSale checks that the attestor is appointed. Any address
		//     could attest, so a sponsor met any threshold with fresh keys and
		//     the one guard between a shareholder and a falsified sale price was
		//     decorative.
		//   - MsgFinaliseSale exists, so an asset can reach REALISED and Redeem
		//     can succeed. Without it every fractionalised vehicle was a one-way
		//     door.
		//   - DisputeSale's refusal after the window said the window was open.
		//
		// No store changes and no state migration. What is NOT fixed by this
		// upgrade is worth stating: a vehicle fractionalised BEFORE it has no
		// seeded positions, and income already paid into its vault stays owed to
		// nobody. Reconstructing who held what across funding events that were
		// never recorded is not something a handler can honestly do, so the
		// pre-existing vehicle on this chain keeps its stranded balance and the
		// demonstration uses one minted after this height.
		Name: "income-that-arrives",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}
			ctx.Logger().Info(
				"x/tokenisation can pay its shareholders now",
				"settlement", "a share transfer settles both sides; before this, no transfer settled and "+
					"every entitlement read zero however much the vault held",
				"issuance", "the first holder gets a position at issuance rather than at the index "+
					"prevailing when they finally move",
				"attestation", "an attestor must be appointed by governance; before this any address could attest",
				"exit", "MsgFinaliseSale opens redemption, which nothing could reach before",
				"note", "vehicles fractionalised before this height have no seeded positions and income "+
					"already in their vaults remains owed to nobody",
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
